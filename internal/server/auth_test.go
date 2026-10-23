package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/IUCLabs/encapi/internal/auth"
	"github.com/IUCLabs/encapi/internal/database"
	"github.com/IUCLabs/encapi/internal/testsupport"
	"github.com/IUCLabs/encapi/pkg/models"
)

func oidcServer(t *testing.T) (*httptest.Server, *fakeStore, *testsupport.OIDCIssuer) {
	t.Helper()
	iss := testsupport.NewOIDCIssuer()
	t.Cleanup(iss.Close)
	f := newFake()
	f.nodes["h1.example"] = models.Node{Certname: "h1.example", Role: "roles::base", Environment: "production", Params: map[string]any{"k": "v"}}
	srv := New(f, nil, Auth{
		WriteToken:      "s3cret",
		OIDC:            auth.NewBearerOIDC(iss.URL, []string{"encapi", "encapi-testing"}),
		Audience:        "encapi",
		TestingAudience: "encapi-testing",
	})
	ts := httptest.NewServer(srv.Router())
	t.Cleanup(ts.Close)
	return ts, f, iss
}

func put(t *testing.T, url, token, body string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, url, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestOIDCWriteScopes(t *testing.T) {
	ts, f, iss := oidcServer(t)
	full := iss.Token(nil, "encapi", "terraform-enc", time.Hour)
	testingTok := iss.Token(nil, "encapi-testing", "agent", time.Hour)
	other := iss.Token(nil, "artifactapi", "x", time.Hour)
	nodeURL := ts.URL + "/api/v1/nodes/h1.example"
	envURL := nodeURL + "/environment"
	nodeBody := `{"role":"roles::base","environment":"develop"}`

	cases := []struct {
		name, url, token, body string
		want                   int
	}{
		{"full aud PUT node", ts.URL + "/api/v1/nodes/h2", full, nodeBody, http.StatusOK},
		{"full aud PUT role", ts.URL + "/api/v1/roles/r", full, `{}`, http.StatusOK},
		{"full aud set develop", envURL, full, `{"environment":"develop"}`, http.StatusOK},
		{"static token set develop", envURL, "s3cret", `{"environment":"develop"}`, http.StatusOK},
		{"static token PUT node", ts.URL + "/api/v1/nodes/h3", "s3cret", nodeBody, http.StatusOK},
		{"testing aud set testing", envURL, testingTok, `{"environment":"testing"}`, http.StatusOK},
		{"testing aud set develop", envURL, testingTok, `{"environment":"develop"}`, http.StatusForbidden},
		{"testing aud PUT node", nodeURL, testingTok, nodeBody, http.StatusForbidden},
		{"testing aud PUT role", ts.URL + "/api/v1/roles/r", testingTok, `{}`, http.StatusForbidden},
		{"other aud set testing", envURL, other, `{"environment":"testing"}`, http.StatusUnauthorized},
		{"no token set testing", envURL, "", `{"environment":"testing"}`, http.StatusUnauthorized},
		{"unknown node", ts.URL + "/api/v1/nodes/ghost/environment", full, `{"environment":"testing"}`, http.StatusNotFound},
		{"empty env", envURL, full, `{}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		if got := put(t, c.url, c.token, c.body); got != c.want {
			t.Errorf("%s: status %d, want %d", c.name, got, c.want)
		}
	}

	req, _ := http.NewRequest(http.MethodDelete, nodeURL, nil)
	req.Header.Set("Authorization", "Bearer "+testingTok)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("testing aud DELETE node: status %d, want 403", resp.StatusCode)
	}
	if _, ok := f.nodes["h1.example"]; !ok {
		t.Fatal("node deleted by testing token")
	}
}

func TestSetEnvironmentPreservesRoleAndParams(t *testing.T) {
	ts, f, iss := oidcServer(t)
	tok := iss.Token(nil, "encapi-testing", "agent", time.Hour)
	if got := put(t, ts.URL+"/api/v1/nodes/h1.example/environment", tok, `{"environment":"testing"}`); got != http.StatusOK {
		t.Fatalf("status %d", got)
	}
	n := f.nodes["h1.example"]
	if n.Environment != "testing" || n.Role != "roles::base" || n.Params["k"] != "v" {
		t.Errorf("node = %+v", n)
	}
}

func TestOIDCIssuerDown503(t *testing.T) {
	ts, _, iss := oidcServer(t)
	tok := iss.Token(nil, "encapi", "x", time.Hour)
	iss.Close()
	if got := put(t, ts.URL+"/api/v1/nodes/h1.example/environment", tok, `{"environment":"testing"}`); got != http.StatusServiceUnavailable {
		t.Errorf("status %d, want 503", got)
	}
	if got := put(t, ts.URL+"/api/v1/nodes/h1.example/environment", "s3cret", `{"environment":"testing"}`); got != http.StatusOK {
		t.Errorf("static token with issuer down: status %d, want 200", got)
	}
}

func TestUnknownReferenceIs422(t *testing.T) {
	ts, f, _ := oidcServer(t)
	f.nodeErr = fmt.Errorf("upsert node: %w", database.ErrInvalidReference)
	for _, c := range []struct{ url, body string }{
		{ts.URL + "/api/v1/nodes/h1.example/environment", `{"environment":"ghost"}`},
		{ts.URL + "/api/v1/nodes/h2", `{"role":"roles::base","environment":"ghost"}`},
	} {
		req, _ := http.NewRequest(http.MethodPut, c.url, strings.NewReader(c.body))
		req.Header.Set("Authorization", "Bearer s3cret")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("PUT %s: status %d, want 422", c.url, resp.StatusCode)
		}
		if strings.Contains(string(body), "upsert node") {
			t.Errorf("PUT %s leaks internal error: %s", c.url, body)
		}
	}
}
