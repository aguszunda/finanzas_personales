package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port          string
	DatabaseURL   string
	JWTSecret     string
	JWTExpiration time.Duration
	CORSOrigin    string
	LogLevel      string
}

func Load() *Config {
	return &Config{
		Port:          getEnv("PORT", "8080"),
		DatabaseURL:   getEnv("DATABASE_URL", "root:@tcp(127.0.0.1:3306)/finanzas?parseTime=true&multiStatements=true&charset=utf8mb4&loc=Local"),
		JWTSecret:     getEnv("JWT_SECRET", "secret-super-seguro-cambiar-en-produccion"),
		JWTExpiration: time.Duration(getEnvInt("JWT_EXPIRATION_HOURS", 72)) * time.Hour,
		CORSOrigin:    getEnv("CORS_ORIGIN", "*"),
		LogLevel:      getEnv("LOG_LEVEL", "info"),
	}
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
