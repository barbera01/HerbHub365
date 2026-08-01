package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"HerbHub365/services/herbhub-manager/internal/config"
)

const (
	testKeyID    = "test-key"
	testAudience = "api://manager"
	testRole     = "Manager.Operator"
)

func newTenant(t *testing.T) (*httptest.Server, *rsa.PrivateKey) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":   server.URL,
			"jwks_uri": server.URL + "/keys",
		})
	})

	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kid": testKeyID,
				"kty": "RSA",
				"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			}},
		})
	})

	return server, key
}

func newTenantWithCounters(t *testing.T) (*httptest.Server, *rsa.PrivateKey, *atomic.Int32) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	count := &atomic.Int32{}
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":   server.URL,
			"jwks_uri": server.URL + "/keys",
		})
	})

	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kid": testKeyID,
				"kty": "RSA",
				"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			}},
		})
	})

	return server, key, count
}

func newVerifier(authorityURL string) *Verifier {
	return NewVerifier(config.AuthConfig{
		IssuerURL:    authorityURL,
		Audience:     testAudience,
		RequiredRole: testRole,
	}, nil)
}

func newVerifierWithDiscovery(authorityURL, discoveryURL string) *Verifier {
	return NewVerifier(config.AuthConfig{
		IssuerURL:    authorityURL,
		DiscoveryURL: discoveryURL,
		Audience:     testAudience,
		RequiredRole: testRole,
	}, nil)
}

func signToken(t *testing.T, key *rsa.PrivateKey, issuer, audience string, roles []string, exp time.Time) string {
	return signTokenWithKid(t, key, issuer, audience, roles, exp, testKeyID)
}

func signTokenWithKid(t *testing.T, key *rsa.PrivateKey, issuer, audience string, roles []string, exp time.Time, kid string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss":   issuer,
		"aud":   audience,
		"roles": roles,
		"oid":   "user-object-id",
		"exp":   exp.Unix(),
		"iat":   time.Now().Add(-time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid

	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func TestVerifyAcceptsValidToken(t *testing.T) {
	server, key := newTenant(t)
	verifier := newVerifier(server.URL)

	token := signToken(t, key, server.URL, testAudience, []string{testRole}, time.Now().Add(time.Hour))
	verified, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("expected valid token: %v", err)
	}
	if verified.Subject != "user-object-id" {
		t.Fatalf("unexpected subject: %s", verified.Subject)
	}
}

func TestVerifyRejectsMissingRole(t *testing.T) {
	server, key := newTenant(t)
	verifier := newVerifier(server.URL)

	token := signToken(t, key, server.URL, testAudience, []string{"Reader"}, time.Now().Add(time.Hour))
	_, err := verifier.Verify(context.Background(), token)
	if err != ErrForbidden {
		t.Fatalf("expected forbidden, got: %v", err)
	}
}

func TestVerifyRejectsBadAudience(t *testing.T) {
	server, key := newTenant(t)
	verifier := newVerifier(server.URL)

	token := signToken(t, key, server.URL, "api://other", []string{testRole}, time.Now().Add(time.Hour))
	_, err := verifier.Verify(context.Background(), token)
	if err != ErrUnauthorized {
		t.Fatalf("expected unauthorized, got: %v", err)
	}
}

func TestVerifyUsesDiscoveryOverride(t *testing.T) {
	server, key := newTenant(t)
	verifier := newVerifierWithDiscovery("https://unused.example", server.URL+"/.well-known/openid-configuration")

	token := signToken(t, key, server.URL, testAudience, []string{testRole}, time.Now().Add(time.Hour))
	if _, err := verifier.Verify(context.Background(), token); err != nil {
		t.Fatalf("expected token to verify via discovery override: %v", err)
	}
}

func TestVerifyUnknownKidRefreshRateLimited(t *testing.T) {
	server, key, jwksHits := newTenantWithCounters(t)
	verifier := NewVerifier(config.AuthConfig{
		IssuerURL:    server.URL,
		Audience:     testAudience,
		RequiredRole: testRole,
		JWKSRefresh:  time.Minute,
	}, nil)

	token := signTokenWithKid(t, key, server.URL, testAudience, []string{testRole}, time.Now().Add(time.Hour), "unknown-kid")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = verifier.Verify(context.Background(), token)
		}()
	}
	wg.Wait()

	if got := jwksHits.Load(); got > 1 {
		t.Fatalf("expected at most one JWKS refresh attempt during interval, got %d", got)
	}
}
