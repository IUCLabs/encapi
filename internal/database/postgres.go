// Package database is the Postgres persistence layer for encapi. It stores
// three entities — statuses (Puppet environments), roles (class assignment
// targets with inheritable default params), and nodes (host-to-role
// assignments) — and enforces referential integrity between them.
package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgx connection pool.
type DB struct {
	Pool *pgxpool.Pool
}

// New connects to Postgres, verifies the connection, and runs migrations.
func New(dsn string) (*DB, error) {
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	db := &DB{Pool: pool}
	if err := db.migrate(); err != nil {
		pool.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return db, nil
}

// Close releases the pool.
func (db *DB) Close() { db.Pool.Close() }

func (db *DB) migrate() error {
	_, err := db.Pool.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS statuses (
			name        TEXT PRIMARY KEY,
			description TEXT NOT NULL DEFAULT '',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS roles (
			name           TEXT PRIMARY KEY,
			description    TEXT NOT NULL DEFAULT '',
			default_params JSONB NOT NULL DEFAULT '{}',
			created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS nodes (
			certname    TEXT PRIMARY KEY,
			role        TEXT NOT NULL REFERENCES roles(name) ON UPDATE CASCADE,
			environment TEXT NOT NULL REFERENCES statuses(name) ON UPDATE CASCADE,
			params      JSONB NOT NULL DEFAULT '{}',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		return err
	}
	return nil
}
