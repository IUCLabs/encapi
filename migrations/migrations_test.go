package migrations

import (
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/IUCLabs/golib/pg"
)

// readDir returns the .sql files on disk, keyed by name.
func readDir(t *testing.T) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	out := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		b, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		out[e.Name()] = string(b)
	}
	if len(out) == 0 {
		t.Fatal("no .sql files in migrations/")
	}
	return out
}

// The embedded set is the schema the binary ships, so it must match the
// directory exactly — a file on disk but outside the embed pattern would never
// run in production while still passing a local review.
func TestEmbeddedMatchesDirectory(t *testing.T) {
	onDisk := readDir(t)

	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	embedded := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		b, err := FS.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read embedded %s: %v", e.Name(), err)
		}
		embedded[e.Name()] = string(b)
	}

	if len(embedded) != len(onDisk) {
		t.Fatalf("embedded %d files, migrations/ has %d", len(embedded), len(onDisk))
	}
	for name, want := range onDisk {
		got, ok := embedded[name]
		if !ok {
			t.Errorf("migrations/%s is not embedded", name)
			continue
		}
		if got != want {
			t.Errorf("%s: embedded body differs from migrations/%s", name, name)
		}
	}
}

// Migrations are re-run against a database whose schema predates
// schema_migrations, so every CREATE must tolerate the objects already
// existing.
func TestMigrationsAreIdempotent(t *testing.T) {
	for name, body := range readDir(t) {
		for _, line := range strings.Split(body, "\n") {
			upper := strings.ToUpper(strings.TrimSpace(line))
			if !strings.HasPrefix(upper, "CREATE ") {
				continue
			}
			if !strings.Contains(upper, "IF NOT EXISTS") && !strings.Contains(upper, "CREATE OR REPLACE") {
				t.Errorf("%s: %q is not guarded with IF NOT EXISTS", name, strings.TrimSpace(line))
			}
		}
	}
}

// golib derives the advisory lock key from LockName. Replicas only serialize
// against each other while they agree on the key, so renaming the lock during a
// rolling deploy would let two versions migrate at once.
func TestLockKeyIsStable(t *testing.T) {
	// FNV-1a/64 of "encapi-migrations", reinterpreted as int64.
	const deployedKey int64 = -7124093699699113719
	if got := pg.LockKey(LockName); got != deployedKey {
		t.Fatalf("LockKey(%q) = %d, want %d", LockName, got, deployedKey)
	}
}
