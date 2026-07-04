package database

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/IUCLabs/encapi/internal/testsupport"
	"github.com/IUCLabs/encapi/pkg/models"
)

var testDB *DB

func TestMain(m *testing.M) {
	ctx := context.Background()
	dsn, terminate, err := testsupport.StartPostgres(ctx)
	if err != nil {
		// Docker unavailable: run so tests self-skip via requireDB.
		os.Exit(m.Run())
	}
	db, err := New(dsn)
	if err != nil {
		terminate()
		panic(err)
	}
	testDB = db

	code := m.Run()
	db.Close()
	terminate()
	if code != 0 {
		os.Exit(code)
	}
}

func requireDB(t *testing.T) {
	t.Helper()
	if testDB == nil {
		t.Skip("Docker unavailable; skipping database integration test")
	}
}

// clean truncates all tables between tests for isolation.
func clean(t *testing.T) {
	t.Helper()
	_, err := testDB.Pool.Exec(context.Background(), `TRUNCATE nodes, roles, statuses CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func seed(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if err := testDB.UpsertStatus(ctx, &models.Status{Name: "testing"}); err != nil {
		t.Fatalf("seed status: %v", err)
	}
	if err := testDB.UpsertRole(ctx, &models.Role{Name: "roles::base"}); err != nil {
		t.Fatalf("seed role: %v", err)
	}
}

func TestStatusCRUD(t *testing.T) {
	requireDB(t)
	clean(t)
	ctx := context.Background()

	if err := testDB.UpsertStatus(ctx, &models.Status{Name: "production", Description: "prod"}); err != nil {
		t.Fatal(err)
	}
	got, err := testDB.GetStatus(ctx, "production")
	if err != nil || got.Description != "prod" {
		t.Fatalf("GetStatus = %+v, %v", got, err)
	}
	// upsert updates description
	if err := testDB.UpsertStatus(ctx, &models.Status{Name: "production", Description: "changed"}); err != nil {
		t.Fatal(err)
	}
	got, _ = testDB.GetStatus(ctx, "production")
	if got.Description != "changed" {
		t.Errorf("description = %q, want changed", got.Description)
	}
	list, err := testDB.ListStatuses(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListStatuses = %v, %v", list, err)
	}
	if err := testDB.DeleteStatus(ctx, "production"); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.GetStatus(ctx, "production"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetStatus after delete = %v, want ErrNotFound", err)
	}
}

func TestRoleCRUDWithParams(t *testing.T) {
	requireDB(t)
	clean(t)
	ctx := context.Background()

	r := &models.Role{Name: "roles::infra::storage::minio", Description: "minio", DefaultParams: map[string]any{"minio_pool": "pool1", "replicas": float64(3)}}
	if err := testDB.UpsertRole(ctx, r); err != nil {
		t.Fatal(err)
	}
	got, err := testDB.GetRole(ctx, r.Name)
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultParams["minio_pool"] != "pool1" || got.DefaultParams["replicas"] != float64(3) {
		t.Errorf("default_params = %#v", got.DefaultParams)
	}
	if _, err := testDB.GetRole(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetRole(nope) = %v, want ErrNotFound", err)
	}
}

func TestNodeCRUDAndForeignKeys(t *testing.T) {
	requireDB(t)
	clean(t)
	seed(t)
	ctx := context.Background()

	// node referencing an unknown role must fail the FK
	badRole := &models.Node{Certname: "h1", Role: "roles::ghost", Environment: "testing"}
	if err := testDB.UpsertNode(ctx, badRole); err == nil {
		t.Error("expected FK violation for unknown role")
	}
	// node referencing an unknown environment must fail the FK
	badEnv := &models.Node{Certname: "h1", Role: "roles::base", Environment: "ghost"}
	if err := testDB.UpsertNode(ctx, badEnv); err == nil {
		t.Error("expected FK violation for unknown environment")
	}

	n := &models.Node{Certname: "h1", Role: "roles::base", Environment: "testing", Params: map[string]any{"x": "y"}}
	if err := testDB.UpsertNode(ctx, n); err != nil {
		t.Fatal(err)
	}
	got, err := testDB.GetNode(ctx, "h1")
	if err != nil || got.Role != "roles::base" || got.Params["x"] != "y" {
		t.Fatalf("GetNode = %+v, %v", got, err)
	}

	// role in use cannot be deleted
	if err := testDB.DeleteRole(ctx, "roles::base"); err == nil {
		t.Error("expected error deleting role in use")
	}
	// status in use cannot be deleted
	if err := testDB.DeleteStatus(ctx, "testing"); err == nil {
		t.Error("expected error deleting status in use")
	}

	list, err := testDB.ListNodes(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListNodes = %v, %v", list, err)
	}
	if err := testDB.DeleteNode(ctx, "h1"); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.GetNode(ctx, "h1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetNode after delete = %v, want ErrNotFound", err)
	}
	if err := testDB.DeleteNode(ctx, "h1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteNode missing = %v, want ErrNotFound", err)
	}
}
