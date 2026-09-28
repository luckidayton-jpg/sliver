package evilginx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bishopfox/sliver/server/log"
	"gopkg.in/yaml.v3"
)

// PlatformAIConfig carries the platform's BYOK AI provider so it can be shared
// with the embedded server. The API key is written only to a 0600 yaml file in
// the local app dir and is never logged, echoed, or surfaced through the
// dashboard (which continues to treat provider keys as server-side state).
type PlatformAIConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

// aiYAMLEnvelope is the minimal surface of the server's configs/ai.yaml we
// touch. It mirrors configs.AIConfig; an incomplete file is fine because the
// server normalizes + rewrites missing fields on its next config load.
type aiYAMLEnvelope struct {
	AI *aiYAMLAIConfig `yaml:"ai"`
}

type aiYAMLAIConfig struct {
	Provider     string          `yaml:"provider"`
	Anthropic    *aiYAMLProvider `yaml:"anthropic"`
	Google       *aiYAMLProvider `yaml:"google"`
	OpenAI       *aiYAMLProvider `yaml:"openai"`
	OpenAICompat *aiYAMLProvider `yaml:"openai_compat"`
	OpenRouter   *aiYAMLProvider `yaml:"openrouter"`
}

type aiYAMLProvider struct {
	APIKey  string   `yaml:"api_key"`
	BaseURL string   `yaml:"base_url"`
	Models  []string `yaml:"models,omitempty"`
}

func (s *SliverBridge) localAIConfigPath() string {
	return filepath.Join(s.config.AppDir, "configs", "ai.yaml")
}

func emptyAIYAMLProvider() *aiYAMLProvider {
	return &aiYAMLProvider{}
}

// localAIProvider - the credential block named by the file's provider selector, or
// nil when the selector is empty or names something unknown.
//
// A base URL alone is not credentials. It is the default every block carries, so
// counting it as "configured" made an empty file look configured.
func localAIProvider(ai *aiYAMLAIConfig) *aiYAMLProvider {
	if ai == nil {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(ai.Provider)) {
	case "anthropic":
		return ai.Anthropic
	case "google":
		return ai.Google
	case "openai":
		return ai.OpenAI
	case "openai-compat", "openai_compat":
		return ai.OpenAICompat
	case "openrouter":
		return ai.OpenRouter
	}
	return nil
}

func providerHasCredentials(p *aiYAMLProvider) bool {
	return p != nil && strings.TrimSpace(p.APIKey) != ""
}

// LocalAIConfigured - true when ai.yaml names a provider AND that provider holds an
// API key. File-only; does not require a running server, so it can be consulted
// before Start().
//
// The selector matters. The previous version reported true when ANY provider block
// held a key or a base URL, which is a much weaker statement than "the server has a
// provider": the server reads `provider`, so a file with credentials under
// openrouter and an empty selector is exactly the state it calls unconfigured. That
// mismatch made StagePlatformAI skip the seed -- it saw a configured file -- so the
// self-heal ran, did nothing, and returned success, leaving a working key in place
// and the server still reporting none.
func (s *SliverBridge) LocalAIConfigured() bool {
	data, err := os.ReadFile(s.localAIConfigPath())
	if err != nil {
		return false
	}
	var env aiYAMLEnvelope
	if err := yaml.Unmarshal(data, &env); err != nil {
		return false
	}
	return providerHasCredentials(localAIProvider(env.AI))
}

// LocalAIState - why LocalAIConfigured answered what it did, for a log line or an
// API response. A boolean that has silently said the wrong thing for a long time is
// worth being able to explain.
type LocalAIState struct {
	Path     string
	Exists   bool
	Provider string
	HasKey   bool
}

func (s *SliverBridge) LocalAIState() LocalAIState {
	st := LocalAIState{Path: s.localAIConfigPath()}
	data, err := os.ReadFile(st.Path)
	if err != nil {
		return st
	}
	st.Exists = true
	var env aiYAMLEnvelope
	if err := yaml.Unmarshal(data, &env); err != nil {
		return st
	}
	if env.AI == nil {
		return st
	}
	st.Provider = strings.TrimSpace(env.AI.Provider)
	st.HasKey = providerHasCredentials(localAIProvider(env.AI))
	return st
}

// StagePlatformAI - when the server AI has no usable provider, seed ai.yaml with the
// platform's BYOK provider (OpenRouter when the base URL points at OpenRouter,
// OpenAI-compatible otherwise).
//
// Three cases, because collapsing them into "configured or not" is what let this sit
// broken for so long:
//
//   - no provider selected, or the selected one has no key: seed it.
//   - a provider is selected and has a key: the operator's choice, left alone.
//   - a provider is selected but has no key: keep THEIR provider and supply only the
//     missing credentials. Switching providers out from under someone is worse than
//     completing what they started.
//
// The previous version returned nil for the first two cases alike, so "already
// configured" and "did nothing" were indistinguishable from the caller's side, and
// the one caller that cared logged nothing either.
func (s *SliverBridge) StagePlatformAI(cfg PlatformAIConfig) error {
	if strings.TrimSpace(cfg.APIKey) == "" {
		log.NamedLogger("bridge", "ai").Info(
			"platform AI not staged: the platform has no API key configured")
		return nil
	}
	if s.LocalAIConfigured() {
		st := s.LocalAIState()
		log.NamedLogger("bridge", "ai").Info(
			"platform AI not staged: the server already has a provider configured",
			"path", st.Path, "provider", st.Provider)
		return nil
	}

	// Default to the provider implied by the platform's base URL, but let an
	// operator who has already chosen one keep it.
	provider := "openai-compat"
	if strings.Contains(strings.ToLower(cfg.BaseURL), "openrouter.ai") {
		provider = "openrouter"
	}
	st := s.LocalAIState()
	if chosen := normaliseProviderName(st.Provider); chosen != "" {
		provider = chosen
	}

	model := strings.TrimSpace(cfg.Model)
	prov := &aiYAMLProvider{APIKey: strings.TrimSpace(cfg.APIKey), BaseURL: strings.TrimSpace(cfg.BaseURL)}
	if model != "" {
		prov.Models = []string{model}
	}
	env := &aiYAMLEnvelope{AI: &aiYAMLAIConfig{
		Provider:     provider,
		Anthropic:    emptyAIYAMLProvider(),
		Google:       emptyAIYAMLProvider(),
		OpenAI:       emptyAIYAMLProvider(),
		OpenAICompat: emptyAIYAMLProvider(),
		OpenRouter:   emptyAIYAMLProvider(),
	}}
	switch provider {
	case "anthropic":
		env.AI.Anthropic = prov
	case "google":
		env.AI.Google = prov
	case "openai":
		env.AI.OpenAI = prov
	case "openrouter":
		env.AI.OpenRouter = prov
	default:
		env.AI.OpenAICompat = prov
	}

	data, err := yaml.Marshal(env)
	if err != nil {
		return fmt.Errorf("sliver bridge: marshal platform AI config: %w", err)
	}
	dir := filepath.Dir(s.localAIConfigPath())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("sliver bridge: create config dir: %w", err)
	}
	if err := os.WriteFile(s.localAIConfigPath(), data, 0o600); err != nil {
		return fmt.Errorf("sliver bridge: write platform AI config: %w", err)
	}

	// Read it back. A write that reports success and leaves the file unchanged is the
	// failure mode this whole function is meant to be immune to, and the only way to
	// know it did not happen is to look.
	if !s.LocalAIConfigured() {
		return fmt.Errorf("sliver bridge: staged platform AI config at %s but the file "+
			"still reads as unconfigured", s.localAIConfigPath())
	}
	log.NamedLogger("bridge", "ai").Info(
		"staged the platform AI provider into the server config",
		"path", s.localAIConfigPath(), "provider", provider, "model", model)
	return nil
}

// normaliseProviderName - the file spells the OpenAI-compatible provider with a
// hyphen and the yaml key with an underscore; accept either and reject anything
// unrecognised, so a stray value cannot be written back as a provider that does not
// exist.
func normaliseProviderName(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "anthropic":
		return "anthropic"
	case "google":
		return "google"
	case "openai":
		return "openai"
	case "openai-compat", "openai_compat":
		return "openai-compat"
	case "openrouter":
		return "openrouter"
	}
	return ""
}
