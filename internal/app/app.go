package app

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"tecora/internal/auth/clerk"
	"tecora/internal/config"
	"tecora/internal/database"
	"tecora/internal/migrations"
	"tecora/internal/server"
)

type serverRunner interface {
	Run(context.Context) error
}

var (
	loadConfig    = config.Load
	newDatabase   = database.New
	closeDatabase = func(pool *pgxpool.Pool) {
		pool.Close()
	}
	newVerifier   = clerk.NewVerifier
	runMigrations = migrations.Up
	newServer     = func(deps server.Dependencies) serverRunner {
		return server.New(deps)
	}
)

func Run(ctx context.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	logger := newLogger(cfg.AppEnv)

	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	dbPool, err := newDatabase(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer closeDatabase(dbPool)

	if err := runMigrations(ctx, logger, cfg.DatabaseURL); err != nil {
		return err
	}

	var verifier *clerk.Verifier
	if cfg.Clerk.Enabled() {
		verifier, err = newVerifier(cfg.Clerk, logger)
		if err != nil {
			return err
		}
	}

	srv := newServer(server.Dependencies{
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
