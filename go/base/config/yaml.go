package config

import (
	"fmt"

	"github.com/michelangelo-ai/michelangelo/go/base/env"

	"path/filepath"

	"go.uber.org/config"
)

const (
	baseFile = "base"
)

func newYAML(env env.Context, lookupFun config.LookupFunc) (config.Provider, error) {
	opts := []config.YAMLOption{config.Expand(lookupFun)}

	cfg, err := Load(getYAMLFiles(env), opts...)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// getYAMLFiles returns all config yaml files, includes
// expanded files: base.yaml and {environment}.yaml, raw files: secretes.yaml
// variable is defined like ${ENV_VAR_NAME:default} and expended by looking up os envs
func getYAMLFiles(env env.Context) []FileInfo {
	files := defaultExpandedFiles(env)
	files = append(files, defaultRawFiles(env)...)

	dirs := getConfigDirs(env)
	cfgFileList := make([]FileInfo, 0, len(files)*len(dirs))
	for _, f := range files {
		for _, d := range dirs {
			cfgFileList = append(cfgFileList, FileInfo{
				Name:        filepath.Join(d, f.Name),
				Interpolate: f.Interpolate,
			})
		}
	}
	return cfgFileList
}

func defaultExpandedFiles(env env.Context) []FileInfo {
	environment := env.RuntimeEnvironment

	// Always load base configuration and environment-specific configuration.
	names := []string{
		baseFile,   // base
		environment, // production
	}

	return namesToInfo(names, true)
}

// defaultRawFiles returns yaml files shouldn't be expanded,
// e.g. secrets files which often unescaped contain special characters, like "S"
func defaultRawFiles(env env.Context) []FileInfo {
	names := []string{SecretsFile}
	return namesToInfo(names, false)
}

func namesToInfo(names []string, interpolate bool) []FileInfo {
	infos := make([]FileInfo, len(names))
	for i, n := range names {
		infos[i].Name = fmt.Sprintf("%s.yaml", n)
		infos[i].Interpolate = interpolate
	}
	return infos
}
