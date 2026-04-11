package config

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/caarlos0/env/v11"
)

// Environment values.
const (
	EnvLocal      = "local"
	EnvStaging    = "staging"
	EnvProduction = "production"
)

// Config holds all application configuration, loaded from environment variables.
type Config struct {
	Environment string `env:"ENVIRONMENT" envDefault:"local"`
	Port        int    `env:"PORT" envDefault:"8080"`
	DatabaseURL string `env:"DATABASE_URL,required"`
	JWTSecret   string `env:"JWT_SECRET,required"`
	JWTExpiry   time.Duration `env:"JWT_EXPIRY" envDefault:"15m"`
	CORSOrigins []string      `env:"CORS_ORIGINS" envDefault:"http://localhost:5173,http://localhost:5174" envSeparator:","`
	LogLevel    slog.Level    `env:"LOG_LEVEL" envDefault:"INFO"`

	// Admin seed
	AdminEmail    string `env:"ADMIN_EMAIL" envDefault:"admin@ugcboost.kz"`
	AdminPassword string `env:"ADMIN_PASSWORD"`

	// Security
	BcryptCost    int           `env:"BCRYPT_COST" envDefault:"12"`
	RefreshExpiry time.Duration `env:"REFRESH_EXPIRY" envDefault:"168h"`
	ResetExpiry   time.Duration `env:"RESET_EXPIRY" envDefault:"1h"`

	// HTTP server
	ReadTimeout  time.Duration `env:"READ_TIMEOUT" envDefault:"10s"`
	WriteTimeout time.Duration `env:"WRITE_TIMEOUT" envDefault:"30s"`
	IdleTimeout  time.Duration `env:"IDLE_TIMEOUT" envDefault:"60s"`

	// Feature flags for mock integrations
	LiveDuneMock bool `env:"LIVEDUNE_MOCK" envDefault:"false"`
	TrustMeMock  bool `env:"TRUSTME_MOCK" envDefault:"false"`
	TelegramMock bool `env:"TELEGRAM_MOCK" envDefault:"false"`
	EmailMock    bool `env:"EMAIL_MOCK" envDefault:"false"`
	StorageMock  bool `env:"STORAGE_MOCK" envDefault:"false"`

	// Derived (not from env vars)
	CookieSecure        bool `env:"-"`
	EnableTestEndpoints bool `env:"-"`
}

// Load reads configuration from environment variables.
// Returns an error if a required variable is missing or a value cannot be parsed.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	switch cfg.Environment {
	case EnvLocal, EnvStaging, EnvProduction:
		// valid
	default:
		return nil, fmt.Errorf("invalid ENVIRONMENT %q: must be one of local, staging, production", cfg.Environment)
	}

	// Derive values from Environment
	cfg.CookieSecure = cfg.Environment != EnvLocal
	cfg.EnableTestEndpoints = cfg.Environment == EnvLocal

	return cfg, nil
}
