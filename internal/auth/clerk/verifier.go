package clerk

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"log/slog"

	"tecora/internal/config"
)

var ErrNotConfigured = errors.New("clerk auth is not configured")

type Verifier struct {
	cfg        config.ClerkConfig
	logger     *slog.Logger
	httpClient *http.Client

	mu        sync.Mutex
	cachedAt  time.Time
	cacheTTL  time.Duration
	keysByKID map[string]*rsa.PublicKey
}

type Claims struct {
	Subject   string          `json:"sub"`
	Issuer    string          `json:"iss"`
	Audience  json.RawMessage `json:"aud"`
	Expiry    int64           `json:"exp"`
	NotBefore int64           `json:"nbf"`
	SubjectID string          `json:"sid"`
}

func NewVerifier(cfg config.ClerkConfig, logger *slog.Logger) (*Verifier, error) {
	if !cfg.Enabled() {
		return &Verifier{
			cfg:        cfg,
			logger:     logger,
			httpClient: &http.Client{Timeout: 10 * time.Second},
			cacheTTL:   30 * time.Minute,
		}, nil
	}

	return &Verifier{
		cfg:        cfg,
		logger:     logger,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		cacheTTL:   30 * time.Minute,
	}, nil
}

func (v *Verifier) Enabled() bool {
	return v != nil && v.cfg.Enabled()
}

func (v *Verifier) Verify(ctx context.Context, token string) (*Claims, error) {
	if !v.Enabled() {
		return nil, ErrNotConfigured
	}

	header, payload, signingInput, signature, err := splitJWT(token)
	if err != nil {
		return nil, err
	}

	if header.Alg != "RS256" {
		return nil, fmt.Errorf("unsupported jwt alg %q", header.Alg)
	}

	pubKey, err := v.keyForKID(ctx, header.KID)
	if err != nil {
		return nil, err
	}

	digest := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, digest[:], signature); err != nil {
		return nil, fmt.Errorf("invalid jwt signature: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("decode claims: %w", err)
	}

	if err := v.validateClaims(&claims); err != nil {
		return nil, err
	}

	return &claims, nil
}

type jwtHeader struct {
	Alg string `json:"alg"`
	KID string `json:"kid"`
	Typ string `json:"typ"`
}

func splitJWT(token string) (jwtHeader, []byte, string, []byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return jwtHeader{}, nil, "", nil, errors.New("token must have three parts")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return jwtHeader{}, nil, "", nil, fmt.Errorf("decode jwt header: %w", err)
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return jwtHeader{}, nil, "", nil, fmt.Errorf("decode jwt payload: %w", err)
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return jwtHeader{}, nil, "", nil, fmt.Errorf("decode jwt signature: %w", err)
	}

	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return jwtHeader{}, nil, "", nil, fmt.Errorf("decode jwt header json: %w", err)
	}

	return header, payload, parts[0] + "." + parts[1], signature, nil
}

func (v *Verifier) validateClaims(claims *Claims) error {
	if claims.Issuer != v.cfg.IssuerURL {
		return fmt.Errorf("invalid issuer")
	}

	if time.Unix(claims.Expiry, 0).Before(time.Now()) {
		return fmt.Errorf("token expired")
	}

	if claims.NotBefore != 0 && time.Unix(claims.NotBefore, 0).After(time.Now()) {
		return fmt.Errorf("token not yet valid")
	}

	if v.cfg.Audience != "" && !audienceMatches(claims.Audience, v.cfg.Audience) {
		return fmt.Errorf("invalid audience")
	}

	return nil
}

func audienceMatches(raw json.RawMessage, expected string) bool {
	if len(raw) == 0 {
		return false
	}

	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return single == expected
	}

	var multiple []string
	if err := json.Unmarshal(raw, &multiple); err == nil {
		for _, item := range multiple {
			if item == expected {
				return true
			}
		}
	}

	return false
}

type jwksResponse struct {
	Keys []jwksKey `json:"keys"`
}

type jwksKey struct {
	KID string `json:"kid"`
	KTY string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (v *Verifier) keyForKID(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	if key, ok := v.keysByKID[kid]; ok && time.Since(v.cachedAt) < v.cacheTTL {
		v.mu.Unlock()
		return key, nil
	}
	v.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.cfg.JWKSURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create jwks request: %w", err)
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("fetch jwks: unexpected status %s", resp.Status)
	}

	var parsed jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode jwks: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(parsed.Keys))
	for _, key := range parsed.Keys {
		if key.KID == "" || key.KTY != "RSA" {
			continue
		}

		pubKey, err := rsaPublicKeyFromJWKS(key.N, key.E)
		if err != nil {
			return nil, err
		}
		keys[key.KID] = pubKey
	}

	v.mu.Lock()
	v.keysByKID = keys
	v.cachedAt = time.Now()
	v.mu.Unlock()

	if key, ok := keys[kid]; ok {
		return key, nil
	}

	return nil, fmt.Errorf("jwks key %q not found", kid)
}

func rsaPublicKeyFromJWKS(nEncoded, eEncoded string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nEncoded)
	if err != nil {
		return nil, fmt.Errorf("decode jwks modulus: %w", err)
	}

	eBytes, err := base64.RawURLEncoding.DecodeString(eEncoded)
	if err != nil {
		return nil, fmt.Errorf("decode jwks exponent: %w", err)
	}

	exponent := 0
	for _, b := range eBytes {
		exponent = exponent<<8 + int(b)
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: exponent,
	}, nil
}
