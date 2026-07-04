package server

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// requireToken enforces a static bearer token on mutating endpoints. The token
// is accepted either as "Authorization: Bearer <token>" or a bare "token"
// header. When no server token is configured, all writes are refused.
func (s *Server) requireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.writeToken == "" {
			writeError(w, http.StatusServiceUnavailable, "writes disabled: ENCAPI_WRITE_TOKEN not set")
			return
		}
		presented := bearer(r)
		if presented == "" || subtle.ConstantTimeCompare([]byte(presented), []byte(s.writeToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "invalid or missing write token")
			return
		}
		next.ServeHTTP(w, r)
	})
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
