package customers

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

func (r *Repository) ListActiveByOrganizationID(ctx context.Context, organizationID string) ([]Customer, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			c.id, c.name, c.email, c.phone, c.address, c.notes,
			o.id, o.slug, o.name, o.created_at, o.updated_at, o.deleted_at,
			c.created_at, c.updated_at, c.deleted_at
		FROM customers c
		JOIN organizations o ON o.id = c.organization_id
		WHERE c.organization_id = $1 AND c.deleted_at IS NULL
		ORDER BY c.name ASC, c.id ASC
	`, strings.TrimSpace(organizationID))
	if err != nil {
		return nil, fmt.Errorf("list customers: %w", err)
	}
	defer rows.Close()

	customers := make([]Customer, 0)
	for rows.Next() {
		customer, err := scanCustomer(rows)
		if err != nil {
			return nil, fmt.Errorf("scan customer: %w", err)
		}
		customers = append(customers, *customer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list customers rows: %w", err)
	}

	return customers, nil
}

func (r *Repository) GetByID(ctx context.Context, organizationID, customerID string) (*Customer, error) {
	row := r.db.QueryRow(ctx, `
		SELECT
			c.id, c.name, c.email, c.phone, c.address, c.notes,
			o.id, o.slug, o.name, o.created_at, o.updated_at, o.deleted_at,
			c.created_at, c.updated_at, c.deleted_at
		FROM customers c
		JOIN organizations o ON o.id = c.organization_id
		WHERE c.organization_id = $1 AND c.id = $2 AND c.deleted_at IS NULL
	`, strings.TrimSpace(organizationID), strings.TrimSpace(customerID))

	customer, err := scanCustomer(row)
	if err != nil {
		return nil, translateRowError(err)
	}

	return customer, nil
}

func (r *Repository) Create(ctx context.Context, input CreateInput) (*Customer, error) {
	name := strings.TrimSpace(input.Name)
	organizationID := strings.TrimSpace(input.OrganizationID)
	if organizationID == "" || !ValidateName(name) {
		return nil, ErrInvalidInput
	}

	row := r.db.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO customers (organization_id, name, email, phone, address, notes)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING *
		)
		SELECT
			c.id, c.name, c.email, c.phone, c.address, c.notes,
			o.id, o.slug, o.name, o.created_at, o.updated_at, o.deleted_at,
			c.created_at, c.updated_at, c.deleted_at
		FROM inserted c
		JOIN organizations o ON o.id = c.organization_id
	`, organizationID, name, input.Email, input.Phone, input.Address, input.Notes)

	customer, err := scanCustomer(row)
	if err != nil {
		return nil, translateWriteError(err)
	}

	return customer, nil
}

func (r *Repository) Update(ctx context.Context, organizationID, customerID string, input UpdateInput) (*Customer, error) {
	organizationID = strings.TrimSpace(organizationID)
	customerID = strings.TrimSpace(customerID)
	if organizationID == "" || customerID == "" {
		return nil, ErrInvalidInput
	}

	clauses := make([]string, 0, 6)
	args := []any{organizationID, customerID}
	argPos := 3

	addClause := func(column string, value any) {
		clauses = append(clauses, fmt.Sprintf("%s = $%d", column, argPos))
		args = append(args, value)
		argPos++
	}

	if input.Name.Set {
		if input.Name.Null || !ValidateName(input.Name.Trimmed()) {
			return nil, ErrInvalidInput
		}
		addClause("name", input.Name.Trimmed())
	}
	if input.Email.Set {
		if input.Email.Null {
			addClause("email", nil)
		} else {
			addClause("email", input.Email.Trimmed())
		}
	}
	if input.Phone.Set {
		if input.Phone.Null {
			addClause("phone", nil)
		} else {
			addClause("phone", input.Phone.Trimmed())
		}
	}
	if input.Address.Set {
		if input.Address.Null {
			addClause("address", nil)
		} else {
			addClause("address", input.Address.Trimmed())
		}
	}
	if input.Notes.Set {
		if input.Notes.Null {
			addClause("notes", nil)
		} else {
			addClause("notes", input.Notes.Trimmed())
		}
	}

	if len(clauses) == 0 {
		return nil, ErrInvalidInput
	}

	clauses = append(clauses, "updated_at = NOW()")

	query := fmt.Sprintf(`
		WITH updated AS (
			UPDATE customers
			SET %s
			WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL
			RETURNING *
		)
		SELECT
			c.id, c.name, c.email, c.phone, c.address, c.notes,
			o.id, o.slug, o.name, o.created_at, o.updated_at, o.deleted_at,
			c.created_at, c.updated_at, c.deleted_at
		FROM updated c
		JOIN organizations o ON o.id = c.organization_id
	`, strings.Join(clauses, ", "))

	customer, err := scanCustomer(r.db.QueryRow(ctx, query, args...))
	if err != nil {
		return nil, translateRowError(err)
	}

	return customer, nil
}

func (r *Repository) Delete(ctx context.Context, organizationID, customerID string) error {
	var id string
	err := r.db.QueryRow(ctx, `
		UPDATE customers
		SET deleted_at = NOW(),
			updated_at = NOW()
		WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL
		RETURNING id
	`, strings.TrimSpace(organizationID), strings.TrimSpace(customerID)).Scan(&id)
	if err != nil {
		return translateRowError(err)
	}
	return nil
}

func scanCustomer(scanner interface{ Scan(...any) error }) (*Customer, error) {
	var customer Customer
	var email sql.NullString
	var phone sql.NullString
	var address sql.NullString
	var notes sql.NullString
	var org organizations.Organization
	var orgDeletedAt sql.NullTime
	var customerDeletedAt sql.NullTime

	err := scanner.Scan(
		&customer.ID,
		&customer.Name,
		&email,
		&phone,
		&address,
		&notes,
		&org.ID,
		&org.Slug,
		&org.Name,
		&org.CreatedAt,
		&org.UpdatedAt,
		&orgDeletedAt,
		&customer.CreatedAt,
		&customer.UpdatedAt,
		&customerDeletedAt,
	)
	if err != nil {
		return nil, err
	}

	if email.Valid {
		value := email.String
		customer.Email = &value
	}
	if phone.Valid {
		value := phone.String
		customer.Phone = &value
	}
	if address.Valid {
		value := address.String
		customer.Address = &value
	}
	if notes.Valid {
		value := notes.String
		customer.Notes = &value
	}
	if customerDeletedAt.Valid {
		value := customerDeletedAt.Time
		customer.DeletedAt = &value
	}
	if orgDeletedAt.Valid {
		value := orgDeletedAt.Time
		org.DeletedAt = &value
	}
	org.Active = org.DeletedAt == nil
	customer.Organization = org

	return &customer, nil
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
	if errors.As(err, &pgErr) && strings.TrimSpace(pgErr.Code) == "23503" {
		return ErrOrganizationNotFound
	}

	return err
}
