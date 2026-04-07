package workorders

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

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

func (r *Repository) Create(ctx context.Context, input CreateInput) (*WorkOrder, error) {
	organizationID := strings.TrimSpace(input.OrganizationID)
	workOrderID := strings.TrimSpace(input.WorkOrderID)
	customerName := strings.TrimSpace(input.CustomerName)
	customerAddress := strings.TrimSpace(input.CustomerAddress)
	if organizationID == "" || workOrderID == "" || customerName == "" || customerAddress == "" || input.JobDate.Time.IsZero() {
		return nil, ErrInvalidInput
	}

	row := r.db.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO work_orders (
				organization_id,
				work_order_id,
				customer_name,
				customer_email,
				customer_phone,
				customer_address,
				job_date,
				status
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING *
		)
		SELECT
			w.id,
			w.organization_id,
			w.work_order_id,
			w.customer_name,
			w.customer_email,
			w.customer_phone,
			w.customer_address,
			w.job_date,
			w.status,
			w.created_at,
			w.updated_at,
			w.deleted_at
		FROM inserted w
	`, organizationID, workOrderID, customerName, input.CustomerEmail, input.CustomerPhone, customerAddress, input.JobDate.Time, input.Status)

	workOrder, err := scanWorkOrder(row)
	if err != nil {
		return nil, translateWriteError(err)
	}

	return workOrder, nil
}

func (r *Repository) ListActiveByOrganizationID(ctx context.Context, organizationID string) ([]WorkOrder, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			w.id,
			w.organization_id,
			w.work_order_id,
			w.customer_name,
			w.customer_email,
			w.customer_phone,
			w.customer_address,
			w.job_date,
			w.status,
			w.created_at,
			w.updated_at,
			w.deleted_at
		FROM work_orders w
		WHERE w.organization_id = $1 AND w.deleted_at IS NULL
		ORDER BY w.job_date DESC, w.created_at DESC, w.id ASC
	`, strings.TrimSpace(organizationID))
	if err != nil {
		return nil, fmt.Errorf("list work orders: %w", err)
	}
	defer rows.Close()

	workOrders := make([]WorkOrder, 0)
	for rows.Next() {
		workOrder, err := scanWorkOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("scan work order: %w", err)
		}
		workOrders = append(workOrders, *workOrder)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list work orders rows: %w", err)
	}

	return workOrders, nil
}

func (r *Repository) GetByWorkOrderID(ctx context.Context, organizationID, workOrderID string) (*WorkOrder, error) {
	row := r.db.QueryRow(ctx, `
		SELECT
			w.id,
			w.organization_id,
			w.work_order_id,
			w.customer_name,
			w.customer_email,
			w.customer_phone,
			w.customer_address,
			w.job_date,
			w.status,
			w.created_at,
			w.updated_at,
			w.deleted_at
		FROM work_orders w
		WHERE w.organization_id = $1 AND w.work_order_id = $2 AND w.deleted_at IS NULL
	`, strings.TrimSpace(organizationID), strings.TrimSpace(workOrderID))

	workOrder, err := scanWorkOrder(row)
	if err != nil {
		return nil, translateRowError(err)
	}

	return workOrder, nil
}

func scanWorkOrder(scanner interface{ Scan(...any) error }) (*WorkOrder, error) {
	var workOrder WorkOrder
	var customerEmail sql.NullString
	var customerPhone sql.NullString
	var status sql.NullString
	var deletedAt sql.NullTime
	var jobDate time.Time

	err := scanner.Scan(
		&workOrder.ID,
		&workOrder.OrganizationID,
		&workOrder.WorkOrderID,
		&workOrder.CustomerName,
		&customerEmail,
		&customerPhone,
		&workOrder.CustomerAddress,
		&jobDate,
		&status,
		&workOrder.CreatedAt,
		&workOrder.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		return nil, err
	}

	workOrder.JobDate = Date{Time: jobDate.UTC()}
	if customerEmail.Valid {
		value := customerEmail.String
		workOrder.CustomerEmail = &value
	}
	if customerPhone.Valid {
		value := customerPhone.String
		workOrder.CustomerPhone = &value
	}
	if status.Valid {
		value := status.String
		workOrder.Status = &value
	}
	if deletedAt.Valid {
		value := deletedAt.Time
		workOrder.DeletedAt = &value
	}

	return &workOrder, nil
}

func translateRowError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func translateWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return err
}
