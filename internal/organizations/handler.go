package organizations

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type Store interface {
	ListActive(ctx context.Context) ([]Organization, error)
	GetBySlug(ctx context.Context, slug string) (*Organization, error)
	Create(ctx context.Context, slug, name string) (*Organization, error)
	Update(ctx context.Context, slug string, input UpdateInput) (*Organization, error)
	Delete(ctx context.Context, slug string) error
}

type Handler struct {
	store Store
}

func NewHandler(store Store) *Handler {
	return &Handler{store: store}
}

func RegisterAdminRoutes(group *gin.RouterGroup, handler *Handler) {
	group.GET("/organizations", handler.List)
	group.GET("/organizations/:slug", handler.Get)
	group.POST("/organizations", handler.Create)
	group.PATCH("/organizations/:slug", handler.Update)
	group.DELETE("/organizations/:slug", handler.Delete)
}

func (h *Handler) List(c *gin.Context) {
	orgs, err := h.store.ListActive(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"organizations": orgs})
}

func (h *Handler) Get(c *gin.Context) {
	slug := NormalizeSlug(c.Param("slug"))
	if !ValidateSlug(slug) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid slug"})
		return
	}

	org, err := h.store.GetBySlug(c.Request.Context(), slug)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, org)
}

type createRequest struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

func (h *Handler) Create(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}

	req.Slug = NormalizeSlug(req.Slug)
	req.Name = strings.TrimSpace(req.Name)

	if !ValidateSlug(req.Slug) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid slug"})
		return
	}
	if !ValidateName(req.Name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name"})
		return
	}

	org, err := h.store.Create(c.Request.Context(), req.Slug, req.Name)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusCreated, org)
}

type updateRequest struct {
	Name   *string `json:"name"`
	Active *bool   `json:"active"`
}

func (h *Handler) Update(c *gin.Context) {
	slug := NormalizeSlug(c.Param("slug"))
	if !ValidateSlug(slug) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid slug"})
		return
	}

	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}

	if req.Name == nil && req.Active == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one field must be provided"})
		return
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if !ValidateName(name) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name"})
			return
		}
		req.Name = &name
	}

	org, err := h.store.Update(c.Request.Context(), slug, UpdateInput{
		Name:   req.Name,
		Active: req.Active,
	})
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, org)
}

func (h *Handler) Delete(c *gin.Context) {
	slug := NormalizeSlug(c.Param("slug"))
	if !ValidateSlug(slug) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid slug"})
		return
	}

	if err := h.store.Delete(c.Request.Context(), slug); err != nil {
		writeError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
	case errors.Is(err, ErrConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "organization slug already exists"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
