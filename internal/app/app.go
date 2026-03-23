package app

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"

	"tecora/internal/auth/clerk"
	"tecora/internal/config"
	"tecora/internal/database"
	"tecora/internal/server"
)

func Run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg.AppEnv)

	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	dbPool, err := database.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer dbPool.Close()

	var verifier *clerk.Verifier
	if cfg.Clerk.Enabled() {
		verifier, err = clerk.NewVerifier(cfg.Clerk, logger)
		if err != nil {
			return err
		}
	}

	srv := server.New(server.Dependencies{
		Config:   cfg,
		Logger:   logger,
		DB:       dbPool,
		Verifier: verifier,
	})

	err = srv.Run(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func newLogger(env string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if env != "production" {
		opts.AddSource = true
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
