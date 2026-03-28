package customers

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"tecora/internal/organizations"
)

var (
	ErrNotFound             = errors.New("customer not found")
	ErrOrganizationNotFound = errors.New("organization not found")
	ErrInvalidInput         = errors.New("invalid customer input")
)

type Customer struct {
	ID           string                     `json:"id"`
	Organization organizations.Organization `json:"organization"`
	Name         string                     `json:"name"`
	Email        *string                    `json:"email"`
	Phone        *string                    `json:"phone"`
	Address      *string                    `json:"address"`
	Notes        *string                    `json:"notes"`
	CreatedAt    time.Time                  `json:"created_at"`
	UpdatedAt    time.Time                  `json:"updated_at"`
	DeletedAt    *time.Time                 `json:"deleted_at,omitempty"`
}

type CreateInput struct {
	OrganizationID string
	Name           string
	Email          *string
	Phone          *string
	Address        *string
	Notes          *string
}

type UpdateInput struct {
	Name    NullableString
	Email   NullableString
	Phone   NullableString
	Address NullableString
	Notes   NullableString
}

type NullableString struct {
	Set   bool
	Null  bool
	Value string
}

func (n *NullableString) UnmarshalJSON(data []byte) error {
	n.Set = true
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("null")) {
		n.Null = true
		n.Value = ""
		return nil
	}

	var raw string
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return err
	}

	n.Value = raw
	n.Null = false
	return nil
}

func (n NullableString) Present() bool {
	return n.Set
}

func (n NullableString) IsNull() bool {
	return n.Set && n.Null
}

func (n NullableString) Trimmed() string {
	return strings.TrimSpace(n.Value)
}

func ValidateName(name string) bool {
	return strings.TrimSpace(name) != ""
}
