package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestUpReturnsErrorForInvalidDatabaseURL(t *testing.T) {
	err := Up(context.Background(), slog.New(slog.NewTextHandler(os.Stdout, nil)), "://bad-url")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpAppliesMigrationsAndThenNoOps(t *testing.T) {
	databaseURL := createDisposableTestDatabase(t)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	if err := Up(context.Background(), logger, databaseURL); err != nil {
		t.Fatalf("first Up: %v", err)
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open migrated database: %v", err)
	}
	defer db.Close()

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM organizations`).Scan(&count); err != nil {
		t.Fatalf("count organizations: %v", err)
	}
	if count != 2 {
		t.Fatalf("organizations count = %d, want 2", count)
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 0 {
		t.Fatalf("users count = %d, want 0", count)
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM customers`).Scan(&count); err != nil {
		t.Fatalf("count customers: %v", err)
	}
	if count != 0 {
		t.Fatalf("customers count = %d, want 0", count)
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM acceptances`).Scan(&count); err != nil {
		t.Fatalf("count acceptances: %v", err)
	}
	if count != 0 {
		t.Fatalf("acceptances count = %d, want 0", count)
	}

	var orgID string
	if err := db.QueryRow(`SELECT id FROM organizations WHERE slug = 'demo-alpha'`).Scan(&orgID); err != nil {
		t.Fatalf("lookup organization id: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO users (clerk_user_id, email, first_name, last_name, organization_id) VALUES ($1, $2, $3, $4, $5)`, "user_123", nil, nil, nil, orgID); err != nil {
		t.Fatalf("insert nullable user: %v", err)
	}

	var email sql.NullString
	if err := db.QueryRow(`SELECT email FROM users WHERE clerk_user_id = $1`, "user_123").Scan(&email); err != nil {
		t.Fatalf("read nullable user email: %v", err)
	}
	if email.Valid {
		t.Fatalf("email should be null: %#v", email)
	}

	if _, err := db.Exec(`INSERT INTO customers (organization_id, name, email, phone, address, notes) VALUES ($1, $2, $3, $4, $5, $6)`, orgID, "Acme Co", nil, nil, nil, nil); err != nil {
		t.Fatalf("insert nullable customer: %v", err)
	}

	var customerEmail sql.NullString
	var customerPhone sql.NullString
	var customerAddress sql.NullString
	var customerNotes sql.NullString
	if err := db.QueryRow(`SELECT email, phone, address, notes FROM customers WHERE name = $1`, "Acme Co").Scan(&customerEmail, &customerPhone, &customerAddress, &customerNotes); err != nil {
		t.Fatalf("read nullable customer fields: %v", err)
	}
	if customerEmail.Valid || customerPhone.Valid || customerAddress.Valid || customerNotes.Valid {
		t.Fatalf("customer nullable fields should be null: email=%#v phone=%#v address=%#v notes=%#v", customerEmail, customerPhone, customerAddress, customerNotes)
	}

	if _, err := db.Exec(`
		INSERT INTO acceptances (
			organization_id,
			work_order_id,
			customer_name,
			customer_email,
			service_date,
			service_expiration_date,
			service_type,
			products,
			notes,
			approved,
			signature_image_base64,
			signed_at,
			signed_by_technician_id,
			pdf_status,
			email_status,
			pdf_storage_key,
			pdf_mime_type,
			pdf_error,
			pdf_generated_at,
			email_sent_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20
		)
	`,
		orgID,
		"wo-123",
		"Acme Co",
		"ops@acme.test",
		"2025-03-01",
		"2025-04-01",
		"Quarterly",
		"{Sealant,Inspection}",
		"",
		true,
		"data:image/png;base64,abc",
		time.Now().UTC(),
		"tech-1",
		"pending",
		"pending",
		nil,
		nil,
		nil,
		nil,
		nil,
	); err != nil {
		t.Fatalf("insert nullable acceptance fields: %v", err)
	}

	var pdfStorageKey sql.NullString
	var pdfMimeType sql.NullString
	var pdfError sql.NullString
	var pdfGeneratedAt sql.NullTime
	var emailSentAt sql.NullTime
	if err := db.QueryRow(`
		SELECT pdf_storage_key, pdf_mime_type, pdf_error, pdf_generated_at, email_sent_at
		FROM acceptances
		WHERE work_order_id = $1
	`, "wo-123").Scan(&pdfStorageKey, &pdfMimeType, &pdfError, &pdfGeneratedAt, &emailSentAt); err != nil {
		t.Fatalf("read acceptance nullable fields: %v", err)
	}
	if pdfStorageKey.Valid || pdfMimeType.Valid || pdfError.Valid || pdfGeneratedAt.Valid || emailSentAt.Valid {
		t.Fatalf("acceptance nullable fields should be null: key=%#v mime=%#v err=%#v generated=%#v emailSent=%#v", pdfStorageKey, pdfMimeType, pdfError, pdfGeneratedAt, emailSentAt)
	}

	if err := Up(context.Background(), logger, databaseURL); err != nil {
		t.Fatalf("second Up: %v", err)
	}
}

func createDisposableTestDatabase(t *testing.T) string {
	t.Helper()

	baseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}

	adminURL := *parsed
	adminURL.Path = "/postgres"

	adminDB, err := sql.Open("pgx", adminURL.String())
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	defer adminDB.Close()

	dbName := fmt.Sprintf("tecora_migrate_test_%d", time.Now().UnixNano())
	if _, err := adminDB.Exec(`CREATE DATABASE "` + dbName + `"`); err != nil {
		t.Fatalf("create test database: %v", err)
	}

	t.Cleanup(func() {
		if _, err := adminDB.Exec(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`, dbName); err != nil {
			t.Fatalf("terminate test database sessions: %v", err)
		}
		if _, err := adminDB.Exec(`DROP DATABASE IF EXISTS "` + dbName + `"`); err != nil {
			t.Fatalf("drop test database: %v", err)
		}
	})

	testURL := *parsed
	testURL.Path = "/" + dbName
	return testURL.String()
}
