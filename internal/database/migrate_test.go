package database

import (
	"context"
	"testing"
)

// encapi's schema predates schema_migrations, so the first start after this
// change re-applies 0001_init against a database that already has the tables.
// The file is IF NOT EXISTS-guarded for exactly that: the run must succeed and
// leave only the tracking row behind.
func TestMigrateAdoptsAnExistingSchema(t *testing.T) {
	requireDB(t)
	ctx := context.Background()

	if _, err := testDB.Pool.Exec(ctx, `DROP TABLE schema_migrations`); err != nil {
		t.Fatalf("drop schema_migrations: %v", err)
	}

	db, err := New(ctx, testDSN, nil)
	if err != nil {
		t.Fatalf("migrate over an existing schema: %v", err)
	}
	defer db.Close()

	var n int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if n != 1 {
		t.Fatalf("schema_migrations rows = %d, want 1", n)
	}
}
