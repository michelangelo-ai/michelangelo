package config

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"

	"go.uber.org/config"
)

const (
	// SecretsFile is the base filename for yaml config. containing secrets.
	SecretsFile = "secrets"
)

// FileInfo represents a file to load via Load function.
type FileInfo struct {
	Name        string // file name
	Interpolate bool   // enable variable expansion
}

// NoConfigErr represents no configuration files being found.
type NoConfigErr struct {
	Files []string
}

func (err NoConfigErr) Error() string {
	return fmt.Sprintf("no configuration files found, checked files %s", err.Files)
}

// Load loads config files
func Load(files []FileInfo, options ...config.YAMLOption) (config.Provider, error) {
	opts := make([]config.YAMLOption, len(options))
	copy(opts, options)
	var found bool

	for _, file := range files {
		contents, err := ioutil.ReadFile(file.Name)
		if err != nil && os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		found = true
		r := bytes.NewReader(contents)
		if file.Interpolate {
			opts = append(opts, config.Source(r))
		} else {
			opts = append(opts, config.RawSource(r))
		}
	}

	if !found {
		// Haven't added any sources.
		names := make([]string, len(files))
		for i := range files {
			names[i] = files[i].Name
		}
		return nil, NoConfigErr{Files: names}
	}

	return config.NewYAML(opts...)
}
