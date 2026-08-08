package config

import (
	"os"
	"testing"
	"time"
)

const validTestSecret = "clave-de-prueba-aleatoria-de-al-menos-32-caracteres"

func setEnv(t *testing.T, key, value string) {
	t.Helper()
	if value == "" {
		os.Unsetenv(key)
		return
	}
	os.Setenv(key, value)
	t.Cleanup(func() { os.Unsetenv(key) })
}

func validEnv(t *testing.T) {
	t.Helper()
	setEnv(t, "JWT_SECRET", validTestSecret)
	setEnv(t, "DATABASE_URL", "root:pass@tcp(127.0.0.1:3306)/finanzas?parseTime=true")
}

func TestLoad_Defaults(t *testing.T) {
	for _, k := range []string{"PORT", "JWT_EXPIRATION_HOURS", "CORS_ORIGIN", "LOG_LEVEL"} {
		os.Unsetenv(k)
	}
	validEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != "8080" {
		t.Errorf("expected default PORT 8080, got %q", cfg.Port)
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
	setEnv(t, "JWT_SECRET", validTestSecret)
	setEnv(t, "JWT_EXPIRATION_HOURS", "24")
	setEnv(t, "CORS_ORIGIN", "https://app.example.com")
	setEnv(t, "LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != "9090" {
		t.Errorf("expected PORT 9090, got %q", cfg.Port)
	}
	if cfg.DatabaseURL != "root:pass@tcp(db:3306)/finanzas?parseTime=true" {
		t.Errorf("unexpected DatabaseURL: %q", cfg.DatabaseURL)
	}
	if cfg.JWTSecret != validTestSecret {
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
	validEnv(t)
	setEnv(t, "JWT_EXPIRATION_HOURS", "no-es-un-numero")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.JWTExpiration != 72*time.Hour {
		t.Errorf("expected fallback 72h on invalid input, got %v", cfg.JWTExpiration)
	}
}

func TestLoad_RequiereJWTSecret(t *testing.T) {
	setEnv(t, "DATABASE_URL", "root:pass@tcp(127.0.0.1:3306)/finanzas?parseTime=true")
	os.Unsetenv("JWT_SECRET")

	_, err := Load()
	if err == nil {
		t.Error("expected error when JWT_SECRET is missing")
	}
}

func TestLoad_RequiereDatabaseURL(t *testing.T) {
	setEnv(t, "JWT_SECRET", validTestSecret)
	os.Unsetenv("DATABASE_URL")

	_, err := Load()
	if err == nil {
		t.Error("expected error when DATABASE_URL is missing")
	}
}

func TestLoad_RechazaJWTSecretCorto(t *testing.T) {
	setEnv(t, "JWT_SECRET", "muy-corto")
	setEnv(t, "DATABASE_URL", "root:pass@tcp(127.0.0.1:3306)/finanzas?parseTime=true")

	_, err := Load()
	if err == nil {
		t.Error("expected error for short JWT_SECRET")
	}
}

func TestLoad_RechazaPlaceholders(t *testing.T) {
	for _, secret := range []string{
		"secret-super-seguro-cambiar-en-produccion",
		"reemplazar-con-secreto-seguro-en-produccion",
		"cambiar-este-secreto-por-uno-aleatorio-de-64-caracteres",
		"clave-de-prueba",
		"test-secret",
	} {
		t.Run(secret, func(t *testing.T) {
			setEnv(t, "JWT_SECRET", secret)
			setEnv(t, "DATABASE_URL", "root:pass@tcp(127.0.0.1:3306)/finanzas?parseTime=true")

			_, err := Load()
			if err == nil {
				t.Errorf("expected error for insecure secret %q", secret)
			}
		})
	}
}

func TestLoad_RechazaDatabaseURLInvalida(t *testing.T) {
	setEnv(t, "JWT_SECRET", validTestSecret)
	setEnv(t, "DATABASE_URL", "postgres://user:pass@localhost/db")

	_, err := Load()
	if err == nil {
		t.Error("expected error for non-MySQL DATABASE_URL")
	}
}
