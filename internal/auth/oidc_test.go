package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/IUCLabs/encapi/internal/testsupport"
)

func claims(iss string) jwt.Claims {
	now := time.Now()
	return jwt.Claims{
		Issuer:   iss,
		Subject:  "ci",
		Audience: jwt.Audience{"encapi"},
		IssuedAt: jwt.NewNumericDate(now.Add(-time.Minute)),
		Expiry:   jwt.NewNumericDate(now.Add(time.Hour)),
	}
}

func hs256Token(t *testing.T, c jwt.Claims) string {
	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS256, Key: []byte("0123456789abcdef0123456789abcdef")},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k1"))
	if err != nil {
		t.Fatal(err)
	}
	tok, err := jwt.Signed(sig).Claims(c).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func noneToken(iss string) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"none","typ":"JWT","kid":"k1"}`)) + "." +
		enc([]byte(`{"iss":"`+iss+`","sub":"ci","aud":"encapi","exp":`+strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)+`}`)) + "."
}

func verify(b *BearerOIDC, tok string) error {
	_, err := b.Verify(context.Background(), tok)
	return err
}

func TestVerify(t *testing.T) {
	iss := testsupport.NewOIDCIssuer()
	defer iss.Close()
	b := NewBearerOIDC(iss.URL, []string{"encapi", "encapi-testing"})

	tok, err := b.Verify(context.Background(), iss.Token(nil, "encapi-testing", "agent", time.Hour))
	if err != nil {
		t.Fatalf("valid: %v", err)
	}
	if tok.Subject != "agent" || tok.Audience[0] != "encapi-testing" {
		t.Errorf("token = %+v", tok)
	}

	otherKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	bad := map[string]string{
		"garbage":       "not.a.jwt",
		"bad signature": iss.Token(otherKey, "encapi", "ci", time.Hour),
		"expired":       iss.Token(nil, "encapi", "ci", -time.Hour),
		"wrong iss":     iss.Sign(nil, claims("https://evil.example")),
		"hs256":         hs256Token(t, claims(iss.URL)),
		"alg none":      noneToken(iss.URL),
	}
	for name, tok := range bad {
		if err := verify(b, tok); err == nil || errors.Is(err, ErrIssuerUnavailable) {
			t.Errorf("%s: err = %v, want rejection", name, err)
		}
	}
	if err := verify(b, iss.Token(nil, "nope", "ci", time.Hour)); !errors.Is(err, ErrAudience) {
		t.Errorf("wrong aud: err = %v, want ErrAudience", err)
	}
}

func TestVerifyHangingIssuerBacksOff(t *testing.T) {
	var hits atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)

	b := NewBearerOIDC(srv.URL, nil)
	b.timeout = 100 * time.Millisecond
	b.backoff = time.Hour

	start := time.Now()
	if err := verify(b, "x.y.z"); !errors.Is(err, ErrIssuerUnavailable) {
		t.Fatalf("hanging issuer: err = %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("hanging issuer: took %s, timeout not applied", d)
	}
	if err := verify(b, "x.y.z"); !errors.Is(err, ErrIssuerUnavailable) {
		t.Fatalf("during backoff: err = %v", err)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("issuer hit %d times, want 1", n)
	}
}

func TestVerifyPanickingDiscoveryBacksOff(t *testing.T) {
	b := NewBearerOIDC("http://issuer.invalid", nil)
	b.backoff = time.Hour
	b.discover = func(context.Context, string) (*oidc.Provider, error) { panic("boom") }

	func() {
		defer func() { _ = recover() }()
		_ = verify(b, "x.y.z")
	}()

	got := make(chan error, 1)
	go func() { got <- verify(b, "x.y.z") }()
	select {
	case err := <-got:
		if !errors.Is(err, ErrIssuerUnavailable) {
			t.Fatalf("after panic: err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("after panic: request hung on stale inflight discovery")
	}
}

func TestVerifyConcurrentFirstRequestsWaitForDiscovery(t *testing.T) {
	iss := testsupport.NewOIDCIssuer()
	defer iss.Close()
	var hits atomic.Int32
	inner := iss.Config.Handler
	iss.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			hits.Add(1)
			time.Sleep(200 * time.Millisecond)
		}
		inner.ServeHTTP(w, r)
	})

	b := NewBearerOIDC(iss.URL, nil)
	tok := iss.Token(nil, "encapi", "ci", time.Hour)
	var wg sync.WaitGroup
	errs := make([]error, 20)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = verify(b, tok)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("request %d: %v", i, err)
		}
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("discovery hit %d times, want 1", n)
	}
}

func TestVerifyRetriesAfterBackoff(t *testing.T) {
	iss := testsupport.NewOIDCIssuer()
	defer iss.Close()
	b := NewBearerOIDC(iss.URL, nil)
	b.failedAt = time.Now()
	b.backoff = 50 * time.Millisecond
	tok := iss.Token(nil, "encapi", "ci", time.Hour)
	if err := verify(b, tok); !errors.Is(err, ErrIssuerUnavailable) {
		t.Fatalf("within backoff: err = %v", err)
	}
	time.Sleep(60 * time.Millisecond)
	if err := verify(b, tok); err != nil {
		t.Fatalf("after backoff: %v", err)
	}
}

func TestVerifyIssuerDown(t *testing.T) {
	iss := testsupport.NewOIDCIssuer()
	b := NewBearerOIDC(iss.URL, []string{"encapi"})
	tok := iss.Token(nil, "encapi", "ci", time.Hour)
	iss.Close()
	if err := verify(b, tok); !errors.Is(err, ErrIssuerUnavailable) {
		t.Errorf("issuer down: err = %v", err)
	}
}

func TestDisabled(t *testing.T) {
	if NewBearerOIDC("", nil) != nil {
		t.Error("empty issuer must disable OIDC")
	}
}
