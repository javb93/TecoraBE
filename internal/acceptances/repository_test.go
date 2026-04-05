package acceptances

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeQueryer struct {
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (f fakeQueryer) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return f.queryRowFn(ctx, sql, args...)
}

type fakeRow struct {
	values []any
	err    error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	return assignRowValues(dest, r.values)
}

func assignRowValues(dest []any, values []any) error {
	if len(dest) != len(values) {
		return errors.New("unexpected scan destination count")
	}

	for i := range dest {
		if err := assignValue(dest[i], values[i]); err != nil {
			return err
		}
	}

	return nil
}

func assignValue(dest any, value any) error {
	switch d := dest.(type) {
	case *string:
		if value == nil {
			*d = ""
			return nil
		}
		*d = value.(string)
		return nil
	case *bool:
		*d = value.(bool)
		return nil
	case *time.Time:
		*d = value.(time.Time)
		return nil
	case *[]string:
		if value == nil {
			*d = nil
			return nil
		}
		*d = append([]string(nil), value.([]string)...)
		return nil
	case *PDFStatus:
		*d = value.(PDFStatus)
		return nil
	case *EmailStatus:
		*d = value.(EmailStatus)
		return nil
	case *sql.NullString:
		if value == nil {
			*d = sql.NullString{}
			return nil
		}
		*d = sql.NullString{String: value.(string), Valid: true}
		return nil
	case *sql.NullTime:
		if value == nil {
			*d = sql.NullTime{}
			return nil
		}
		*d = sql.NullTime{Time: value.(time.Time), Valid: true}
		return nil
	default:
		return errors.New("unsupported scan destination")
	}
}

func TestRepositoryGetByIDScopesByOrganization(t *testing.T) {
	repo := NewRepository(fakeQueryer{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if !strings.Contains(sql, "WHERE a.organization_id = $1 AND a.id = $2") {
				t.Fatalf("sql = %s", sql)
			}
			if len(args) != 2 || args[0] != "org-1" || args[1] != "acc-1" {
				t.Fatalf("args = %#v", args)
			}
			return fakeRow{values: recordValues("acc-1", "org-1", "wo-1")}
		},
	})

	record, err := repo.GetByID(context.Background(), "org-1", "acc-1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if record.OrganizationID != "org-1" || record.ID != "acc-1" {
		t.Fatalf("record = %#v", record)
	}
}

func TestRepositoryGetByWorkOrderMapsNotFound(t *testing.T) {
	repo := NewRepository(fakeQueryer{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if !strings.Contains(sql, "WHERE a.organization_id = $1 AND a.work_order_id = $2") {
				t.Fatalf("sql = %s", sql)
			}
			return fakeRow{err: pgx.ErrNoRows}
		},
	})

	_, err := repo.GetByWorkOrderID(context.Background(), "org-1", "wo-missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestRepositoryCreateMapsConflict(t *testing.T) {
	repo := NewRepository(fakeQueryer{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if !strings.Contains(sql, "INSERT INTO acceptances") {
				t.Fatalf("sql = %s", sql)
			}
			return fakeRow{err: &pgconn.PgError{Code: "23505"}}
		},
	})

	_, err := repo.Create(context.Background(), CreateInput{
		OrganizationID:        "org-1",
		WorkOrderID:           "wo-1",
		CustomerName:          "Acme",
		CustomerEmail:         "ops@acme.test",
		ServiceDate:           "2025-03-01",
		ServiceExpirationDate: "2025-04-01",
		ServiceType:           "Quarterly",
		Products:              []string{"Sealant"},
		SignatureImageBase64:  "abc",
		SignedAt:              time.Now().UTC(),
		SignedByTechnicianID:  "tech-1",
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v", err)
	}
}

func TestRepositoryMarkPDFGeneratedPersistsStorageFields(t *testing.T) {
	now := time.Now().UTC()
	repo := NewRepository(fakeQueryer{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if !strings.Contains(sql, "SET pdf_status = $3") || !strings.Contains(sql, "pdf_storage_key = $4") {
				t.Fatalf("sql = %s", sql)
			}
			values := recordValues("acc-1", "org-1", "wo-1")
			values[14] = PDFStatusGenerated
			values[16] = "acceptances/org-1/acc-1.pdf"
			values[17] = "application/pdf"
			values[19] = now
			values[22] = now
			return fakeRow{values: values}
		},
	})

	record, err := repo.MarkPDFGenerated(context.Background(), "org-1", "acc-1", "acceptances/org-1/acc-1.pdf", "application/pdf")
	if err != nil {
		t.Fatalf("MarkPDFGenerated: %v", err)
	}
	if record.PDFStorageKey == nil || *record.PDFStorageKey != "acceptances/org-1/acc-1.pdf" {
		t.Fatalf("storage key = %#v", record.PDFStorageKey)
	}
	if record.PDFMimeType == nil || *record.PDFMimeType != "application/pdf" {
		t.Fatalf("mime type = %#v", record.PDFMimeType)
	}
}

func TestScanRecordHandlesNullableFields(t *testing.T) {
	record, err := scanRecord(fakeRow{values: recordValues("acc-1", "org-1", "wo-1")})
	if err != nil {
		t.Fatalf("scanRecord: %v", err)
	}
	if record.PDFStorageKey != nil || record.PDFMimeType != nil || record.PDFError != nil || record.EmailSentAt != nil {
		t.Fatalf("nullable fields should be nil: %#v", record)
	}
	if len(record.Products) != 2 {
		t.Fatalf("products = %#v", record.Products)
	}
}

func recordValues(acceptanceID, organizationID, workOrderID string) []any {
	now := time.Now().UTC()
	return []any{
		acceptanceID,
		organizationID,
		workOrderID,
		"Acme Co",
		"ops@acme.test",
		"2025-03-01",
		"2025-04-01",
		"Quarterly service",
		[]string{"Sealant", "Inspection"},
		"Everything looks good.",
		true,
		"data:image/png;base64,abc",
		now,
		"tech-1",
		PDFStatusPending,
		EmailStatusPending,
		nil,
		nil,
		nil,
		nil,
		nil,
		now,
		now,
	}
}
