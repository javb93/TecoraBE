package acceptances

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
)

type Store interface {
	GetByID(ctx context.Context, organizationID, acceptanceID string) (*Record, error)
	GetByWorkOrderID(ctx context.Context, organizationID, workOrderID string) (*Record, error)
	Create(ctx context.Context, input CreateInput) (*Record, error)
	MarkPDFGenerated(ctx context.Context, organizationID, acceptanceID, storageKey, mimeType string) (*Record, error)
	MarkPDFFailed(ctx context.Context, organizationID, acceptanceID, pdfError string) (*Record, error)
}

type PDFRenderer interface {
	Render(record Record) (PDFDocument, error)
}

type DocumentStorage interface {
	Upload(ctx context.Context, objectKey string, doc PDFDocument) error
	Download(ctx context.Context, objectKey string) (*PDFDocument, error)
}

type Service struct {
	store          Store
	renderer       PDFRenderer
	storage        DocumentStorage
	documentPrefix string
}

func NewService(store Store, renderer PDFRenderer, storage DocumentStorage, documentPrefix string) *Service {
	return &Service{
		store:          store,
		renderer:       renderer,
		storage:        storage,
		documentPrefix: normalizeDocumentPrefix(documentPrefix),
	}
}

func (s *Service) Submit(ctx context.Context, organizationID string, submission Submission) (*Record, error) {
	input, err := NormalizeSubmission(submission)
	if err != nil {
		return nil, err
	}
	input.OrganizationID = strings.TrimSpace(organizationID)
	if input.OrganizationID == "" {
		return nil, ErrInvalidInput
	}

	existing, err := s.store.GetByWorkOrderID(ctx, input.OrganizationID, input.WorkOrderID)
	switch {
	case err == nil:
		return existing, nil
	case !errors.Is(err, ErrNotFound):
		return nil, err
	}

	record, err := s.store.Create(ctx, input)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return s.store.GetByWorkOrderID(ctx, input.OrganizationID, input.WorkOrderID)
		}
		return nil, err
	}

	pdf, err := s.renderer.Render(*record)
	if err != nil {
		return s.store.MarkPDFFailed(ctx, record.OrganizationID, record.ID, fmt.Sprintf("render pdf: %v", err))
	}

	objectKey := s.objectKey(record.OrganizationID, record.ID)
	if err := s.storage.Upload(ctx, objectKey, pdf); err != nil {
		if errors.Is(err, ErrObjectExists) {
			return s.store.MarkPDFGenerated(ctx, record.OrganizationID, record.ID, objectKey, pdf.ContentType)
		}
		return s.store.MarkPDFFailed(ctx, record.OrganizationID, record.ID, fmt.Sprintf("upload pdf: %v", err))
	}

	return s.store.MarkPDFGenerated(ctx, record.OrganizationID, record.ID, objectKey, pdf.ContentType)
}

func (s *Service) GetStatus(ctx context.Context, organizationID, acceptanceID string) (*Record, error) {
	return s.store.GetByID(ctx, strings.TrimSpace(organizationID), strings.TrimSpace(acceptanceID))
}

func (s *Service) GetPDF(ctx context.Context, organizationID, acceptanceID string) (*PDFDocument, error) {
	record, err := s.store.GetByID(ctx, strings.TrimSpace(organizationID), strings.TrimSpace(acceptanceID))
	if err != nil {
		return nil, err
	}
	if record.PDFStatus != PDFStatusGenerated || record.PDFStorageKey == nil || strings.TrimSpace(*record.PDFStorageKey) == "" {
		return nil, ErrPDFNotReady
	}

	doc, err := s.storage.Download(ctx, *record.PDFStorageKey)
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			return nil, ErrPDFNotReady
		}
		return nil, err
	}
	return doc, nil
}

func normalizeDocumentPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return "acceptances"
	}
	return prefix
}

func (s *Service) objectKey(organizationID, acceptanceID string) string {
	return path.Join(s.documentPrefix, strings.TrimSpace(organizationID), strings.TrimSpace(acceptanceID)+".pdf")
}
