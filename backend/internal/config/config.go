package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration, loaded from environment variables.
type Config struct {
	Port        int
	DatabaseURL string
	JWTSecret   string
	JWTExpiry   time.Duration
	CORSOrigins []string
	LogLevel    string

	// Admin seed
	AdminEmail    string
	AdminPassword string

	// Feature flags for mock integrations
	LiveDuneMock  bool
	TrustMeMock   bool
	TelegramMock  bool
	EmailMock     bool
	StorageMock   bool
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	port, err := strconv.Atoi(getEnv("PORT", "8080"))
	if err != nil {
		return nil, fmt.Errorf("invalid PORT: %w", err)
	}

	dbURL := getEnv("DATABASE_URL", "")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	jwtExpiry, err := time.ParseDuration(getEnv("JWT_EXPIRY", "15m"))
	if err != nil {
		return nil, fmt.Errorf("invalid JWT_EXPIRY: %w", err)
	}

	jwtSecret := getEnv("JWT_SECRET", "")
	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	return &Config{
		Port:        port,
		DatabaseURL: dbURL,
		JWTSecret:   jwtSecret,
		JWTExpiry:   jwtExpiry,
		CORSOrigins: splitComma(getEnv("CORS_ORIGINS", "http://localhost:5173,http://localhost:5174")),
		LogLevel:    getEnv("LOG_LEVEL", "info"),

		AdminEmail:    getEnv("ADMIN_EMAIL", "admin@ugcboost.kz"),
		AdminPassword: getEnv("ADMIN_PASSWORD", ""),

		LiveDuneMock: getBoolEnv("LIVEDUNE_MOCK", false),
		TrustMeMock:  getBoolEnv("TRUSTME_MOCK", false),
		TelegramMock: getBoolEnv("TELEGRAM_MOCK", false),
		EmailMock:    getBoolEnv("EMAIL_MOCK", false),
		StorageMock:  getBoolEnv("STORAGE_MOCK", false),
	}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getBoolEnv(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	parts := make([]string, 0)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			part := s[start:i]
			if part != "" {
				parts = append(parts, part)
			}
			start = i + 1
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}
