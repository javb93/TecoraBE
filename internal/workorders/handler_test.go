package workorders

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"tecora/internal/auth/clerk"
	"tecora/internal/middleware"
	"tecora/internal/organizations"
	"tecora/internal/users"
)

type serviceStub struct {
	createFn func(ctx context.Context, input CreateInput) (*WorkOrder, error)
	listFn   func(ctx context.Context, organizationID string) ([]WorkOrder, error)
	getFn    func(ctx context.Context, organizationID, workOrderID string) (*WorkOrder, error)
}

func (s serviceStub) Create(ctx context.Context, input CreateInput) (*WorkOrder, error) {
	return s.createFn(ctx, input)
}

func (s serviceStub) ListActiveByOrganizationID(ctx context.Context, organizationID string) ([]WorkOrder, error) {
	return s.listFn(ctx, organizationID)
}

func (s serviceStub) GetByWorkOrderID(ctx context.Context, organizationID, workOrderID string) (*WorkOrder, error) {
	return s.getFn(ctx, organizationID, workOrderID)
}

type membershipStore struct {
	users map[string]users.User
}

func (s membershipStore) GetByClerkUserID(ctx context.Context, clerkUserID string) (*users.User, error) {
	user, ok := s.users[clerkUserID]
	if !ok {
		return nil, users.ErrNotFound
	}
	return &user, nil
}

func TestCreateReturnsCreatedWorkOrder(t *testing.T) {
	r := workOrderRouter(serviceStub{
		createFn: func(ctx context.Context, input CreateInput) (*WorkOrder, error) {
			if input.OrganizationID != "org-1" || input.WorkOrderID != "WO-1001" {
				t.Fatalf("input = %#v", input)
			}
			if input.CustomerEmail == nil || *input.CustomerEmail != "ops@acme.test" {
				t.Fatalf("customerEmail = %#v", input.CustomerEmail)
			}
			return testWorkOrder(), nil
		},
		listFn: func(ctx context.Context, organizationID string) ([]WorkOrder, error) {
			t.Fatal("unexpected ListActiveByOrganizationID")
			return nil, nil
		},
		getFn: func(ctx context.Context, organizationID, workOrderID string) (*WorkOrder, error) {
			t.Fatal("unexpected GetByWorkOrderID")
			return nil, nil
		},
	}, &clerk.Claims{Subject: "user-1", OrgSlug: "demo-alpha"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/work-orders", bytes.NewBufferString(validCreateJSON()))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}

	var body WorkOrder
	decodeJSON(t, rec, &body)
	if body.WorkOrderID != "WO-1001" || body.JobDate.String() != "2025-03-15" {
		t.Fatalf("body = %#v", body)
	}
}

func TestCreateRejectsInvalidJSON(t *testing.T) {
	r := workOrderRouter(serviceStub{
		createFn: func(ctx context.Context, input CreateInput) (*WorkOrder, error) {
			t.Fatal("unexpected Create")
			return nil, nil
		},
		listFn: func(ctx context.Context, organizationID string) ([]WorkOrder, error) { return nil, nil },
		getFn:  func(ctx context.Context, organizationID, workOrderID string) (*WorkOrder, error) { return nil, nil },
	}, &clerk.Claims{Subject: "user-1", OrgSlug: "demo-alpha"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/work-orders", bytes.NewBufferString(`{"workOrderId":`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateRejectsInvalidJobDate(t *testing.T) {
	r := workOrderRouter(serviceStub{
		createFn: func(ctx context.Context, input CreateInput) (*WorkOrder, error) {
			t.Fatal("unexpected Create")
			return nil, nil
		},
		listFn: func(ctx context.Context, organizationID string) ([]WorkOrder, error) { return nil, nil },
		getFn:  func(ctx context.Context, organizationID, workOrderID string) (*WorkOrder, error) { return nil, nil },
	}, &clerk.Claims{Subject: "user-1", OrgSlug: "demo-alpha"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/work-orders", bytes.NewBufferString(`{"workOrderId":"WO-1001","customerName":"Acme","customerAddress":"123 Main","jobDate":"03/15/2025"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateRejectsMissingRequiredFields(t *testing.T) {
	r := workOrderRouter(serviceStub{
		createFn: func(ctx context.Context, input CreateInput) (*WorkOrder, error) {
			t.Fatal("unexpected Create")
			return nil, nil
		},
		listFn: func(ctx context.Context, organizationID string) ([]WorkOrder, error) { return nil, nil },
		getFn:  func(ctx context.Context, organizationID, workOrderID string) (*WorkOrder, error) { return nil, nil },
	}, &clerk.Claims{Subject: "user-1", OrgSlug: "demo-alpha"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/work-orders", bytes.NewBufferString(`{"workOrderId":"","customerName":"Acme","customerAddress":"123 Main","jobDate":"2025-03-15"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateReturnsConflict(t *testing.T) {
	r := workOrderRouter(serviceStub{
		createFn: func(ctx context.Context, input CreateInput) (*WorkOrder, error) {
			return nil, ErrConflict
		},
		listFn: func(ctx context.Context, organizationID string) ([]WorkOrder, error) { return nil, nil },
		getFn:  func(ctx context.Context, organizationID, workOrderID string) (*WorkOrder, error) { return nil, nil },
	}, &clerk.Claims{Subject: "user-1", OrgSlug: "demo-alpha"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/work-orders", bytes.NewBufferString(validCreateJSON()))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateRejectsUnauthorized(t *testing.T) {
	r := workOrderRouterWithoutClaims(serviceStub{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/work-orders", bytes.NewBufferString(validCreateJSON()))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestListReturnsWorkOrders(t *testing.T) {
	r := workOrderRouter(serviceStub{
		createFn: func(ctx context.Context, input CreateInput) (*WorkOrder, error) { return nil, nil },
		listFn: func(ctx context.Context, organizationID string) ([]WorkOrder, error) {
			if organizationID != "org-1" {
				t.Fatalf("organizationID = %s", organizationID)
			}
			return []WorkOrder{*testWorkOrder()}, nil
		},
		getFn: func(ctx context.Context, organizationID, workOrderID string) (*WorkOrder, error) { return nil, nil },
	}, &clerk.Claims{Subject: "user-1", OrgSlug: "demo-alpha"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/work-orders", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var body struct {
		WorkOrders []WorkOrder `json:"workOrders"`
	}
	decodeJSON(t, rec, &body)
	if len(body.WorkOrders) != 1 || body.WorkOrders[0].WorkOrderID != "WO-1001" {
		t.Fatalf("body = %#v", body)
	}
}

func TestGetReturnsWorkOrder(t *testing.T) {
	r := workOrderRouter(serviceStub{
		createFn: func(ctx context.Context, input CreateInput) (*WorkOrder, error) { return nil, nil },
		listFn:   func(ctx context.Context, organizationID string) ([]WorkOrder, error) { return nil, nil },
		getFn: func(ctx context.Context, organizationID, workOrderID string) (*WorkOrder, error) {
			if organizationID != "org-1" || workOrderID != "WO-1001" {
				t.Fatalf("organizationID=%s workOrderID=%s", organizationID, workOrderID)
			}
			return testWorkOrder(), nil
		},
	}, &clerk.Claims{Subject: "user-1", OrgSlug: "demo-alpha"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/work-orders/WO-1001", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestGetReturnsNotFound(t *testing.T) {
	r := workOrderRouter(serviceStub{
		createFn: func(ctx context.Context, input CreateInput) (*WorkOrder, error) { return nil, nil },
		listFn:   func(ctx context.Context, organizationID string) ([]WorkOrder, error) { return nil, nil },
		getFn: func(ctx context.Context, organizationID, workOrderID string) (*WorkOrder, error) {
			return nil, ErrNotFound
		},
	}, &clerk.Claims{Subject: "user-1", OrgSlug: "demo-alpha"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/work-orders/missing", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestGetRejectsBlankID(t *testing.T) {
	r := workOrderRouter(serviceStub{
		createFn: func(ctx context.Context, input CreateInput) (*WorkOrder, error) { return nil, nil },
		listFn:   func(ctx context.Context, organizationID string) ([]WorkOrder, error) { return nil, nil },
		getFn: func(ctx context.Context, organizationID, workOrderID string) (*WorkOrder, error) {
			t.Fatal("unexpected GetByWorkOrderID")
			return nil, nil
		},
	}, &clerk.Claims{Subject: "user-1", OrgSlug: "demo-alpha"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/work-orders/%20%20", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func workOrderRouter(service Service, claims *clerk.Claims) *gin.Engine {
	gin.SetMode(gin.TestMode)

	org := organizations.Organization{
		ID:        "org-1",
		Slug:      "demo-alpha",
		Name:      "Demo Alpha",
		Active:    true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	userStore := membershipStore{
		users: map[string]users.User{
			"user-1": {
				ID:           "user-1",
				ClerkUserID:  "user-1",
				Organization: org,
				CreatedAt:    time.Now().UTC(),
				UpdatedAt:    time.Now().UTC(),
			},
		},
	}

	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set(string(middleware.ClaimsKey), claims)
		c.Next()
	})
	api.Use(users.RequireOrgAccess(userStore))
	RegisterRoutes(api, NewHandler(service))
	return r
}

func workOrderRouterWithoutClaims(service Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(users.RequireOrgAccess(membershipStore{}))
	RegisterRoutes(api, NewHandler(service))
	return r
}

func testWorkOrder() *WorkOrder {
	now := time.Now().UTC()
	jobDate, _ := ParseDate("2025-03-15")
	customerEmail := "ops@acme.test"
	customerPhone := "555-0100"
	status := "scheduled"

	return &WorkOrder{
		ID:              "wo-db-1",
		OrganizationID:  "org-1",
		WorkOrderID:     "WO-1001",
		CustomerName:    "Acme Co",
		CustomerEmail:   &customerEmail,
		CustomerPhone:   &customerPhone,
		CustomerAddress: "123 Main St, Austin, TX",
		JobDate:         jobDate,
		Status:          &status,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func validCreateJSON() string {
	return `{
		"workOrderId":" WO-1001 ",
		"customerName":" Acme Co ",
		"customerEmail":" ops@acme.test ",
		"customerPhone":" 555-0100 ",
		"customerAddress":" 123 Main St, Austin, TX ",
		"jobDate":"2025-03-15",
		"status":" scheduled "
	}`
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode json: %v", err)
	}
}
