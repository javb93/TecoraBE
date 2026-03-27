package users

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

func TestRepositoryListActiveFiltersDeletedRows(t *testing.T) {
	repo := NewRepository(fakeQueryer{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			if !strings.Contains(sql, "FROM users u") || !strings.Contains(sql, "JOIN organizations o") {
				t.Fatalf("sql = %s", sql)
			}
			now := time.Now().UTC()
			return &fakeRows{rows: [][]any{{"id-1", "user_1", "user@example.com", "Ada", "Lovelace", "org-1", "demo-alpha", "Demo Alpha", now, now, nil, now, now, nil}}}, nil
		},
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			t.Fatal("unexpected QueryRow call")
			return fakeRow{}
		},
	})

	users, err := repo.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("len = %d", len(users))
	}
	if !users[0].Organization.Active {
		t.Fatal("expected active organization")
	}
	if users[0].Email == nil || *users[0].Email != "user@example.com" {
		t.Fatalf("email = %#v", users[0].Email)
	}
}

func TestRepositoryGetByClerkUserIDMapsNotFound(t *testing.T) {
	repo := NewRepository(fakeQueryer{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if !strings.Contains(sql, "WHERE u.clerk_user_id = $1") {
				t.Fatalf("sql = %s", sql)
			}
			return fakeRow{err: pgx.ErrNoRows}
		},
	})

	_, err := repo.GetByClerkUserID(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestRepositoryCreateMapsUniqueViolation(t *testing.T) {
	repo := NewRepository(fakeQueryer{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if !strings.Contains(sql, "INSERT INTO users") {
				t.Fatalf("sql = %s", sql)
			}
			return fakeRow{err: &pgconn.PgError{Code: "23505"}}
		},
	})

	_, err := repo.Create(context.Background(), CreateInput{ClerkUserID: "user_1", OrganizationID: "org-1"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v", err)
	}
}

func TestRepositoryCreateMapsForeignKeyViolation(t *testing.T) {
	repo := NewRepository(fakeQueryer{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			return fakeRow{err: &pgconn.PgError{Code: "23503"}}
		},
	})

	_, err := repo.Create(context.Background(), CreateInput{ClerkUserID: "user_1", OrganizationID: "missing-org"})
	if !errors.Is(err, ErrOrganizationNotFound) {
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

func TestRepositoryUpdateRejectsNullOrganizationID(t *testing.T) {
	repo := NewRepository(fakeQueryer{})
	_, err := repo.Update(context.Background(), "user_1", UpdateInput{
		OrganizationID: NullableString{Set: true, Null: true},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v", err)
	}
}
