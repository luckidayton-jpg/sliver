package assets

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type runner struct {
	logger             *logger
	httpClient         *http.Client
	outputDir          string
	workDir            string
	target             platformTarget
	downloadAttempts   int
	downloadRetryDelay time.Duration
	goIndex            int
	zigIndex           int
	garbleIndex        int
	zigMirrors         []string
}

const (
	downloadTimeout         = 15 * time.Minute
	defaultDownloadAttempts = 3
	defaultDownloadDelay    = 250 * time.Millisecond
)

type config struct {
	verbose bool
	quiet   bool
	noColor bool
	os      string
	arch    string
}

// Run executes the asset generation flow.
func Run(args []string) error {
	cfg, showHelp, err := parseArgs(args)
	if err != nil {
		return err
	}
	if showHelp {
		fmt.Println(usage())
		return nil
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	outputDir := filepath.Join(repoRoot, "server", "assets", "fs")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	target, err := resolvePlatformTarget(cfg.os, cfg.arch)
	if err != nil {
		return err
	}

	workDir, err := os.MkdirTemp("", "sliver-assets-")
	if err != nil {
		return fmt.Errorf("create work dir: %w", err)
	}

	log := newLogger(cfg.verbose, cfg.quiet, cfg.noColor)
	r := &runner{
		logger:             log,
		httpClient:         &http.Client{Timeout: downloadTimeout},
		outputDir:          outputDir,
		workDir:            workDir,
		target:             target,
		downloadAttempts:   defaultDownloadAttempts,
		downloadRetryDelay: defaultDownloadDelay,
	}

	log.Header("Sliver Assets")
	log.Meta("Workdir", workDir)
	log.Meta("Output", outputDir)
	if target.scoped() {
		log.Meta("Target", target.String())
	}

	defer func() {
		log.ClearSection()
		log.Logf("clean up: %s", workDir)
		os.RemoveAll(workDir)
	}()

	if err := r.buildGoAssets(); err != nil {
		return err
	}
	if err := r.buildZigAssets(); err != nil {
		return err
	}
	if err := r.buildGarbleAssets(); err != nil {
		return err
	}

	log.ClearSection()
	log.Logf("")
	log.Successf("Done")

	return nil
}

func parseArgs(args []string) (config, bool, error) {
	cfg := config{}
	showHelp := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		// Normalize "--flag=value" into "--flag", "value" so both spellings
		// reach the same case below.
		flag, inlineValue, hasInline := strings.Cut(arg, "=")
		hasInline = hasInline && strings.HasPrefix(flag, "-")

		var flagTarget *string
		switch flag {
		case "-v", "--verbose":
			cfg.verbose = true
		case "--no-colors":
			cfg.noColor = true
		case "--quiet":
			cfg.quiet = true
		case "-h", "--help":
			showHelp = true
		case "-os", "--os":
			flagTarget = &cfg.os
		case "-arch", "--arch":
			flagTarget = &cfg.arch
		default:
			return config{}, false, fmt.Errorf("unknown argument: %s", arg)
		}

		if flagTarget == nil {
			// Boolean flags take no value; "--verbose=x" is a mistake.
			if hasInline {
				return config{}, false, fmt.Errorf("%s does not take a value", flag)
			}
			continue
		}

		value := inlineValue
		if !hasInline {
			if i+1 >= len(args) {
				return config{}, false, fmt.Errorf("%s requires a value", flag)
			}
			i++
			value = args[i]
		}
		if strings.TrimSpace(value) == "" {
			return config{}, false, fmt.Errorf("%s requires a non-empty value", flag)
		}
		// Catch a forgotten value ("-os --verbose") rather than silently
		// accepting the next flag as the platform name.
		if strings.HasPrefix(value, "-") {
			return config{}, false, fmt.Errorf("%s requires a value, got flag %s", flag, value)
		}
		*flagTarget = value
	}

	if cfg.quiet {
		cfg.verbose = false
	}

	return cfg, showHelp, nil
}

func usage() string {
	return `Usage: assets [options]

Options:
  -v, --verbose   Verbose output
      --quiet     Suppress progress output
      --no-colors Disable colored output
  -h, --help      Show this help

Platform selection (defaults to every platform; falls back to GOOS/GOARCH):
  -os, --os       Target operating system (darwin, linux, windows)
  -arch, --arch   Target architecture (amd64, arm64)

Examples:
  assets                          # build every platform bundle
  assets -os linux -arch amd64    # build only linux/amd64
  GOARCH=arm64 assets             # build only the current GOARCH
`
}

func findRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("unable to locate go.mod from current directory")
		}
		dir = parent
	}
}

func ensureDir(path string) error {
	if path == "" {
		return errors.New("empty directory path")
	}
	return os.MkdirAll(path, 0o755)
}

func trimLeadingDot(path string) string {
	return strings.TrimPrefix(path, "./")
}
