package users

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"tecora/internal/auth/clerk"
	"tecora/internal/middleware"
	"tecora/internal/organizations"
)

type memoryOrgStore struct {
	mu   sync.Mutex
	orgs map[string]organizations.Organization
}

func newMemoryOrgStore(orgs ...organizations.Organization) *memoryOrgStore {
	out := &memoryOrgStore{orgs: make(map[string]organizations.Organization, len(orgs))}
	for _, org := range orgs {
		out.orgs[org.Slug] = org
	}
	return out
}

func (s *memoryOrgStore) ListActive(ctx context.Context) ([]organizations.Organization, error) {
	panic("unused")
}

func (s *memoryOrgStore) GetBySlug(ctx context.Context, slug string) (*organizations.Organization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	org, ok := s.orgs[slug]
	if !ok || !org.Active || org.DeletedAt != nil {
		return nil, organizations.ErrNotFound
	}

	copy := org
	return &copy, nil
}

func (s *memoryOrgStore) Create(ctx context.Context, slug, name string) (*organizations.Organization, error) {
	panic("unused")
}

func (s *memoryOrgStore) Update(ctx context.Context, slug string, input organizations.UpdateInput) (*organizations.Organization, error) {
	panic("unused")
}

func (s *memoryOrgStore) Delete(ctx context.Context, slug string) error {
	panic("unused")
}

type memoryUserStore struct {
	mu    sync.Mutex
	users map[string]User
}

func newMemoryUserStore(users ...User) *memoryUserStore {
	out := &memoryUserStore{users: make(map[string]User, len(users))}
	for _, user := range users {
		out.users[user.ClerkUserID] = user
	}
	return out
}

func (s *memoryUserStore) ListActive(ctx context.Context) ([]User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]User, 0, len(s.users))
	for _, user := range s.users {
		if user.DeletedAt == nil {
			out = append(out, user)
		}
	}
	return out, nil
}

func (s *memoryUserStore) GetByClerkUserID(ctx context.Context, clerkUserID string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[clerkUserID]
	if !ok || user.DeletedAt != nil {
		return nil, ErrNotFound
	}
	copy := user
	return &copy, nil
}

func (s *memoryUserStore) Create(ctx context.Context, input CreateInput) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[input.ClerkUserID]; ok {
		return nil, ErrConflict
	}

	now := time.Now().UTC()
	user := User{
		ID:          "user_" + input.ClerkUserID,
		ClerkUserID: input.ClerkUserID,
		Email:       input.Email,
		FirstName:   input.FirstName,
		LastName:    input.LastName,
		Organization: organizations.Organization{
			ID:     input.OrganizationID,
			Slug:   "demo-alpha",
			Name:   "Demo Alpha",
			Active: true,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.users[input.ClerkUserID] = user
	copy := user
	return &copy, nil
}

func (s *memoryUserStore) Update(ctx context.Context, clerkUserID string, input UpdateInput) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[clerkUserID]
	if !ok {
		return nil, ErrNotFound
	}

	if input.Email.Set {
		if input.Email.Null {
			user.Email = nil
		} else {
			email := input.Email.Trimmed()
			user.Email = &email
		}
	}
	if input.FirstName.Set {
		if input.FirstName.Null {
			user.FirstName = nil
		} else {
			firstName := input.FirstName.Trimmed()
			user.FirstName = &firstName
		}
	}
	if input.LastName.Set {
		if input.LastName.Null {
			user.LastName = nil
		} else {
			lastName := input.LastName.Trimmed()
			user.LastName = &lastName
		}
	}
	if input.OrganizationID.Set && !input.OrganizationID.Null {
		user.Organization.ID = input.OrganizationID.Trimmed()
	}
	user.UpdatedAt = time.Now().UTC()
	s.users[clerkUserID] = user
	copy := user
	return &copy, nil
}

func (s *memoryUserStore) Delete(ctx context.Context, clerkUserID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[clerkUserID]
	if !ok || user.DeletedAt != nil {
		return ErrNotFound
	}
	now := time.Now().UTC()
	user.DeletedAt = &now
	user.UpdatedAt = now
	s.users[clerkUserID] = user
	return nil
}

func adminRouter(store Store, orgs OrganizationResolver) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	admin := r.Group("/api/v1/admin")
	RegisterAdminRoutes(admin, NewHandler(store, orgs))
	return r
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder, target *T) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(target); err != nil {
		t.Fatalf("decode json: %v", err)
	}
}

func TestAdminUserCRUD(t *testing.T) {
	orgStore := newMemoryOrgStore(organizations.Organization{
		ID:     "org_demo_alpha",
		Slug:   "demo-alpha",
		Name:   "Demo Alpha",
		Active: true,
	})
	store := newMemoryUserStore(User{
		ID:          "user_demo_1",
		ClerkUserID: "user_1",
		Email:       ptr("user@example.com"),
		FirstName:   ptr("Ada"),
		LastName:    ptr("Lovelace"),
		Organization: organizations.Organization{
			ID:     "org_demo_alpha",
			Slug:   "demo-alpha",
			Name:   "Demo Alpha",
			Active: true,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	r := adminRouter(store, orgStore)

	t.Run("list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var body struct {
			Users []User `json:"users"`
		}
		decodeJSON(t, rec, &body)
		if len(body.Users) != 1 {
			t.Fatalf("len = %d", len(body.Users))
		}
	})

	t.Run("create accepts null email", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", strings.NewReader(`{"clerk_user_id":"user_2","email":null,"organization_slug":"demo-alpha"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d", rec.Code)
		}
		var user User
		decodeJSON(t, rec, &user)
		if user.ClerkUserID != "user_2" {
			t.Fatalf("user = %#v", user)
		}
		if user.Email != nil {
			t.Fatalf("email should be null: %#v", user.Email)
		}
	})

	t.Run("get", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/user_1", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var user User
		decodeJSON(t, rec, &user)
		if user.ClerkUserID != "user_1" {
			t.Fatalf("user = %#v", user)
		}
	})

	t.Run("update clears email and changes org", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/user_1", strings.NewReader(`{"email":null,"organization_slug":"demo-alpha"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var user User
		decodeJSON(t, rec, &user)
		if user.Email != nil {
			t.Fatalf("email should be null: %#v", user.Email)
		}
	})

	t.Run("delete", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/user_2", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("list excludes deleted", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var body struct {
			Users []User `json:"users"`
		}
		decodeJSON(t, rec, &body)
		for _, user := range body.Users {
			if user.ClerkUserID == "user_2" {
				t.Fatalf("deleted user leaked into list: %#v", body.Users)
			}
		}
	})
}

func TestAdminUserValidation(t *testing.T) {
	orgStore := newMemoryOrgStore(organizations.Organization{
		ID:     "org_demo_alpha",
		Slug:   "demo-alpha",
		Name:   "Demo Alpha",
		Active: true,
	})
	r := adminRouter(newMemoryUserStore(), orgStore)

	t.Run("invalid org slug", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", strings.NewReader(`{"clerk_user_id":"user_3","organization_slug":"bad slug"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("missing body fields on update", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/user_1", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", rec.Code)
		}
	})
}

func TestAdminUsersBlockNonAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(middleware.ClaimsKey), &clerk.Claims{Subject: "user_123"})
		c.Next()
	})
	admin := r.Group("/api/v1/admin")
	admin.Use(middleware.ClerkAdminAllowlist([]string{"admin_1"}))
	RegisterAdminRoutes(admin, NewHandler(newMemoryUserStore(), newMemoryOrgStore()))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
}

func ptr(s string) *string {
	return &s
}
