package acceptances

import (
	"context"
	"errors"
	"testing"
	"time"
)

type storeStub struct {
	getByWorkOrderIDFn func(ctx context.Context, organizationID, workOrderID string) (*Record, error)
	createFn           func(ctx context.Context, input CreateInput) (*Record, error)
	markPDFGeneratedFn func(ctx context.Context, organizationID, acceptanceID, storageKey, mimeType string) (*Record, error)
	markPDFFailedFn    func(ctx context.Context, organizationID, acceptanceID, pdfError string) (*Record, error)
	getByIDFn          func(ctx context.Context, organizationID, acceptanceID string) (*Record, error)
}

func (s storeStub) GetByID(ctx context.Context, organizationID, acceptanceID string) (*Record, error) {
	if s.getByIDFn == nil {
		return nil, errors.New("unexpected GetByID")
	}
	return s.getByIDFn(ctx, organizationID, acceptanceID)
}

func (s storeStub) GetByWorkOrderID(ctx context.Context, organizationID, workOrderID string) (*Record, error) {
	if s.getByWorkOrderIDFn == nil {
		return nil, errors.New("unexpected GetByWorkOrderID")
	}
	return s.getByWorkOrderIDFn(ctx, organizationID, workOrderID)
}

func (s storeStub) Create(ctx context.Context, input CreateInput) (*Record, error) {
	if s.createFn == nil {
		return nil, errors.New("unexpected Create")
	}
	return s.createFn(ctx, input)
}

func (s storeStub) MarkPDFGenerated(ctx context.Context, organizationID, acceptanceID, storageKey, mimeType string) (*Record, error) {
	if s.markPDFGeneratedFn == nil {
		return nil, errors.New("unexpected MarkPDFGenerated")
	}
	return s.markPDFGeneratedFn(ctx, organizationID, acceptanceID, storageKey, mimeType)
}

func (s storeStub) MarkPDFFailed(ctx context.Context, organizationID, acceptanceID, pdfError string) (*Record, error) {
	if s.markPDFFailedFn == nil {
		return nil, errors.New("unexpected MarkPDFFailed")
	}
	return s.markPDFFailedFn(ctx, organizationID, acceptanceID, pdfError)
}

type rendererStub struct {
	renderFn func(record Record) (PDFDocument, error)
}

func (r rendererStub) Render(record Record) (PDFDocument, error) {
	if r.renderFn == nil {
		return PDFDocument{}, errors.New("unexpected Render")
	}
	return r.renderFn(record)
}

type storageStub struct {
	uploadFn   func(ctx context.Context, objectKey string, doc PDFDocument) error
	downloadFn func(ctx context.Context, objectKey string) (*PDFDocument, error)
}

func (s storageStub) Upload(ctx context.Context, objectKey string, doc PDFDocument) error {
	if s.uploadFn == nil {
		return errors.New("unexpected Upload")
	}
	return s.uploadFn(ctx, objectKey, doc)
}

func (s storageStub) Download(ctx context.Context, objectKey string) (*PDFDocument, error) {
	if s.downloadFn == nil {
		return nil, errors.New("unexpected Download")
	}
	return s.downloadFn(ctx, objectKey)
}

func TestSubmitMarksGeneratedWhenObjectAlreadyExists(t *testing.T) {
	record := testAcceptanceRecord("org-1", "acc-1", "wo-1")
	service := NewService(
		storeStub{
			getByWorkOrderIDFn: func(ctx context.Context, organizationID, workOrderID string) (*Record, error) {
				return nil, ErrNotFound
			},
			createFn: func(ctx context.Context, input CreateInput) (*Record, error) {
				return record, nil
			},
			markPDFGeneratedFn: func(ctx context.Context, organizationID, acceptanceID, storageKey, mimeType string) (*Record, error) {
				if storageKey != "acceptances/org-1/acc-1.pdf" {
					t.Fatalf("storageKey=%s", storageKey)
				}
				if mimeType != "application/pdf" {
					t.Fatalf("mimeType=%s", mimeType)
				}
				generated := *record
				generated.PDFStatus = PDFStatusGenerated
				generated.PDFStorageKey = &storageKey
				return &generated, nil
			},
		},
		rendererStub{
			renderFn: func(record Record) (PDFDocument, error) {
				return PDFDocument{Bytes: []byte("%PDF-1.4"), ContentType: "application/pdf"}, nil
			},
		},
		storageStub{
			uploadFn: func(ctx context.Context, objectKey string, doc PDFDocument) error {
				return ErrObjectExists
			},
		},
		"acceptances",
	)

	result, err := service.Submit(context.Background(), "org-1", validSubmission())
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.PDFStatus != PDFStatusGenerated {
		t.Fatalf("pdfStatus=%s", result.PDFStatus)
	}
}

func TestSubmitMarksFailedOnUploadError(t *testing.T) {
	record := testAcceptanceRecord("org-1", "acc-1", "wo-1")
	service := NewService(
		storeStub{
			getByWorkOrderIDFn: func(ctx context.Context, organizationID, workOrderID string) (*Record, error) {
				return nil, ErrNotFound
			},
			createFn: func(ctx context.Context, input CreateInput) (*Record, error) {
				return record, nil
			},
			markPDFFailedFn: func(ctx context.Context, organizationID, acceptanceID, pdfError string) (*Record, error) {
				if pdfError == "" {
					t.Fatal("expected pdf error")
				}
				failed := *record
				failed.PDFStatus = PDFStatusFailed
				return &failed, nil
			},
		},
		rendererStub{
			renderFn: func(record Record) (PDFDocument, error) {
				return PDFDocument{Bytes: []byte("%PDF-1.4"), ContentType: "application/pdf"}, nil
			},
		},
		storageStub{
			uploadFn: func(ctx context.Context, objectKey string, doc PDFDocument) error {
				return errors.New("boom")
			},
		},
		"acceptances",
	)

	result, err := service.Submit(context.Background(), "org-1", validSubmission())
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.PDFStatus != PDFStatusFailed {
		t.Fatalf("pdfStatus=%s", result.PDFStatus)
	}
}

func validSubmission() Submission {
	return Submission{
		WorkOrderID:           "wo-1",
		CustomerName:          "Acme Co",
		CustomerEmail:         "ops@acme.test",
		ServiceDate:           "2025-03-01",
		ServiceExpirationDate: "2025-04-01",
		ServiceType:           "Quarterly service",
		Products:              []string{"Sealant"},
		Notes:                 "Everything looks good.",
		Approved:              true,
		SignatureImageBase64:  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aKMsAAAAASUVORK5CYII=",
		SignedAt:              "2025-03-01T12:00:00Z",
		SignedByTechnicianID:  "tech-1",
	}
}

func testAcceptanceRecord(organizationID, acceptanceID, workOrderID string) *Record {
	now := time.Now().UTC()
	return &Record{
		ID:                    acceptanceID,
		OrganizationID:        organizationID,
		WorkOrderID:           workOrderID,
		CustomerName:          "Acme Co",
		CustomerEmail:         "ops@acme.test",
		ServiceDate:           "2025-03-01",
		ServiceExpirationDate: "2025-04-01",
		ServiceType:           "Quarterly service",
		Products:              []string{"Sealant"},
		Notes:                 "Everything looks good.",
		Approved:              true,
		SignatureImageBase64:  validSubmission().SignatureImageBase64,
		SignedAt:              now,
		SignedByTechnicianID:  "tech-1",
		PDFStatus:             PDFStatusPending,
		EmailStatus:           EmailStatusPending,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
}
