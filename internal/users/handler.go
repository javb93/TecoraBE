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

type Store interface {
	ListActive(ctx context.Context) ([]User, error)
	GetByClerkUserID(ctx context.Context, clerkUserID string) (*User, error)
	Create(ctx context.Context, input CreateInput) (*User, error)
	Update(ctx context.Context, clerkUserID string, input UpdateInput) (*User, error)
	Delete(ctx context.Context, clerkUserID string) error
}

type OrganizationResolver interface {
	GetBySlug(ctx context.Context, slug string) (*organizations.Organization, error)
}

type Handler struct {
	store Store
	orgs  OrganizationResolver
}

func NewHandler(store Store, orgs OrganizationResolver) *Handler {
	return &Handler{store: store, orgs: orgs}
}

func RegisterAdminRoutes(group *gin.RouterGroup, handler *Handler) {
	group.GET("/users", handler.List)
	group.GET("/users/:clerk_user_id", handler.Get)
	group.POST("/users", handler.Create)
	group.PATCH("/users/:clerk_user_id", handler.Update)
	group.DELETE("/users/:clerk_user_id", handler.Delete)
}

func RegisterOrgRoutes(group *gin.RouterGroup, handler *Handler) {
	group.GET("/me", handler.Me)
}

func (h *Handler) List(c *gin.Context) {
	users, err := h.store.ListActive(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"users": users})
}

func (h *Handler) Get(c *gin.Context) {
	clerkUserID := normalizeClerkUserID(c.Param("clerk_user_id"))
	if clerkUserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid clerk user id"})
		return
	}

	user, err := h.store.GetByClerkUserID(c.Request.Context(), clerkUserID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, user)
}

func (h *Handler) Me(c *gin.Context) {
	user, ok := CurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing current user"})
		return
	}

	org, ok := CurrentOrganization(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing current organization"})
		return
	}

	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing clerk claims"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user":         user,
		"organization": org,
		"auth": gin.H{
			"clerk_user_id": claims.Subject,
			"org_slug":      org.Slug,
		},
	})
}

type createRequest struct {
	ClerkUserID      string         `json:"clerk_user_id"`
	Email            NullableString `json:"email"`
	FirstName        NullableString `json:"first_name"`
	LastName         NullableString `json:"last_name"`
	OrganizationSlug string         `json:"organization_slug"`
}

func (h *Handler) Create(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}

	req.ClerkUserID = normalizeClerkUserID(req.ClerkUserID)
	req.OrganizationSlug = organizations.NormalizeSlug(req.OrganizationSlug)

	if req.ClerkUserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid clerk user id"})
		return
	}
	if !organizations.ValidateSlug(req.OrganizationSlug) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid organization slug"})
		return
	}

	org, err := h.orgs.GetBySlug(c.Request.Context(), req.OrganizationSlug)
	if err != nil {
		writeOrgError(c, err)
		return
	}

	email, err := optionalToPtr(req.Email)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	firstName, err := optionalToPtr(req.FirstName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	lastName, err := optionalToPtr(req.LastName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.store.Create(c.Request.Context(), CreateInput{
		ClerkUserID:    req.ClerkUserID,
		Email:          email,
		FirstName:      firstName,
		LastName:       lastName,
		OrganizationID: org.ID,
	})
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusCreated, user)
}

type updateRequest struct {
	Email            NullableString `json:"email"`
	FirstName        NullableString `json:"first_name"`
	LastName         NullableString `json:"last_name"`
	OrganizationSlug NullableString `json:"organization_slug"`
}

func (h *Handler) Update(c *gin.Context) {
	clerkUserID := normalizeClerkUserID(c.Param("clerk_user_id"))
	if clerkUserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid clerk user id"})
		return
	}

	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}

	if !req.Email.Present() && !req.FirstName.Present() && !req.LastName.Present() && !req.OrganizationSlug.Present() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one field must be provided"})
		return
	}

	var input UpdateInput
	if req.Email.Present() {
		if req.Email.IsNull() {
			input.Email = req.Email
		} else {
			email := req.Email.Trimmed()
			if email == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid email"})
				return
			}
			input.Email = NullableString{Set: true, Value: email}
		}
	}
	if req.FirstName.Present() {
		if req.FirstName.IsNull() {
			input.FirstName = req.FirstName
		} else {
			firstName := req.FirstName.Trimmed()
			if firstName == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid first name"})
				return
			}
			input.FirstName = NullableString{Set: true, Value: firstName}
		}
	}
	if req.LastName.Present() {
		if req.LastName.IsNull() {
			input.LastName = req.LastName
		} else {
			lastName := req.LastName.Trimmed()
			if lastName == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid last name"})
				return
			}
			input.LastName = NullableString{Set: true, Value: lastName}
		}
	}
	if req.OrganizationSlug.Present() {
		if req.OrganizationSlug.IsNull() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid organization slug"})
			return
		}

		orgSlug := organizations.NormalizeSlug(req.OrganizationSlug.Trimmed())
		if !organizations.ValidateSlug(orgSlug) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid organization slug"})
			return
		}
		org, err := h.orgs.GetBySlug(c.Request.Context(), orgSlug)
		if err != nil {
			writeOrgError(c, err)
			return
		}
		input.OrganizationID = NullableString{Set: true, Value: org.ID}
	}

	user, err := h.store.Update(c.Request.Context(), clerkUserID, input)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, user)
}

func (h *Handler) Delete(c *gin.Context) {
	clerkUserID := normalizeClerkUserID(c.Param("clerk_user_id"))
	if clerkUserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid clerk user id"})
		return
	}

	if err := h.store.Delete(c.Request.Context(), clerkUserID); err != nil {
		writeError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func normalizeClerkUserID(raw string) string {
	return strings.TrimSpace(raw)
}

func optionalToPtr(value NullableString) (*string, error) {
	if !value.Present() || value.IsNull() {
		return nil, nil
	}

	trimmed := value.Trimmed()
	if trimmed == "" {
		return nil, errors.New("invalid value")
	}

	return &trimmed, nil
}

func writeOrgError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, organizations.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
	case errors.Is(err, ErrConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "clerk user already exists"})
	case errors.Is(err, ErrOrganizationNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
	case errors.Is(err, ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user input"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
