// Package auth verifies OIDC bearer JWTs (Vault identity tokens) for encapi writes.
package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// BearerOIDC verifies JWTs against an OIDC issuer's JWKS. Discovery is lazy so
// startup never depends on the issuer being reachable; a failed discovery is
// not retried until the backoff elapses.
type BearerOIDC struct {
	issuer    string
	audiences []string
	timeout   time.Duration
	backoff   time.Duration
	discover  func(context.Context, string) (*oidc.Provider, error)

	mu       sync.Mutex
	verifier *oidc.IDTokenVerifier
	inflight chan struct{}
	failedAt time.Time
}

// ErrIssuerUnavailable means discovery failed; callers should answer 503.
var ErrIssuerUnavailable = errors.New("oidc issuer unavailable")

// ErrAudience means the token is valid but carries none of the allowed audiences.
var ErrAudience = errors.New("audience not allowed")

// NewBearerOIDC returns nil when issuer is empty, meaning OIDC is disabled.
func NewBearerOIDC(issuer string, audiences []string) *BearerOIDC {
	if issuer == "" {
		return nil
	}
	return &BearerOIDC{issuer: issuer, audiences: audiences, timeout: 10 * time.Second, backoff: 15 * time.Second, discover: oidc.NewProvider}
}

// Verify checks raw's signature, issuer, expiry and that it carries at least
// one allowed audience.
func (b *BearerOIDC) Verify(ctx context.Context, raw string) (*oidc.IDToken, error) {
	v, err := b.getVerifier(ctx)
	if err != nil {
		return nil, ErrIssuerUnavailable
	}
	tok, err := v.Verify(ctx, raw)
	if err != nil {
		return nil, err
	}
	if len(b.audiences) > 0 && !slices.ContainsFunc(tok.Audience, func(a string) bool { return slices.Contains(b.audiences, a) }) {
		return nil, ErrAudience
	}
	return tok, nil
}

// getVerifier runs at most one discovery at a time without holding the lock
// across network I/O; concurrent callers wait for it, and only callers within
// the post-failure backoff fail fast.
func (b *BearerOIDC) getVerifier(ctx context.Context) (*oidc.IDTokenVerifier, error) {
	b.mu.Lock()
	if v := b.verifier; v != nil {
		b.mu.Unlock()
		return v, nil
	}
	if done := b.inflight; done != nil {
		b.mu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.verifier == nil {
			return nil, ErrIssuerUnavailable
		}
		return b.verifier, nil
	}
	if time.Since(b.failedAt) < b.backoff {
		b.mu.Unlock()
		return nil, ErrIssuerUnavailable
	}
	done := make(chan struct{})
	b.inflight = done
	b.mu.Unlock()

	v, err := b.discoverOnce(done)
	if err != nil {
		slog.Error("oidc discovery failed", "issuer", b.issuer, "error", err)
		return nil, err
	}
	return v, nil
}

// discoverOnce releases inflight and waiters on every exit, a panic included,
// recording failedAt unless discovery succeeded.
func (b *BearerOIDC) discoverOnce(done chan struct{}) (v *oidc.IDTokenVerifier, err error) {
	defer func() {
		b.mu.Lock()
		b.inflight = nil
		if v == nil {
			b.failedAt = time.Now()
		}
		b.verifier = v
		b.mu.Unlock()
		close(done)
	}()
	// The provider keeps this context and client for JWKS refreshes, so the
	// deadline lives on the client rather than the context.
	client := &http.Client{Timeout: b.timeout}
	p, err := b.discover(oidc.ClientContext(context.Background(), client), b.issuer)
	if err != nil {
		return nil, err
	}
	return p.Verifier(&oidc.Config{SkipClientIDCheck: true}), nil
}
