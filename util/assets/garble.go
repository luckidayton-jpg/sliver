package assets

import (
	"fmt"
	"path/filepath"
)

type garblePlatform struct {
	os       string
	arch     string
	filename string
	url      string
}

// garblePlatforms is the full matrix of garble binaries the server embeds.
// Only entries matching the active target are downloaded.
var garblePlatforms = []garblePlatform{
	{
		os:       "linux",
		arch:     "amd64",
		filename: "garble",
		url:      fmt.Sprintf("https://github.com/moloch--/garble/releases/download/v%s/garble_linux-amd64", garbleVersion),
	},
	{
		os:       "linux",
		arch:     "arm64",
		filename: "garble",
		url:      fmt.Sprintf("https://github.com/moloch--/garble/releases/download/v%s/garble_linux-arm64", garbleVersion),
	},
	{
		os:       "windows",
		arch:     "amd64",
		filename: "garble.exe",
		url:      fmt.Sprintf("https://github.com/moloch--/garble/releases/download/v%s/garble_windows-amd64.exe", garbleVersion),
	},
	{
		os:       "windows",
		arch:     "arm64",
		filename: "garble.exe",
		url:      fmt.Sprintf("https://github.com/moloch--/garble/releases/download/v%s/garble_windows-arm64.exe", garbleVersion),
	},
	{
		os:       "darwin",
		arch:     "amd64",
		filename: "garble",
		url:      fmt.Sprintf("https://github.com/moloch--/garble/releases/download/v%s/garble_darwin-amd64", garbleVersion),
	},
	{
		os:       "darwin",
		arch:     "arm64",
		filename: "garble",
		url:      fmt.Sprintf("https://github.com/moloch--/garble/releases/download/v%s/garble_darwin-arm64", garbleVersion),
	},
}

func (r *runner) buildGarbleAssets() error {
	r.logger.Section("Garble")

	platforms := selectGarblePlatforms(garblePlatforms, r.target)
	if len(platforms) == 0 {
		return fmt.Errorf("no garble assets available for target %s", r.target)
	}

	for _, platform := range platforms {
		r.garbleIndex++
		r.logger.Logf("Fetch garble %s/%s (%d/%d)", platform.os, platform.arch, r.garbleIndex, len(platforms))
		outputDir := filepath.Join(r.outputDir, platform.os, platform.arch)
		if err := ensureDir(outputDir); err != nil {
			return err
		}
		destPath := filepath.Join(outputDir, platform.filename)
		if err := r.downloadFile(platform.url, destPath); err != nil {
			return err
		}
	}

	return nil
}

func selectGarblePlatforms(platforms []garblePlatform, t platformTarget) []garblePlatform {
	selected := make([]garblePlatform, 0, len(platforms))
	for _, p := range platforms {
		if t.matches(p.os, p.arch) {
			selected = append(selected, p)
		}
	}
	return selected
}
