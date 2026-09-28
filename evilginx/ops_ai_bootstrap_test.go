package evilginx

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// The two bugs this file exists for.
//
// The bridge set SLIVER_APP_DIR, but the server reads SLIVER_ROOT_DIR
// (assets.EnvVarName). So the daemon resolved its own default root -- $HOME/.sliver,
// or <cwd>/.sliver -- while the staged AI config was written under AppDir. The two
// never met: a working provider sat in a file nothing read, and the server reported
// no provider configured with no error anywhere to explain the gap.
//
// On top of that, LocalAIConfigured reported true when ANY provider block held a key
// or a base URL. The server reads the `provider` selector, so a file with credentials
// and an empty selector is exactly what the server calls unconfigured. The guard
// therefore said "configured", StagePlatformAI skipped the seed, and the self-heal
// that called it returned success having done nothing.
//
// Every fixture below is FLAT, because the file the server reads has no envelope.
// See ops_ai_shape_test.go for the shape itself and why it was wrong for so long.

func writeAIYAML(t *testing.T, appDir, body string) {
	t.Helper()
	dir := filepath.Join(appDir, "configs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ai.yaml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func newTestBridge(t *testing.T, appDir string) *SliverBridge {
	t.Helper()
	return &SliverBridge{config: &SliverConfig{AppDir: appDir}}
}

func TestLocalAIConfigured_RequiresASelectedProvider(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want bool
	}{
		{
			// The state that was reported as configured and served as unconfigured.
			name: "credentials present but nothing selected",
			yaml: "provider: \"\"\nopenrouter:\n  api_key: sk-x\n  base_url: https://openrouter.ai/api/v1\n",
			want: false,
		},
		{
			name: "selected and keyed",
			yaml: "provider: openrouter\nopenrouter:\n  api_key: sk-x\n  base_url: https://openrouter.ai/api/v1\n",
			want: true,
		},
		{
			name: "selected but the key is blank",
			yaml: "provider: openrouter\nopenrouter:\n  api_key: \"\"\n  base_url: https://openrouter.ai/api/v1\n",
			want: false,
		},
		{
			// A base URL is what every block carries by default, so counting it as
			// credentials made an untouched file look configured.
			name: "selected with only a base URL",
			yaml: "provider: openrouter\nopenrouter:\n  api_key: \"\"\n  base_url: https://openrouter.ai/api/v1\n",
			want: false,
		},
		{
			// The key is under a different provider than the one selected.
			name: "keyed but not under the selected provider",
			yaml: "provider: anthropic\nanthropic:\n  api_key: \"\"\nopenrouter:\n  api_key: sk-x\n",
			want: false,
		},
		{
			name: "empty file",
			yaml: "",
			want: false,
		},
		{
			name: "no ai section at all",
			yaml: "other: 1\n",
			want: false,
		},
		{
			name: "openai-compat spelled with an underscore",
			yaml: "provider: openai_compat\nopenai_compat:\n  api_key: sk-x\n",
			want: true,
		},
		{
			// The shape this whole feature used to write, kept as a case: the server
			// reads nothing inside an envelope, so it must read as unconfigured.
			name: "wrapped in an envelope, which the server cannot read",
			yaml: "ai:\n  provider: openrouter\n  openrouter:\n    api_key: sk-x\n",
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeAIYAML(t, dir, tc.yaml)
			b := newTestBridge(t, dir)
			if got := b.LocalAIConfigured(); got != tc.want {
				t.Errorf("LocalAIConfigured() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStagePlatformAI_SeedsWhenTheSelectorIsEmpty(t *testing.T) {
	dir := t.TempDir()
	// The real-world shape: credentials landed in the file, the selector did not.
	writeAIYAML(t, dir, "provider: \"\"\nopenrouter:\n  api_key: stale\n  base_url: https://openrouter.ai/api/v1\n")
	b := newTestBridge(t, dir)

	if b.LocalAIConfigured() {
		t.Fatal("precondition: an empty selector must not read as configured")
	}
	if err := b.StagePlatformAI(PlatformAIConfig{
		BaseURL: "https://openrouter.ai/api/v1",
		APIKey:  "sk-platform",
		Model:   "openrouter/auto",
	}); err != nil {
		t.Fatalf("StagePlatformAI: %v", err)
	}

	if !b.LocalAIConfigured() {
		t.Error("after staging, the file still reads as unconfigured")
	}
	raw, err := os.ReadFile(b.localAIConfigPath())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var cfg aiYAMLConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg.Provider != "openrouter" {
		t.Errorf("provider = %q, want openrouter", cfg.Provider)
	}
	if cfg.OpenRouter == nil || cfg.OpenRouter.APIKey != "sk-platform" {
		t.Errorf("openrouter block = %+v, want the platform key", cfg.OpenRouter)
	}
	if len(cfg.OpenRouter.Models) != 1 || cfg.OpenRouter.Models[0] != "openrouter/auto" {
		t.Errorf("models = %v, want [openrouter/auto]", cfg.OpenRouter.Models)
	}
}

// The operator's own configuration always wins. That promise was kept before and is
// kept now -- but it used to be kept by a guard that also skipped the incomplete
// case, which is why the two were conflated.
func TestStagePlatformAI_LeavesACompleteOperatorConfigAlone(t *testing.T) {
	dir := t.TempDir()
	writeAIYAML(t, dir, "provider: anthropic\nanthropic:\n  api_key: sk-operator\n")
	b := newTestBridge(t, dir)

	if err := b.StagePlatformAI(PlatformAIConfig{
		BaseURL: "https://openrouter.ai/api/v1",
		APIKey:  "sk-platform",
	}); err != nil {
		t.Fatalf("StagePlatformAI: %v", err)
	}

	raw, _ := os.ReadFile(b.localAIConfigPath())
	var cfg aiYAMLConfig
	_ = yaml.Unmarshal(raw, &cfg)
	if cfg.Anthropic.APIKey != "sk-operator" {
		t.Errorf("the operator's key was overwritten: %+v", cfg.Anthropic)
	}
}

// Someone who chose a provider and left the key blank has made half a decision.
// Completing it is the helpful reading; switching them to a different provider is
// not.
func TestStagePlatformAI_KeepsAnOperatorChosenProviderAndFillsTheKey(t *testing.T) {
	dir := t.TempDir()
	writeAIYAML(t, dir, "provider: anthropic\nanthropic:\n  api_key: \"\"\n")
	b := newTestBridge(t, dir)

	if err := b.StagePlatformAI(PlatformAIConfig{
		BaseURL: "https://openrouter.ai/api/v1",
		APIKey:  "sk-platform",
	}); err != nil {
		t.Fatalf("StagePlatformAI: %v", err)
	}

	raw, _ := os.ReadFile(b.localAIConfigPath())
	var cfg aiYAMLConfig
	_ = yaml.Unmarshal(raw, &cfg)
	if cfg.Provider != "anthropic" {
		t.Errorf("provider = %q, want the operator's anthropic kept, not switched to "+
			"the platform's implied provider", cfg.Provider)
	}
	if cfg.Anthropic.APIKey != "sk-platform" {
		t.Errorf("the blank key was not filled: %+v", cfg.Anthropic)
	}
}

func TestStagePlatformAI_VerifiesTheWriteTookEffect(t *testing.T) {
	// A write that reports success and changes nothing is the failure this whole
	// path is meant to be immune to, and the only way to know it did not happen is
	// to read the file back.
	dir := t.TempDir()
	b := newTestBridge(t, dir)
	if err := b.StagePlatformAI(PlatformAIConfig{
		BaseURL: "https://openrouter.ai/api/v1",
		APIKey:  "sk-platform",
		Model:   "openrouter/auto",
	}); err != nil {
		t.Fatalf("StagePlatformAI: %v", err)
	}
	if _, err := os.Stat(b.localAIConfigPath()); err != nil {
		t.Errorf("the file was not created at %s: %v", b.localAIConfigPath(), err)
	}
	if !b.LocalAIConfigured() {
		t.Error("staging reported success but the file does not read as configured")
	}
}

func TestStagePlatformAI_LeavesTheFileAloneWithNoPlatformKey(t *testing.T) {
	dir := t.TempDir()
	writeAIYAML(t, dir, "provider: anthropic\nanthropic:\n  api_key: sk-operator\n")
	b := newTestBridge(t, dir)

	if err := b.StagePlatformAI(PlatformAIConfig{BaseURL: "https://x.test"}); err != nil {
		t.Fatalf("StagePlatformAI: %v", err)
	}
	raw, _ := os.ReadFile(b.localAIConfigPath())
	var cfg aiYAMLConfig
	_ = yaml.Unmarshal(raw, &cfg)
	if cfg.Anthropic.APIKey != "sk-operator" {
		t.Errorf("an empty platform key still rewrote the file: %+v", cfg.Anthropic)
	}
}

func TestLocalAIState_ExplainsWhatLocalAIConfiguredCouldNot(t *testing.T) {
	dir := t.TempDir()
	writeAIYAML(t, dir, "provider: \"\"\nopenrouter:\n  api_key: sk-x\n")
	b := newTestBridge(t, dir)

	st := b.LocalAIState()
	if !st.Exists {
		t.Error("Exists = false for a file that is there")
	}
	if st.Provider != "" {
		t.Errorf("Provider = %q, want empty", st.Provider)
	}
	if st.HasKey {
		t.Error("HasKey = true when the selector is empty: the key is not under any " +
			"selected provider, which is the whole point")
	}
	if st.Path == "" {
		t.Error("Path is empty, so the reason cannot be shown to anyone")
	}
}

func TestNormaliseProviderName(t *testing.T) {
	for in, want := range map[string]string{
		"openrouter":    "openrouter",
		"OpenRouter":    "openrouter",
		"openai":        "openai",
		"anthropic":     "anthropic",
		"google":        "google",
		"openai-compat": "openai-compat",
		"openai_compat": "openai-compat",
		"  openrouter ": "openrouter",
		"":              "",
		"nonsense":      "",
		"../../etc":     "",
	} {
		if got := normaliseProviderName(in); got != want {
			t.Errorf("normaliseProviderName(%q) = %q, want %q", in, got, want)
		}
	}
}
