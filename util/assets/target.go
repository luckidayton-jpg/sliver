package assets

import (
	"fmt"
	"os"
	"strings"
)

// Target platforms the asset bundle knows how to build.
var (
	supportedOS   = []string{"darwin", "linux", "windows"}
	supportedArch = []string{"amd64", "arm64"}
)

// platformTarget selects which platform bundles to build. Empty fields match
// every platform, which is how the tool behaves when nothing is requested.
type platformTarget struct {
	os   string
	arch string
}

// scoped reports whether the target narrows the default all-platforms build.
func (t platformTarget) scoped() bool {
	return t.os != "" || t.arch != ""
}

func (t platformTarget) String() string {
	switch {
	case t.os != "" && t.arch != "":
		return t.os + "/" + t.arch
	case t.os != "":
		return t.os + "/*"
	case t.arch != "":
		return "*/" + t.arch
	default:
		return "all"
	}
}

// matches reports whether a platform pair is selected by the target.
func (t platformTarget) matches(platOS, platArch string) bool {
	if t.os != "" && t.os != platOS {
		return false
	}
	if t.arch != "" && t.arch != platArch {
		return false
	}
	return true
}

// resolvePlatformTarget builds a target from explicit flags, falling back to
// the conventional GOOS/GOARCH environment variables. Cross-compiles set those
// automatically, so honoring them keeps the tool scoped to the platform
// actually being built instead of downloading the full multi-platform bundle.
func resolvePlatformTarget(flagOS, flagArch string) (platformTarget, error) {
	t := platformTarget{
		os:   strings.TrimSpace(flagOS),
		arch: strings.TrimSpace(flagArch),
	}
	if t.os == "" {
		t.os = strings.TrimSpace(os.Getenv("GOOS"))
	}
	if t.arch == "" {
		t.arch = strings.TrimSpace(os.Getenv("GOARCH"))
	}
	if err := t.validate(); err != nil {
		return platformTarget{}, err
	}
	return t, nil
}

func (t platformTarget) validate() error {
	if t.os != "" && !containsString(supportedOS, t.os) {
		return fmt.Errorf("unsupported os %q (supported: %s)", t.os, strings.Join(supportedOS, ", "))
	}
	if t.arch != "" && !containsString(supportedArch, t.arch) {
		return fmt.Errorf("unsupported arch %q (supported: %s)", t.arch, strings.Join(supportedArch, ", "))
	}
	return nil
}

func containsString(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}
