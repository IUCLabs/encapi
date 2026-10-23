package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/IUCLabs/encapi/internal/auth"
)

type scope int

const (
	scopeTesting scope = iota + 1
	scopeFull
)

type scopeKey struct{}

// requireWrite authenticates a write: the static token or a JWT with the full
// audience grants scopeFull, a JWT with the testing audience grants
// scopeTesting, which only routes registered with allowTesting accept.
func (s *Server) requireWrite(allowTesting bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sc, ok := s.authenticate(w, r)
			if !ok {
				return
			}
			if sc == scopeTesting && !allowTesting {
				writeError(w, http.StatusForbidden, "token audience not permitted on this route")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), scopeKey{}, sc)))
		})
	}
}

func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (scope, bool) {
	a := s.auth
	if a.WriteToken == "" && a.OIDC == nil {
		writeError(w, http.StatusServiceUnavailable, "writes disabled: neither ENCAPI_WRITE_TOKEN nor ENCAPI_OIDC_ISSUER set")
		return 0, false
	}
	presented := bearer(r)
	if presented == "" {
		writeError(w, http.StatusUnauthorized, "invalid or missing write token")
		return 0, false
	}
	if a.WriteToken != "" && subtle.ConstantTimeCompare([]byte(presented), []byte(a.WriteToken)) == 1 {
		return scopeFull, true
	}
	if a.OIDC != nil {
		tok, err := a.OIDC.Verify(r.Context(), presented)
		if errors.Is(err, auth.ErrIssuerUnavailable) {
			writeError(w, http.StatusServiceUnavailable, "token issuer unavailable")
			return 0, false
		}
		if err == nil {
			sc := scopeTesting
			if slices.Contains(tok.Audience, a.Audience) {
				sc = scopeFull
			}
			slog.Info("bearer token accepted", "sub", tok.Subject, "aud", tok.Audience, "method", r.Method, "path", r.URL.Path)
			return sc, true
		}
		slog.Info("bearer token rejected", "error", err)
	}
	writeError(w, http.StatusUnauthorized, "invalid or missing write token")
	return 0, false
}

func bearer(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if after, ok := strings.CutPrefix(h, "Bearer "); ok {
			return after
		}
	}
	return r.Header.Get("token")
}

func structuredLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			slog.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"duration_ms", time.Since(start).Milliseconds(),
				"remote", r.RemoteAddr,
				"request_id", middleware.GetReqID(r.Context()),
			)
		}()
		next.ServeHTTP(ww, r)
	})
}
