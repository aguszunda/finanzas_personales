package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// minJWTSecretLen es la longitud mínima recomendada para un secreto HS256
// (256 bits ≈ 32 bytes): por debajo de eso la clave es forzable.
const minJWTSecretLen = 32

// knownInsecureSecrets son los valores que históricamente se usaban como
// default o como placeholder de JWT_SECRET. Se rechazan incluso si llegan por
// variable de entorno: un secreto que está en el repo no es un secreto.
var knownInsecureSecrets = []string{
	"secret-super-seguro-cambiar-en-produccion",
	"reemplazar-con-secreto-seguro-en-produccion",
	"cambiar-este-secreto-por-uno-aleatorio-de-64-caracteres",
	"clave-de-prueba",
	"test-secret",
}

type Config struct {
	Port          string
	DatabaseURL   string
	JWTSecret     string
	JWTExpiration time.Duration
	CORSOrigin    string
	LogLevel      string
}

// Load construye la configuración desde variables de entorno con fail-fast:
// los secretos (JWT_SECRET, DATABASE_URL) no tienen default y su ausencia o
// un valor inseguro impiden arrancar la aplicación.
func Load() (*Config, error) {
	cfg := &Config{
		Port:          getEnv("PORT", "8080"),
		JWTSecret:     os.Getenv("JWT_SECRET"),
		JWTExpiration: time.Duration(getEnvInt("JWT_EXPIRATION_HOURS", 72)) * time.Hour,
		CORSOrigin:    getEnv("CORS_ORIGIN", "*"),
		LogLevel:      getEnv("LOG_LEVEL", "info"),
	}

	secret, err := getEnvRequired("JWT_SECRET")
	if err != nil {
		return nil, err
	}
	if len(secret) < minJWTSecretLen {
		return nil, errors.New("JWT_SECRET debe tener al menos 32 caracteres")
	}
	if isInsecureSecret(secret) {
		return nil, errors.New("JWT_SECRET no puede ser un valor conocido/placeholder; generá uno aleatorio")
	}
	cfg.JWTSecret = secret

	databaseURL, err := getEnvRequired("DATABASE_URL")
	if err != nil {
		return nil, err
	}
	if !strings.Contains(databaseURL, "@tcp(") {
		return nil, errors.New("DATABASE_URL debe ser una DSN de MySQL válida (ej: usuario:pass@tcp(host:3306)/db)")
	}
	cfg.DatabaseURL = databaseURL

	return cfg, nil
}

func getEnvRequired(key string) (string, error) {
	if v := os.Getenv(key); v != "" {
		return v, nil
	}
	return "", errors.New("variable de entorno " + key + " requerida (ver env.secrets)")
}

func isInsecureSecret(secret string) bool {
	for _, known := range knownInsecureSecrets {
		if secret == known {
			return true
		}
	}
	return false
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
