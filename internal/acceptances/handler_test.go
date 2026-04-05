package acceptances

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	submitFn func(ctx context.Context, organizationID string, submission Submission) (*Record, error)
	statusFn func(ctx context.Context, organizationID, acceptanceID string) (*Record, error)
	getPDFFn func(ctx context.Context, organizationID, acceptanceID string) (*PDFDocument, error)
}

func (s serviceStub) Submit(ctx context.Context, organizationID string, submission Submission) (*Record, error) {
	return s.submitFn(ctx, organizationID, submission)
}

func (s serviceStub) GetStatus(ctx context.Context, organizationID, acceptanceID string) (*Record, error) {
	return s.statusFn(ctx, organizationID, acceptanceID)
}

func (s serviceStub) GetPDF(ctx context.Context, organizationID, acceptanceID string) (*PDFDocument, error) {
	return s.getPDFFn(ctx, organizationID, acceptanceID)
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

func TestSubmitReturnsAccepted(t *testing.T) {
	r := acceptanceRouter(serviceStub{
		submitFn: func(ctx context.Context, organizationID string, submission Submission) (*Record, error) {
			if organizationID != "org-1" || submission.WorkOrderID != "wo-1" {
				t.Fatalf("organizationID=%s submission=%#v", organizationID, submission)
			}
			return acceptedRecord("acc-1", PDFStatusGenerated), nil
		},
		statusFn: func(ctx context.Context, organizationID, acceptanceID string) (*Record, error) {
			t.Fatal("unexpected GetStatus")
			return nil, nil
		},
		getPDFFn: func(ctx context.Context, organizationID, acceptanceID string) (*PDFDocument, error) {
			t.Fatal("unexpected GetPDF")
			return nil, nil
		},
	}, &clerk.Claims{Subject: "user-1", OrgSlug: "demo-alpha"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/work-orders/wo-1/acceptance", bytes.NewBufferString(validSubmissionJSON("wo-1")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}

	var body AcceptedResponse
	decodeJSON(t, rec, &body)
	if body.AcceptanceID != "acc-1" || body.Status != "accepted" {
		t.Fatalf("body = %#v", body)
	}
}

func TestSubmitDuplicateReusesAcceptance(t *testing.T) {
	record := acceptedRecord("acc-1", PDFStatusGenerated)
	r := acceptanceRouter(serviceStub{
		submitFn: func(ctx context.Context, organizationID string, submission Submission) (*Record, error) {
			return record, nil
		},
		statusFn: func(ctx context.Context, organizationID, acceptanceID string) (*Record, error) {
			t.Fatal("unexpected GetStatus")
			return nil, nil
		},
		getPDFFn: func(ctx context.Context, organizationID, acceptanceID string) (*PDFDocument, error) {
			t.Fatal("unexpected GetPDF")
			return nil, nil
		},
	}, &clerk.Claims{Subject: "user-1", OrgSlug: "demo-alpha"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/work-orders/wo-1/acceptance", bytes.NewBufferString(validSubmissionJSON("wo-1")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestSubmitRejectsInvalidPayload(t *testing.T) {
	r := acceptanceRouter(serviceStub{
		submitFn: func(ctx context.Context, organizationID string, submission Submission) (*Record, error) {
			t.Fatal("unexpected Submit")
			return nil, nil
		},
		statusFn: func(ctx context.Context, organizationID, acceptanceID string) (*Record, error) { return nil, nil },
		getPDFFn: func(ctx context.Context, organizationID, acceptanceID string) (*PDFDocument, error) { return nil, nil },
	}, &clerk.Claims{Subject: "user-1", OrgSlug: "demo-alpha"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/work-orders/wo-1/acceptance", bytes.NewBufferString(`{"workOrderId":"wo-2"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestSubmitRejectsUnauthorized(t *testing.T) {
	r := acceptanceRouterWithoutClaims(serviceStub{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/work-orders/wo-1/acceptance", bytes.NewBufferString(validSubmissionJSON("wo-1")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestSubmitRejectsOrgMismatch(t *testing.T) {
	r := acceptanceRouter(serviceStub{}, &clerk.Claims{Subject: "user-1", OrgSlug: "demo-beta"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/work-orders/wo-1/acceptance", bytes.NewBufferString(validSubmissionJSON("wo-1")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestGetStatusReturnsExpectedPayload(t *testing.T) {
	record := acceptedRecord("acc-1", PDFStatusGenerated)
	key := "acceptances/org-1/acc-1.pdf"
	record.PDFStorageKey = &key
	r := acceptanceRouter(serviceStub{
		submitFn: func(ctx context.Context, organizationID string, submission Submission) (*Record, error) {
			return nil, nil
		},
		statusFn: func(ctx context.Context, organizationID, acceptanceID string) (*Record, error) {
			return record, nil
		},
		getPDFFn: func(ctx context.Context, organizationID, acceptanceID string) (*PDFDocument, error) { return nil, nil },
	}, &clerk.Claims{Subject: "user-1", OrgSlug: "demo-alpha"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/acceptances/acc-1", nil)
	req.Host = "api.example.test"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var body StatusResponse
	decodeJSON(t, rec, &body)
	if body.PDFURL == nil || *body.PDFURL != "https://api.example.test/api/v1/acceptances/acc-1/pdf" {
		t.Fatalf("pdfUrl = %#v", body.PDFURL)
	}
}

func TestGetPDFReturnsApplicationPDF(t *testing.T) {
	r := acceptanceRouter(serviceStub{
		submitFn: func(ctx context.Context, organizationID string, submission Submission) (*Record, error) {
			return nil, nil
		},
		statusFn: func(ctx context.Context, organizationID, acceptanceID string) (*Record, error) { return nil, nil },
		getPDFFn: func(ctx context.Context, organizationID, acceptanceID string) (*PDFDocument, error) {
			return &PDFDocument{Bytes: []byte("%PDF-1.4"), ContentType: "application/pdf"}, nil
		},
	}, &clerk.Claims{Subject: "user-1", OrgSlug: "demo-alpha"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/acceptances/acc-1/pdf", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("content-type = %s", rec.Header().Get("Content-Type"))
	}
}

func TestGetStatusReturnsNotFound(t *testing.T) {
	r := acceptanceRouter(serviceStub{
		submitFn: func(ctx context.Context, organizationID string, submission Submission) (*Record, error) {
			return nil, nil
		},
		statusFn: func(ctx context.Context, organizationID, acceptanceID string) (*Record, error) {
			return nil, ErrNotFound
		},
		getPDFFn: func(ctx context.Context, organizationID, acceptanceID string) (*PDFDocument, error) { return nil, nil },
	}, &clerk.Claims{Subject: "user-1", OrgSlug: "demo-alpha"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/acceptances/missing", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestGetPDFReturnsNotFound(t *testing.T) {
	r := acceptanceRouter(serviceStub{
		submitFn: func(ctx context.Context, organizationID string, submission Submission) (*Record, error) {
			return nil, nil
		},
		statusFn: func(ctx context.Context, organizationID, acceptanceID string) (*Record, error) { return nil, nil },
		getPDFFn: func(ctx context.Context, organizationID, acceptanceID string) (*PDFDocument, error) {
			return nil, ErrPDFNotReady
		},
	}, &clerk.Claims{Subject: "user-1", OrgSlug: "demo-alpha"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/acceptances/acc-1/pdf", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func acceptanceRouter(service AcceptanceService, claims *clerk.Claims) *gin.Engine {
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

func acceptanceRouterWithoutClaims(service AcceptanceService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(users.RequireOrgAccess(membershipStore{}))
	RegisterRoutes(api, NewHandler(service))
	return r
}

func acceptedRecord(acceptanceID string, pdfStatus PDFStatus) *Record {
	now := time.Now().UTC()
	return &Record{
		ID:                    acceptanceID,
		OrganizationID:        "org-1",
		WorkOrderID:           "wo-1",
		CustomerName:          "Acme Co",
		CustomerEmail:         "ops@acme.test",
		ServiceDate:           "2025-03-01",
		ServiceExpirationDate: "2025-04-01",
		ServiceType:           "Quarterly service",
		Products:              []string{"Sealant"},
		Notes:                 "Looks good.",
		Approved:              true,
		SignatureImageBase64:  "data:image/png;base64," + base64.StdEncoding.EncodeToString(validPNG()),
		SignedAt:              now,
		SignedByTechnicianID:  "tech-1",
		PDFStatus:             pdfStatus,
		EmailStatus:           EmailStatusPending,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
}

func validSubmissionJSON(workOrderID string) string {
	return `{
		"workOrderId":"` + workOrderID + `",
		"customerName":"Acme Co",
		"customerEmail":"ops@acme.test",
		"serviceDate":"2025-03-01",
		"serviceExpirationDate":"2025-04-01",
		"serviceType":"Quarterly service",
		"products":["Sealant","Inspection"],
		"notes":"All good",
		"approved":true,
		"signatureImageBase64":"data:image/png;base64,` + base64.StdEncoding.EncodeToString(validPNG()) + `",
		"signedAt":"2025-03-01T12:00:00Z",
		"signedByTechnicianId":"tech-1"
	}`
}

func validPNG() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
		0x0d, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0xf8, 0xcf, 0xc0, 0xf0,
		0x1f, 0x00, 0x05, 0x00, 0x01, 0xff, 0x89, 0x99, 0x3d, 0x1d, 0x00, 0x00,
		0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("decode json: %v", err)
	}
}

func TestNormalizeSubmissionRejectsBadSignedAt(t *testing.T) {
	_, err := NormalizeSubmission(Submission{
		WorkOrderID:           "wo-1",
		CustomerName:          "Acme",
		CustomerEmail:         "ops@acme.test",
		ServiceDate:           "2025-03-01",
		ServiceExpirationDate: "2025-04-01",
		ServiceType:           "Quarterly",
		Products:              []string{"Sealant"},
		SignatureImageBase64:  "abc",
		SignedAt:              "bad",
		SignedByTechnicianID:  "tech-1",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v", err)
	}
}
