package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/IUCLabs/encapi/pkg/models"
)

func TestPutNodeSendsTokenAndBody(t *testing.T) {
	var gotAuth, gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"certname":"h1","role":"roles::base","environment":"testing"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	n, err := c.PutNode(context.Background(), &models.Node{Certname: "h1", Role: "roles::base", Environment: "testing"})
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/nodes/h1" {
		t.Errorf("%s %s", gotMethod, gotPath)
	}
	if n.Role != "roles::base" {
		t.Errorf("node = %+v", n)
	}
}

func TestGetRoleEscapesColons(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"name":"roles::infra::x"}`))
	}))
	defer srv.Close()
	if _, err := New(srv.URL, "").GetRole(context.Background(), "roles::infra::x"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/roles/roles::infra::x" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestErrorMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()
	_, err := New(srv.URL, "").GetNode(context.Background(), "ghost")
	if err == nil || !NotFound(err) {
		t.Fatalf("err = %v, want NotFound", err)
	}
}

func TestENCReturnsRawYAML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("classes:\n- roles::base\n"))
	}))
	defer srv.Close()
	b, err := New(srv.URL, "").ENC(context.Background(), "h1")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "classes:\n- roles::base\n" {
		t.Errorf("enc = %q", b)
	}
}

func TestRestoreFromTesting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/nodes/h1/environment/testing" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"certname":"h1","role":"r","environment":"production"}`))
	}))
	defer srv.Close()
	n, err := New(srv.URL, "t").RestoreFromTesting(context.Background(), "h1")
	if err != nil || n.Environment != "production" {
		t.Fatalf("RestoreFromTesting = %+v, %v", n, err)
	}
}
