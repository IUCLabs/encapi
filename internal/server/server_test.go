package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/IUCLabs/encapi/internal/database"
	"github.com/IUCLabs/encapi/pkg/models"
)

// fakeStore is an in-memory Store for handler tests.
type fakeStore struct {
	roles    map[string]models.Role
	statuses map[string]models.Status
	nodes    map[string]models.Node
}

func newFake() *fakeStore {
	return &fakeStore{
		roles:    map[string]models.Role{},
		statuses: map[string]models.Status{},
		nodes:    map[string]models.Node{},
	}
}

func (f *fakeStore) UpsertRole(_ context.Context, r *models.Role) error {
	f.roles[r.Name] = *r
	return nil
}
func (f *fakeStore) GetRole(_ context.Context, name string) (*models.Role, error) {
	r, ok := f.roles[name]
	if !ok {
		return nil, database.ErrNotFound
	}
	return &r, nil
}
func (f *fakeStore) ListRoles(context.Context) ([]models.Role, error) {
	out := []models.Role{}
	for _, r := range f.roles {
		out = append(out, r)
	}
	return out, nil
}
func (f *fakeStore) DeleteRole(_ context.Context, name string) error {
	if _, ok := f.roles[name]; !ok {
		return database.ErrNotFound
	}
	delete(f.roles, name)
	return nil
}
func (f *fakeStore) UpsertStatus(_ context.Context, s *models.Status) error {
	f.statuses[s.Name] = *s
	return nil
}
func (f *fakeStore) GetStatus(_ context.Context, name string) (*models.Status, error) {
	s, ok := f.statuses[name]
	if !ok {
		return nil, database.ErrNotFound
	}
	return &s, nil
}
func (f *fakeStore) ListStatuses(context.Context) ([]models.Status, error) {
	out := []models.Status{}
	for _, s := range f.statuses {
		out = append(out, s)
	}
	return out, nil
}
func (f *fakeStore) DeleteStatus(_ context.Context, name string) error {
	if _, ok := f.statuses[name]; !ok {
		return database.ErrNotFound
	}
	delete(f.statuses, name)
	return nil
}
func (f *fakeStore) UpsertNode(_ context.Context, n *models.Node) error {
	f.nodes[n.Certname] = *n
	return nil
}
func (f *fakeStore) GetNode(_ context.Context, certname string) (*models.Node, error) {
	n, ok := f.nodes[certname]
	if !ok {
		return nil, database.ErrNotFound
	}
	return &n, nil
}
func (f *fakeStore) ListNodes(context.Context) ([]models.Node, error) {
	out := []models.Node{}
	for _, n := range f.nodes {
		out = append(out, n)
	}
	return out, nil
}
func (f *fakeStore) DeleteNode(_ context.Context, certname string) error {
	if _, ok := f.nodes[certname]; !ok {
		return database.ErrNotFound
	}
	delete(f.nodes, certname)
	return nil
}

func testServer() (*httptest.Server, *fakeStore) {
	f := newFake()
	f.statuses["testing"] = models.Status{Name: "testing"}
	f.statuses["production"] = models.Status{Name: "production"}
	f.roles["roles::infra::storage::vault"] = models.Role{Name: "roles::infra::storage::vault"}
	f.nodes["h1.example"] = models.Node{Certname: "h1.example", Role: "roles::infra::storage::vault", Environment: "testing"}
	srv := New(f, nil, Auth{WriteToken: "s3cret"})
	return httptest.NewServer(srv.Router()), f
}

func TestReadsAreOpen(t *testing.T) {
	ts, _ := testServer()
	defer ts.Close()
	for _, path := range []string{"/healthz", "/api/v1/roles", "/api/v1/nodes", "/api/v1/statuses", "/api/v1/nodes/h1.example"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestWriteRequiresToken(t *testing.T) {
	ts, _ := testServer()
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/roles/roles::x", strings.NewReader(`{}`))
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()

	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/api/v1/roles/roles::x", strings.NewReader(`{"description":"d"}`))
	req.Header.Set("Authorization", "Bearer s3cret")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("with token = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestWriteBareTokenHeader(t *testing.T) {
	ts, _ := testServer()
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/statuses/dev", strings.NewReader(`{"description":"d"}`))
	req.Header.Set("token", "s3cret")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bare token header = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestWritesDisabledWithoutServerToken(t *testing.T) {
	f := newFake()
	srv := New(f, nil, Auth{}) // no auth configured
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/roles/r", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer anything")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("= %d, want 503", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestENCFinalEndpoint(t *testing.T) {
	ts, _ := testServer()
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/nodes/h1.example/enc")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var doc map[string]any
	if err := yaml.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc["environment"]; ok {
		t.Error("testing environment must be dropped in final ENC")
	}
	classes := doc["classes"].([]any)
	if classes[0] != "roles::infra::storage::vault" {
		t.Errorf("classes = %#v", classes)
	}
}

func TestENCCobblerEndpoint(t *testing.T) {
	ts, _ := testServer()
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/cblr/svc/op/puppet/hostname/h1.example")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var doc map[string]any
	if err := yaml.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc["environment"] != "testing" {
		t.Errorf("cobbler form must keep environment, got %v", doc["environment"])
	}
	if _, ok := doc["classes"].(map[string]any); !ok {
		t.Errorf("cobbler classes must be a map, got %#v", doc["classes"])
	}
}

func TestENCUnknownNode404(t *testing.T) {
	ts, _ := testServer()
	defer ts.Close()
	resp, _ := http.Get(ts.URL + "/api/v1/nodes/ghost/enc")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("= %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestPutNodeValidation(t *testing.T) {
	ts, _ := testServer()
	defer ts.Close()
	// missing role/environment
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/nodes/h2", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer s3cret")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("= %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestPutAndGetNodeRoundTrip(t *testing.T) {
	ts, _ := testServer()
	defer ts.Close()
	body := `{"role":"roles::infra::storage::vault","environment":"production","params":{"k":"v"}}`
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/nodes/h9", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer s3cret")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err := http.Get(ts.URL + "/api/v1/nodes/h9")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var n models.Node
	_ = json.NewDecoder(resp.Body).Decode(&n)
	if n.Role != "roles::infra::storage::vault" || n.Environment != "production" || n.Params["k"] != "v" {
		t.Errorf("round trip node = %+v", n)
	}
}
