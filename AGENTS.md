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

## Branch And PR Workflow

This repo should be worked through short-lived feature branches, not by merging agent work directly into `main`.

### Default Rule

- Do not implement feature work directly on `main`.
- Each substantial task should use its own branch.
- Finished work should be reviewed in a PR before merging.

### Recommended Setup For Multiple Agents

When several agents work in parallel, use one branch and one Git worktree per agent.

- `main` stays as the integration branch.
- Each agent gets a dedicated branch created from the latest `main`.
- Each agent should work in its own worktree, not in the same filesystem checkout as another agent.
- Do not have multiple agents editing the same checkout on different assumptions.

Recommended branch naming:

- `feat/<roadmap-id>-<short-name>`
- `fix/<short-name>`
- `chore/<short-name>`

Examples:

- `feat/roadmap-01-auth-org-scope`
- `feat/roadmap-02-customers`
- `feat/roadmap-03-work-orders`

### Agent Execution Rules

Before making feature changes, an agent should:

1. Check the current branch.
2. If on `main`, create or switch to the task branch before editing.
3. Prefer a dedicated worktree for parallel feature work.
4. Keep its scope limited to the assigned task.

While implementing:

- Do not rebase, force-push, or rewrite history unless explicitly requested.
- Do not mix unrelated roadmap tasks in the same branch.
- Keep changes reviewable and focused.
- Update or add tests relevant to the branch scope.

When finished:

1. Run relevant verification, at minimum `go test ./...` when application logic changes.
2. Summarize the change, risks, and any follow-up work.
3. Open or prepare a PR against `main`.

### Worktree Guidance

For parallel agent work, prefer a layout like:

- main checkout: `/path/to/TecoraBE`
- agent worktree 1: `/path/to/TecoraBE-auth`
- agent worktree 2: `/path/to/TecoraBE-customers`
- agent worktree 3: `/path/to/TecoraBE-work-orders`

Example flow:

1. Create branch from `main`
2. Create worktree for that branch
3. Assign one agent to that worktree
4. Review changes in a PR
5. Merge back into `main`

### PR Expectations

Each PR should ideally map to one roadmap task or one tightly related slice of a roadmap task.

Good PR examples:

- auth and organization scoping only
- customer schema plus customer CRUD
- work-order schema plus lifecycle endpoints

Avoid PRs that combine:

- customers and work orders
- work orders and document generation
- auth refactors and unrelated feature work

### Coordination Notes

- If one branch depends on another unmerged branch, state that explicitly in the PR.
- Prefer dependency order from `ROADMAP.md` when choosing which branches can run in parallel.
- If two agents may touch the same files, split responsibilities before implementation to avoid merge churn.
