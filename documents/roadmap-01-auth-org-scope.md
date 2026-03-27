# Roadmap 01: Tenancy And Authorization Alignment

## Summary

This milestone establishes the first organization-scoped application route for authenticated members and separates that flow from the existing admin-only management routes.

The implementation keeps Clerk as the authentication layer and uses the local `users` table as the source of truth for organization membership. A valid Clerk token alone is not enough for org-scoped access. The request must resolve to an active local user whose linked organization is also active.

## What Was Implemented

### Organization-scoped route group

- Added a new `/api/v1/org` route group in `internal/server/server.go`
- The route group applies:
  - `middleware.ClerkAuth(...)` for Clerk bearer token verification
  - `users.RequireOrgAccess(...)` for local membership resolution and tenancy enforcement

### Local membership authorization

- Added `internal/users/context.go`
- Introduced `users.RequireOrgAccess(...)` middleware that:
  - reads Clerk claims from request context
  - resolves the local user by `claims.Subject`
  - rejects requests when the local user does not exist
  - rejects requests when the linked organization is inactive
  - cross-checks Clerk `org_slug` when present
  - stores the resolved user and organization in Gin context for handler reuse

### First org-scoped endpoint

- Added `RegisterOrgRoutes(...)` and `Handler.Me(...)` in `internal/users/handler.go`
- Exposed `GET /api/v1/org/me`
- The endpoint returns:
  - the resolved local user
  - the resolved organization
  - Clerk auth context used for the request

### Context helper cleanup

- Kept generic Clerk claim helpers in `internal/middleware/auth.go`
- Avoided putting business-domain membership types into the generic middleware package to prevent package cycles and keep the auth boundary explicit

## Files Updated

- `internal/server/server.go`
- `internal/users/context.go`
- `internal/users/handler.go`
- `internal/users/handler_test.go`
- `internal/middleware/auth.go`
- `internal/middleware/auth_test.go`
- `README.md`
- `documents/README.md`

## Functionality Added

- Non-admin authenticated users now have a dedicated org-scoped application route group
- Organization access is enforced through local database membership, not only Clerk token claims
- Clerk `org_slug` is used as a consistency check when present
- The backend now exposes a stable starting point for future tenant-scoped resources such as customers and work orders

## How To Test

### Automated verification

Run:

```bash
GOCACHE=/tmp/tecora-gocache go test ./...
```

Expected result:

- all tests pass

### Manual API verification

Prerequisites:

- Clerk environment variables configured
- a local `users` row exists for the Clerk `sub`
- that user is linked to an active organization

Request:

```bash
curl http://localhost:8080/api/v1/org/me \
  -H "Authorization: Bearer <clerk-session-jwt>"
```

Expected behavior:

- `200 OK` when the Clerk user maps to an active local membership
- `403 Forbidden` when no local user exists or the Clerk `org_slug` conflicts with the stored organization
- `503 Service Unavailable` when Clerk auth is not configured

## Review Notes

- This milestone intentionally does not add customers, work orders, or new schema changes
- Admin CRUD routes remain unchanged under `/api/v1/admin`
- The new org-scoped authorization path is designed to be reused by Milestone 2 customer endpoints
