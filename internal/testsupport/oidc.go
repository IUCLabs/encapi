package testsupport

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// OIDCIssuer is a minimal OIDC issuer serving discovery and JWKS, signing
// RS256 tokens with a generated key.
type OIDCIssuer struct {
	*httptest.Server
	key *rsa.PrivateKey
}

func NewOIDCIssuer() *OIDCIssuer {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	iss := &OIDCIssuer{key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                iss.URL,
			"jwks_uri":                              iss.URL + "/keys",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
			{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"},
		}})
	})
	iss.Server = httptest.NewServer(mux)
	return iss
}

// Token signs a token for aud and sub. Pass a nil key to use the issuer's own;
// expiry is relative to now.
func (o *OIDCIssuer) Token(key *rsa.PrivateKey, aud, sub string, expiry time.Duration) string {
	now := time.Now()
	return o.Sign(key, jwt.Claims{
		Issuer:   o.URL,
		Subject:  sub,
		Audience: jwt.Audience{aud},
		IssuedAt: jwt.NewNumericDate(now.Add(-time.Minute)),
		Expiry:   jwt.NewNumericDate(now.Add(expiry)),
	})
}

// Sign signs arbitrary claims with RS256. Pass a nil key to use the issuer's own.
func (o *OIDCIssuer) Sign(key *rsa.PrivateKey, claims jwt.Claims) string {
	if key == nil {
		key = o.key
	}
	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k1"))
	if err != nil {
		panic(err)
	}
	tok, err := jwt.Signed(sig).Claims(claims).Serialize()
	if err != nil {
		panic(err)
	}
	return tok
}
