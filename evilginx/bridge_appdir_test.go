package evilginx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bishopfox/sliver/server/assets"
)

// The failure this file exists for, in one test.
//
// The bridge set SLIVER_APP_DIR. The server reads SLIVER_ROOT_DIR
// (assets.envVarName). So the daemon resolved its own default root -- $HOME/.sliver,
// or <cwd>/.sliver -- while the bridge staged its AI config under AppDir. The two
// paths never met. The operator saw "Xc2 AI is not configured" while the platform AI
// worked, a correct provider sat in a file nothing read, and nothing anywhere reported
// an error, because setting an env var that nothing reads is not a failure by any
// local measure.
//
// Two variables were meant to be one. Nothing asserted the name, which is how they
// drifted apart without a single test going red.

func TestApplyAppDirEnv_SetsTheVariableTheServerActuallyReads(t *testing.T) {
	// Whatever the server's own constant says it reads, the bridge must set it. If
	// assets.envVarName is ever renamed, this fails rather than the two drifting
	// apart silently a second time.
	t.Setenv(assets.EnvVarName, "")
	os.Unsetenv(assets.EnvVarName)

	dir := t.TempDir()
	if err := applyAppDirEnv(dir); err != nil {
		t.Fatalf("applyAppDirEnv: %v", err)
	}

	if got := os.Getenv(assets.EnvVarName); got != dir {
		t.Fatalf("after applyAppDirEnv, %s = %q, want %q. The server reads its root "+
			"directory from that variable, so the staged config and the daemon's own "+
			"layout are in different places.", assets.EnvVarName, got, dir)
	}
	// The bridge's own documented knob stays set too: harmless, and something else
	// may read it.
	if got := os.Getenv("SLIVER_APP_DIR"); got != dir {
		t.Errorf("SLIVER_APP_DIR = %q, want %q", got, dir)
	}
}

// The end of the chain, checked rather than assumed: with the variable set, the
// server's own resolver agrees with the bridge about where the config lives.
func TestApplyAppDirEnv_ServerResolvesItsRootToAppDir(t *testing.T) {
	dir := t.TempDir()
	if err := applyAppDirEnv(dir); err != nil {
		t.Fatalf("applyAppDirEnv: %v", err)
	}
	// GetRootAppDir creates the directory if absent, which is fine in a temp dir.
	if got := assets.GetRootAppDir(); got != dir {
		t.Errorf("assets.GetRootAppDir() = %q, want %q. The bridge writes the staged "+
			"AI config to filepath.Join(AppDir, \"configs\", \"ai.yaml\"); if the server "+
			"resolves its root elsewhere it never reads that file.", got, dir)
	}
}

// And the path the bridge stages to is the one the server will read, which is the
// property that was actually broken.
func TestStagedConfigLandsWhereTheServerReadsIt(t *testing.T) {
	dir := t.TempDir()
	if err := applyAppDirEnv(dir); err != nil {
		t.Fatalf("applyAppDirEnv: %v", err)
	}
	b := &SliverBridge{config: &SliverConfig{AppDir: dir}}

	want := filepath.Join(assets.GetRootAppDir(), "configs", "ai.yaml")
	if got := b.localAIConfigPath(); got != want {
		t.Errorf("localAIConfigPath() = %q, want %q. Staging to any other path is a "+
			"write the server will never read.", got, want)
	}

	if err := b.StagePlatformAI(PlatformAIConfig{
		BaseURL: "https://openrouter.ai/api/v1",
		APIKey:  "sk-platform",
		Model:   "openrouter/auto",
	}); err != nil {
		t.Fatalf("StagePlatformAI: %v", err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("the staged config is not where the server looks: %v", err)
	}
}

func TestApplyAppDirEnv_EmptyDirSetsNothing(t *testing.T) {
	os.Unsetenv("SLIVER_APP_DIR")
	os.Unsetenv(assets.EnvVarName)

	if err := applyAppDirEnv(""); err != nil {
		t.Fatalf("applyAppDirEnv: %v", err)
	}
	if got := os.Getenv(assets.EnvVarName); got != "" {
		t.Errorf("%s = %q, want unset when AppDir is empty", assets.EnvVarName, got)
	}
}
