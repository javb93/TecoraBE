package config

import (
	"testing"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadParsesDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("APP_ENV", "development")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %s", cfg.HTTPAddr)
	}
	if cfg.AppEnv != "development" {
		t.Fatalf("AppEnv = %s", cfg.AppEnv)
	}
	if len(cfg.AdminClerkUserIDs) != 0 {
		t.Fatalf("AdminClerkUserIDs = %#v", cfg.AdminClerkUserIDs)
	}
}

func TestLoadUsesPortEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("PORT", "9090")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Fatalf("HTTPAddr = %s", cfg.HTTPAddr)
	}
}

func TestLoadParsesAdminClerkUserIDs(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("ADMIN_CLERK_USER_IDS", "user_1, user_2 ,")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.AdminClerkUserIDs) != 2 {
		t.Fatalf("AdminClerkUserIDs len = %d", len(cfg.AdminClerkUserIDs))
	}
	if cfg.AdminClerkUserIDs[0] != "user_1" || cfg.AdminClerkUserIDs[1] != "user_2" {
		t.Fatalf("AdminClerkUserIDs = %#v", cfg.AdminClerkUserIDs)
	}
}
