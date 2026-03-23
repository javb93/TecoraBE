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
