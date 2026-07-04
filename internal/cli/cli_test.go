package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/IUCLabs/encapi/internal/database"
	"github.com/IUCLabs/encapi/internal/distro"
	"github.com/IUCLabs/encapi/internal/server"
	"github.com/IUCLabs/encapi/pkg/models"
)

// memStore implements server.Store in memory for end-to-end CLI tests.
type memStore struct {
	roles    map[string]models.Role
	statuses map[string]models.Status
	nodes    map[string]models.Node
}

func newMem() *memStore {
	return &memStore{
		roles:    map[string]models.Role{"roles::base": {Name: "roles::base"}},
		statuses: map[string]models.Status{"testing": {Name: "testing"}, "production": {Name: "production"}},
		nodes:    map[string]models.Node{},
	}
}

func (m *memStore) UpsertRole(_ context.Context, r *models.Role) error {
	m.roles[r.Name] = *r
	return nil
}
func (m *memStore) GetRole(_ context.Context, n string) (*models.Role, error) {
	r, ok := m.roles[n]
	if !ok {
		return nil, database.ErrNotFound
	}
	return &r, nil
}
func (m *memStore) ListRoles(context.Context) ([]models.Role, error) {
	out := []models.Role{}
	for _, r := range m.roles {
		out = append(out, r)
	}
	return out, nil
}
func (m *memStore) DeleteRole(_ context.Context, n string) error {
	if _, ok := m.roles[n]; !ok {
		return database.ErrNotFound
	}
	delete(m.roles, n)
	return nil
}
func (m *memStore) UpsertStatus(_ context.Context, s *models.Status) error {
	m.statuses[s.Name] = *s
	return nil
}
func (m *memStore) GetStatus(_ context.Context, n string) (*models.Status, error) {
	s, ok := m.statuses[n]
	if !ok {
		return nil, database.ErrNotFound
	}
	return &s, nil
}
func (m *memStore) ListStatuses(context.Context) ([]models.Status, error) {
	out := []models.Status{}
	for _, s := range m.statuses {
		out = append(out, s)
	}
	return out, nil
}
func (m *memStore) DeleteStatus(_ context.Context, n string) error {
	if _, ok := m.statuses[n]; !ok {
		return database.ErrNotFound
	}
	delete(m.statuses, n)
	return nil
}
func (m *memStore) UpsertNode(_ context.Context, n *models.Node) error {
	m.nodes[n.Certname] = *n
	return nil
}
func (m *memStore) GetNode(_ context.Context, c string) (*models.Node, error) {
	n, ok := m.nodes[c]
	if !ok {
		return nil, database.ErrNotFound
	}
	return &n, nil
}
func (m *memStore) ListNodes(context.Context) ([]models.Node, error) {
	out := []models.Node{}
	for _, n := range m.nodes {
		out = append(out, n)
	}
	return out, nil
}
func (m *memStore) DeleteNode(_ context.Context, c string) error {
	if _, ok := m.nodes[c]; !ok {
		return database.ErrNotFound
	}
	delete(m.nodes, c)
	return nil
}

func run(t *testing.T, env Env, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Run(args, env, &out, &errb)
	return code, out.String(), errb.String()
}

func serverEnv(t *testing.T) (Env, *memStore) {
	t.Helper()
	store := newMem()
	srv := server.New(store, distro.Noop{}, "tok")
	ts := httptest.NewServer(srv.Router())
	t.Cleanup(ts.Close)
	return Env{URL: ts.URL, Token: "tok"}, store
}

func TestParseParams(t *testing.T) {
	got, err := parseParams([]string{"s=hello", "n=3", "b=true", `j={"a":1}`})
	if err != nil {
		t.Fatal(err)
	}
	if got["s"] != "hello" || got["n"] != float64(3) || got["b"] != true {
		t.Errorf("params = %#v", got)
	}
	if obj, ok := got["j"].(map[string]any); !ok || obj["a"] != float64(1) {
		t.Errorf("json param = %#v", got["j"])
	}
	if _, err := parseParams([]string{"noequals"}); err == nil {
		t.Error("expected error for malformed param")
	}
}

func TestNodeSetAndClassify(t *testing.T) {
	env, _ := serverEnv(t)

	code, _, errb := run(t, env, "node", "set", "web1.example", "--role", "roles::base", "--env", "testing", "--param", "x=1")
	if code != 0 {
		t.Fatalf("node set exit %d: %s", code, errb)
	}

	code, out, errb := run(t, env, "classify", "web1.example")
	if code != 0 {
		t.Fatalf("classify exit %d: %s", code, errb)
	}
	// testing env dropped, classes list form, enc_role present
	if strings.Contains(out, "environment:") {
		t.Errorf("classify output should drop testing environment:\n%s", out)
	}
	if !strings.Contains(out, "- roles::base") || !strings.Contains(out, "enc_role") {
		t.Errorf("classify output missing expected fields:\n%s", out)
	}
}

func TestNodeSetRequiresFlags(t *testing.T) {
	env, _ := serverEnv(t)
	code, _, _ := run(t, env, "node", "set", "h")
	if code != 2 {
		t.Errorf("exit = %d, want 2 for missing flags", code)
	}
}

func TestRoleSetWithDefaultParams(t *testing.T) {
	env, store := serverEnv(t)
	// epel=9 auto-parses as a JSON number; the quoted form forces a string.
	code, _, errb := run(t, env, "role", "set", "roles::infra::x", "--desc", "X role", "--param", "replicas=9", "--param", `epel="9"`)
	if code != 0 {
		t.Fatalf("role set exit %d: %s", code, errb)
	}
	r := store.roles["roles::infra::x"]
	if r.Description != "X role" || r.DefaultParams["replicas"] != float64(9) || r.DefaultParams["epel"] != "9" {
		t.Errorf("stored role = %+v", r)
	}
}

func TestStatusLifecycle(t *testing.T) {
	env, store := serverEnv(t)
	if code, _, e := run(t, env, "status", "set", "development", "--desc", "dev"); code != 0 {
		t.Fatalf("status set: %s", e)
	}
	if _, ok := store.statuses["development"]; !ok {
		t.Error("development status not stored")
	}
	if code, _, _ := run(t, env, "status", "delete", "development"); code != 0 {
		t.Error("status delete failed")
	}
}

func TestClassifyUnknownNodeFails(t *testing.T) {
	env, _ := serverEnv(t)
	code, _, _ := run(t, env, "classify", "ghost")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
}

func TestUnknownCommand(t *testing.T) {
	code, _, _ := run(t, Env{URL: "http://x"}, "bogus")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

func TestImportCobblerDryRun(t *testing.T) {
	// Fake PuppetDB + Cobbler.
	pdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"certname":"h1.example"},{"certname":"h2.example"}]`))
	}))
	defer pdb.Close()
	cob := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "h1.example") {
			_, _ = w.Write([]byte("classes:\n  roles::infra::storage::vault: {}\nenvironment: testing\nparameters: {}\n"))
		} else {
			_, _ = w.Write([]byte("classes:\n  roles::base: {}\nenvironment: production\nparameters: {}\n"))
		}
	}))
	defer cob.Close()

	env, _ := serverEnv(t)
	code, out, errb := run(t, env, "import-cobbler", "--puppetdb-url", pdb.URL, "--cobbler-url", cob.URL, "--dry-run")
	if code != 0 {
		t.Fatalf("import exit %d: %s", code, errb)
	}
	if !strings.Contains(out, "roles::infra::storage::vault") || !strings.Contains(out, "roles::base") {
		t.Errorf("dry-run output:\n%s", out)
	}
}

func TestImportCobblerWrites(t *testing.T) {
	pdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"certname":"h1.example"}]`))
	}))
	defer pdb.Close()
	cob := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("classes:\n  roles::infra::dns::master: {}\nenvironment: production\nparameters: {}\n"))
	}))
	defer cob.Close()

	env, store := serverEnv(t)
	code, out, errb := run(t, env, "import-cobbler", "--puppetdb-url", pdb.URL, "--cobbler-url", cob.URL)
	if code != 0 {
		t.Fatalf("import exit %d: %s", code, errb)
	}
	n, ok := store.nodes["h1.example"]
	if !ok || n.Role != "roles::infra::dns::master" || n.Environment != "production" {
		t.Errorf("imported node = %+v (out=%s)", n, out)
	}
	if _, ok := store.roles["roles::infra::dns::master"]; !ok {
		t.Error("role not created by import")
	}
}
