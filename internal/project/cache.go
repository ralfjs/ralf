package project

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ralfjs/ralf/internal/engine"

	_ "modernc.org/sqlite" // SQLite driver
)

// Cache provides persistent per-file lint result storage backed by SQLite.
type Cache struct {
	db         *sql.DB
	configHash uint64
}

// CacheEntry holds cached data for a single file.
type CacheEntry struct {
	Path        string
	ContentHash uint64
	ModTimeNS   int64
	Diagnostics []engine.Diagnostic
}

const schema = `
CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS files (
    path         TEXT PRIMARY KEY,
    content_hash INTEGER NOT NULL,
    mod_time_ns  INTEGER NOT NULL,
    diag_json    BLOB,
    updated_at   INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_files_hash ON files (content_hash);

CREATE TABLE IF NOT EXISTS exports (
    path TEXT NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    line INTEGER NOT NULL,
    PRIMARY KEY (path, name)
);

CREATE TABLE IF NOT EXISTS imports (
    path   TEXT NOT NULL,
    source TEXT NOT NULL,
    name   TEXT NOT NULL,
    line   INTEGER NOT NULL,
    PRIMARY KEY (path, source, name)
);
`

// Open opens or creates the cache database at <projectRoot>/.ralf/cache.db.
// If the config hash differs from the stored one, all cached tables (files and module graph tables) are wiped.
func Open(ctx context.Context, projectRoot string, configHash uint64) (*Cache, error) {
	cacheDir := filepath.Join(projectRoot, ".ralf")
	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}

	dbPath := filepath.Join(cacheDir, "cache.db")
	dsn := dbPath + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open cache db: %w", err)
	}

	// Single connection — modernc.org/sqlite is not safe for concurrent access
	// from the same process. WAL mode + busy_timeout help with concurrent CLI invocations.
	db.SetMaxOpenConns(1)

	if _, err := db.ExecContext(ctx, schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	c := &Cache{db: db, configHash: configHash}

	if err := c.checkConfigHash(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return c, nil
}

// checkConfigHash compares the stored config hash with the current one.
// If they differ, all cached data (files, exports, and imports) is deleted.
func (c *Cache) checkConfigHash(ctx context.Context) error {
	hashStr := strconv.FormatUint(c.configHash, 10)

	var stored string
	err := c.db.QueryRowContext(ctx, "SELECT value FROM meta WHERE key = 'config_hash'").Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		// First run or partial state — ensure both meta rows exist.
		if _, err := c.db.ExecContext(ctx,
			"INSERT OR IGNORE INTO meta (key, value) VALUES ('schema_version', '1')"); err != nil {
			return fmt.Errorf("init schema_version: %w", err)
		}
		if _, err := c.db.ExecContext(ctx,
			"INSERT OR REPLACE INTO meta (key, value) VALUES ('config_hash', ?)", hashStr); err != nil {
			return fmt.Errorf("init config hash: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read config hash: %w", err)
	}

	if stored != hashStr {
		// Config changed — invalidate all cached data (files + module graph).
		for _, table := range []string{"files", "exports", "imports"} {
			if _, err := c.db.ExecContext(ctx, "DELETE FROM "+table); err != nil { //nolint:gosec // table names are hardcoded
				return fmt.Errorf("invalidate cache table %s: %w", table, err)
			}
		}
		if _, err := c.db.ExecContext(ctx, "UPDATE meta SET value = ? WHERE key = 'config_hash'", hashStr); err != nil {
			return fmt.Errorf("update config hash: %w", err)
		}
	}
	return nil
}

// Lookup returns cached diagnostics for a file if the content hash matches.
// Returns (diags, true, nil) on hit, (nil, false, nil) on miss.
func (c *Cache) Lookup(ctx context.Context, path string, contentHash uint64) ([]engine.Diagnostic, bool, error) {
	var diagJSON []byte
	var storedHash int64

	err := c.db.QueryRowContext(ctx,
		"SELECT content_hash, diag_json FROM files WHERE path = ?", path,
	).Scan(&storedHash, &diagJSON)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("cache lookup %s: %w", path, err)
	}

	if uint64(storedHash) != contentHash { //nolint:gosec // intentional uint64↔int64 for SQLite
		return nil, false, nil
	}

	var diags []engine.Diagnostic
	if diagJSON != nil {
		if err := json.Unmarshal(diagJSON, &diags); err != nil {
			return nil, false, fmt.Errorf("unmarshal cached diagnostics for %s: %w", path, err)
		}
	}
	if diags == nil {
		diags = []engine.Diagnostic{}
	}

	return diags, true, nil
}

// Store upserts a single file entry in the cache.
func (c *Cache) Store(ctx context.Context, entry CacheEntry) error {
	diagJSON, err := marshalDiags(entry.Diagnostics)
	if err != nil {
		return err
	}

	_, err = c.db.ExecContext(ctx, upsertFileSQL,
		entry.Path, int64(entry.ContentHash), entry.ModTimeNS, diagJSON, time.Now().UnixNano()) //nolint:gosec // intentional uint64→int64
	if err != nil {
		return fmt.Errorf("cache store %s: %w", entry.Path, err)
	}
	return nil
}

// StoreBatch upserts multiple file entries in a single transaction.
func (c *Cache) StoreBatch(ctx context.Context, entries []CacheEntry) error {
	if len(entries) == 0 {
		return nil
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin batch transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback on commit is a no-op

	stmt, err := tx.PrepareContext(ctx, upsertFileSQL)
	if err != nil {
		return fmt.Errorf("prepare batch statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	now := time.Now().UnixNano()
	for _, e := range entries {
		diagJSON, err := marshalDiags(e.Diagnostics)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, e.Path, int64(e.ContentHash), e.ModTimeNS, diagJSON, now); err != nil { //nolint:gosec // intentional uint64→int64
			return fmt.Errorf("cache store batch %s: %w", e.Path, err)
		}
	}

	return tx.Commit()
}

// Remove deletes a file entry from the cache.
func (c *Cache) Remove(ctx context.Context, path string) error {
	_, err := c.db.ExecContext(ctx, "DELETE FROM files WHERE path = ?", path)
	if err != nil {
		return fmt.Errorf("cache remove %s: %w", path, err)
	}
	return nil
}

// Close closes the database connection.
func (c *Cache) Close() error {
	return c.db.Close()
}

const upsertFileSQL = `INSERT INTO files (path, content_hash, mod_time_ns, diag_json, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(path) DO UPDATE SET
		   content_hash = excluded.content_hash,
		   mod_time_ns  = excluded.mod_time_ns,
		   diag_json    = excluded.diag_json,
		   updated_at   = excluded.updated_at`

// FileGraphEntry holds graph data for a single file.
type FileGraphEntry struct {
	Path    string
	Exports []ExportInfo
	Imports []ImportInfo
}

// StoreFileGraph replaces all export and import entries for a file in a single transaction.
func (c *Cache) StoreFileGraph(ctx context.Context, path string, exports []ExportInfo, imports []ImportInfo) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin graph transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is no-op

	if _, err := tx.ExecContext(ctx, "DELETE FROM exports WHERE path = ?", path); err != nil {
		return fmt.Errorf("delete exports for %s: %w", path, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM imports WHERE path = ?", path); err != nil {
		return fmt.Errorf("delete imports for %s: %w", path, err)
	}

	for _, e := range exports {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO exports (path, name, kind, line) VALUES (?, ?, ?, ?)",
			path, e.Name, e.Kind, e.Line); err != nil {
			return fmt.Errorf("insert export %s:%s: %w", path, e.Name, err)
		}
	}
	for _, imp := range imports {
		if _, err := tx.ExecContext(ctx,
			"INSERT OR REPLACE INTO imports (path, source, name, line) VALUES (?, ?, ?, ?)",
			path, imp.Source, imp.Name, imp.Line); err != nil {
			return fmt.Errorf("insert import %s:%s: %w", path, imp.Source, err)
		}
	}

	return tx.Commit()
}

// StoreFileGraphBatch stores graph data for multiple files in a single transaction.
func (c *Cache) StoreFileGraphBatch(ctx context.Context, entries []FileGraphEntry) error {
	if len(entries) == 0 {
		return nil
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin graph batch transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is no-op

	for _, entry := range entries {
		if _, err := tx.ExecContext(ctx, "DELETE FROM exports WHERE path = ?", entry.Path); err != nil {
			return fmt.Errorf("delete exports for %s: %w", entry.Path, err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM imports WHERE path = ?", entry.Path); err != nil {
			return fmt.Errorf("delete imports for %s: %w", entry.Path, err)
		}
		for _, e := range entry.Exports {
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO exports (path, name, kind, line) VALUES (?, ?, ?, ?)",
				entry.Path, e.Name, e.Kind, e.Line); err != nil {
				return fmt.Errorf("insert export %s:%s: %w", entry.Path, e.Name, err)
			}
		}
		for _, imp := range entry.Imports {
			if _, err := tx.ExecContext(ctx,
				"INSERT OR REPLACE INTO imports (path, source, name, line) VALUES (?, ?, ?, ?)",
				entry.Path, imp.Source, imp.Name, imp.Line); err != nil {
				return fmt.Errorf("insert import %s:%s: %w", entry.Path, imp.Source, err)
			}
		}
	}

	return tx.Commit()
}

// LoadAllExports loads all exports from the database, grouped by file path.
func (c *Cache) LoadAllExports(ctx context.Context) (map[string][]ExportInfo, error) {
	rows, err := c.db.QueryContext(ctx, "SELECT path, name, kind, line FROM exports")
	if err != nil {
		return nil, fmt.Errorf("load exports: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string][]ExportInfo)
	for rows.Next() {
		var path, name, kind string
		var line int
		if err := rows.Scan(&path, &name, &kind, &line); err != nil {
			return nil, fmt.Errorf("scan export row: %w", err)
		}
		result[path] = append(result[path], ExportInfo{Name: name, Kind: kind, Line: line})
	}
	return result, rows.Err()
}

// LoadAllImports loads all imports from the database, grouped by file path.
func (c *Cache) LoadAllImports(ctx context.Context) (map[string][]ImportInfo, error) {
	rows, err := c.db.QueryContext(ctx, "SELECT path, source, name, line FROM imports")
	if err != nil {
		return nil, fmt.Errorf("load imports: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string][]ImportInfo)
	for rows.Next() {
		var path, source, name string
		var line int
		if err := rows.Scan(&path, &source, &name, &line); err != nil {
			return nil, fmt.Errorf("scan import row: %w", err)
		}
		result[path] = append(result[path], ImportInfo{Source: source, Name: name, Line: line})
	}
	return result, rows.Err()
}

func marshalDiags(diags []engine.Diagnostic) ([]byte, error) {
	if len(diags) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(diags)
	if err != nil {
		return nil, fmt.Errorf("marshal diagnostics: %w", err)
	}
	return data, nil
}
