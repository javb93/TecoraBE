package organizations

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"tecora/internal/auth/clerk"
	"tecora/internal/middleware"
)

type memoryStore struct {
	mu   sync.Mutex
	orgs map[string]Organization
}

func newMemoryStore(orgs ...Organization) *memoryStore {
	out := &memoryStore{orgs: make(map[string]Organization, len(orgs))}
	for _, org := range orgs {
		out.orgs[org.Slug] = org
	}
	return out
}

func (s *memoryStore) ListActive(ctx context.Context) ([]Organization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	orgs := make([]Organization, 0, len(s.orgs))
	for _, org := range s.orgs {
		if org.Active && org.DeletedAt == nil {
			orgs = append(orgs, org)
		}
	}

	sort.Slice(orgs, func(i, j int) bool {
		if orgs[i].Name == orgs[j].Name {
			return orgs[i].Slug < orgs[j].Slug
		}
		return orgs[i].Name < orgs[j].Name
	})

	return orgs, nil
}

func (s *memoryStore) GetBySlug(ctx context.Context, slug string) (*Organization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	org, ok := s.orgs[slug]
	if !ok || !org.Active || org.DeletedAt != nil {
		return nil, ErrNotFound
	}

	copy := org
	return &copy, nil
}

func (s *memoryStore) Create(ctx context.Context, slug, name string) (*Organization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.orgs[slug]; ok {
		return nil, ErrConflict
	}

	now := time.Now().UTC()
	org := Organization{
		ID:        "org_" + slug,
		Slug:      slug,
		Name:      name,
		Active:    true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.orgs[slug] = org
	copy := org
	return &copy, nil
}

func (s *memoryStore) Update(ctx context.Context, slug string, input UpdateInput) (*Organization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	org, ok := s.orgs[slug]
	if !ok {
		return nil, ErrNotFound
	}

	if input.Name != nil {
		org.Name = *input.Name
	}
	if input.Active != nil {
		org.Active = *input.Active
		if *input.Active {
			org.DeletedAt = nil
		} else {
			now := time.Now().UTC()
			org.DeletedAt = &now
		}
	} else {
		org.Active = org.DeletedAt == nil
	}
	org.UpdatedAt = time.Now().UTC()
	s.orgs[slug] = org
	copy := org
	return &copy, nil
}

func (s *memoryStore) Delete(ctx context.Context, slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	org, ok := s.orgs[slug]
	if !ok || !org.Active || org.DeletedAt != nil {
		return ErrNotFound
	}
	now := time.Now().UTC()
	org.Active = false
	org.DeletedAt = &now
	org.UpdatedAt = now
	s.orgs[slug] = org
	return nil
}

func adminRouter(store Store) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(middleware.ClaimsKey), &clerk.Claims{Subject: "admin_1", OrgSlug: "demo-alpha"})
		c.Next()
	})

	admin := r.Group("/api/v1/admin")
	admin.Use(middleware.ClerkAdminAllowlist([]string{"admin_1"}))
	RegisterAdminRoutes(admin, NewHandler(store))
	return r
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder, target *T) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(target); err != nil {
		t.Fatalf("decode json: %v", err)
	}
}

func TestAdminOrganizationCRUD(t *testing.T) {
	store := newMemoryStore(Organization{
		ID:        "org_demo_alpha",
		Slug:      "demo-alpha",
		Name:      "Demo Alpha",
		Active:    true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	r := adminRouter(store)

	t.Run("list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/organizations", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var body struct {
			Organizations []Organization `json:"organizations"`
		}
		decodeJSON(t, rec, &body)
		if len(body.Organizations) != 1 {
			t.Fatalf("len = %d", len(body.Organizations))
		}
	})

	t.Run("create", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/organizations", strings.NewReader(`{"slug":"demo-beta","name":"Demo Beta"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d", rec.Code)
		}
		var org Organization
		decodeJSON(t, rec, &org)
		if org.Slug != "demo-beta" || !org.Active {
			t.Fatalf("org = %#v", org)
		}
	})

	t.Run("get", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/organizations/demo-alpha", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var org Organization
		decodeJSON(t, rec, &org)
		if org.Slug != "demo-alpha" {
			t.Fatalf("org = %#v", org)
		}
	})

	t.Run("update", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/organizations/demo-alpha", strings.NewReader(`{"name":"Demo Alpha Renamed","active":false}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var org Organization
		decodeJSON(t, rec, &org)
		if org.Active {
			t.Fatalf("org should be inactive: %#v", org)
		}
	})

	t.Run("delete", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/organizations/demo-beta", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("list excludes deleted", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/organizations", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var body struct {
			Organizations []Organization `json:"organizations"`
		}
		decodeJSON(t, rec, &body)
		for _, org := range body.Organizations {
			if org.Slug == "demo-beta" {
				t.Fatalf("deleted org leaked into list: %#v", body.Organizations)
			}
		}
	})
}

func TestAdminOrganizationDuplicateSlugReturnsConflict(t *testing.T) {
	store := newMemoryStore(Organization{
		ID:        "org_demo_alpha",
		Slug:      "demo-alpha",
		Name:      "Demo Alpha",
		Active:    true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	r := adminRouter(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/organizations", strings.NewReader(`{"slug":"demo-alpha","name":"Duplicate"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAdminNonAdminDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(middleware.ClaimsKey), &clerk.Claims{Subject: "user_123"})
		c.Next()
	})
	admin := r.Group("/api/v1/admin")
	admin.Use(middleware.ClerkAdminAllowlist([]string{"admin_1"}))
	RegisterAdminRoutes(admin, NewHandler(newMemoryStore()))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/organizations", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAdminMissingAllowlistIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(middleware.ClaimsKey), &clerk.Claims{Subject: "admin_1"})
		c.Next()
	})
	admin := r.Group("/api/v1/admin")
	admin.Use(middleware.ClerkAdminAllowlist(nil))
	RegisterAdminRoutes(admin, NewHandler(newMemoryStore()))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/organizations", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestUpdateRequiresFields(t *testing.T) {
	store := newMemoryStore(Organization{
		ID:        "org_demo_alpha",
		Slug:      "demo-alpha",
		Name:      "Demo Alpha",
		Active:    true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	r := adminRouter(store)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/organizations/demo-alpha", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestGetMissingOrganizationReturnsNotFound(t *testing.T) {
	r := adminRouter(newMemoryStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/organizations/missing", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestHandlerRejectsInvalidSlug(t *testing.T) {
	r := adminRouter(newMemoryStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/organizations/bad_slug", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestDeleteMissingOrganizationReturnsNotFound(t *testing.T) {
	r := adminRouter(newMemoryStore())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/organizations/missing", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateRejectsDuplicateBodyValidation(t *testing.T) {
	r := adminRouter(newMemoryStore())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/organizations", strings.NewReader(`{"slug":"bad slug","name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestDeleteThenGetReturnsNotFound(t *testing.T) {
	store := newMemoryStore(Organization{
		ID:        "org_demo_alpha",
		Slug:      "demo-alpha",
		Name:      "Demo Alpha",
		Active:    true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	r := adminRouter(store)

	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/organizations/demo-alpha", nil)
	delRec := httptest.NewRecorder()
	r.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", delRec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/organizations/demo-alpha", nil)
	getRec := httptest.NewRecorder()
	r.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("get status = %d", getRec.Code)
	}
}

func TestUpdateCanRestoreDeletedOrganization(t *testing.T) {
	now := time.Now().UTC()
	store := newMemoryStore(Organization{
		ID:        "org_demo_alpha",
		Slug:      "demo-alpha",
		Name:      "Demo Alpha",
		Active:    false,
		CreatedAt: now,
		UpdatedAt: now,
		DeletedAt: &now,
	})
	r := adminRouter(store)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/organizations/demo-alpha", strings.NewReader(`{"active":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var org Organization
	decodeJSON(t, rec, &org)
	if !org.Active {
		t.Fatalf("expected restored organization")
	}
}

func TestDeleteUsesSoftDeleteSemantics(t *testing.T) {
	now := time.Now().UTC()
	store := newMemoryStore(Organization{
		ID:        "org_demo_alpha",
		Slug:      "demo-alpha",
		Name:      "Demo Alpha",
		Active:    true,
		CreatedAt: now,
		UpdatedAt: now,
	})
	r := adminRouter(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/organizations/demo-alpha", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}

	org, err := store.GetBySlug(context.Background(), "demo-alpha")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected deleted org to be hidden, got org=%#v err=%v", org, err)
	}
}
