package organizations

import (
	"context"
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
	case **time.Time:
		if value == nil {
			*d = nil
			return nil
		}
		t := value.(time.Time)
		*d = &t
		return nil
	default:
		return errors.New("unsupported scan destination")
	}
}

func TestRepositoryListActiveFiltersDeletedRows(t *testing.T) {
	repo := NewRepository(fakeQueryer{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			if !strings.Contains(sql, "WHERE deleted_at IS NULL") {
				t.Fatalf("sql = %s", sql)
			}
			now := time.Now().UTC()
			return &fakeRows{rows: [][]any{{"id-1", "demo-alpha", "Demo Alpha", now, now, nil}}}, nil
		},
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			t.Fatal("unexpected QueryRow call")
			return fakeRow{}
		},
	})

	orgs, err := repo.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(orgs) != 1 {
		t.Fatalf("len = %d", len(orgs))
	}
	if !orgs[0].Active {
		t.Fatal("expected active organization")
	}
}

func TestRepositoryCreateMapsUniqueViolation(t *testing.T) {
	repo := NewRepository(fakeQueryer{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if !strings.Contains(sql, "INSERT INTO organizations") {
				t.Fatalf("sql = %s", sql)
			}
			return fakeRow{err: &pgconn.PgError{Code: "23505"}}
		},
	})

	_, err := repo.Create(context.Background(), "demo-alpha", "Demo Alpha")
	if !errors.Is(err, ErrConflict) {
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

	if err := repo.Delete(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}
