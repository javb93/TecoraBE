package health

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type DBPinger interface {
	Ping(ctx context.Context) error
}

type Response struct {
	Service  string    `json:"service"`
	Status   string    `json:"status"`
	Time     time.Time `json:"time"`
	Database string    `json:"database"`
}

type Handler struct {
	DB            DBPinger
	ServiceName   string
	HealthTimeout time.Duration
}

func New(db DBPinger, serviceName string, healthTimeout time.Duration) *Handler {
	return &Handler{
		DB:            db,
		ServiceName:   serviceName,
		HealthTimeout: healthTimeout,
	}
}

func (h *Handler) Get(c *gin.Context) {
	status := "ok"
	dbStatus := "unknown"

	if h.DB != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), h.HealthTimeout)
		defer cancel()

		if err := h.DB.Ping(ctx); err != nil {
			dbStatus = "down"
			status = "degraded"
			c.JSON(http.StatusServiceUnavailable, Response{
				Service:  h.ServiceName,
				Status:   status,
				Time:     time.Now().UTC(),
				Database: dbStatus,
			})
			return
		}

		dbStatus = "up"
	}

	c.JSON(http.StatusOK, Response{
		Service:  h.ServiceName,
		Status:   status,
		Time:     time.Now().UTC(),
		Database: dbStatus,
	})
}
