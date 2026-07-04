// Package server exposes encapi over HTTP: open read endpoints (including the
// two ENC document shapes Puppet consumes) and token-guarded write endpoints.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/IUCLabs/encapi/internal/distro"
	"github.com/IUCLabs/encapi/pkg/models"
)

// Store is the persistence surface the HTTP handlers depend on. *database.DB
// satisfies it; tests supply a fake.
type Store interface {
	UpsertRole(ctx context.Context, r *models.Role) error
	GetRole(ctx context.Context, name string) (*models.Role, error)
	ListRoles(ctx context.Context) ([]models.Role, error)
	DeleteRole(ctx context.Context, name string) error

	UpsertStatus(ctx context.Context, s *models.Status) error
	GetStatus(ctx context.Context, name string) (*models.Status, error)
	ListStatuses(ctx context.Context) ([]models.Status, error)
	DeleteStatus(ctx context.Context, name string) error

	UpsertNode(ctx context.Context, n *models.Node) error
	GetNode(ctx context.Context, certname string) (*models.Node, error)
	ListNodes(ctx context.Context) ([]models.Node, error)
	DeleteNode(ctx context.Context, certname string) error
}

// Server holds handler dependencies.
type Server struct {
	store      Store
	resolver   distro.Resolver
	writeToken string
}

// New builds a Server. writeToken guards mutating endpoints; an empty token
// fails all writes closed.
func New(store Store, resolver distro.Resolver, writeToken string) *Server {
	if resolver == nil {
		resolver = distro.Noop{}
	}
	return &Server{store: store, resolver: resolver, writeToken: writeToken}
}

// Router returns the fully-wired HTTP handler.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(structuredLogger)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	// --- ENC documents Puppet consumes (open) ---
	r.Get("/api/v1/nodes/{certname}/enc", s.handleENCFinal)
	r.Get("/cblr/svc/op/puppet/hostname/{certname}", s.handleENCCobbler)

	// --- JSON reads (open) ---
	r.Get("/api/v1/roles", s.listRoles)
	r.Get("/api/v1/roles/{name}", s.getRole)
	r.Get("/api/v1/statuses", s.listStatuses)
	r.Get("/api/v1/statuses/{name}", s.getStatus)
	r.Get("/api/v1/nodes", s.listNodes)
	r.Get("/api/v1/nodes/{certname}", s.getNode)

	// --- writes (token-guarded) ---
	r.Group(func(r chi.Router) {
		r.Use(s.requireToken)
		r.Put("/api/v1/roles/{name}", s.putRole)
		r.Delete("/api/v1/roles/{name}", s.deleteRole)
		r.Put("/api/v1/statuses/{name}", s.putStatus)
		r.Delete("/api/v1/statuses/{name}", s.deleteStatus)
		r.Put("/api/v1/nodes/{certname}", s.putNode)
		r.Delete("/api/v1/nodes/{certname}", s.deleteNode)
	})

	return r
}

// ListenAndServe runs the HTTP server until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	slog.Info("encapi listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
