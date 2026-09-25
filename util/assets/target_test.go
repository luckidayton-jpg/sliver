package assets

import (
	"testing"
)

func goPlatformLabels(platforms []goPlatform) []string {
	labels := make([]string, 0, len(platforms))
	for _, p := range platforms {
		labels = append(labels, p.os+"/"+p.arch)
	}
	return labels
}

func zigPlatformLabels(platforms []zigPlatform) []string {
	labels := make([]string, 0, len(platforms))
	for _, p := range platforms {
		labels = append(labels, p.os+"/"+p.arch)
	}
	return labels
}

func garblePlatformLabels(platforms []garblePlatform) []string {
	labels := make([]string, 0, len(platforms))
	for _, p := range platforms {
		labels = append(labels, p.os+"/"+p.arch)
	}
	return labels
}

func assertLabels(t *testing.T, kind string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %v, want %v", kind, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: got %v, want %v", kind, got, want)
		}
	}
}

func TestSelectGoPlatformsUnscopedKeepsEveryPlatform(t *testing.T) {
	platforms := selectGoPlatforms(goPlatforms, platformTarget{})

	assertLabels(t, "go", goPlatformLabels(platforms), []string{
		"darwin/amd64",
		"darwin/arm64",
		"linux/amd64",
		"linux/arm64",
		"windows/amd64",
		"windows/arm64",
	})

	// Unscoped runs must preserve the upstream behavior where only the darwin
	// entries pack src.zip.
	var srcPackers []string
	for _, p := range platforms {
		if p.includeSrc {
			srcPackers = append(srcPackers, p.os+"/"+p.arch)
		}
	}
	assertLabels(t, "src packers", srcPackers, []string{"darwin/amd64", "darwin/arm64"})
}

func TestSelectGoPlatformsScopedToSinglePlatform(t *testing.T) {
	tests := []struct {
		name   string
		target platformTarget
		want   string
	}{
		{name: "linux amd64", target: platformTarget{os: "linux", arch: "amd64"}, want: "linux/amd64"},
		{name: "linux arm64", target: platformTarget{os: "linux", arch: "arm64"}, want: "linux/arm64"},
		{name: "darwin arm64", target: platformTarget{os: "darwin", arch: "arm64"}, want: "darwin/arm64"},
		{name: "windows amd64", target: platformTarget{os: "windows", arch: "amd64"}, want: "windows/amd64"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			platforms := selectGoPlatforms(goPlatforms, test.target)

			assertLabels(t, "go", goPlatformLabels(platforms), []string{test.want})

			// fs/src.zip is required by the embed directive on every platform,
			// so a scoped run must still produce it — from whichever platform
			// it selected.
			if !platforms[0].includeSrc {
				t.Fatalf("%s: expected selected platform to pack src.zip", test.name)
			}
		})
	}
}

func TestSelectGoPlatformsScopedByOSOnly(t *testing.T) {
	platforms := selectGoPlatforms(goPlatforms, platformTarget{os: "linux"})

	assertLabels(t, "go", goPlatformLabels(platforms), []string{"linux/amd64", "linux/arm64"})

	// Exactly one entry packs src.zip even though two platforms are selected,
	// so fs/src.zip is not rewritten from a second toolchain.
	srcCount := 0
	for _, p := range platforms {
		if p.includeSrc {
			srcCount++
		}
	}
	if srcCount != 1 {
		t.Fatalf("expected exactly 1 src.zip packer, got %d", srcCount)
	}
}

func TestSelectGoPlatformsScopedByArchOnly(t *testing.T) {
	platforms := selectGoPlatforms(goPlatforms, platformTarget{arch: "arm64"})

	assertLabels(t, "go", goPlatformLabels(platforms), []string{
		"darwin/arm64",
		"linux/arm64",
		"windows/arm64",
	})
}

func TestSelectGoPlatformsUnknownTargetIsEmpty(t *testing.T) {
	platforms := selectGoPlatforms(goPlatforms, platformTarget{os: "linux", arch: "riscv"})

	if len(platforms) != 0 {
		t.Fatalf("expected no platforms, got %v", goPlatformLabels(platforms))
	}
}

func TestSelectZigPlatforms(t *testing.T) {
	all := selectZigPlatforms(zigPlatforms, platformTarget{})
	if len(all) != 6 {
		t.Fatalf("unscoped: expected 6 platforms, got %d", len(all))
	}

	scoped := selectZigPlatforms(zigPlatforms, platformTarget{os: "linux", arch: "amd64"})
	assertLabels(t, "zig", zigPlatformLabels(scoped), []string{"linux/amd64"})

	if got := selectZigPlatforms(zigPlatforms, platformTarget{os: "plan9"}); len(got) != 0 {
		t.Fatalf("expected no zig platforms for plan9, got %d", len(got))
	}
}

func TestSelectGarblePlatforms(t *testing.T) {
	all := selectGarblePlatforms(garblePlatforms, platformTarget{})
	if len(all) != 6 {
		t.Fatalf("unscoped: expected 6 platforms, got %d", len(all))
	}

	scoped := selectGarblePlatforms(garblePlatforms, platformTarget{os: "linux", arch: "arm64"})
	assertLabels(t, "garble", garblePlatformLabels(scoped), []string{"linux/arm64"})

	if got := selectGarblePlatforms(garblePlatforms, platformTarget{os: "plan9"}); len(got) != 0 {
		t.Fatalf("expected no garble platforms for plan9, got %d", len(got))
	}
}

func TestSelectPlatformsDoNotMutatePackageTables(t *testing.T) {
	// selectGoPlatforms rewrites includeSrc on the selected slice, so it must
	// operate on a copy — otherwise a scoped run would corrupt the shared table
	// for every later unscoped run in the same process.
	selectGoPlatforms(goPlatforms, platformTarget{os: "linux", arch: "amd64"})

	for _, p := range goPlatforms {
		if p.os == "linux" && p.includeSrc {
			t.Fatalf("goPlatforms was mutated: linux/amd64 now packs src.zip")
		}
	}
}

func TestPlatformTargetMatches(t *testing.T) {
	tests := []struct {
		name   string
		target platformTarget
		os     string
		arch   string
		want   bool
	}{
		{name: "unscoped matches all", target: platformTarget{}, os: "linux", arch: "amd64", want: true},
		{name: "os only matches same os", target: platformTarget{os: "linux"}, os: "linux", arch: "arm64", want: true},
		{name: "os only rejects other os", target: platformTarget{os: "linux"}, os: "darwin", arch: "amd64", want: false},
		{name: "arch only matches same arch", target: platformTarget{arch: "arm64"}, os: "windows", arch: "arm64", want: true},
		{name: "arch only rejects other arch", target: platformTarget{arch: "arm64"}, os: "windows", arch: "amd64", want: false},
		{name: "exact match", target: platformTarget{os: "linux", arch: "amd64"}, os: "linux", arch: "amd64", want: true},
		{name: "exact rejects wrong arch", target: platformTarget{os: "linux", arch: "amd64"}, os: "linux", arch: "arm64", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.target.matches(test.os, test.arch); got != test.want {
				t.Fatalf("matches(%q, %q) = %v, want %v", test.os, test.arch, got, test.want)
			}
		})
	}
}

func TestPlatformTargetScoped(t *testing.T) {
	if (platformTarget{}).scoped() {
		t.Fatal("empty target should not be scoped")
	}
	if !(platformTarget{os: "linux"}).scoped() {
		t.Fatal("os-only target should be scoped")
	}
	if !(platformTarget{arch: "amd64"}).scoped() {
		t.Fatal("arch-only target should be scoped")
	}
}

func TestPlatformTargetString(t *testing.T) {
	tests := []struct {
		target platformTarget
		want   string
	}{
		{target: platformTarget{}, want: "all"},
		{target: platformTarget{os: "linux"}, want: "linux/*"},
		{target: platformTarget{arch: "amd64"}, want: "*/amd64"},
		{target: platformTarget{os: "linux", arch: "amd64"}, want: "linux/amd64"},
	}

	for _, test := range tests {
		if got := test.target.String(); got != test.want {
			t.Fatalf("String() = %q, want %q", got, test.want)
		}
	}
}

func TestResolvePlatformTargetFlags(t *testing.T) {
	t.Setenv("GOOS", "")
	t.Setenv("GOARCH", "")

	target, err := resolvePlatformTarget("linux", "amd64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.os != "linux" || target.arch != "amd64" {
		t.Fatalf("got %+v, want linux/amd64", target)
	}
}

func TestResolvePlatformTargetFallsBackToEnv(t *testing.T) {
	t.Setenv("GOOS", "linux")
	t.Setenv("GOARCH", "arm64")

	target, err := resolvePlatformTarget("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.os != "linux" || target.arch != "arm64" {
		t.Fatalf("got %+v, want linux/arm64", target)
	}
}

func TestResolvePlatformTargetFlagsOverrideEnv(t *testing.T) {
	t.Setenv("GOOS", "darwin")
	t.Setenv("GOARCH", "arm64")

	target, err := resolvePlatformTarget("windows", "amd64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.os != "windows" || target.arch != "amd64" {
		t.Fatalf("got %+v, want windows/amd64", target)
	}
}

func TestResolvePlatformTargetEmptyEnvStaysUnscoped(t *testing.T) {
	// `make` invokes the target with GOOS= and GOARCH= set to empty strings to
	// request a full multi-platform bundle; that must not be read as a filter.
	t.Setenv("GOOS", "")
	t.Setenv("GOARCH", "")

	target, err := resolvePlatformTarget("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.scoped() {
		t.Fatalf("empty GOOS/GOARCH should stay unscoped, got %+v", target)
	}
}

func TestResolvePlatformTargetRejectsUnknown(t *testing.T) {
	t.Setenv("GOOS", "")
	t.Setenv("GOARCH", "")

	if _, err := resolvePlatformTarget("plan9", ""); err == nil {
		t.Fatal("expected error for unsupported os")
	}
	if _, err := resolvePlatformTarget("", "riscv64"); err == nil {
		t.Fatal("expected error for unsupported arch")
	}
}

func TestParseArgsPlatformFlags(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantOS    string
		wantArch  string
		wantError bool
	}{
		{name: "none", args: []string{}},
		{name: "space separated", args: []string{"-os", "linux", "-arch", "amd64"}, wantOS: "linux", wantArch: "amd64"},
		{name: "long flags", args: []string{"--os", "linux", "--arch", "arm64"}, wantOS: "linux", wantArch: "arm64"},
		{name: "equals separated", args: []string{"--os=linux", "--arch=amd64"}, wantOS: "linux", wantArch: "amd64"},
		{name: "combined with verbose", args: []string{"-v", "-os", "linux"}, wantOS: "linux"},
		{name: "mixed spellings", args: []string{"-os=linux", "--arch", "amd64"}, wantOS: "linux", wantArch: "amd64"},
		{name: "missing os value", args: []string{"-os"}, wantError: true},
		{name: "missing arch value", args: []string{"-os", "linux", "-arch"}, wantError: true},
		{name: "empty equals value", args: []string{"--os="}, wantError: true},
		{name: "empty space separated value", args: []string{"-os", ""}, wantError: true},
		{name: "value that looks like a flag", args: []string{"-os", "--verbose"}, wantError: true},
		{name: "unknown flag", args: []string{"--nope"}, wantError: true},
		{name: "boolean flag with value", args: []string{"--quiet=1"}, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, _, err := parseArgs(test.args)
			if test.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.os != test.wantOS {
				t.Fatalf("os = %q, want %q", cfg.os, test.wantOS)
			}
			if cfg.arch != test.wantArch {
				t.Fatalf("arch = %q, want %q", cfg.arch, test.wantArch)
			}
		})
	}
}

func TestParseArgsHelp(t *testing.T) {
	_, showHelp, err := parseArgs([]string{"--help"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !showHelp {
		t.Fatal("expected showHelp to be true")
	}
}
