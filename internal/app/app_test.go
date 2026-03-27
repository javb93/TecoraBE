package app

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"tecora/internal/auth/clerk"
	"tecora/internal/config"
	"tecora/internal/server"
)

type stubServer struct {
	run func(context.Context) error
}

func (s stubServer) Run(ctx context.Context) error {
	if s.run != nil {
		return s.run(ctx)
	}
	return nil
}

func TestRunAppliesMigrationsBeforeServerStart(t *testing.T) {
	originalLoadConfig := loadConfig
	originalNewDatabase := newDatabase
	originalCloseDatabase := closeDatabase
	originalNewVerifier := newVerifier
	originalRunMigrations := runMigrations
	originalNewServer := newServer

	t.Cleanup(func() {
		loadConfig = originalLoadConfig
		newDatabase = originalNewDatabase
		closeDatabase = originalCloseDatabase
		newVerifier = originalNewVerifier
		runMigrations = originalRunMigrations
		newServer = originalNewServer
	})

	loadConfig = func() (config.Config, error) {
		return config.Config{
			AppEnv:      "test",
			HTTPAddr:    ":8080",
			DatabaseURL: "postgres://example",
		}, nil
	}

	newDatabase = func(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
		return &pgxpool.Pool{}, nil
	}
	closeDatabase = func(pool *pgxpool.Pool) {}

	newVerifier = func(cfg config.ClerkConfig, logger *slog.Logger) (*clerk.Verifier, error) {
		return nil, nil
	}

	calls := make([]string, 0, 2)
	runMigrations = func(ctx context.Context, logger *slog.Logger, databaseURL string) error {
		calls = append(calls, "migrate")
		return nil
	}

	newServer = func(deps server.Dependencies) serverRunner {
		return stubServer{
			run: func(ctx context.Context) error {
				calls = append(calls, "serve")
				return nil
			},
		}
	}

	if err := Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("calls len = %d, want 2", len(calls))
	}
	if calls[0] != "migrate" || calls[1] != "serve" {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestRunReturnsMigrationError(t *testing.T) {
	originalLoadConfig := loadConfig
	originalNewDatabase := newDatabase
	originalCloseDatabase := closeDatabase
	originalRunMigrations := runMigrations
	originalNewServer := newServer

	t.Cleanup(func() {
		loadConfig = originalLoadConfig
		newDatabase = originalNewDatabase
		closeDatabase = originalCloseDatabase
		runMigrations = originalRunMigrations
		newServer = originalNewServer
	})

	loadConfig = func() (config.Config, error) {
		return config.Config{
			AppEnv:      "test",
			HTTPAddr:    ":8080",
			DatabaseURL: "postgres://example",
		}, nil
	}

	newDatabase = func(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
		return &pgxpool.Pool{}, nil
	}
	closeDatabase = func(pool *pgxpool.Pool) {}

	runMigrations = func(ctx context.Context, logger *slog.Logger, databaseURL string) error {
		return context.DeadlineExceeded
	}

	serverStarted := false
	newServer = func(deps server.Dependencies) serverRunner {
		return stubServer{
			run: func(ctx context.Context) error {
				serverStarted = true
				return nil
			},
		}
	}

	err := Run(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if serverStarted {
		t.Fatal("server should not start when migrations fail")
	}
}
