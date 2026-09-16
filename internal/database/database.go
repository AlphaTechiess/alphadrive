package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type DB struct{ *sql.DB }

func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "alphadrive.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, statement := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA temp_store = MEMORY",
		"PRAGMA cache_size = -2000",
	} {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err := migrate(context.Background(), db); err != nil {
		db.Close()
		return nil, err
	}
	return &DB{db}, nil
}

type migration struct {
	version int
	name    string
	queries []string
}

var migrations = []migration{
	{
		version: 1,
		name:    "initial_schema",
		queries: []string{
			`CREATE TABLE IF NOT EXISTS users (
				id TEXT PRIMARY KEY,
				username TEXT NOT NULL UNIQUE,
				password_hash TEXT NOT NULL,
				is_admin INTEGER NOT NULL DEFAULT 0,
				created_at INTEGER NOT NULL,
				updated_at INTEGER NOT NULL,
				last_login_at INTEGER,
				disabled_at INTEGER
			)`,
			`CREATE TABLE IF NOT EXISTS sessions (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				token_hash BLOB NOT NULL UNIQUE,
				csrf_secret BLOB NOT NULL,
				created_at INTEGER NOT NULL,
				last_seen_at INTEGER NOT NULL,
				expires_at INTEGER NOT NULL,
				revoked_at INTEGER
			)`,
			`CREATE TABLE IF NOT EXISTS nodes (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				parent_id TEXT REFERENCES nodes(id) ON DELETE RESTRICT,
				kind TEXT NOT NULL CHECK(kind IN ('file','folder')),
				name TEXT NOT NULL,
				storage_key TEXT UNIQUE,
				mime_type TEXT,
				size_bytes INTEGER NOT NULL DEFAULT 0 CHECK(size_bytes >= 0),
				created_at INTEGER NOT NULL,
				updated_at INTEGER NOT NULL,
				trashed_at INTEGER,
				CHECK((kind='folder' AND storage_key IS NULL) OR (kind='file' AND storage_key IS NOT NULL))
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS nodes_visible_sibling_name ON nodes(user_id, parent_id, name) WHERE trashed_at IS NULL`,
			`CREATE INDEX IF NOT EXISTS nodes_children ON nodes(user_id, parent_id, trashed_at, name)`,
			`CREATE INDEX IF NOT EXISTS sessions_lookup ON sessions(token_hash, expires_at)`,
		},
	},
	{
		version: 2,
		name:    "public_shares",
		queries: []string{
			`CREATE TABLE IF NOT EXISTS shares (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
				slug TEXT NOT NULL UNIQUE,
				password_hash TEXT,
				expires_at INTEGER,
				view_count INTEGER NOT NULL DEFAULT 0,
				created_at INTEGER NOT NULL,
				updated_at INTEGER NOT NULL,
				revoked_at INTEGER
			)`,
			`CREATE INDEX IF NOT EXISTS shares_lookup ON shares(slug, revoked_at, expires_at)`,
			`CREATE INDEX IF NOT EXISTS shares_node ON shares(node_id, revoked_at)`,
			`CREATE INDEX IF NOT EXISTS shares_user ON shares(user_id, revoked_at)`,
		},
	},
	{
		version: 3,
		name:    "share_grants",
		queries: []string{
			`CREATE TABLE IF NOT EXISTS share_grants (
				id TEXT PRIMARY KEY,
				share_id TEXT NOT NULL REFERENCES shares(id) ON DELETE CASCADE,
				token_hash BLOB NOT NULL UNIQUE,
				created_at INTEGER NOT NULL,
				expires_at INTEGER NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS share_grants_lookup ON share_grants(token_hash, expires_at)`,
			`CREATE INDEX IF NOT EXISTS share_grants_share ON share_grants(share_id)`,
		},
	},
	{
		version: 4,
		name:    "add_name_to_users",
		queries: []string{
			`ALTER TABLE users ADD COLUMN name TEXT NOT NULL DEFAULT ''`,
		},
	},
}

func migrate(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	for _, m := range migrations {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE version=?`, m.version).Scan(&count); err != nil {
			return fmt.Errorf("check migration version %d: %w", m.version, err)
		}
		if count > 0 {
			continue
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin tx for migration %d (%s): %w", m.version, m.name, err)
		}

		for _, query := range m.queries {
			if _, err := tx.ExecContext(ctx, query); err != nil {
				tx.Rollback()
				return fmt.Errorf("execute migration %d query: %w", m.version, err)
			}
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, unixepoch())`, m.version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}
	}

	return nil
}
