package workorders

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"tecora/internal/users"
)

type Service interface {
	Create(ctx context.Context, input CreateInput) (*WorkOrder, error)
	ListActiveByOrganizationID(ctx context.Context, organizationID string) ([]WorkOrder, error)
	GetByWorkOrderID(ctx context.Context, organizationID, workOrderID string) (*WorkOrder, error)
}

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func RegisterRoutes(group *gin.RouterGroup, handler *Handler) {
	group.POST("/work-orders", handler.Create)
	group.GET("/work-orders", handler.List)
	group.GET("/work-orders/:id", handler.Get)
}

func (h *Handler) Create(c *gin.Context) {
	org, ok := users.CurrentOrganization(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing current organization"})
		return
	}

	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}

	input, err := NormalizeCreateRequest(req)
	if err != nil {
		writeError(c, err)
		return
	}
	input.OrganizationID = org.ID

	workOrder, err := h.service.Create(c.Request.Context(), input)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusCreated, workOrder)
}

func (h *Handler) List(c *gin.Context) {
	org, ok := users.CurrentOrganization(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing current organization"})
		return
	}

	workOrders, err := h.service.ListActiveByOrganizationID(c.Request.Context(), org.ID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"workOrders": workOrders})
}

func (h *Handler) Get(c *gin.Context) {
	org, ok := users.CurrentOrganization(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing current organization"})
		return
	}

	workOrderID := strings.TrimSpace(c.Param("id"))
	if workOrderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid work order id"})
		return
	}

	workOrder, err := h.service.GetByWorkOrderID(c.Request.Context(), org.ID, workOrderID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, workOrder)
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid work order input"})
	case errors.Is(err, ErrConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "work order already exists"})
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "work order not found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
