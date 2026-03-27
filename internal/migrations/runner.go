package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	dbmigrations "tecora/db/migrations"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func Up(ctx context.Context, logger *slog.Logger, databaseURL string) error {
	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open sql database for migrations: %w", err)
	}
	defer sqlDB.Close()

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sql database for migrations: %w", err)
	}

	sourceDriver, err := iofs.New(dbmigrations.Files, ".")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}

	databaseDriver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("configure postgres migration driver: %w", err)
	}

	runner, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", databaseDriver)
	if err != nil {
		return fmt.Errorf("create migration runner: %w", err)
	}
	defer closeRunner(logger, runner)

	logVersion(logger, "starting migration check", runner)

	if err := runner.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			logger.Info("no pending migrations")
			return nil
		}
		return fmt.Errorf("apply migrations: %w", err)
	}

	logVersion(logger, "migrations applied", runner)
	return nil
}

func closeRunner(logger *slog.Logger, runner *migrate.Migrate) {
	sourceErr, databaseErr := runner.Close()
	if sourceErr != nil {
		logger.Warn("close migration source", "error", sourceErr)
	}
	if databaseErr != nil {
		logger.Warn("close migration database", "error", databaseErr)
	}
}

func logVersion(logger *slog.Logger, message string, runner *migrate.Migrate) {
	version, dirty, err := runner.Version()
	if err != nil {
		if errors.Is(err, migrate.ErrNilVersion) {
			logger.Info(message, "version", "none", "dirty", false)
			return
		}
		logger.Warn("read migration version", "error", err)
		return
	}

	logger.Info(message, "version", version, "dirty", dirty)
}
