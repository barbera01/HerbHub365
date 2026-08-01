package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"HerbHub365/services/herbhub-manager/internal/config"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
)

const defaultMinRefreshInterval = time.Minute

type Verified struct {
	Subject string
	Roles   []string
	Issuer  string
}

type Verifier struct {
	requiredRole string
	issuer       *issuerVerifier
}

type issuerVerifier struct {
	authorityURL string
	discoveryURL string
	audience     string
	client       *http.Client

	minRefreshInterval time.Duration

	mu          sync.Mutex
	discovered  bool
	issuer      string
	jwksURI     string
	keys        map[string]*rsa.PublicKey
	lastRefresh time.Time
	refreshing  bool
}

func NewVerifier(cfg config.AuthConfig, client *http.Client) *Verifier {
	if client == nil {
		client = &http.Client{Timeout: cfg.HTTPTimeout}
		if cfg.HTTPTimeout <= 0 {
			client.Timeout = 10 * time.Second
		}
	}

	refresh := cfg.JWKSRefresh
	if refresh <= 0 {
		refresh = defaultMinRefreshInterval
	}

	return &Verifier{
		requiredRole: cfg.RequiredRole,
		issuer: &issuerVerifier{
			authorityURL:       strings.TrimSuffix(cfg.IssuerURL, "/"),
			discoveryURL:       strings.TrimSpace(cfg.DiscoveryURL),
			audience:           cfg.Audience,
			client:             client,
			keys:               make(map[string]*rsa.PublicKey),
			minRefreshInterval: refresh,
		},
	}
}

func (v *Verifier) Verify(ctx context.Context, token string) (*Verified, error) {
	verified, err := v.issuer.verify(ctx, token)
	if err != nil {
		return nil, ErrUnauthorized
	}

	if !slices.Contains(verified.Roles, v.requiredRole) {
		return nil, ErrForbidden
	}

	return verified, nil
}

func (i *issuerVerifier) verify(ctx context.Context, token string) (*Verified, error) {
	metadata, err := i.metadata(ctx)
	if err != nil {
		return nil, ErrUnauthorized
	}

	claims := jwt.MapClaims{}
	_, err = jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		return i.keyFor(ctx, t)
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(metadata.issuer),
		jwt.WithAudience(i.audience),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, ErrUnauthorized
	}

	return &Verified{
		Subject: subjectOf(claims),
		Roles:   parseRoles(claims),
		Issuer:  metadata.issuer,
	}, nil
}

func (i *issuerVerifier) keyFor(ctx context.Context, token *jwt.Token) (any, error) {
	kid, _ := token.Header["kid"].(string)
	if kid == "" {
		return nil, errors.New("missing kid")
	}

	i.mu.Lock()
	key, ok := i.keys[kid]
	shouldRefresh := !ok && time.Since(i.lastRefresh) > i.minRefreshInterval
	refreshing := i.refreshing
	if shouldRefresh && !refreshing {
		i.refreshing = true
	}
	i.mu.Unlock()

	if ok {
		return key, nil
	}
	if !shouldRefresh || refreshing {
		return nil, errors.New("unknown signing key")
	}

	if err := i.refreshKeys(ctx); err != nil {
		i.mu.Lock()
		i.refreshing = false
		i.lastRefresh = time.Now()
		i.mu.Unlock()
		return nil, err
	}
	i.mu.Lock()
	i.refreshing = false
	i.mu.Unlock()

	i.mu.Lock()
	defer i.mu.Unlock()
	if key, ok := i.keys[kid]; ok {
		return key, nil
	}
	return nil, errors.New("unknown signing key")
}

type issuerMetadata struct {
	issuer  string
	jwksURI string
}

func (i *issuerVerifier) metadata(ctx context.Context) (issuerMetadata, error) {
	i.mu.Lock()
	if i.discovered {
		defer i.mu.Unlock()
		return issuerMetadata{issuer: i.issuer, jwksURI: i.jwksURI}, nil
	}
	i.mu.Unlock()

	discoveryURL := i.discoveryURL
	if discoveryURL == "" {
		discoveryURL = i.authorityURL + "/.well-known/openid-configuration"
	}
	var doc struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if err := i.getJSON(ctx, discoveryURL, &doc); err != nil {
		return issuerMetadata{}, err
	}
	if doc.Issuer == "" || doc.JWKSURI == "" {
		return issuerMetadata{}, fmt.Errorf("incomplete discovery doc")
	}

	i.mu.Lock()
	defer i.mu.Unlock()
	i.issuer = strings.TrimSuffix(doc.Issuer, "/")
	i.jwksURI = doc.JWKSURI
	i.discovered = true

	return issuerMetadata{issuer: i.issuer, jwksURI: i.jwksURI}, nil
}

func (i *issuerVerifier) refreshKeys(ctx context.Context) error {
	metadata, err := i.metadata(ctx)
	if err != nil {
		return err
	}

	var keySet struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := i.getJSON(ctx, metadata.jwksURI, &keySet); err != nil {
		return err
	}

	keys := make(map[string]*rsa.PublicKey, len(keySet.Keys))
	for _, key := range keySet.Keys {
		if key.Kty != "RSA" || key.Kid == "" {
			continue
		}
		pk, err := parseRSAKey(key.N, key.E)
		if err != nil {
			continue
		}
		keys[key.Kid] = pk
	}
	if len(keys) == 0 {
		return errors.New("no usable rsa keys")
	}

	i.mu.Lock()
	defer i.mu.Unlock()
	i.keys = keys
	i.lastRefresh = time.Now()
	return nil
}

func (i *issuerVerifier) getJSON(ctx context.Context, url string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := i.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func parseRSAKey(modulus, exponent string) (*rsa.PublicKey, error) {
	mb, err := base64.RawURLEncoding.DecodeString(modulus)
	if err != nil {
		return nil, err
	}
	eb, err := base64.RawURLEncoding.DecodeString(exponent)
	if err != nil {
		return nil, err
	}
	if len(mb) == 0 || len(eb) == 0 {
		return nil, errors.New("empty rsa key material")
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(mb),
		E: int(new(big.Int).SetBytes(eb).Int64()),
	}, nil
}

func parseRoles(claims jwt.MapClaims) []string {
	raw, ok := claims["roles"]
	if !ok {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	roles := make([]string, 0, len(arr))
	for _, v := range arr {
		if role, ok := v.(string); ok && strings.TrimSpace(role) != "" {
			roles = append(roles, role)
		}
	}
	return roles
}

func subjectOf(claims jwt.MapClaims) string {
	if oid, ok := claims["oid"].(string); ok && oid != "" {
		return oid
	}
	if sub, ok := claims["sub"].(string); ok && sub != "" {
		return sub
	}
	return "unknown"
}
