package organizations

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("organization not found")
	ErrConflict = errors.New("organization slug already exists")
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Organization struct {
	ID        string     `json:"id"`
	Slug      string     `json:"slug"`
	Name      string     `json:"name"`
	Active    bool       `json:"active"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

type UpdateInput struct {
	Name   *string
	Active *bool
}

func NormalizeSlug(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func ValidateSlug(slug string) bool {
	return slugPattern.MatchString(slug)
}

func ValidateName(name string) bool {
	return strings.TrimSpace(name) != ""
}

func (o *Organization) markActive() {
	o.Active = o.DeletedAt == nil
}
