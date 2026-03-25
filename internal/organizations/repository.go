package organizations

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

func (r *Repository) ListActive(ctx context.Context) ([]Organization, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, slug, name, created_at, updated_at, deleted_at
		FROM organizations
		WHERE deleted_at IS NULL
		ORDER BY name ASC, slug ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	defer rows.Close()

	orgs := make([]Organization, 0)
	for rows.Next() {
		var org Organization
		if err := rows.Scan(&org.ID, &org.Slug, &org.Name, &org.CreatedAt, &org.UpdatedAt, &org.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan organization: %w", err)
		}
		org.markActive()
		orgs = append(orgs, org)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list organizations rows: %w", err)
	}

	return orgs, nil
}

func (r *Repository) GetBySlug(ctx context.Context, slug string) (*Organization, error) {
	var org Organization
	err := r.db.QueryRow(ctx, `
		SELECT id, slug, name, created_at, updated_at, deleted_at
		FROM organizations
		WHERE slug = $1 AND deleted_at IS NULL
	`, slug).Scan(&org.ID, &org.Slug, &org.Name, &org.CreatedAt, &org.UpdatedAt, &org.DeletedAt)
	if err != nil {
		return nil, translateRowError(err)
	}

	org.markActive()
	return &org, nil
}

func (r *Repository) Create(ctx context.Context, slug, name string) (*Organization, error) {
	var org Organization
	err := r.db.QueryRow(ctx, `
		INSERT INTO organizations (slug, name)
		VALUES ($1, $2)
		RETURNING id, slug, name, created_at, updated_at, deleted_at
	`, slug, name).Scan(&org.ID, &org.Slug, &org.Name, &org.CreatedAt, &org.UpdatedAt, &org.DeletedAt)
	if err != nil {
		return nil, translateWriteError(err)
	}

	org.markActive()
	return &org, nil
}

func (r *Repository) Update(ctx context.Context, slug string, input UpdateInput) (*Organization, error) {
	var name any
	if input.Name != nil {
		name = *input.Name
	}

	var active any
	if input.Active != nil {
		active = *input.Active
	}

	var org Organization
	err := r.db.QueryRow(ctx, `
		UPDATE organizations
		SET name = COALESCE($2, name),
			deleted_at = CASE
				WHEN $3 IS NULL THEN deleted_at
				WHEN $3 THEN NULL
				ELSE NOW()
			END,
			updated_at = NOW()
		WHERE slug = $1
		RETURNING id, slug, name, created_at, updated_at, deleted_at
	`, slug, name, active).Scan(&org.ID, &org.Slug, &org.Name, &org.CreatedAt, &org.UpdatedAt, &org.DeletedAt)
	if err != nil {
		return nil, translateRowError(err)
	}

	org.markActive()
	return &org, nil
}

func (r *Repository) Delete(ctx context.Context, slug string) error {
	var id string
	err := r.db.QueryRow(ctx, `
		UPDATE organizations
		SET deleted_at = NOW(),
			updated_at = NOW()
		WHERE slug = $1 AND deleted_at IS NULL
		RETURNING id
	`, slug).Scan(&id)
	if err != nil {
		return translateRowError(err)
	}
	return nil
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
	if errors.As(err, &pgErr) && strings.TrimSpace(pgErr.Code) == "23505" {
		return ErrConflict
	}

	return err
}
