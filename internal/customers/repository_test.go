package customers

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
	queryFn    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (f fakeQueryer) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return f.queryFn(ctx, sql, args...)
}

func (f fakeQueryer) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return f.queryRowFn(ctx, sql, args...)
}

type fakeRows struct {
	rows [][]any
	i    int
	err  error
}

func (r *fakeRows) Close() {}

func (r *fakeRows) Err() error { return r.err }

func (r *fakeRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }

func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (r *fakeRows) Next() bool {
	if r.i >= len(r.rows) {
		return false
	}
	r.i++
	return true
}

func (r *fakeRows) Scan(dest ...any) error {
	return assignRowValues(dest, r.rows[r.i-1])
}

func (r *fakeRows) Values() ([]any, error) { return r.rows[r.i-1], nil }

func (r *fakeRows) RawValues() [][]byte { return nil }

func (r *fakeRows) Conn() *pgx.Conn { return nil }

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
	case *time.Time:
		*d = value.(time.Time)
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

func TestRepositoryListActiveByOrganizationIDFiltersDeletedRows(t *testing.T) {
	repo := NewRepository(fakeQueryer{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			if !strings.Contains(sql, "FROM customers c") || !strings.Contains(sql, "c.organization_id = $1") {
				t.Fatalf("sql = %s", sql)
			}
			if len(args) != 1 || args[0] != "org-1" {
				t.Fatalf("args = %#v", args)
			}
			now := time.Now().UTC()
			return &fakeRows{rows: [][]any{{"cust-1", "Acme Co", "ops@acme.test", nil, nil, nil, "org-1", "demo-alpha", "Demo Alpha", now, now, nil, now, now, nil}}}, nil
		},
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			t.Fatal("unexpected QueryRow call")
			return fakeRow{}
		},
	})

	customers, err := repo.ListActiveByOrganizationID(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("ListActiveByOrganizationID: %v", err)
	}
	if len(customers) != 1 {
		t.Fatalf("len = %d", len(customers))
	}
	if customers[0].Name != "Acme Co" {
		t.Fatalf("customer = %#v", customers[0])
	}
	if customers[0].Email == nil || *customers[0].Email != "ops@acme.test" {
		t.Fatalf("email = %#v", customers[0].Email)
	}
}

func TestRepositoryGetByIDMapsNotFound(t *testing.T) {
	repo := NewRepository(fakeQueryer{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if !strings.Contains(sql, "WHERE c.organization_id = $1 AND c.id = $2") {
				t.Fatalf("sql = %s", sql)
			}
			return fakeRow{err: pgx.ErrNoRows}
		},
	})

	_, err := repo.GetByID(context.Background(), "org-1", "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestRepositoryCreateMapsForeignKeyViolation(t *testing.T) {
	repo := NewRepository(fakeQueryer{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if !strings.Contains(sql, "INSERT INTO customers") {
				t.Fatalf("sql = %s", sql)
			}
			return fakeRow{err: &pgconn.PgError{Code: "23503"}}
		},
	})

	_, err := repo.Create(context.Background(), CreateInput{
		OrganizationID: "missing-org",
		Name:           "Acme Co",
	})
	if !errors.Is(err, ErrOrganizationNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestRepositoryUpdateRejectsEmptyInput(t *testing.T) {
	repo := NewRepository(fakeQueryer{})
	_, err := repo.Update(context.Background(), "org-1", "cust-1", UpdateInput{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v", err)
	}
}

func TestRepositoryDeleteMapsNotFound(t *testing.T) {
	repo := NewRepository(fakeQueryer{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if !strings.Contains(sql, "deleted_at IS NULL") {
				t.Fatalf("sql = %s", sql)
			}
			return fakeRow{err: pgx.ErrNoRows}
		},
	})

	if err := repo.Delete(context.Background(), "org-1", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestScanCustomerHandlesNullableFields(t *testing.T) {
	now := time.Now().UTC()
	customer, err := scanCustomer(fakeRow{
		values: []any{
			"cust-1",
			"Acme Co",
			nil,
			"555-0100",
			nil,
			"Priority account",
			"org-1",
			"demo-alpha",
			"Demo Alpha",
			now,
			now,
			nil,
			now,
			now,
			nil,
		},
	})
	if err != nil {
		t.Fatalf("scanCustomer: %v", err)
	}
	if customer.Email != nil {
		t.Fatalf("email should be nil: %#v", customer.Email)
	}
	if customer.Phone == nil || *customer.Phone != "555-0100" {
		t.Fatalf("phone = %#v", customer.Phone)
	}
	if customer.Address != nil {
		t.Fatalf("address should be nil: %#v", customer.Address)
	}
	if customer.Notes == nil || *customer.Notes != "Priority account" {
		t.Fatalf("notes = %#v", customer.Notes)
	}
	if !customer.Organization.Active {
		t.Fatal("expected organization to be active")
	}
}
