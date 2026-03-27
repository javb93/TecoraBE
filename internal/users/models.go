package users

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"tecora/internal/organizations"
)

var (
	ErrNotFound             = errors.New("user not found")
	ErrConflict             = errors.New("clerk user already exists")
	ErrOrganizationNotFound = errors.New("organization not found")
	ErrInvalidInput         = errors.New("invalid user input")
)

type User struct {
	ID           string                     `json:"id"`
	ClerkUserID  string                     `json:"clerk_user_id"`
	Email        *string                    `json:"email"`
	FirstName    *string                    `json:"first_name"`
	LastName     *string                    `json:"last_name"`
	Organization organizations.Organization `json:"organization"`
	CreatedAt    time.Time                  `json:"created_at"`
	UpdatedAt    time.Time                  `json:"updated_at"`
	DeletedAt    *time.Time                 `json:"deleted_at,omitempty"`
}

type CreateInput struct {
	ClerkUserID    string
	Email          *string
	FirstName      *string
	LastName       *string
	OrganizationID string
}

type UpdateInput struct {
	Email          NullableString
	FirstName      NullableString
	LastName       NullableString
	OrganizationID NullableString
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

func (n NullableString) Ptr() *string {
	if !n.Set || n.Null {
		return nil
	}

	value := strings.TrimSpace(n.Value)
	return &value
}
