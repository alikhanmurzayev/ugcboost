package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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

	// Test endpoints (never enable in production)
	EnableTestEndpoints bool

	// Security
	BcryptCost     int
	RefreshExpiry  time.Duration
	ResetExpiry    time.Duration
	CookieSecure   bool

	// HTTP server
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration

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

	bcryptCost := getIntEnv("BCRYPT_COST", 12)
	refreshExpiry, err := time.ParseDuration(getEnv("REFRESH_EXPIRY", "168h")) // 7 days
	if err != nil {
		return nil, fmt.Errorf("invalid REFRESH_EXPIRY: %w", err)
	}
	resetExpiry, err := time.ParseDuration(getEnv("RESET_EXPIRY", "1h"))
	if err != nil {
		return nil, fmt.Errorf("invalid RESET_EXPIRY: %w", err)
	}

	corsOrigins := splitComma(getEnv("CORS_ORIGINS", "http://localhost:5173,http://localhost:5174"))
	cookieSecure := getBoolEnv("COOKIE_SECURE", !hasLocalhostOrigin(corsOrigins))

	return &Config{
		Port:        port,
		DatabaseURL: dbURL,
		JWTSecret:   jwtSecret,
		JWTExpiry:   jwtExpiry,
		CORSOrigins: corsOrigins,
		LogLevel:    getEnv("LOG_LEVEL", "info"),

		AdminEmail:    getEnv("ADMIN_EMAIL", "admin@ugcboost.kz"),
		AdminPassword: getEnv("ADMIN_PASSWORD", ""),

		BcryptCost:    bcryptCost,
		RefreshExpiry: refreshExpiry,
		ResetExpiry:   resetExpiry,
		CookieSecure:  cookieSecure,

		ReadTimeout:  getDurationEnv("READ_TIMEOUT", 10*time.Second),
		WriteTimeout: getDurationEnv("WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:  getDurationEnv("IDLE_TIMEOUT", 60*time.Second),

		EnableTestEndpoints: getBoolEnv("ENABLE_TEST_ENDPOINTS", false),

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

func getIntEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func hasLocalhostOrigin(origins []string) bool {
	for _, o := range origins {
		if strings.HasPrefix(o, "http://localhost") {
			return true
		}
	}
	return false
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
