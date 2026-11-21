package apihook

import (
	"fmt"
	"sort"
	"strings"

	"go.uber.org/config"

	"github.com/michelangelo-ai/michelangelo/go/components/pipeline/declarative"
)

// configKey is the config-provider path for Config. The keys are camelCase to
// match apiserver.pipelineRunDefaults.
const configKey = "apiserver.declarative"

// Config is the apiserver.declarative block of the apiserver config.
type Config struct {
	// Enabled turns on DECLARATIVE pipeline writes. When it is false, the
	// apiserver rejects them with FailedPrecondition.
	Enabled bool `yaml:"enabled"`
	// DefaultInterpreter is the release version that a new pipeline is
	// pinned to.
	DefaultInterpreter string `yaml:"defaultInterpreter"`
	// ReleasesDir holds one <version>.yaml file per interpreter release.
	ReleasesDir string `yaml:"releasesDir"`
}

// NewConfig populates a Config from the given provider. An absent key gives
// the zero Config, which disables declarative pipelines.
func NewConfig(provider config.Provider) (Config, error) {
	var conf Config
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}

// Declarative is the state the hook reads: the flag and the releases, loaded
// once at boot. Adding a release needs an apiserver restart.
type Declarative struct {
	enabled        bool
	releases       map[string]*declarative.Release
	defaultRelease *declarative.Release
}

// NewDeclarative loads the releases when cfg.Enabled is true. It fails when
// LoadReleases fails or when no release file declares cfg.DefaultInterpreter.
func NewDeclarative(cfg Config) (*Declarative, error) {
	if !cfg.Enabled {
		return &Declarative{}, nil
	}
	releases, err := declarative.LoadReleases(cfg.ReleasesDir)
	if err != nil {
		return nil, fmt.Errorf("load declarative releases: %w", err)
	}
	def, ok := releases[cfg.DefaultInterpreter]
	if !ok {
		return nil, fmt.Errorf("default interpreter %q has no release file in %s", cfg.DefaultInterpreter, cfg.ReleasesDir)
	}
	return &Declarative{enabled: true, releases: releases, defaultRelease: def}, nil
}

// Versions returns the loaded release versions in sorted order.
func (d *Declarative) Versions() []string {
	out := make([]string, 0, len(d.releases))
	for v := range d.releases {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// versionList formats the loaded release versions as "[a, b]".
func (d *Declarative) versionList() string {
	return "[" + strings.Join(d.Versions(), ", ") + "]"
}
