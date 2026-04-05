# Roadmap 03 Implementation Note

This slice adds the backend implementation for the acceptance workflow already used by the mobile app.

## Included

- `POST /api/v1/work-orders/:id/acceptance`
- `GET /api/v1/acceptances/:id`
- `GET /api/v1/acceptances/:id/pdf`
- Org-scoped acceptance persistence in Postgres
- Deterministic server-side PDF generation in Go
- GCS upload and backend-mediated PDF download
- Idempotent submission by `(organization_id, work_order_id)`

## Data model

The `acceptances` table stores a full snapshot of the submitted acceptance payload so the generated document stays deterministic even if upstream work-order data changes later.

The backend stores only metadata plus the object key for the generated PDF:

- `pdf_status`
- `email_status`
- `pdf_storage_key`
- `pdf_mime_type`
- `pdf_error`
- `pdf_generated_at`

## Storage

Acceptance PDFs are stored in the configured GCS bucket under:

`acceptances/<organization_id>/<acceptance_id>.pdf`

The backend uses the Cloud Run service account to obtain an access token from the metadata server and calls the GCS HTTP API directly. No local filesystem fallback is implemented in this slice.

## Behavior notes

- Duplicate acceptance submissions for the same org and work-order ID return the existing acceptance instead of creating a second record.
- `emailStatus` remains in the contract and is persisted, but email delivery is still out of scope.
- `pdfUrl` returned by the status endpoint points to the backend download route, not a raw GCS URL.
