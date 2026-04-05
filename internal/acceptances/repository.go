package acceptances

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Repository struct {
	db Queryer
}

func NewRepository(db Queryer) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetByID(ctx context.Context, organizationID, acceptanceID string) (*Record, error) {
	record, err := scanRecord(r.db.QueryRow(ctx, baseSelectQuery+`
		WHERE a.organization_id = $1 AND a.id = $2
	`, strings.TrimSpace(organizationID), strings.TrimSpace(acceptanceID)))
	if err != nil {
		return nil, translateRowError(err)
	}
	return record, nil
}

func (r *Repository) GetByWorkOrderID(ctx context.Context, organizationID, workOrderID string) (*Record, error) {
	record, err := scanRecord(r.db.QueryRow(ctx, baseSelectQuery+`
		WHERE a.organization_id = $1 AND a.work_order_id = $2
	`, strings.TrimSpace(organizationID), strings.TrimSpace(workOrderID)))
	if err != nil {
		return nil, translateRowError(err)
	}
	return record, nil
}

func (r *Repository) Create(ctx context.Context, input CreateInput) (*Record, error) {
	record, err := scanRecord(r.db.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO acceptances (
				organization_id,
				work_order_id,
				customer_name,
				customer_email,
				service_date,
				service_expiration_date,
				service_type,
				products,
				notes,
				approved,
				signature_image_base64,
				signed_at,
				signed_by_technician_id,
				pdf_status,
				email_status
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
			RETURNING *
		)
		SELECT
			a.id,
			a.organization_id,
			a.work_order_id,
			a.customer_name,
			a.customer_email,
			a.service_date,
			a.service_expiration_date,
			a.service_type,
			a.products,
			a.notes,
			a.approved,
			a.signature_image_base64,
			a.signed_at,
			a.signed_by_technician_id,
			a.pdf_status,
			a.email_status,
			a.pdf_storage_key,
			a.pdf_mime_type,
			a.pdf_error,
			a.pdf_generated_at,
			a.email_sent_at,
			a.created_at,
			a.updated_at
		FROM inserted a
	`,
		strings.TrimSpace(input.OrganizationID),
		strings.TrimSpace(input.WorkOrderID),
		strings.TrimSpace(input.CustomerName),
		strings.TrimSpace(input.CustomerEmail),
		strings.TrimSpace(input.ServiceDate),
		strings.TrimSpace(input.ServiceExpirationDate),
		strings.TrimSpace(input.ServiceType),
		input.Products,
		strings.TrimSpace(input.Notes),
		input.Approved,
		strings.TrimSpace(input.SignatureImageBase64),
		input.SignedAt.UTC(),
		strings.TrimSpace(input.SignedByTechnicianID),
		PDFStatusPending,
		EmailStatusPending,
	))
	if err != nil {
		return nil, translateWriteError(err)
	}
	return record, nil
}

func (r *Repository) MarkPDFGenerated(ctx context.Context, organizationID, acceptanceID, storageKey, mimeType string) (*Record, error) {
	record, err := scanRecord(r.db.QueryRow(ctx, `
		WITH updated AS (
			UPDATE acceptances
			SET pdf_status = $3,
				pdf_storage_key = $4,
				pdf_mime_type = $5,
				pdf_error = NULL,
				pdf_generated_at = NOW(),
				updated_at = NOW()
			WHERE organization_id = $1 AND id = $2
			RETURNING *
		)
		SELECT
			a.id,
			a.organization_id,
			a.work_order_id,
			a.customer_name,
			a.customer_email,
			a.service_date,
			a.service_expiration_date,
			a.service_type,
			a.products,
			a.notes,
			a.approved,
			a.signature_image_base64,
			a.signed_at,
			a.signed_by_technician_id,
			a.pdf_status,
			a.email_status,
			a.pdf_storage_key,
			a.pdf_mime_type,
			a.pdf_error,
			a.pdf_generated_at,
			a.email_sent_at,
			a.created_at,
			a.updated_at
		FROM updated a
	`, strings.TrimSpace(organizationID), strings.TrimSpace(acceptanceID), PDFStatusGenerated, strings.TrimSpace(storageKey), strings.TrimSpace(mimeType)))
	if err != nil {
		return nil, translateRowError(err)
	}
	return record, nil
}

func (r *Repository) MarkPDFFailed(ctx context.Context, organizationID, acceptanceID, pdfError string) (*Record, error) {
	record, err := scanRecord(r.db.QueryRow(ctx, `
		WITH updated AS (
			UPDATE acceptances
			SET pdf_status = $3,
				pdf_error = $4,
				updated_at = NOW()
			WHERE organization_id = $1 AND id = $2
			RETURNING *
		)
		SELECT
			a.id,
			a.organization_id,
			a.work_order_id,
			a.customer_name,
			a.customer_email,
			a.service_date,
			a.service_expiration_date,
			a.service_type,
			a.products,
			a.notes,
			a.approved,
			a.signature_image_base64,
			a.signed_at,
			a.signed_by_technician_id,
			a.pdf_status,
			a.email_status,
			a.pdf_storage_key,
			a.pdf_mime_type,
			a.pdf_error,
			a.pdf_generated_at,
			a.email_sent_at,
			a.created_at,
			a.updated_at
		FROM updated a
	`, strings.TrimSpace(organizationID), strings.TrimSpace(acceptanceID), PDFStatusFailed, truncateError(pdfError)))
	if err != nil {
		return nil, translateRowError(err)
	}
	return record, nil
}

const baseSelectQuery = `
	SELECT
		a.id,
		a.organization_id,
		a.work_order_id,
		a.customer_name,
		a.customer_email,
		a.service_date,
		a.service_expiration_date,
		a.service_type,
		a.products,
		a.notes,
		a.approved,
		a.signature_image_base64,
		a.signed_at,
		a.signed_by_technician_id,
		a.pdf_status,
		a.email_status,
		a.pdf_storage_key,
		a.pdf_mime_type,
		a.pdf_error,
		a.pdf_generated_at,
		a.email_sent_at,
		a.created_at,
		a.updated_at
	FROM acceptances a
`

func scanRecord(scanner interface{ Scan(...any) error }) (*Record, error) {
	var record Record
	var pdfStorageKey sql.NullString
	var pdfMimeType sql.NullString
	var pdfError sql.NullString
	var pdfGeneratedAt sql.NullTime
	var emailSentAt sql.NullTime

	err := scanner.Scan(
		&record.ID,
		&record.OrganizationID,
		&record.WorkOrderID,
		&record.CustomerName,
		&record.CustomerEmail,
		&record.ServiceDate,
		&record.ServiceExpirationDate,
		&record.ServiceType,
		&record.Products,
		&record.Notes,
		&record.Approved,
		&record.SignatureImageBase64,
		&record.SignedAt,
		&record.SignedByTechnicianID,
		&record.PDFStatus,
		&record.EmailStatus,
		&pdfStorageKey,
		&pdfMimeType,
		&pdfError,
		&pdfGeneratedAt,
		&emailSentAt,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if pdfStorageKey.Valid {
		record.PDFStorageKey = &pdfStorageKey.String
	}
	if pdfMimeType.Valid {
		record.PDFMimeType = &pdfMimeType.String
	}
	if pdfError.Valid {
		record.PDFError = &pdfError.String
	}
	if pdfGeneratedAt.Valid {
		record.PDFGeneratedAt = &pdfGeneratedAt.Time
	}
	if emailSentAt.Valid {
		record.EmailSentAt = &emailSentAt.Time
	}

	return &record, nil
}

func translateRowError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func translateWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrConflict
		}
	}
	return err
}

func truncateError(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) <= 2048 {
		return raw
	}
	return raw[:2048]
}
