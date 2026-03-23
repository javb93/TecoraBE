package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type mockDB struct {
	err error
}

func (m mockDB) Ping(ctx context.Context) error {
	return m.err
}

func TestHealthHandlerOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := New(mockDB{}, "tecora-backend", time.Second)
	r.GET("/health", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestHealthHandlerReturns503WhenDBDown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := New(mockDB{err: context.DeadlineExceeded}, "tecora-backend", time.Second)
	r.GET("/health", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", w.Code)
	}
}
