package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"tecora/internal/auth/clerk"
	"tecora/internal/config"
	"tecora/internal/health"
	"tecora/internal/middleware"
	"tecora/internal/organizations"
)

type Dependencies struct {
	Config   config.Config
	Logger   *slog.Logger
	DB       *pgxpool.Pool
	Verifier *clerk.Verifier
}

type Server struct {
	cfg config.Config
	log *slog.Logger
	db  *pgxpool.Pool
	v   *clerk.Verifier
}

func New(deps Dependencies) *Server {
	return &Server{
		cfg: deps.Config,
		log: deps.Logger,
		db:  deps.DB,
		v:   deps.Verifier,
	}
}

func (s *Server) Run(parent context.Context) error {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestLogger(s.log))
	router.Use(middleware.CORS(s.cfg.CORSAllowedOrigins))

	api := router.Group("/api/v1")
	healthHandler := health.New(s.db, "tecora-backend", s.cfg.HealthTimeout)
	api.GET("/health", healthHandler.Get)

	private := api.Group("/private")
	private.Use(middleware.ClerkAuth(s.v))
	private.GET("/me", func(c *gin.Context) {
		claims, _ := c.Get(string(middleware.ClaimsKey))
		c.JSON(http.StatusOK, gin.H{
			"message": "protected route",
			"claims":  claims,
		})
	})

	orgRepo := organizations.NewRepository(s.db)
	orgHandler := organizations.NewHandler(orgRepo)

	admin := api.Group("/admin")
	admin.Use(middleware.ClerkAuth(s.v))
	admin.Use(middleware.ClerkAdminAllowlist(s.cfg.AdminClerkUserIDs))
	organizations.RegisterAdminRoutes(admin, orgHandler)

	srv := &http.Server{
		Addr:         s.cfg.HTTPAddr,
		Handler:      router,
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
		IdleTimeout:  s.cfg.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		s.log.Info("starting http server", "addr", s.cfg.HTTPAddr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-parent.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
		defer cancel()
		s.log.Info("shutting down http server")
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown http server: %w", err)
		}
		return nil
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("listen and serve: %w", err)
		}
		return nil
	}
}

func requestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("request",
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"client_ip", c.ClientIP(),
			"request_id", c.GetHeader("X-Request-Id"),
		)
	}
}
