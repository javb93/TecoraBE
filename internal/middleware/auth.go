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

type TokenVerifier interface {
	Verify(ctx context.Context, token string) (*clerk.Claims, error)
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
