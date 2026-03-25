package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"tecora/internal/auth/clerk"
)

type verifierStub struct {
	enabled bool
	claims  *clerk.Claims
	err     error
}

func (v verifierStub) Verify(ctx context.Context, token string) (*clerk.Claims, error) {
	return v.claims, v.err
}

func (v verifierStub) Enabled() bool {
	return v.enabled
}

func TestClerkAuthRejectsMissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ClerkAuth(verifierStub{enabled: true}))
	r.GET("/", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestClerkAuthRejectsInvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ClerkAuth(verifierStub{enabled: true, err: context.Canceled}))
	r.GET("/", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestClerkAuthAcceptsValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ClerkAuth(verifierStub{
		enabled: true,
		claims: &clerk.Claims{
			Subject: "user_123",
		},
	}))
	r.GET("/", func(c *gin.Context) {
		claims, ok := c.Get(string(ClaimsKey))
		if !ok {
			c.Status(http.StatusInternalServerError)
			return
		}
		if claims.(*clerk.Claims).Subject != "user_123" {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestClerkAdminAllowlistRejectsNonAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(ClaimsKey), &clerk.Claims{Subject: "user_123"})
		c.Next()
	})
	r.Use(ClerkAdminAllowlist([]string{"admin_1"}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestClerkAdminAllowlistAcceptsAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(ClaimsKey), &clerk.Claims{Subject: "admin_1"})
		c.Next()
	})
	r.Use(ClerkAdminAllowlist([]string{"admin_1"}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestCurrentOrgSlug(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(ClaimsKey), &clerk.Claims{OrgSlug: "demo-alpha"})
		c.Next()
	})
	r.Use(ClerkOrgScope())
	r.GET("/", func(c *gin.Context) {
		slug, ok := CurrentOrgSlug(c)
		if !ok || slug != "demo-alpha" {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
