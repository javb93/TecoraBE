package acceptances

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"tecora/internal/users"
)

type AcceptanceService interface {
	Submit(ctx context.Context, organizationID string, submission Submission) (*Record, error)
	GetStatus(ctx context.Context, organizationID, acceptanceID string) (*Record, error)
	GetPDF(ctx context.Context, organizationID, acceptanceID string) (*PDFDocument, error)
}

type Handler struct {
	service AcceptanceService
}

func NewHandler(service AcceptanceService) *Handler {
	return &Handler{service: service}
}

func RegisterRoutes(group *gin.RouterGroup, handler *Handler) {
	group.POST("/work-orders/:id/acceptance", handler.Submit)
	group.GET("/acceptances/:id", handler.GetStatus)
	group.GET("/acceptances/:id/pdf", handler.GetPDF)
}

func (h *Handler) Submit(c *gin.Context) {
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

	var req Submission
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}
	if strings.TrimSpace(req.WorkOrderID) != workOrderID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "work order id mismatch"})
		return
	}

	record, err := h.service.Submit(c.Request.Context(), org.ID, req)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusAccepted, record.AcceptedResponse())
}

func (h *Handler) GetStatus(c *gin.Context) {
	org, ok := users.CurrentOrganization(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing current organization"})
		return
	}

	acceptanceID := strings.TrimSpace(c.Param("id"))
	if acceptanceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid acceptance id"})
		return
	}

	record, err := h.service.GetStatus(c.Request.Context(), org.ID, acceptanceID)
	if err != nil {
		writeError(c, err)
		return
	}

	var pdfURL *string
	if record.PDFStatus == PDFStatusGenerated && record.PDFStorageKey != nil {
		url := absoluteURL(c, "/api/v1/acceptances/"+record.ID+"/pdf")
		pdfURL = &url
	}

	c.JSON(http.StatusOK, record.StatusResponse(pdfURL))
}

func (h *Handler) GetPDF(c *gin.Context) {
	org, ok := users.CurrentOrganization(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing current organization"})
		return
	}

	acceptanceID := strings.TrimSpace(c.Param("id"))
	if acceptanceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid acceptance id"})
		return
	}

	doc, err := h.service.GetPDF(c.Request.Context(), org.ID, acceptanceID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.Header("Content-Type", doc.ContentType)
	c.Header("Content-Disposition", `inline; filename="acceptance-`+acceptanceID+`.pdf"`)
	c.Data(http.StatusOK, doc.ContentType, doc.Bytes)
}

func absoluteURL(c *gin.Context, path string) string {
	scheme := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}

	host := strings.TrimSpace(c.Request.Host)
	if host == "" {
		return path
	}

	return scheme + "://" + host + path
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid acceptance input"})
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "acceptance not found"})
	case errors.Is(err, ErrPDFNotReady):
		c.JSON(http.StatusNotFound, gin.H{"error": "acceptance pdf not found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
