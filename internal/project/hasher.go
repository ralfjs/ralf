// Package project provides project-level analysis: cache, module graph, scanner.
package project

import (
	"encoding/json"
	"fmt"

	"github.com/cespare/xxhash/v2"
	"github.com/ralfjs/ralf/internal/config"
)

// HashFile returns the xxhash-64 digest of raw file contents.
func HashFile(data []byte) uint64 {
	return xxhash.Sum64(data)
}

// HashConfig returns the xxhash-64 of the JSON-serialized config.
// Used to invalidate the entire cache when config changes.
func HashConfig(cfg *config.Config) (uint64, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return 0, fmt.Errorf("hash config: %w", err)
	}
	return xxhash.Sum64(data), nil
}
