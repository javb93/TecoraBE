package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tecora/internal/organizations"
)

type Queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Repository struct {
	db Queryer
}

func NewRepository(db Queryer) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListActive(ctx context.Context) ([]User, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			u.id, u.clerk_user_id, u.email, u.first_name, u.last_name,
			o.id, o.slug, o.name, o.created_at, o.updated_at, o.deleted_at,
			u.created_at, u.updated_at, u.deleted_at
		FROM users u
		JOIN organizations o ON o.id = u.organization_id
		WHERE u.deleted_at IS NULL
		ORDER BY u.clerk_user_id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := make([]User, 0)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, *user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list users rows: %w", err)
	}

	return users, nil
}

func (r *Repository) GetByClerkUserID(ctx context.Context, clerkUserID string) (*User, error) {
	row := r.db.QueryRow(ctx, `
		SELECT
			u.id, u.clerk_user_id, u.email, u.first_name, u.last_name,
			o.id, o.slug, o.name, o.created_at, o.updated_at, o.deleted_at,
			u.created_at, u.updated_at, u.deleted_at
		FROM users u
		JOIN organizations o ON o.id = u.organization_id
		WHERE u.clerk_user_id = $1 AND u.deleted_at IS NULL
	`, clerkUserID)

	user, err := scanUser(row)
	if err != nil {
		return nil, translateRowError(err)
	}

	return user, nil
}

func (r *Repository) Create(ctx context.Context, input CreateInput) (*User, error) {
	row := r.db.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO users (clerk_user_id, email, first_name, last_name, organization_id)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING *
		)
		SELECT
			u.id, u.clerk_user_id, u.email, u.first_name, u.last_name,
			o.id, o.slug, o.name, o.created_at, o.updated_at, o.deleted_at,
			u.created_at, u.updated_at, u.deleted_at
		FROM inserted u
		JOIN organizations o ON o.id = u.organization_id
	`, input.ClerkUserID, input.Email, input.FirstName, input.LastName, input.OrganizationID)

	user, err := scanUser(row)
	if err != nil {
		return nil, translateWriteError(err)
	}

	return user, nil
}

func (r *Repository) Update(ctx context.Context, clerkUserID string, input UpdateInput) (*User, error) {
	clauses := make([]string, 0, 4)
	args := []any{clerkUserID}
	argPos := 2

	addClause := func(column string, value any) {
		clauses = append(clauses, fmt.Sprintf("%s = $%d", column, argPos))
		args = append(args, value)
		argPos++
	}

	if input.Email.Set {
		if input.Email.Null {
			addClause("email", nil)
		} else {
			addClause("email", input.Email.Trimmed())
		}
	}
	if input.FirstName.Set {
		if input.FirstName.Null {
			addClause("first_name", nil)
		} else {
			addClause("first_name", input.FirstName.Trimmed())
		}
	}
	if input.LastName.Set {
		if input.LastName.Null {
			addClause("last_name", nil)
		} else {
			addClause("last_name", input.LastName.Trimmed())
		}
	}
	if input.OrganizationID.Set {
		if input.OrganizationID.Null {
			return nil, ErrInvalidInput
		}
		addClause("organization_id", input.OrganizationID.Trimmed())
	}

	if len(clauses) == 0 {
		return nil, ErrInvalidInput
	}

	clauses = append(clauses, "updated_at = NOW()")

	query := fmt.Sprintf(`
		WITH updated AS (
			UPDATE users
			SET %s
			WHERE clerk_user_id = $1 AND deleted_at IS NULL
			RETURNING *
		)
		SELECT
			u.id, u.clerk_user_id, u.email, u.first_name, u.last_name,
			o.id, o.slug, o.name, o.created_at, o.updated_at, o.deleted_at,
			u.created_at, u.updated_at, u.deleted_at
		FROM updated u
		JOIN organizations o ON o.id = u.organization_id
	`, strings.Join(clauses, ", "))

	user, err := scanUser(r.db.QueryRow(ctx, query, args...))
	if err != nil {
		return nil, translateRowError(err)
	}

	return user, nil
}

func (r *Repository) Delete(ctx context.Context, clerkUserID string) error {
	var id string
	err := r.db.QueryRow(ctx, `
		UPDATE users
		SET deleted_at = NOW(),
			updated_at = NOW()
		WHERE clerk_user_id = $1 AND deleted_at IS NULL
		RETURNING id
	`, clerkUserID).Scan(&id)
	if err != nil {
		return translateRowError(err)
	}
	return nil
}

func scanUser(scanner interface{ Scan(...any) error }) (*User, error) {
	var user User
	var email sql.NullString
	var firstName sql.NullString
	var lastName sql.NullString
	var org organizations.Organization
	var orgDeletedAt sql.NullTime
	var userDeletedAt sql.NullTime

	err := scanner.Scan(
		&user.ID,
		&user.ClerkUserID,
		&email,
		&firstName,
		&lastName,
		&org.ID,
		&org.Slug,
		&org.Name,
		&org.CreatedAt,
		&org.UpdatedAt,
		&orgDeletedAt,
		&user.CreatedAt,
		&user.UpdatedAt,
		&userDeletedAt,
	)
	if err != nil {
		return nil, err
	}

	if email.Valid {
		value := email.String
		user.Email = &value
	}
	if firstName.Valid {
		value := firstName.String
		user.FirstName = &value
	}
	if lastName.Valid {
		value := lastName.String
		user.LastName = &value
	}
	if userDeletedAt.Valid {
		value := userDeletedAt.Time
		user.DeletedAt = &value
	}
	if orgDeletedAt.Valid {
		value := orgDeletedAt.Time
		org.DeletedAt = &value
	}
	org.Active = org.DeletedAt == nil
	user.Organization = org

	return &user, nil
}

func translateRowError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func translateWriteError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch strings.TrimSpace(pgErr.Code) {
		case "23505":
			return ErrConflict
		case "23503":
			return ErrOrganizationNotFound
		}
	}

	return err
}
