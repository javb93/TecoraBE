package clerk

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"log/slog"

	"tecora/internal/config"
)

func TestVerifierRejectsMissingConfig(t *testing.T) {
	v, err := NewVerifier(config.ClerkConfig{}, slog.Default())
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	if _, err := v.Verify(context.Background(), "token"); err != ErrNotConfigured {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestVerifierAcceptsValidRS256Token(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	kid := "test-key"
	now := time.Now().UTC()
	claims := map[string]any{
		"sub":      "user_123",
		"iss":      "https://clerk.example.com",
		"aud":      "tecora",
		"org_slug": "demo-alpha",
		"exp":      now.Add(time.Hour).Unix(),
		"nbf":      now.Add(-time.Minute).Unix(),
	}

	token, err := signToken(priv, kid, claims)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	v, err := NewVerifier(config.ClerkConfig{
		IssuerURL: "https://clerk.example.com",
		JWKSURL:   "https://jwks.example.com",
		Audience:  "tecora",
	}, slog.Default())
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	v.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			body := `{"keys":[` + jwkFromPublicKey(kid, &priv.PublicKey) + `]}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	}

	got, err := v.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.Subject != "user_123" {
		t.Fatalf("subject = %q", got.Subject)
	}
	if got.OrgSlug != "demo-alpha" {
		t.Fatalf("org slug = %q", got.OrgSlug)
	}
}

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func signToken(priv *rsa.PrivateKey, kid string, claims map[string]any) (string, error) {
	header := map[string]any{"alg": "RS256", "typ": "JWT", "kid": kid}
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)

	unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func jwkFromPublicKey(kid string, pub *rsa.PublicKey) string {
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
	return `{"kty":"RSA","use":"sig","alg":"RS256","kid":"` + kid + `","n":"` + n + `","e":"` + e + `"}`
}
