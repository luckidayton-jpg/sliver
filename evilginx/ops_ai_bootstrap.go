package evilginx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	Provider     string              `yaml:"provider"`
	Anthropic    *aiYAMLProvider     `yaml:"anthropic"`
	Google       *aiYAMLProvider     `yaml:"google"`
	OpenAI       *aiYAMLProvider     `yaml:"openai"`
	OpenAICompat *aiYAMLProvider     `yaml:"openai_compat"`
	OpenRouter   *aiYAMLProvider     `yaml:"openrouter"`
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

// LocalAIConfigured - true when ai.yaml already holds credentials for at least
// one provider (mirrors configs.aiProviderConfigured). File-only; does not
// require a running server, so it can be consulted before Start().
func (s *SliverBridge) LocalAIConfigured() bool {
	data, err := os.ReadFile(s.localAIConfigPath())
	if err != nil {
		return false
	}
	var env aiYAMLEnvelope
	if err := yaml.Unmarshal(data, &env); err != nil {
		return false
	}
	if env.AI == nil {
		return false
	}
	for _, p := range []*aiYAMLProvider{
		env.AI.Anthropic, env.AI.Google, env.AI.OpenAI,
		env.AI.OpenAICompat, env.AI.OpenRouter,
	} {
		if p == nil {
			continue
		}
		if strings.TrimSpace(p.APIKey) != "" || strings.TrimSpace(p.BaseURL) != "" {
			return true
		}
	}
	return false
}

// StagePlatformAI - when the server AI has no configured provider, seed ai.yaml
// with the platform's BYOK provider (OpenRouter when the base URL points at
// OpenRouter, OpenAI-compatible otherwise). If the operator has already set up
// the server AI (e.g. via the server console ai-config command) this is a
// no-op: the operator's choice always wins.
func (s *SliverBridge) StagePlatformAI(cfg PlatformAIConfig) error {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil
	}
	if s.LocalAIConfigured() {
		return nil
	}
	provider := "openai-compat"
	if strings.Contains(strings.ToLower(cfg.BaseURL), "openrouter.ai") {
		provider = "openrouter"
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
	return nil
}