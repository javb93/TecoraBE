package acceptances

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const metadataTokenURL = "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token"

type tokenSource interface {
	Token(ctx context.Context) (string, error)
}

type GCSStorage struct {
	bucketName string
	client     *http.Client
	tokens     tokenSource
}

func NewGCSStorage(bucketName string, client *http.Client) *GCSStorage {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &GCSStorage{
		bucketName: strings.TrimSpace(bucketName),
		client:     client,
		tokens:     &metadataTokenSource{client: client},
	}
}

func (s *GCSStorage) Upload(ctx context.Context, objectKey string, doc PDFDocument) error {
	token, err := s.tokens.Token(ctx)
	if err != nil {
		return err
	}

	endpoint := "https://storage.googleapis.com/upload/storage/v1/b/" + url.PathEscape(s.bucketName) + "/o?uploadType=media&ifGenerationMatch=0&name=" + url.QueryEscape(strings.TrimSpace(objectKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(doc.Bytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", doc.ContentType)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusPreconditionFailed {
		return ErrObjectExists
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("gcs upload failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

func (s *GCSStorage) Download(ctx context.Context, objectKey string) (*PDFDocument, error) {
	token, err := s.tokens.Token(ctx)
	if err != nil {
		return nil, err
	}

	endpoint := "https://storage.googleapis.com/storage/v1/b/" + url.PathEscape(s.bucketName) + "/o/" + url.PathEscape(strings.TrimSpace(objectKey)) + "?alt=media"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrObjectNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("gcs download failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/pdf"
	}

	return &PDFDocument{
		Bytes:       body,
		ContentType: contentType,
	}, nil
}

type metadataTokenSource struct {
	client *http.Client

	mu     sync.Mutex
	token  string
	expiry time.Time
}

type metadataTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

func (s *metadataTokenSource) Token(ctx context.Context) (string, error) {
	if envToken := strings.TrimSpace(os.Getenv("GCP_ACCESS_TOKEN")); envToken != "" {
		return envToken, nil
	}

	s.mu.Lock()
	if s.token != "" && time.Now().Before(s.expiry.Add(-30*time.Second)) {
		token := s.token
		s.mu.Unlock()
		return token, nil
	}
	s.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataTokenURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("metadata token failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload metadataTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return "", fmt.Errorf("metadata token response missing access token")
	}

	expiry := time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)

	s.mu.Lock()
	s.token = payload.AccessToken
	s.expiry = expiry
	s.mu.Unlock()

	return payload.AccessToken, nil
}
