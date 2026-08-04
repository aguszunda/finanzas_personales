package config

import (
	"os"
	"testing"
	"time"
)

func setEnv(t *testing.T, key, value string) {
	t.Helper()
	if value == "" {
		os.Unsetenv(key)
		return
	}
	os.Setenv(key, value)
	t.Cleanup(func() { os.Unsetenv(key) })
}

func TestLoad_Defaults(t *testing.T) {
	for _, k := range []string{"PORT", "DATABASE_URL", "JWT_SECRET", "JWT_EXPIRATION_HOURS", "CORS_ORIGIN", "LOG_LEVEL"} {
		os.Unsetenv(k)
	}
	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("expected default PORT 8080, got %q", cfg.Port)
	}
	if cfg.DatabaseURL != "root:@tcp(127.0.0.1:3306)/finanzas?parseTime=true&multiStatements=true&charset=utf8mb4&loc=Local" {
		t.Errorf("unexpected default DatabaseURL: %q", cfg.DatabaseURL)
	}
	if cfg.JWTSecret != "secret-super-seguro-cambiar-en-produccion" {
		t.Errorf("unexpected default JWTSecret: %q", cfg.JWTSecret)
	}
	if cfg.JWTExpiration != 72*time.Hour {
		t.Errorf("expected default JWTExpiration 72h, got %v", cfg.JWTExpiration)
	}
	if cfg.CORSOrigin != "*" {
		t.Errorf("expected default CORSOrigin *, got %q", cfg.CORSOrigin)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default LogLevel info, got %q", cfg.LogLevel)
	}
}

func TestLoad_Overrides(t *testing.T) {
	setEnv(t, "PORT", "9090")
	setEnv(t, "DATABASE_URL", "root:pass@tcp(db:3306)/finanzas?parseTime=true")
	setEnv(t, "JWT_SECRET", "clave-de-prueba")
	setEnv(t, "JWT_EXPIRATION_HOURS", "24")
	setEnv(t, "CORS_ORIGIN", "https://app.example.com")
	setEnv(t, "LOG_LEVEL", "debug")

	cfg := Load()

	if cfg.Port != "9090" {
		t.Errorf("expected PORT 9090, got %q", cfg.Port)
	}
	if cfg.DatabaseURL != "root:pass@tcp(db:3306)/finanzas?parseTime=true" {
		t.Errorf("unexpected DatabaseURL: %q", cfg.DatabaseURL)
	}
	if cfg.JWTSecret != "clave-de-prueba" {
		t.Errorf("unexpected JWTSecret: %q", cfg.JWTSecret)
	}
	if cfg.JWTExpiration != 24*time.Hour {
		t.Errorf("expected JWTExpiration 24h, got %v", cfg.JWTExpiration)
	}
	if cfg.CORSOrigin != "https://app.example.com" {
		t.Errorf("unexpected CORSOrigin: %q", cfg.CORSOrigin)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected LogLevel debug, got %q", cfg.LogLevel)
	}
}

func TestLoad_InvalidJWTExpirationFallsBack(t *testing.T) {
	setEnv(t, "JWT_EXPIRATION_HOURS", "no-es-un-numero")
	cfg := Load()
	if cfg.JWTExpiration != 72*time.Hour {
		t.Errorf("expected fallback 72h on invalid input, got %v", cfg.JWTExpiration)
	}
}
