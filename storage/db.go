package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite connection.
type DB struct {
	conn *sql.DB
}

// Open opens or creates a SQLite database at the given path.
func Open(path string) (*DB, error) {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Test connection
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &DB{conn: conn}, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Migrate applies the schema and runs pending migrations.
func (db *DB) Migrate() error {
	// Apply base schema
	if _, err := db.conn.Exec(schema); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}

	// Run migrations
	for _, m := range migrations {
		applied, err := db.isMigrationApplied(m.id)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", m.id, err)
		}
		if applied {
			continue
		}

		if _, err := db.conn.Exec(m.sql); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.id, err)
		}
		if _, err := db.conn.Exec("INSERT INTO migrations (id) VALUES (?)", m.id); err != nil {
			return fmt.Errorf("record migration %s: %w", m.id, err)
		}
	}

	return nil
}

func (db *DB) isMigrationApplied(id string) (bool, error) {
	var count int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM migrations WHERE id = ?", id).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

const schema = `
-- Main chunks table
CREATE TABLE IF NOT EXISTS chunks (
    id TEXT PRIMARY KEY,
    content TEXT NOT NULL,
    metadata JSON,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- FTS5 virtual table for search
CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
    id,
    content,
    metadata,
    content='chunks',
    content_rowid='rowid'
);

-- Triggers to keep FTS in sync
CREATE TRIGGER IF NOT EXISTS chunks_ai AFTER INSERT ON chunks BEGIN
    INSERT INTO chunks_fts(rowid, id, content, metadata)
    VALUES (NEW.rowid, NEW.id, NEW.content, NEW.metadata);
END;

CREATE TRIGGER IF NOT EXISTS chunks_ad AFTER DELETE ON chunks BEGIN
    INSERT INTO chunks_fts(chunks_fts, rowid, id, content, metadata)
    VALUES('delete', OLD.rowid, OLD.id, OLD.content, OLD.metadata);
END;

CREATE TRIGGER IF NOT EXISTS chunks_au AFTER UPDATE ON chunks BEGIN
    INSERT INTO chunks_fts(chunks_fts, rowid, id, content, metadata)
    VALUES('delete', OLD.rowid, OLD.id, OLD.content, OLD.metadata);
    INSERT INTO chunks_fts(rowid, id, content, metadata)
    VALUES (NEW.rowid, NEW.id, NEW.content, NEW.metadata);
END;

-- Migration tracking
CREATE TABLE IF NOT EXISTS migrations (
    id TEXT PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`

type migration struct {
	id  string
	sql string
}

var migrations = []migration{
	{
		"001_settings",
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
	},
	{
		"002_tokens",
		`CREATE TABLE IF NOT EXISTS tokens (
			hash TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			client_id TEXT NOT NULL,
			expires_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_tokens_expires ON tokens(expires_at);
		CREATE INDEX IF NOT EXISTS idx_tokens_type ON tokens(type, expires_at);`,
	},
	{
		"003_oauth_clients",
		`CREATE TABLE IF NOT EXISTS oauth_clients (
			client_id TEXT PRIMARY KEY,
			client_name TEXT,
			redirect_uris TEXT NOT NULL,
			created_at INTEGER DEFAULT (unixepoch()),
			last_used_at INTEGER DEFAULT (unixepoch())
		);`,
	},
	{
		"004_tokens_data",
		`ALTER TABLE tokens ADD COLUMN data TEXT;`,
	},
}
