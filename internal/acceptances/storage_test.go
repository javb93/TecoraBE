package acceptances

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type tokenSourceStub struct {
	token string
	err   error
}

func (s tokenSourceStub) Token(ctx context.Context) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.token, nil
}

func TestGCSUploadUsesCreateOnlyPrecondition(t *testing.T) {
	var gotQuery string
	storage := &GCSStorage{
		bucketName: "bucket-1",
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				gotQuery = req.URL.RawQuery
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       http.NoBody,
					Header:     make(http.Header),
				}, nil
			}),
		},
		tokens: tokenSourceStub{token: "token-1"},
	}

	err := storage.Upload(context.Background(), "acceptances/org-1/acc-1.pdf", PDFDocument{
		Bytes:       []byte("%PDF-1.4"),
		ContentType: "application/pdf",
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if !strings.Contains(gotQuery, "ifGenerationMatch=0") {
		t.Fatalf("query=%s", gotQuery)
	}
}

func TestGCSUploadMapsPreconditionFailureToObjectExists(t *testing.T) {
	storage := &GCSStorage{
		bucketName: "bucket-1",
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusPreconditionFailed,
					Body:       http.NoBody,
					Header:     make(http.Header),
				}, nil
			}),
		},
		tokens: tokenSourceStub{token: "token-1"},
	}

	err := storage.Upload(context.Background(), "acceptances/org-1/acc-1.pdf", PDFDocument{
		Bytes:       []byte("%PDF-1.4"),
		ContentType: "application/pdf",
	})
	if !errors.Is(err, ErrObjectExists) {
		t.Fatalf("err=%v", err)
	}
}
