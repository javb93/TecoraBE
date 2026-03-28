# Roadmap 02: Customers Foundation

## Summary

This milestone adds the first CRM data model for Tecora: customers owned by organizations. The slice is intentionally limited to schema, repository behavior, repository tests, and migration verification so it remains small and reviewable.

Customer HTTP handlers and org-scoped route wiring are intentionally deferred to the next PR. This branch creates the data layer that those routes will use.

## What Was Implemented

### Customer schema

- Added `db/migrations/000004_customers.up.sql`
- Added `db/migrations/000004_customers.down.sql`
- Introduced a new `customers` table with:
  - `organization_id`
  - `name`
  - nullable `email`
  - nullable `phone`
  - nullable `address`
  - nullable `notes`
  - timestamps and `deleted_at`
- Added organization-scoped partial indexes for active customer listing

### Customer repository package

- Added `internal/customers/models.go`
- Added `internal/customers/repository.go`
- Repository methods are scoped by `organization_id` from the start:
  - `ListActiveByOrganizationID`
  - `GetByID`
  - `Create`
  - `Update`
  - `Delete`

### Testing

- Added `internal/customers/repository_test.go`
- Extended `internal/migrations/runner_test.go` to verify:
  - customer migrations apply on a clean database
  - customer nullable fields can be inserted as `NULL`
  - rerunning migrations remains a no-op

## Files Updated

- `db/migrations/000004_customers.up.sql`
- `db/migrations/000004_customers.down.sql`
- `internal/customers/models.go`
- `internal/customers/repository.go`
- `internal/customers/repository_test.go`
- `internal/migrations/runner_test.go`
- `documents/roadmap-02-customers-foundation.md`

## Functionality Added

- The backend now has a persistent `customers` table linked to organizations
- Customer repository reads and writes are tenant-scoped by construction
- Soft deletion semantics match the existing repository posture
- The codebase now has a clean data-layer foundation for customer handlers in the next PR

## How To Test

### Automated verification

Run:

```bash
GOCACHE=/tmp/tecora-gocache go test ./...
```

Expected result:

- all tests pass

### Optional migration integration

If `TEST_DATABASE_URL` is configured, the migration integration test also verifies:

- clean database migration to the latest version
- `customers` table creation
- nullable customer fields persist correctly
- repeat startup migrations are safe

## Review Notes

- This PR does not add customer HTTP endpoints yet
- This PR does not change admin or org route registration
- The next customer PR can build handlers and route wiring on top of this repository layer
