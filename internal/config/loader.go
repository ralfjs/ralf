package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// searchNames lists config file names in priority order.
var searchNames = []string{
	".lintrc.json",
	".lintrc.yaml",
	".lintrc.yml",
	".lintrc.toml",
}

// Load searches dir for a config file and loads the first one found.
// Returns an error if no config file is found.
func Load(dir string) (*Config, error) {
	for _, name := range searchNames {
		path := filepath.Join(dir, name)
		_, err := os.Stat(path)
		if err == nil {
			return LoadFile(path)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config: stat %s: %w", path, err)
		}
	}
	return nil, fmt.Errorf("config: no config file found in %s (searched: %v)", dir, searchNames)
}

// LoadFile loads a config from an explicit file path. The format is
// determined by the file extension.
func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // caller-controlled path
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	ext := filepath.Ext(path)
	var cfg Config

	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("config: parse JSON %s: %w", path, err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("config: parse YAML %s: %w", path, err)
		}
	case ".toml":
		if err := toml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("config: parse TOML %s: %w", path, err)
		}
	default:
		return nil, fmt.Errorf("config: unsupported file extension %q", ext)
	}

	return &cfg, nil
}
