package database

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/IUCLabs/encapi/internal/testsupport"
	"github.com/IUCLabs/encapi/pkg/models"
)

// testDB is the shared throwaway database; testDSN reaches the same server.
var (
	testDB  *DB
	testDSN string
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	dsn, terminate, err := testsupport.StartPostgres(ctx)
	if err != nil {
		// Docker unavailable: run so tests self-skip via requireDB.
		os.Exit(m.Run())
	}
	db, err := New(ctx, dsn, nil)
	if err != nil {
		terminate()
		panic(err)
	}
	testDB = db
	testDSN = dsn

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
	if err := testDB.UpsertNode(ctx, badRole); !errors.Is(err, ErrInvalidReference) {
		t.Errorf("UpsertNode unknown role = %v, want ErrInvalidReference", err)
	}
	// node referencing an unknown environment must fail the FK
	badEnv := &models.Node{Certname: "h1", Role: "roles::base", Environment: "ghost"}
	if err := testDB.UpsertNode(ctx, badEnv); !errors.Is(err, ErrInvalidReference) {
		t.Errorf("UpsertNode unknown environment = %v, want ErrInvalidReference", err)
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

func TestSetNodeEnvironment(t *testing.T) {
	requireDB(t)
	clean(t)
	seed(t)
	ctx := context.Background()
	if err := testDB.UpsertStatus(ctx, &models.Status{Name: "production"}); err != nil {
		t.Fatal(err)
	}
	n := &models.Node{Certname: "h1", Role: "roles::base", Environment: "production", Params: map[string]any{"x": "y"}}
	if err := testDB.UpsertNode(ctx, n); err != nil {
		t.Fatal(err)
	}

	got, err := testDB.SetNodeEnvironment(ctx, "h1", "testing")
	if err != nil || got.Environment != "testing" || got.Role != "roles::base" || got.Params["x"] != "y" {
		t.Fatalf("SetNodeEnvironment = %+v, %v", got, err)
	}
	stored, _ := testDB.GetNode(ctx, "h1")
	if stored.Environment != "testing" || stored.Params["x"] != "y" {
		t.Errorf("stored node = %+v", stored)
	}

	if _, err := testDB.SetNodeEnvironment(ctx, "h1", "ghost"); !errors.Is(err, ErrInvalidReference) {
		t.Errorf("unknown environment = %v, want ErrInvalidReference", err)
	}
	if _, err := testDB.SetNodeEnvironment(ctx, "missing", "testing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown node = %v, want ErrNotFound", err)
	}
}

func TestRestoreFromTesting(t *testing.T) {
	requireDB(t)
	clean(t)
	seed(t)
	ctx := context.Background()
	for _, s := range []string{"production", "develop"} {
		if err := testDB.UpsertStatus(ctx, &models.Status{Name: s}); err != nil {
			t.Fatal(err)
		}
	}
	if err := testDB.UpsertNode(ctx, &models.Node{Certname: "h1", Role: "roles::base", Environment: "production"}); err != nil {
		t.Fatal(err)
	}

	if _, err := testDB.RestoreFromTesting(ctx, "h1"); !errors.Is(err, ErrNotInTesting) {
		t.Errorf("restore non-testing = %v, want ErrNotInTesting", err)
	}
	if got, err := testDB.SetNodeEnvironment(ctx, "h1", "testing"); err != nil || got.PreviousEnvironment != "production" {
		t.Fatalf("set testing = %+v, %v", got, err)
	}
	// re-entering testing must not overwrite the remembered environment
	if got, _ := testDB.SetNodeEnvironment(ctx, "h1", "testing"); got.PreviousEnvironment != "production" {
		t.Errorf("repeat set testing previous = %q, want production", got.PreviousEnvironment)
	}
	got, err := testDB.RestoreFromTesting(ctx, "h1")
	if err != nil || got.Environment != "production" || got.PreviousEnvironment != "" || got.Role != "roles::base" {
		t.Fatalf("restore = %+v, %v", got, err)
	}
	if stored, _ := testDB.GetNode(ctx, "h1"); stored.Environment != "production" || stored.PreviousEnvironment != "" {
		t.Errorf("stored after restore = %+v", stored)
	}

	// leaving testing by any other route clears the remembered environment
	_, _ = testDB.SetNodeEnvironment(ctx, "h1", "testing")
	if got, _ := testDB.SetNodeEnvironment(ctx, "h1", "develop"); got.PreviousEnvironment != "" {
		t.Errorf("leave testing previous = %q, want empty", got.PreviousEnvironment)
	}

	// PutNode follows the same rules
	n := &models.Node{Certname: "h1", Role: "roles::base", Environment: "testing"}
	if err := testDB.UpsertNode(ctx, n); err != nil || n.PreviousEnvironment != "develop" {
		t.Fatalf("upsert testing = %+v, %v", n, err)
	}
	n = &models.Node{Certname: "h1", Role: "roles::base", Environment: "production"}
	if err := testDB.UpsertNode(ctx, n); err != nil || n.PreviousEnvironment != "" {
		t.Fatalf("upsert production = %+v, %v", n, err)
	}

	// a node created straight into testing has nothing to restore
	if err := testDB.UpsertNode(ctx, &models.Node{Certname: "h2", Role: "roles::base", Environment: "testing"}); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.RestoreFromTesting(ctx, "h2"); !errors.Is(err, ErrNoPreviousEnvironment) {
		t.Errorf("restore without previous = %v, want ErrNoPreviousEnvironment", err)
	}
	if _, err := testDB.RestoreFromTesting(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("restore unknown = %v, want ErrNotFound", err)
	}

	// deleting the remembered status drops the reference rather than blocking
	_, _ = testDB.SetNodeEnvironment(ctx, "h1", "develop")
	_, _ = testDB.SetNodeEnvironment(ctx, "h1", "testing")
	if err := testDB.DeleteStatus(ctx, "develop"); err != nil {
		t.Fatalf("delete remembered status: %v", err)
	}
	if _, err := testDB.RestoreFromTesting(ctx, "h1"); !errors.Is(err, ErrNoPreviousEnvironment) {
		t.Errorf("restore after status delete = %v, want ErrNoPreviousEnvironment", err)
	}
}
