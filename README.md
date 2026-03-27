# Tecora Backend

This repository contains the first backend bootstrap for Tecora.

The initial goal is intentionally small: ship a Go API that can be deployed now, exposes a public health endpoint, and establishes the conventions we will use as the service grows.

## What is in this bootstrap

- Go 1.21 service using the `gin` framework
- Versioned routes under `/api/v1`
- Public health endpoint at `GET /api/v1/health`
- Sample protected route at `GET /api/v1/private/me`
- Organization-scoped member endpoint at `GET /api/v1/org/me`
- Admin organization registry routes under `/api/v1/admin/organizations`
- PostgreSQL wiring using `pgx`
- Clerk JWT verification middleware for protected routes, deferred for the first Cloud Run rollout
- Docker-first local development setup
- Basic graceful shutdown and structured logging
- Startup-applied SQL migrations for additive schema changes

## Architecture

The code is organized as a small, conventional Go service:

- `cmd/api` contains the executable entrypoint.
- `internal/config` loads and validates environment configuration.
- `internal/database` connects to PostgreSQL.
- `internal/auth/clerk` verifies Clerk bearer JWTs.
- `internal/middleware` holds HTTP middleware.
- `internal/server` wires the Gin engine and route groups.
- `internal/health` provides the health endpoint handler.
- `db/migrations` stores SQL migrations.

The service uses a single binary and relies on environment variables for configuration.
On startup it connects to PostgreSQL, applies any pending embedded migrations, and only then starts serving HTTP traffic.

## Health endpoint

`GET /api/v1/health`

This endpoint is public and is meant to be used for deployment checks and uptime monitoring.

The response includes:

- service name
- status
- timestamp
- PostgreSQL connectivity state

If PostgreSQL is unavailable, the endpoint returns a non-200 response so readiness issues are visible quickly.
The service itself does not fail startup on a transient database outage, so this endpoint is the source of truth for DB health.

## Clerk authentication

Protected routes are wired to validate Clerk session JWTs sent from the React Native app as:

```http
Authorization: Bearer <token>
```

The verifier checks:

- JWT signature using Clerk JWKS
- issuer
- expiration
- optional audience

Important: if Clerk environment variables are not provided yet, the protected route group remains present but returns a clear configuration error until Clerk is configured.
For the initial Cloud Run deployment, leave Clerk environment variables empty so the first rollout stays focused on Cloud SQL and the health endpoint.

For organization-scoped application routes, Clerk authentication is only the first step. The API resolves the authenticated local `users` record from the Clerk `sub` claim and uses that stored organization membership as the tenancy boundary. If the Clerk token also includes `org_slug`, the API treats it as a cross-check against the local membership before allowing access.

Required Clerk variables:

- `CLERK_ISSUER_URL`
- `CLERK_JWKS_URL`
- `CLERK_AUDIENCE` if your project uses an audience value
- `ADMIN_CLERK_USER_IDS` for admin organization management routes, as a comma-separated list of Clerk `sub` values

## Local development

### 1. Copy environment variables

Use `.env.example` as the starting point.
The API entrypoint also loads `.env` from the repository root when you run it locally.

### 2. Start PostgreSQL and the API

```bash
docker compose up --build
```

The API listens on `http://localhost:8080`.
The Dockerized PostgreSQL instance publishes on host port `5433` to avoid collisions with any local Postgres service on `5432`.

### 3. Check health

```bash
curl http://localhost:8080/api/v1/health
```

### 4. Exercise the protected route

When Clerk is configured, send a bearer token to:

```bash
curl http://localhost:8080/api/v1/private/me \
  -H "Authorization: Bearer <clerk-session-jwt>"
```

### 5. Exercise the organization-scoped route

When Clerk is configured and the authenticated Clerk user has a matching local `users` row, send a bearer token to:

```bash
curl http://localhost:8080/api/v1/org/me \
  -H "Authorization: Bearer <clerk-session-jwt>"
```

The response returns the resolved local user, the organization derived from local membership, and the Clerk auth context used for the request.

## Environment variables

- `APP_ENV` - runtime mode, for example `development` or `production`
- `PORT` - listen port, for example `8080`
- `DATABASE_URL` - PostgreSQL connection string
- `CLERK_ISSUER_URL` - Clerk issuer URL
- `CLERK_JWKS_URL` - Clerk JWKS URL
- `CLERK_AUDIENCE` - optional Clerk audience
- `ADMIN_CLERK_USER_IDS` - comma-separated Clerk user IDs allowed to manage organizations
- `CORS_ALLOWED_ORIGINS` - comma-separated allowed origins

`HTTP_ADDR` is still accepted as a fallback listen address for older local setups, but new deployment configuration should use `PORT`.

For Cloud Run with Cloud SQL, `DATABASE_URL` should point at the Unix socket mount, for example:

```text
postgres://USER:PASSWORD@/DBNAME?host=/cloudsql/PROJECT_ID:REGION:INSTANCE_CONNECTION_NAME&sslmode=disable
```

## Database and migrations

PostgreSQL runs locally through Docker Compose.

Migration files live in `db/migrations` and follow the standard `up` and `down` SQL pattern.
They are embedded into the binary and applied automatically during service startup.

Current strategy:

- keep the schema in SQL files
- commit each schema change as a new migration pair
- let the service apply pending migrations during startup before the new revision becomes ready

Startup migration guardrails for the current Cloud Run phase:

- keep migrations additive and backward-compatible
- keep migrations short
- avoid destructive schema changes, long backfills, and rollout-incompatible constraint changes
- keep Cloud Run capped at one instance while this simplified model is in use
- revisit this approach before enabling normal autoscaling or non-additive schema evolution

The bootstrap now includes the first application schema: an `organizations` table for multitenant scoping.

The initial migration pair still exists as the base schema placeholder, and the next migration creates the `organizations` table and seeds temporary demo organizations so the admin registry exists before the dashboard ships.

## Deployment

The repository includes a Dockerfile and a Cloud Run service manifest at `deploy/cloud-run.yaml`.

### 1. Build and push an image

Build the container and push it to Artifact Registry:

```bash
gcloud builds submit --tag REGION-docker.pkg.dev/PROJECT_ID/REPOSITORY/tecora-backend:TAG
```

### 2. Create the database secret

Store the production `DATABASE_URL` in Secret Manager so the password is not committed to the repo or baked into the manifest.

```bash
printf '%s' 'postgres://USER:PASSWORD@/DBNAME?host=/cloudsql/PROJECT_ID:REGION:INSTANCE_CONNECTION_NAME&sslmode=disable' \
  | gcloud secrets create tecora-database-url --replication-policy=automatic --data-file=-
```

If the secret already exists, add a new version instead:

```bash
printf '%s' 'postgres://USER:PASSWORD@/DBNAME?host=/cloudsql/PROJECT_ID:REGION:INSTANCE_CONNECTION_NAME&sslmode=disable' \
  | gcloud secrets versions add tecora-database-url --data-file=-
```

### 3. Grant Cloud SQL access

Make sure the Cloud Run service account has the `roles/cloudsql.client` role and `roles/secretmanager.secretAccessor` so it can reach both Cloud SQL and the `DATABASE_URL` secret.

### 4. Deploy the service

Replace the placeholders in `deploy/cloud-run.yaml` and deploy the revision:

```bash
gcloud run services replace deploy/cloud-run.yaml --region REGION --project PROJECT_ID
```

The manifest intentionally caps the service at one instance for this phase so startup migrations stay serialized in normal operation.
Cloud Run can still briefly overlap revisions during rollout, so each migration must remain additive and safe to run with the Postgres migration lock in place.

### 5. Verify health

After deployment, call the public health endpoint:

```bash
curl https://YOUR_CLOUD_RUN_URL/api/v1/health
```

Expect `status=ok` and `database=up` when Cloud SQL is reachable.

### 6. Add Clerk later

When protected routes are needed in production, add the Clerk environment variables back to Cloud Run and keep the same deployment shape.

## Next steps

The next likely additions are:

- user/session endpoints
- Clerk-protected resource routes scoped by resolved organization membership
- request validation helpers
- Postgres-backed domain tables referencing `organization_id`
- migration runner or deploy-time migration job
- observability improvements
