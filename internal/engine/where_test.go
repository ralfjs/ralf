package engine

import (
	"testing"

	"github.com/Hideart/bepro/internal/config"
)

func TestMatchesWhere(t *testing.T) {
	tests := []struct {
		name     string
		where    *config.WherePredicate
		filePath string
		want     bool
	}{
		{
			name:     "nil matches all",
			where:    nil,
			filePath: "src/index.js",
			want:     true,
		},
		{
			name:     "file glob match",
			where:    &config.WherePredicate{File: "src/*.js"},
			filePath: "src/index.js",
			want:     true,
		},
		{
			name:     "file glob no match",
			where:    &config.WherePredicate{File: "src/*.ts"},
			filePath: "src/index.js",
			want:     false,
		},
		{
			name:     "doublestar pattern",
			where:    &config.WherePredicate{File: "src/**/*.js"},
			filePath: "src/deep/nested/index.js",
			want:     true,
		},
		{
			name:     "doublestar no match",
			where:    &config.WherePredicate{File: "lib/**/*.js"},
			filePath: "src/index.js",
			want:     false,
		},
		{
			name:     "not inversion - matches",
			where:    &config.WherePredicate{Not: &config.WherePredicate{File: "test/**"}},
			filePath: "src/index.js",
			want:     true,
		},
		{
			name:     "not inversion - blocked",
			where:    &config.WherePredicate{Not: &config.WherePredicate{File: "test/**"}},
			filePath: "test/index.test.js",
			want:     false,
		},
		{
			name:     "nested not (double negation)",
			where:    &config.WherePredicate{Not: &config.WherePredicate{Not: &config.WherePredicate{File: "src/*.js"}}},
			filePath: "src/index.js",
			want:     true,
		},
		{
			name:     "empty where matches all",
			where:    &config.WherePredicate{},
			filePath: "anything.js",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesWhere(tt.where, tt.filePath)
			if got != tt.want {
				t.Errorf("matchesWhere(%v, %q) = %v, want %v", tt.where, tt.filePath, got, tt.want)
			}
		})
	}
}
