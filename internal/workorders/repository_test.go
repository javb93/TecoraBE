package workorders

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

func (r *fakeRows) Close()                                       {}
func (r *fakeRows) Err() error                                   { return r.err }
func (r *fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeRows) Next() bool {
	if r.i >= len(r.rows) {
		return false
	}
	r.i++
	return true
}
func (r *fakeRows) Scan(dest ...any) error { return assignRowValues(dest, r.rows[r.i-1]) }
func (r *fakeRows) Values() ([]any, error) { return r.rows[r.i-1], nil }
func (r *fakeRows) RawValues() [][]byte    { return nil }
func (r *fakeRows) Conn() *pgx.Conn        { return nil }

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

func TestRepositoryCreateTrimsAndStoresOptionalFields(t *testing.T) {
	jobDate, err := ParseDate("2025-03-15")
	if err != nil {
		t.Fatalf("ParseDate: %v", err)
	}

	repo := NewRepository(fakeQueryer{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if !strings.Contains(sql, "INSERT INTO work_orders") {
				t.Fatalf("sql = %s", sql)
			}
			if len(args) != 8 {
				t.Fatalf("args = %#v", args)
			}
			if args[0] != "org-1" || args[1] != "WO-1001" || args[2] != "Acme Co" || args[5] != "123 Main" {
				t.Fatalf("args = %#v", args)
			}
			if email, ok := args[3].(*string); !ok || email != nil {
				t.Fatalf("customerEmail arg = %#v", args[3])
			}
			if phone, ok := args[4].(*string); !ok || phone != nil {
				t.Fatalf("customerPhone arg = %#v", args[4])
			}
			if status, ok := args[7].(*string); !ok || status != nil {
				t.Fatalf("optional args = %#v", args)
			}

			now := time.Now().UTC()
			return fakeRow{values: []any{
				"row-1",
				"org-1",
				"WO-1001",
				"Acme Co",
				nil,
				nil,
				"123 Main",
				jobDate.Time,
				nil,
				now,
				now,
				nil,
			}}
		},
	})

	workOrder, err := repo.Create(context.Background(), CreateInput{
		OrganizationID:  "org-1",
		WorkOrderID:     "WO-1001",
		CustomerName:    "Acme Co",
		CustomerAddress: "123 Main",
		JobDate:         jobDate,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if workOrder.CustomerEmail != nil || workOrder.CustomerPhone != nil || workOrder.Status != nil {
		t.Fatalf("workOrder = %#v", workOrder)
	}
}

func TestRepositoryCreateMapsConflict(t *testing.T) {
	jobDate, _ := ParseDate("2025-03-15")
	repo := NewRepository(fakeQueryer{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			return fakeRow{err: &pgconn.PgError{Code: "23505"}}
		},
	})

	_, err := repo.Create(context.Background(), CreateInput{
		OrganizationID:  "org-1",
		WorkOrderID:     "WO-1001",
		CustomerName:    "Acme Co",
		CustomerAddress: "123 Main",
		JobDate:         jobDate,
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v", err)
	}
}

func TestRepositoryListActiveByOrganizationIDFiltersDeletedRows(t *testing.T) {
	now := time.Now().UTC()
	jobDateA, _ := ParseDate("2025-03-15")
	jobDateB, _ := ParseDate("2025-03-16")

	repo := NewRepository(fakeQueryer{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			if !strings.Contains(sql, "FROM work_orders w") || !strings.Contains(sql, "w.deleted_at IS NULL") {
				t.Fatalf("sql = %s", sql)
			}
			if len(args) != 1 || args[0] != "org-1" {
				t.Fatalf("args = %#v", args)
			}
			return &fakeRows{rows: [][]any{
				{"row-2", "org-1", "WO-1002", "Beta", nil, nil, "456 Main", jobDateB.Time, nil, now, now, nil},
				{"row-1", "org-1", "WO-1001", "Acme", "ops@acme.test", "555-0100", "123 Main", jobDateA.Time, "scheduled", now, now, nil},
			}}, nil
		},
	})

	workOrders, err := repo.ListActiveByOrganizationID(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("ListActiveByOrganizationID: %v", err)
	}
	if len(workOrders) != 2 || workOrders[0].WorkOrderID != "WO-1002" {
		t.Fatalf("workOrders = %#v", workOrders)
	}
}

func TestRepositoryGetByWorkOrderIDMapsNotFound(t *testing.T) {
	repo := NewRepository(fakeQueryer{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if !strings.Contains(sql, "w.work_order_id = $2") {
				t.Fatalf("sql = %s", sql)
			}
			return fakeRow{err: pgx.ErrNoRows}
		},
	})

	_, err := repo.GetByWorkOrderID(context.Background(), "org-1", "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestScanWorkOrderHandlesNullableFields(t *testing.T) {
	now := time.Now().UTC()
	jobDate, _ := ParseDate("2025-03-15")
	workOrder, err := scanWorkOrder(fakeRow{
		values: []any{
			"row-1",
			"org-1",
			"WO-1001",
			"Acme Co",
			nil,
			"555-0100",
			"123 Main",
			jobDate.Time,
			nil,
			now,
			now,
			nil,
		},
	})
	if err != nil {
		t.Fatalf("scanWorkOrder: %v", err)
	}
	if workOrder.CustomerEmail != nil {
		t.Fatalf("customerEmail should be nil: %#v", workOrder.CustomerEmail)
	}
	if workOrder.CustomerPhone == nil || *workOrder.CustomerPhone != "555-0100" {
		t.Fatalf("customerPhone = %#v", workOrder.CustomerPhone)
	}
	if workOrder.Status != nil {
		t.Fatalf("status should be nil: %#v", workOrder.Status)
	}
	if workOrder.JobDate.String() != "2025-03-15" {
		t.Fatalf("jobDate = %s", workOrder.JobDate.String())
	}
}
