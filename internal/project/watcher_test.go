package project

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/engine"
)

// newTestWatcher sets up a watcher with a temp directory, cache, graph, and engine.
// Returns the watcher, the temp project root, and a cleanup function.
func newTestWatcher(t *testing.T) (w *Watcher, root string) {
	t.Helper()

	root = t.TempDir()

	ctx := context.Background()
	cache, err := Open(ctx, root, 0)
	if err != nil {
		t.Fatalf("open cache: %v", err)
	}
	t.Cleanup(func() { _ = cache.Close() })

	graph := NewGraph(make(map[string][]ExportInfo), make(map[string][]ImportInfo))

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-var": {
				Severity: config.SeverityError,
				Regex:    `\bvar\b`,
				Message:  "Use let or const instead of var",
			},
		},
	}
	eng, errs := engine.New(cfg)
	if len(errs) > 0 {
		t.Fatalf("engine init: %v", errs)
	}

	w, err = NewWatcher(WatcherConfig{
		Root:     root,
		Debounce: 50 * time.Millisecond,
	}, cache, graph, eng)
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	return w, root
}

// receiveEvents drains events from the watcher channel with a timeout.
func receiveEvents(t *testing.T, ch <-chan WatchEvent, timeout time.Duration) []WatchEvent {
	t.Helper()
	var events []WatchEvent
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, ev)
		case <-deadline:
			return events
		}
	}
}

func TestWatcher_FileChange(t *testing.T) {
	w, root := newTestWatcher(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = w.Run(ctx) }()

	// Write a JS file with a lint violation.
	file := filepath.Join(root, "test.js")
	if err := os.WriteFile(file, []byte("var x = 1;\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	events := receiveEvents(t, w.Events(), 2*time.Second)
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}

	found := false
	for _, ev := range events {
		if ev.Path == file && len(ev.Diags) > 0 {
			found = true
			if ev.Diags[0].Rule != "no-var" {
				t.Errorf("expected rule no-var, got %q", ev.Diags[0].Rule)
			}
		}
	}
	if !found {
		t.Error("expected diagnostic event for test.js with no-var violation")
	}
}

func TestWatcher_UnchangedHash(t *testing.T) {
	w, root := newTestWatcher(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Pre-populate cache with the file content.
	file := filepath.Join(root, "cached.js")
	content := []byte("const x = 1;\n")
	if err := os.WriteFile(file, content, 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	hash := HashFile(content)
	if err := w.cache.Store(ctx, CacheEntry{
		Path:        file,
		ContentHash: hash,
		ModTimeNS:   time.Now().UnixNano(),
	}); err != nil {
		t.Fatalf("cache store: %v", err)
	}

	go func() { _ = w.Run(ctx) }()

	// Re-write the same content — should be a cache hit.
	if err := os.WriteFile(file, content, 0o600); err != nil {
		t.Fatalf("rewrite file: %v", err)
	}

	events := receiveEvents(t, w.Events(), 500*time.Millisecond)
	for _, ev := range events {
		if ev.Path == file {
			t.Error("expected no event for unchanged file, but got one")
		}
	}
}

func TestWatcher_DeletedFile(t *testing.T) {
	w, root := newTestWatcher(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a file first.
	file := filepath.Join(root, "delete-me.js")
	if err := os.WriteFile(file, []byte("const x = 1;\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	go func() { _ = w.Run(ctx) }()

	// Wait for initial event, then delete.
	receiveEvents(t, w.Events(), 300*time.Millisecond)

	if err := os.Remove(file); err != nil {
		t.Fatalf("remove file: %v", err)
	}

	events := receiveEvents(t, w.Events(), 2*time.Second)
	found := false
	for _, ev := range events {
		if ev.Path == file && ev.Diags == nil && ev.GraphChanged {
			found = true
		}
	}
	if !found {
		t.Error("expected deletion event with nil diags and GraphChanged=true")
	}
}

func TestWatcher_IgnoredPaths(t *testing.T) {
	w, root := newTestWatcher(t)

	// Create a node_modules directory and file before starting watcher.
	nmDir := filepath.Join(root, "node_modules")
	if err := os.Mkdir(nmDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = w.Run(ctx) }()

	// Write to node_modules — should be ignored.
	file := filepath.Join(nmDir, "dep.js")
	if err := os.WriteFile(file, []byte("var x = 1;\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	events := receiveEvents(t, w.Events(), 500*time.Millisecond)
	for _, ev := range events {
		if ev.Path == file {
			t.Error("expected no event for file in node_modules")
		}
	}
}

func TestWatcher_CascadeInvalidation(t *testing.T) {
	w, root := newTestWatcher(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up two files: b.js imports from a.js.
	aFile := filepath.Join(root, "a.js")
	bFile := filepath.Join(root, "b.js")

	aContent := []byte("export function foo() {}\n")
	bContent := []byte("import { foo } from './a.js';\n")

	if err := os.WriteFile(aFile, aContent, 0o600); err != nil {
		t.Fatalf("write a.js: %v", err)
	}
	if err := os.WriteFile(bFile, bContent, 0o600); err != nil {
		t.Fatalf("write b.js: %v", err)
	}

	// Pre-populate graph: b imports from a.
	w.graph.UpdateFile(aFile, []ExportInfo{{Name: "foo", Kind: "function", Line: 1}}, nil)
	w.graph.UpdateFile(bFile, nil, []ImportInfo{{Source: aFile, Name: "foo", Line: 1}})

	go func() { _ = w.Run(ctx) }()

	// Modify a.js to change its exports — remove foo, add bar.
	newAContent := []byte("export function bar() {}\n")
	if err := os.WriteFile(aFile, newAContent, 0o600); err != nil {
		t.Fatalf("rewrite a.js: %v", err)
	}

	events := receiveEvents(t, w.Events(), 2*time.Second)

	// We should see events for both a.js (changed) and b.js (dependent).
	gotA, gotB := false, false
	for _, ev := range events {
		switch ev.Path {
		case aFile:
			gotA = true
			if !ev.GraphChanged {
				t.Error("expected GraphChanged=true for a.js")
			}
		case bFile:
			gotB = true
		}
	}
	if !gotA {
		t.Error("expected event for a.js")
	}
	if !gotB {
		t.Error("expected cascade re-lint event for b.js")
	}
}

func TestExportsDiffer(t *testing.T) {
	tests := []struct {
		name string
		old  []ExportInfo
		new  []ExportInfo
		want bool
	}{
		{
			name: "equal",
			old:  []ExportInfo{{Name: "foo"}, {Name: "bar"}},
			new:  []ExportInfo{{Name: "foo"}, {Name: "bar"}},
			want: false,
		},
		{
			name: "different count",
			old:  []ExportInfo{{Name: "foo"}},
			new:  []ExportInfo{{Name: "foo"}, {Name: "bar"}},
			want: true,
		},
		{
			name: "different names",
			old:  []ExportInfo{{Name: "foo"}},
			new:  []ExportInfo{{Name: "bar"}},
			want: true,
		},
		{
			name: "both nil",
			old:  nil,
			new:  nil,
			want: false,
		},
		{
			name: "old nil new has exports",
			old:  nil,
			new:  []ExportInfo{{Name: "foo"}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exportsDiffer(tt.old, tt.new)
			if got != tt.want {
				t.Errorf("exportsDiffer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWatcher_IgnoreBasename(t *testing.T) {
	root := t.TempDir()

	// Create subdirectory before watcher starts so it's watched from the beginning.
	subdir := filepath.Join(root, "src")
	if err := os.Mkdir(subdir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ctx := context.Background()
	cache, err := Open(ctx, root, 0)
	if err != nil {
		t.Fatalf("open cache: %v", err)
	}
	t.Cleanup(func() { _ = cache.Close() })

	graph := NewGraph(make(map[string][]ExportInfo), make(map[string][]ImportInfo))

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-var": {Severity: config.SeverityError, Regex: `\bvar\b`, Message: "Use let or const"},
		},
	}
	eng, errs := engine.New(cfg)
	if len(errs) > 0 {
		t.Fatalf("engine init: %v", errs)
	}

	w, err := NewWatcher(WatcherConfig{
		Root:           root,
		Debounce:       50 * time.Millisecond,
		IgnorePatterns: []string{"*.generated.*"},
	}, cache, graph, eng)
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	ctx2, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = w.Run(ctx2) }()

	file := filepath.Join(subdir, "foo.generated.js")
	if err := os.WriteFile(file, []byte("var x = 1;\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	events := receiveEvents(t, w.Events(), 500*time.Millisecond)
	for _, ev := range events {
		if ev.Path == file {
			t.Error("expected no event for file matching basename ignore pattern")
		}
	}
}

func TestWatcher_NewDirectory(t *testing.T) {
	w, root := newTestWatcher(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = w.Run(ctx) }()

	// Create a new subdirectory and immediately write a file in it.
	// Retry writing until we get an event — avoids flaky sleeps waiting
	// for fsnotify to register the new directory.
	subdir := filepath.Join(root, "newpkg")
	if err := os.Mkdir(subdir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	file := filepath.Join(subdir, "index.js")
	var found bool
	for attempt := 0; attempt < 5 && !found; attempt++ {
		if err := os.WriteFile(file, []byte("var y = 2;\n"), 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		events := receiveEvents(t, w.Events(), 500*time.Millisecond)
		for _, ev := range events {
			if ev.Path == file {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected event for file in newly created directory")
	}
}
