# AGENTS.md

## Project Overview

- Project: `TecoraBE`
- Language: Go 1.21
- HTTP framework: `gin`
- Database: PostgreSQL via `pgx`
- Deployment target: Google Cloud Run with Cloud SQL
- Auth provider: Clerk

This service is a small Go API with versioned routes under `/api/v1`. The current production posture is intentionally simple: one Cloud Run service, one PostgreSQL database, and schema migrations applied automatically during application startup.

## Important Paths

- Entrypoint: `cmd/api/main.go`
- App bootstrap: `internal/app/app.go`
- Server wiring: `internal/server/server.go`
- Config loading: `internal/config/config.go`
- Database pool: `internal/database/postgres.go`
- Migration runner: `internal/migrations/runner.go`
- SQL migrations: `db/migrations`
- Cloud Run manifest: `deploy/cloud-run.yaml`
- Main project documentation: `README.md`

## Runtime Model

- The service loads environment variables, connects to PostgreSQL, runs pending migrations, and only then starts the HTTP server.
- Migrations are embedded into the binary from `db/migrations`.
- The current Cloud Run manifest caps the service at one instance with `autoscaling.knative.dev/maxScale: "1"`.
- Cloud Run may still briefly overlap revisions during deployment, so startup migrations must remain safe under brief concurrent startup attempts.

## Migration Rules

These rules are intentional and must be preserved unless the deployment model changes.

- Migrations run on startup in `internal/app/app.go`.
- The migration runner uses `golang-migrate` with the Postgres driver.
- Embedded migration files are the source of truth; do not switch to runtime filesystem lookups without a deliberate migration of the deployment model.
- Treat `no change` as success.
- Migration failures must fail startup. Do not swallow migration errors.
- Keep migrations additive and backward-compatible.
- Keep migrations short. Do not add long-running backfills or bulk data rewrites to startup migrations.
- Avoid destructive schema changes in this mode.
- Avoid rollout-incompatible changes such as renaming or dropping columns that older revisions may still reference.
- Prefer idempotent data changes where possible, for example `INSERT ... ON CONFLICT DO NOTHING`.
- Revisit the startup-migration approach before enabling normal autoscaling or before introducing non-additive schema evolution.

## What Is Safe Right Now

- New tables
- New nullable columns
- New indexes that are compatible with live traffic
- Idempotent seed inserts
- Additive constraints that do not break existing rows or older code paths

## What Is Not Safe Right Now

- Dropping tables or columns
- Renaming columns or tables used by deployed code
- Tightening constraints without a compatible backfill strategy
- Long-running data migrations during startup
- Any schema change that assumes a strict single global process during Cloud Run rollout

## Data and Existing Databases

- Existing production data is preserved unless a migration explicitly changes it.
- Applied migration versions are tracked in the database by the migration library.
- On startup, only pending migrations are applied; already-applied versions are skipped.
- Repeated restarts should be safe and typically result in `no pending migrations`.

## Local Development Notes

- Local Postgres runs through `docker-compose.yml`.
- The API loads `.env` from the repo root when started locally.
- `DATABASE_URL` is required.
- For Cloud Run + Cloud SQL, `DATABASE_URL` should use the Unix socket form documented in `README.md`.

## Testing Expectations

- Run `go test ./...` after changing application or migration logic.
- Migration tests may use `TEST_DATABASE_URL` for disposable integration coverage.
- If you add a migration, verify both:
  - a clean database can apply all migrations
  - a database that already has prior versions can start again without changes

## Agent Guidance

- Do not change the migration model casually. It is a project-level operational decision.
- If a task requires destructive, long-running, or non-backward-compatible migrations, stop treating startup migrations as acceptable and propose moving to a deploy-time migration job first.
- Keep docs in `README.md` and deployment assumptions in `deploy/cloud-run.yaml` aligned with any migration-related code changes.
- Prefer small, explicit SQL migrations over implicit schema management in Go code.
