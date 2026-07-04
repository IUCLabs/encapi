package distro

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewReturnsNoopWhenEmpty(t *testing.T) {
	if _, ok := New("").(Noop); !ok {
		t.Fatal("New(\"\") should return Noop")
	}
	got, err := New("").Resolve(context.Background(), "host")
	if err != nil || got != nil {
		t.Errorf("Noop.Resolve = %v, %v; want nil, nil", got, err)
	}
}

func TestHTTPResolverWrappedParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/host.example" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"params":{"epel":"9","operatingsystemrelease":"9.6"}}`))
	}))
	defer srv.Close()

	got, err := New(srv.URL).Resolve(context.Background(), "host.example")
	if err != nil {
		t.Fatal(err)
	}
	if got["epel"] != "9" || got["operatingsystemrelease"] != "9.6" {
		t.Errorf("params = %#v", got)
	}
}

func TestHTTPResolverBareObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"epel":"8"}`))
	}))
	defer srv.Close()

	got, err := New(srv.URL).Resolve(context.Background(), "h")
	if err != nil {
		t.Fatal(err)
	}
	if got["epel"] != "8" {
		t.Errorf("params = %#v", got)
	}
}

func TestHTTPResolver404IsNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	got, err := New(srv.URL).Resolve(context.Background(), "unknown")
	if err != nil || got != nil {
		t.Errorf("got %v, %v; want nil, nil for 404", got, err)
	}
}

func TestHTTPResolverServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := New(srv.URL).Resolve(context.Background(), "h"); err == nil {
		t.Fatal("expected error on HTTP 500")
	}
}
