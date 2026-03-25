package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv             string
	HTTPAddr           string
	DatabaseURL        string
	AdminClerkUserIDs  []string
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	IdleTimeout        time.Duration
	ShutdownTimeout    time.Duration
	HealthTimeout      time.Duration
	CORSAllowedOrigins []string
	Clerk              ClerkConfig
}

type ClerkConfig struct {
	IssuerURL string
	JWKSURL   string
	Audience  string
}

func (c ClerkConfig) Enabled() bool {
	return strings.TrimSpace(c.IssuerURL) != "" && strings.TrimSpace(c.JWKSURL) != ""
}

func Load() (Config, error) {
	clerkIssuer := strings.TrimSpace(os.Getenv("CLERK_ISSUER_URL"))
	clerkJWKS := strings.TrimSpace(os.Getenv("CLERK_JWKS_URL"))
	clerkAudience := strings.TrimSpace(os.Getenv("CLERK_AUDIENCE"))
	adminClerkUserIDs := parseList(getEnv("ADMIN_CLERK_USER_IDS", ""))

	if (clerkIssuer != "" || clerkJWKS != "") && (clerkIssuer == "" || clerkJWKS == "") {
		return Config{}, errors.New("both CLERK_ISSUER_URL and CLERK_JWKS_URL are required when Clerk auth is enabled")
	}

	httpAddr, err := getHTTPAddr()
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		AppEnv:             getEnv("APP_ENV", "development"),
		HTTPAddr:           httpAddr,
		DatabaseURL:        strings.TrimSpace(os.Getenv("DATABASE_URL")),
		AdminClerkUserIDs:  adminClerkUserIDs,
		ReadTimeout:        10 * time.Second,
		WriteTimeout:       10 * time.Second,
		IdleTimeout:        60 * time.Second,
		ShutdownTimeout:    10 * time.Second,
		HealthTimeout:      2 * time.Second,
		CORSAllowedOrigins: parseList(getEnv("CORS_ALLOWED_ORIGINS", "")),
		Clerk: ClerkConfig{
			IssuerURL: clerkIssuer,
			JWKSURL:   clerkJWKS,
			Audience:  clerkAudience,
		},
	}

	var parseErr error
	if cfg.ReadTimeout, parseErr = durationEnv("HTTP_READ_TIMEOUT", cfg.ReadTimeout); parseErr != nil {
		return Config{}, parseErr
	}
	if cfg.WriteTimeout, parseErr = durationEnv("HTTP_WRITE_TIMEOUT", cfg.WriteTimeout); parseErr != nil {
		return Config{}, parseErr
	}
	if cfg.IdleTimeout, parseErr = durationEnv("HTTP_IDLE_TIMEOUT", cfg.IdleTimeout); parseErr != nil {
		return Config{}, parseErr
	}
	if cfg.ShutdownTimeout, parseErr = durationEnv("HTTP_SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout); parseErr != nil {
		return Config{}, parseErr
	}
	if cfg.HealthTimeout, parseErr = durationEnv("HEALTH_TIMEOUT", cfg.HealthTimeout); parseErr != nil {
		return Config{}, parseErr
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	if cfg.AppEnv == "" {
		return Config{}, errors.New("APP_ENV must not be empty")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getHTTPAddr() (string, error) {
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		if strings.HasPrefix(port, ":") {
			return port, nil
		}
		if _, err := strconv.Atoi(port); err == nil {
			return ":" + port, nil
		}
		return "", fmt.Errorf("invalid PORT value: %q", port)
	}

	if addr := strings.TrimSpace(os.Getenv("HTTP_ADDR")); addr != "" {
		return addr, nil
	}

	return ":8080", nil
}

func parseList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(raw)
	if err == nil {
		return parsed, nil
	}

	if seconds, convErr := strconv.Atoi(raw); convErr == nil {
		return time.Duration(seconds) * time.Second, nil
	}

	return 0, fmt.Errorf("invalid duration for %s: %q", key, raw)
}

func (c Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	return nil
}
