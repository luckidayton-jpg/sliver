package evilginx

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/bishopfox/sliver/server/assets"
	"github.com/bishopfox/sliver/server/configs"
	"gopkg.in/yaml.v3"
)

// The bug this file exists for, and the one that mattered most.
//
// StagePlatformAI wrote configs/ai.yaml wrapped in an `ai:` key. The server reads it
// flat:
//
//	config := defaultAIConfig()
//	yaml.Unmarshal(data, config)
//	config.Save()
//
// so every field kept its default, the provider selector stayed empty, and the Save()
// that immediately followed wrote those blanks back over the staged file. The key was
// not being lost at runtime -- it was never read, and then overwritten by the read
// that failed to find it.
//
// What made it survive so long is that the writer and the check both used a local
// mirror of the file. A mirror cannot disagree with its own writer by accident, so
// "staged" and "verified" agreed perfectly while the server saw nothing. The only
// reader that matters is the server's, so that is what these tests parse with.

// TestStagedConfigIsReadableByTheServer is the test that should have existed from the
// start. It writes what StagePlatformAI writes and reads it with configs.AIConfig --
// the type the server itself unmarshals into.
func TestStagedConfigIsReadableByTheServer(t *testing.T) {
	dir := t.TempDir()
	b := &SliverBridge{config: &SliverConfig{AppDir: dir}}

	if err := b.StagePlatformAI(PlatformAIConfig{
		BaseURL: "https://openrouter.ai/api/v1",
		APIKey:  "sk-platform",
		Model:   "openrouter/auto",
	}); err != nil {
		t.Fatalf("StagePlatformAI: %v", err)
	}

	raw, err := os.ReadFile(b.localAIConfigPath())
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// The shape, stated plainly: no envelope. A leading "ai:" key means the server
	// reads nothing, and the test below would pass anyway -- which is the trap.
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "ai:") {
		t.Fatalf("the file is wrapped in an `ai:` envelope; the server unmarshals "+
			"straight into *AIConfig and would read none of this:\n%s", raw)
	}

	// Now parse it the way the server does.
	var server configs.AIConfig
	if err := yaml.Unmarshal(raw, &server); err != nil {
		t.Fatalf("the server's reader rejected the file: %v\n%s", err, raw)
	}

	if server.Provider != "openrouter" {
		t.Errorf("the server's reader sees provider %q, want openrouter. The key was "+
			"written and is not being read.", server.Provider)
	}
	if server.OpenRouter == nil {
		t.Fatal("the server's reader sees no openrouter block at all")
	}
	if server.OpenRouter.APIKey != "sk-platform" {
		t.Errorf("the server's reader sees api_key %q, want the platform key",
			redact(server.OpenRouter.APIKey))
	}
	if server.OpenRouter.BaseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("base_url = %q", server.OpenRouter.BaseURL)
	}
	if len(server.OpenRouter.Models) != 1 || server.OpenRouter.Models[0] != "openrouter/auto" {
		t.Errorf("models = %v, want [openrouter/auto]", server.OpenRouter.Models)
	}
}

// The server normalises and re-saves on every load. Anything the staged file omits
// that the server considers required gets invented at that point, so what the file
// carries has to be a complete enough config for that pass to leave it alone.
func TestStagedConfig_SurvivesTheServersNormaliseAndSavePass(t *testing.T) {
	dir := t.TempDir()
	b := &SliverBridge{config: &SliverConfig{AppDir: dir}}
	if err := b.StagePlatformAI(PlatformAIConfig{
		BaseURL: "https://openrouter.ai/api/v1",
		APIKey:  "sk-platform",
		Model:   "openrouter/auto",
	}); err != nil {
		t.Fatalf("StagePlatformAI: %v", err)
	}

	// Drive the server's own load-and-save cycle against the staged file. The
	// function is unexported, so this goes through the same public path a server
	// startup takes: point the server's config root at our temp dir and read.
	t.Setenv(assets.EnvVarName, dir)
	_ = assets.GetRootAppDir()

	got := configs.GetAIConfig()
	if got.Provider != "openrouter" {
		t.Errorf("after the server's own load-and-save, provider = %q, want "+
			"openrouter. The file must be complete enough that normalisation has "+
			"nothing to overwrite.", got.Provider)
	}
	if got.OpenRouter == nil || got.OpenRouter.APIKey != "sk-platform" {
		t.Errorf("after the server's own load-and-save, the openrouter key is %q, want "+
			"the platform key", redact(got.OpenRouter.APIKey))
	}
	// And the file on disk must still be the one we staged rather than a blanked one.
	raw, _ := os.ReadFile(filepath.Join(dir, "configs", "ai.yaml"))
	var reread configs.AIConfig
	if err := yaml.Unmarshal(raw, &reread); err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if reread.Provider == "" {
		t.Error("the server's save pass blanked the provider on the way through")
	}
}

// The openai-compat path, since it is the default and the other branch.
func TestStagedConfig_OpenAICompatIsAlsoReadableByTheServer(t *testing.T) {
	dir := t.TempDir()
	b := &SliverBridge{config: &SliverConfig{AppDir: dir}}
	if err := b.StagePlatformAI(PlatformAIConfig{
		BaseURL: "https://gateway.internal/v1",
		APIKey:  "sk-local",
		Model:   "some-model",
	}); err != nil {
		t.Fatalf("StagePlatformAI: %v", err)
	}

	raw, _ := os.ReadFile(b.localAIConfigPath())
	var server configs.AIConfig
	if err := yaml.Unmarshal(raw, &server); err != nil {
		t.Fatalf("server reader: %v", err)
	}
	if server.Provider != "openai-compat" {
		t.Errorf("provider = %q, want openai-compat", server.Provider)
	}
	if server.OpenAICompat == nil || server.OpenAICompat.APIKey != "sk-local" {
		t.Errorf("openai_compat block = %+v", server.OpenAICompat)
	}
}

// Every provider name the selector can take has to land in the block the server will
// look in. A name that normalises to something the server does not recognise would
// stage cleanly and read as unconfigured forever.
func TestStagedConfig_EveryProviderNameLandsInItsOwnBlock(t *testing.T) {
	for _, provider := range []string{"openrouter", "openai-compat", "openai", "anthropic", "google"} {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			// Force the selector by pre-seeding it, so the branch under test is the
			// one being chosen rather than the one implied by the base URL.
			writeAIYAML(t, dir, "provider: "+provider+"\n"+provider+":\n  api_key: \"\"\n")
			b := &SliverBridge{config: &SliverConfig{AppDir: dir}}

			if err := b.StagePlatformAI(PlatformAIConfig{
				BaseURL: "https://openrouter.ai/api/v1",
				APIKey:  "sk-platform",
			}); err != nil {
				t.Fatalf("StagePlatformAI: %v", err)
			}

			raw, _ := os.ReadFile(b.localAIConfigPath())
			var server configs.AIConfig
			if err := yaml.Unmarshal(raw, &server); err != nil {
				t.Fatalf("server reader: %v", err)
			}
			if server.Provider != provider {
				t.Errorf("provider = %q, want %q", server.Provider, provider)
			}
			if !providerHasCredentials(localAIProvider(&aiYAMLConfig{
				Provider:     server.Provider,
				Anthropic:    wrap(server.Anthropic),
				Google:       wrap(server.Google),
				OpenAI:       wrap(server.OpenAI),
				OpenAICompat: wrap(server.OpenAICompat),
				OpenRouter:   wrap(server.OpenRouter),
			})) {
				t.Errorf("the key is not under the %q block the server would read", provider)
			}
		})
	}
}

func wrap(p *configs.AIProviderConfig) *aiYAMLProvider {
	if p == nil {
		return nil
	}
	return &aiYAMLProvider{APIKey: p.APIKey, BaseURL: p.BaseURL}
}

// redact never prints a key. Length only, so a failure says whether something was
// written without the failure message itself becoming a credential in a CI log.
func redact(s string) string {
	if s == "" {
		return "(empty)"
	}
	return "(set, " + strconv.Itoa(len(s)) + " chars)"
}
