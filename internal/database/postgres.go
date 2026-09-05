// Package database is the Postgres persistence layer for encapi. It stores
// three entities — statuses (Puppet environments), roles (class assignment
// targets with inheritable default params), and nodes (host-to-role
// assignments) — and enforces referential integrity between them.
package database

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/IUCLabs/golib/pg"

	"github.com/IUCLabs/encapi/migrations"
)

// DB wraps a pgx connection pool.
type DB struct {
	Pool *pgxpool.Pool
}

// New connects to Postgres, verifies the connection, and brings the schema up
// to date from the embedded migrations before returning, so the server never
// serves against a half-migrated database. log may be nil.
func New(ctx context.Context, dsn string, log *slog.Logger) (*DB, error) {
	pool, err := pg.NewMigrated(ctx, dsn, migrations.FS, pg.MigrateOptions{
		LockName: migrations.LockName,
		Logger:   log,
	})
	if err != nil {
		return nil, err
	}
	return &DB{Pool: pool}, nil
}

// Close releases the pool.
func (db *DB) Close() { db.Pool.Close() }
