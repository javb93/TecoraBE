package users

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"tecora/internal/middleware"
	"tecora/internal/organizations"
)

type userContextKey string

const currentUserKey userContextKey = "currentUser"
const currentOrganizationKey userContextKey = "currentOrganization"

type MembershipLookup interface {
	GetByClerkUserID(ctx context.Context, clerkUserID string) (*User, error)
}

func RequireOrgAccess(userLookup MembershipLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := middleware.CurrentClaims(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing clerk claims",
			})
			return
		}

		clerkUserID := strings.TrimSpace(claims.Subject)
		if clerkUserID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing clerk subject",
			})
			return
		}

		user, err := userLookup.GetByClerkUserID(c.Request.Context(), clerkUserID)
		if err != nil {
			switch {
			case errors.Is(err, ErrNotFound):
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": "user membership not found",
				})
			default:
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "internal server error",
				})
			}
			return
		}

		if !user.Organization.Active || user.Organization.DeletedAt != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "organization is inactive",
			})
			return
		}

		orgSlug := organizations.NormalizeSlug(strings.TrimSpace(claims.OrgSlug))
		if orgSlug != "" && orgSlug != user.Organization.Slug {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "organization access denied",
			})
			return
		}

		c.Set(string(middleware.OrgSlugKey), user.Organization.Slug)
		c.Set(string(currentUserKey), user)
		c.Set(string(currentOrganizationKey), user.Organization)
		c.Next()
	}
}

func CurrentUser(c *gin.Context) (*User, bool) {
	raw, ok := c.Get(string(currentUserKey))
	if !ok {
		return nil, false
	}

	user, ok := raw.(*User)
	if !ok || user == nil {
		return nil, false
	}

	return user, true
}

func CurrentOrganization(c *gin.Context) (*organizations.Organization, bool) {
	raw, ok := c.Get(string(currentOrganizationKey))
	if !ok {
		return nil, false
	}

	org, ok := raw.(organizations.Organization)
	if !ok {
		return nil, false
	}

	return &org, true
}
