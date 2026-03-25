package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"tecora/internal/auth/clerk"
)

type ClaimsContextKey string

const ClaimsKey ClaimsContextKey = "clerkClaims"
const OrgSlugKey ClaimsContextKey = "clerkOrgSlug"

type TokenVerifier interface {
	Verify(ctx context.Context, token string) (*clerk.Claims, error)
	Enabled() bool
}

type AdminAllowlist interface {
	Allowed(subject string) bool
	Enabled() bool
}

func ClerkAuth(verifier TokenVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		if verifier == nil || !verifier.Enabled() {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": "auth not configured",
			})
			return
		}

		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if header == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing authorization header",
			})
			return
		}

		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "authorization header must use bearer token",
			})
			return
		}
		token := strings.TrimSpace(parts[1])

		claims, err := verifier.Verify(c.Request.Context(), token)
		if err != nil {
			if errors.Is(err, clerk.ErrNotConfigured) {
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
					"error": "auth not configured",
				})
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid token",
			})
			return
		}

		c.Set(string(ClaimsKey), claims)
		c.Next()
	}
}

func ClerkAdminAllowlist(allowedUserIDs []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedUserIDs))
	for _, id := range allowedUserIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			allowed[id] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		if len(allowed) == 0 {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": "admin auth not configured",
			})
			return
		}

		rawClaims, ok := c.Get(string(ClaimsKey))
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing clerk claims",
			})
			return
		}

		claims, ok := rawClaims.(*clerk.Claims)
		if !ok || claims == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid clerk claims",
			})
			return
		}

		if _, ok := allowed[strings.TrimSpace(claims.Subject)]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "admin access denied",
			})
			return
		}

		c.Next()
	}
}

func ClerkOrgScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		rawClaims, ok := c.Get(string(ClaimsKey))
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing clerk claims",
			})
			return
		}

		claims, ok := rawClaims.(*clerk.Claims)
		if !ok || claims == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid clerk claims",
			})
			return
		}

		orgSlug := strings.TrimSpace(claims.OrgSlug)
		if orgSlug == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "missing org slug",
			})
			return
		}

		c.Set(string(OrgSlugKey), orgSlug)
		c.Next()
	}
}

func CurrentOrgSlug(c *gin.Context) (string, bool) {
	raw, ok := c.Get(string(OrgSlugKey))
	if !ok {
		return "", false
	}

	orgSlug, ok := raw.(string)
	if !ok {
		return "", false
	}

	return strings.TrimSpace(orgSlug), orgSlug != ""
}
