package project

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/fsnotify/fsnotify"
	"github.com/ralfjs/ralf/internal/engine"
	"github.com/ralfjs/ralf/internal/parser"
)

// WatchEvent represents a diagnostic update for a single file.
type WatchEvent struct {
	Path         string
	Diags        []engine.Diagnostic
	GraphChanged bool // true when module graph was modified (caller should re-run cross-file rules)
}

// WatcherConfig holds watcher settings.
type WatcherConfig struct {
	Root           string
	IgnorePatterns []string
	Debounce       time.Duration
}

// Watcher monitors filesystem changes, re-lints changed files, and cascades
// invalidation through the module graph when exports change.
// Cross-file rule evaluation is the caller's responsibility — the watcher
// signals graph changes via WatchEvent.GraphChanged.
type Watcher struct {
	cfg   WatcherConfig
	fsw   *fsnotify.Watcher
	cache *Cache
	graph *Graph
	eng   *engine.Engine

	events chan WatchEvent

	mu      sync.Mutex
	pending map[string]struct{}
}

// NewWatcher creates a file watcher for the project. The caller must call
// Run to start processing events and Close to release resources.
func NewWatcher(cfg WatcherConfig, cache *Cache, graph *Graph, eng *engine.Engine) (*Watcher, error) {
	if cfg.Debounce == 0 {
		cfg.Debounce = 100 * time.Millisecond
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}

	w := &Watcher{
		cfg:     cfg,
		fsw:     fsw,
		cache:   cache,
		graph:   graph,
		eng:     eng,
		events:  make(chan WatchEvent, 64),
		pending: make(map[string]struct{}),
	}

	if err := w.addWatchDirs(); err != nil {
		_ = fsw.Close()
		return nil, fmt.Errorf("add watch directories: %w", err)
	}

	return w, nil
}

// Events returns a read-only channel of diagnostic updates.
func (w *Watcher) Events() <-chan WatchEvent {
	return w.events
}

// Run blocks and processes filesystem events until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) error {
	timer := time.NewTimer(time.Hour) // dormant until first event
	timer.Stop()
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			close(w.events)
			return ctx.Err()

		case ev, ok := <-w.fsw.Events:
			if !ok {
				close(w.events)
				return nil
			}
			if !w.isRelevant(ev) {
				continue
			}
			// Watch newly created directories.
			if ev.Has(fsnotify.Create) {
				w.maybeWatchDir(ev.Name)
			}
			// Watch newly created directories but don't enqueue them for linting.
			if ev.Has(fsnotify.Create) {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					continue
				}
			}
			w.mu.Lock()
			w.pending[ev.Name] = struct{}{}
			w.mu.Unlock()
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(w.cfg.Debounce)

		case err, ok := <-w.fsw.Errors:
			if !ok {
				close(w.events)
				return nil
			}
			slog.Error("watcher error", "error", err)

		case <-timer.C:
			w.mu.Lock()
			batch := w.pending
			w.pending = make(map[string]struct{}, len(batch))
			w.mu.Unlock()

			w.processBatch(ctx, batch)
		}
	}
}

// Close releases the fsnotify watcher.
func (w *Watcher) Close() error {
	return w.fsw.Close()
}

// addWatchDirs walks the project root and adds all non-ignored directories.
func (w *Watcher) addWatchDirs() error {
	return filepath.WalkDir(w.cfg.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable dirs
		}
		if !d.IsDir() {
			return nil
		}
		if w.shouldSkipDir(d.Name()) {
			return filepath.SkipDir
		}
		if w.isIgnored(path) {
			return filepath.SkipDir
		}
		return w.fsw.Add(path)
	})
}

// maybeWatchDir adds a newly created directory to the watcher if it should be watched.
func (w *Watcher) maybeWatchDir(path string) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return
	}
	if w.shouldSkipDir(filepath.Base(path)) || w.isIgnored(path) {
		return
	}
	if err := w.fsw.Add(path); err != nil {
		slog.Debug("failed to watch new directory", "path", path, "error", err)
	}
}

// hardcodedSkipDirs are directories always excluded from watching.
var hardcodedSkipDirs = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"dist":         {},
	"build":        {},
	".next":        {},
	"coverage":     {},
}

func (w *Watcher) shouldSkipDir(name string) bool {
	_, skip := hardcodedSkipDirs[name]
	return skip
}

func (w *Watcher) isIgnored(path string) bool {
	rel, err := filepath.Rel(w.cfg.Root, path)
	if err != nil {
		return false
	}
	base := filepath.Base(rel)
	for _, pattern := range w.cfg.IgnorePatterns {
		if ok, _ := doublestar.Match(pattern, rel); ok {
			return true
		}
		if ok, _ := doublestar.Match(pattern, base); ok {
			return true
		}
	}
	return false
}

// isRelevant returns true if the fsnotify event is for a path we care about.
func (w *Watcher) isRelevant(ev fsnotify.Event) bool {
	if !ev.Has(fsnotify.Write) && !ev.Has(fsnotify.Create) && !ev.Has(fsnotify.Remove) && !ev.Has(fsnotify.Rename) {
		return false
	}
	// Create events for directories are relevant (handled separately in Run loop).
	if ev.Has(fsnotify.Create) {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			return true
		}
	}
	// Filter to JS/TS files only.
	_, ok := parser.LangFromPath(ev.Name)
	if !ok {
		return false
	}
	return !w.isIgnored(ev.Name)
}

// processBatch handles a debounced batch of file change events.
func (w *Watcher) processBatch(ctx context.Context, batch map[string]struct{}) {
	var dependents map[string]struct{}

	for path := range batch {
		_, deps := w.processFile(ctx, path)
		if len(deps) > 0 {
			if dependents == nil {
				dependents = make(map[string]struct{})
			}
			for _, d := range deps {
				// Don't re-lint files already in this batch.
				if _, inBatch := batch[d]; !inBatch {
					dependents[d] = struct{}{}
				}
			}
		}
	}

	// Re-lint dependent files whose imports may now be broken/changed.
	for dep := range dependents {
		w.relintFile(ctx, dep)
	}
}

// processFile handles a single file change. Returns whether exports changed
// and the list of dependent files that need re-linting.
func (w *Watcher) processFile(ctx context.Context, path string) (exportsChanged bool, dependents []string) {
	source, err := os.ReadFile(path) //nolint:gosec // path comes from fsnotify, scoped to project root
	if err != nil {
		// File was deleted or is unreadable.
		if os.IsNotExist(err) {
			slog.Debug("file deleted", "path", path)
			dependents = w.graph.ImportedBy(path)
			w.handleDeletedFile(ctx, path)
			return true, dependents
		}
		slog.Error("read file for watch", "path", path, "error", err)
		return false, nil
	}

	// Check if content actually changed via hash.
	hash := HashFile(source)
	if _, hit, err := w.cache.Lookup(ctx, path, hash); err == nil && hit {
		slog.Debug("file unchanged (cache hit)", "path", path)
		return false, nil
	}

	// Capture previous exports to detect changes.
	oldExports := w.graph.ExportedBy(path)

	// Extract new imports/exports.
	newImports, newExports, err := ExtractFile(ctx, path, source)
	if err != nil {
		slog.Error("extract file", "path", path, "error", err)

		// Clear stale graph data and lint anyway (regex-only rules still apply).
		w.graph.UpdateFile(path, nil, nil)
		if storeErr := w.cache.StoreFileGraph(ctx, path, nil, nil); storeErr != nil {
			slog.Error("store file graph (on extract error)", "path", path, "error", storeErr)
		}
		result := w.eng.LintSources(ctx, []engine.FileSource{{Path: path, Source: source}}, 1)
		w.emit(WatchEvent{Path: path, Diags: result.Diagnostics, GraphChanged: true})
		if storeErr := w.cache.Store(ctx, CacheEntry{
			Path:        path,
			ContentHash: hash,
			ModTimeNS:   time.Now().UnixNano(),
			Diagnostics: result.Diagnostics,
		}); storeErr != nil {
			slog.Error("cache store", "path", path, "error", storeErr)
		}
		return exportsChanged, nil
	}

	exportsChanged = exportsDiffer(oldExports, newExports)

	// Update graph and cache.
	w.graph.UpdateFile(path, newExports, newImports)
	if err := w.cache.StoreFileGraph(ctx, path, newExports, newImports); err != nil {
		slog.Error("store file graph", "path", path, "error", err)
	}

	// Lint the file.
	result := w.eng.LintSources(ctx, []engine.FileSource{{Path: path, Source: source}}, 1)
	// Always signal graph change — import changes affect cross-file rules
	// (circular deps, dead modules) even when exports are unchanged.
	w.emit(WatchEvent{Path: path, Diags: result.Diagnostics, GraphChanged: true})

	// Cache the result.
	if err := w.cache.Store(ctx, CacheEntry{
		Path:        path,
		ContentHash: hash,
		ModTimeNS:   time.Now().UnixNano(),
		Diagnostics: result.Diagnostics,
	}); err != nil {
		slog.Error("cache store", "path", path, "error", err)
	}

	if exportsChanged {
		dependents = w.graph.ImportedBy(path)
	}
	return exportsChanged, dependents
}

// handleDeletedFile cleans up cache and graph state for a removed file.
func (w *Watcher) handleDeletedFile(ctx context.Context, path string) {
	w.graph.RemoveFile(path)
	if err := w.cache.Remove(ctx, path); err != nil {
		slog.Error("cache remove", "path", path, "error", err)
	}
	if err := w.cache.StoreFileGraph(ctx, path, nil, nil); err != nil {
		slog.Error("cache remove graph", "path", path, "error", err)
	}
	w.emit(WatchEvent{Path: path, Diags: nil, GraphChanged: true})
}

// relintFile re-reads, re-lints, and caches results for a dependent file.
func (w *Watcher) relintFile(ctx context.Context, path string) {
	source, err := os.ReadFile(path) //nolint:gosec // path comes from graph, scoped to project root
	if err != nil {
		slog.Debug("cannot re-lint dependent", "path", path, "error", err)
		return
	}

	hash := HashFile(source)
	result := w.eng.LintSources(ctx, []engine.FileSource{{Path: path, Source: source}}, 1)
	w.emit(WatchEvent{Path: path, Diags: result.Diagnostics})

	if err := w.cache.Store(ctx, CacheEntry{
		Path:        path,
		ContentHash: hash,
		ModTimeNS:   time.Now().UnixNano(),
		Diagnostics: result.Diagnostics,
	}); err != nil {
		slog.Error("cache store dependent", "path", path, "error", err)
	}
}

func (w *Watcher) emit(ev WatchEvent) {
	select {
	case w.events <- ev:
	default:
		slog.Debug("watch event channel full, dropping", "path", ev.Path)
	}
}

// exportsDiffer returns true if two export lists have different symbol names.
func exportsDiffer(old, cur []ExportInfo) bool {
	if len(old) != len(cur) {
		return true
	}
	names := make(map[string]struct{}, len(old))
	for _, e := range old {
		names[e.Name] = struct{}{}
	}
	for _, e := range cur {
		if _, ok := names[e.Name]; !ok {
			return true
		}
	}
	return false
}
