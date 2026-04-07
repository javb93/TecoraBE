package acceptances

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound       = errors.New("acceptance not found")
	ErrConflict       = errors.New("acceptance already exists")
	ErrInvalidInput   = errors.New("invalid acceptance input")
	ErrPDFNotReady    = errors.New("acceptance pdf is not available")
	ErrObjectNotFound = errors.New("document object not found")
	ErrObjectExists   = errors.New("document object already exists")
)

type PDFStatus string

const (
	PDFStatusPending   PDFStatus = "pending"
	PDFStatusGenerated PDFStatus = "generated"
	PDFStatusFailed    PDFStatus = "failed"
)

type EmailStatus string

const (
	EmailStatusPending EmailStatus = "pending"
	EmailStatusSent    EmailStatus = "sent"
	EmailStatusFailed  EmailStatus = "failed"
)

type Submission struct {
	WorkOrderID           string   `json:"workOrderId"`
	CustomerName          string   `json:"customerName"`
	CustomerEmail         string   `json:"customerEmail"`
	ServiceDate           string   `json:"serviceDate"`
	ServiceExpirationDate string   `json:"serviceExpirationDate"`
	ServiceType           string   `json:"serviceType"`
	Products              []string `json:"products"`
	Notes                 string   `json:"notes"`
	Approved              bool     `json:"approved"`
	SignatureImageBase64  string   `json:"signatureImageBase64"`
	SignedAt              string   `json:"signedAt"`
	SignedByTechnicianID  string   `json:"signedByTechnicianId"`
}

type CreateInput struct {
	OrganizationID        string
	WorkOrderID           string
	CustomerName          string
	CustomerEmail         string
	ServiceDate           string
	ServiceExpirationDate string
	ServiceType           string
	Products              []string
	Notes                 string
	Approved              bool
	SignatureImageBase64  string
	SignedAt              time.Time
	SignedByTechnicianID  string
}

type Record struct {
	ID                    string
	OrganizationID        string
	WorkOrderID           string
	CustomerName          string
	CustomerEmail         string
	ServiceDate           string
	ServiceExpirationDate string
	ServiceType           string
	Products              []string
	Notes                 string
	Approved              bool
	SignatureImageBase64  string
	SignedAt              time.Time
	SignedByTechnicianID  string
	PDFStatus             PDFStatus
	EmailStatus           EmailStatus
	PDFStorageKey         *string
	PDFMimeType           *string
	PDFError              *string
	PDFGeneratedAt        *time.Time
	EmailSentAt           *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type AcceptedResponse struct {
	AcceptanceID string      `json:"acceptanceId"`
	WorkOrderID  string      `json:"workOrderId"`
	Status       string      `json:"status"`
	PDFStatus    PDFStatus   `json:"pdfStatus"`
	EmailStatus  EmailStatus `json:"emailStatus"`
}

type StatusResponse struct {
	AcceptanceID string      `json:"acceptanceId"`
	WorkOrderID  string      `json:"workOrderId"`
	PDFStatus    PDFStatus   `json:"pdfStatus"`
	EmailStatus  EmailStatus `json:"emailStatus"`
	PDFURL       *string     `json:"pdfUrl"`
	EmailSentAt  *time.Time  `json:"emailSentAt"`
	UpdatedAt    time.Time   `json:"updatedAt"`
}

type PDFDocument struct {
	Bytes       []byte
	ContentType string
}

func NormalizeSubmission(input Submission) (CreateInput, error) {
	signedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(input.SignedAt))
	if err != nil {
		return CreateInput{}, ErrInvalidInput
	}

	products := make([]string, 0, len(input.Products))
	for _, product := range input.Products {
		trimmed := strings.TrimSpace(product)
		if trimmed == "" {
			return CreateInput{}, ErrInvalidInput
		}
		products = append(products, trimmed)
	}

	out := CreateInput{
		WorkOrderID:           strings.TrimSpace(input.WorkOrderID),
		CustomerName:          strings.TrimSpace(input.CustomerName),
		CustomerEmail:         strings.TrimSpace(input.CustomerEmail),
		ServiceDate:           strings.TrimSpace(input.ServiceDate),
		ServiceExpirationDate: strings.TrimSpace(input.ServiceExpirationDate),
		ServiceType:           strings.TrimSpace(input.ServiceType),
		Products:              products,
		Notes:                 strings.TrimSpace(input.Notes),
		Approved:              input.Approved,
		SignatureImageBase64:  strings.TrimSpace(input.SignatureImageBase64),
		SignedAt:              signedAt.UTC(),
		SignedByTechnicianID:  strings.TrimSpace(input.SignedByTechnicianID),
	}

	if out.WorkOrderID == "" ||
		out.CustomerName == "" ||
		out.CustomerEmail == "" ||
		out.ServiceDate == "" ||
		out.ServiceExpirationDate == "" ||
		out.ServiceType == "" ||
		len(out.Products) == 0 ||
		out.SignatureImageBase64 == "" ||
		out.SignedByTechnicianID == "" {
		return CreateInput{}, ErrInvalidInput
	}

	return out, nil
}

func (r Record) AcceptedResponse() AcceptedResponse {
	return AcceptedResponse{
		AcceptanceID: r.ID,
		WorkOrderID:  r.WorkOrderID,
		Status:       "accepted",
		PDFStatus:    r.PDFStatus,
		EmailStatus:  r.EmailStatus,
	}
}

func (r Record) StatusResponse(pdfURL *string) StatusResponse {
	return StatusResponse{
		AcceptanceID: r.ID,
		WorkOrderID:  r.WorkOrderID,
		PDFStatus:    r.PDFStatus,
		EmailStatus:  r.EmailStatus,
		PDFURL:       pdfURL,
		EmailSentAt:  r.EmailSentAt,
		UpdatedAt:    r.UpdatedAt,
	}
}
