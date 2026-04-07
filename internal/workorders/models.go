package workorders

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const dateLayout = "2006-01-02"

var (
	ErrNotFound     = errors.New("work order not found")
	ErrConflict     = errors.New("work order already exists")
	ErrInvalidInput = errors.New("invalid work order input")
)

type Date struct {
	time.Time
}

func ParseDate(raw string) (Date, error) {
	parsed, err := time.Parse(dateLayout, strings.TrimSpace(raw))
	if err != nil {
		return Date{}, ErrInvalidInput
	}
	return Date{Time: parsed.UTC()}, nil
}

func (d Date) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Date) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	parsed, err := ParseDate(raw)
	if err != nil {
		return err
	}

	*d = parsed
	return nil
}

func (d Date) String() string {
	if d.Time.IsZero() {
		return ""
	}
	return d.Time.UTC().Format(dateLayout)
}

type WorkOrder struct {
	ID              string     `json:"id"`
	OrganizationID  string     `json:"organizationId"`
	WorkOrderID     string     `json:"workOrderId"`
	CustomerName    string     `json:"customerName"`
	CustomerEmail   *string    `json:"customerEmail,omitempty"`
	CustomerPhone   *string    `json:"customerPhone,omitempty"`
	CustomerAddress string     `json:"customerAddress"`
	JobDate         Date       `json:"jobDate"`
	Status          *string    `json:"status,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	DeletedAt       *time.Time `json:"deletedAt,omitempty"`
}

type CreateRequest struct {
	WorkOrderID     string `json:"workOrderId"`
	CustomerName    string `json:"customerName"`
	CustomerEmail   string `json:"customerEmail"`
	CustomerPhone   string `json:"customerPhone"`
	CustomerAddress string `json:"customerAddress"`
	JobDate         string `json:"jobDate"`
	Status          string `json:"status"`
}

type CreateInput struct {
	OrganizationID  string
	WorkOrderID     string
	CustomerName    string
	CustomerEmail   *string
	CustomerPhone   *string
	CustomerAddress string
	JobDate         Date
	Status          *string
}

func NormalizeCreateRequest(req CreateRequest) (CreateInput, error) {
	jobDate, err := ParseDate(req.JobDate)
	if err != nil {
		return CreateInput{}, err
	}

	input := CreateInput{
		WorkOrderID:     strings.TrimSpace(req.WorkOrderID),
		CustomerName:    strings.TrimSpace(req.CustomerName),
		CustomerEmail:   normalizeOptionalString(req.CustomerEmail),
		CustomerPhone:   normalizeOptionalString(req.CustomerPhone),
		CustomerAddress: strings.TrimSpace(req.CustomerAddress),
		JobDate:         jobDate,
		Status:          normalizeOptionalString(req.Status),
	}

	if input.WorkOrderID == "" || input.CustomerName == "" || input.CustomerAddress == "" || input.JobDate.Time.IsZero() {
		return CreateInput{}, ErrInvalidInput
	}

	return input, nil
}

func normalizeOptionalString(raw string) *string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
