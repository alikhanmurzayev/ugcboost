package config

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Environment values.
const (
	EnvLocal      = "local"
	EnvStaging    = "staging"
	EnvProduction = "production"
)

// Config holds all application configuration, loaded from environment variables.
type Config struct {
	Environment string        `env:"ENVIRONMENT" envDefault:"local"`
	Port        int           `env:"PORT" envDefault:"8080"`
	DatabaseURL string        `env:"DATABASE_URL,required"`
	JWTSecret   string        `env:"JWT_SECRET,required"`
	JWTExpiry   time.Duration `env:"JWT_EXPIRY" envDefault:"15m"`
	CORSOrigins []string      `env:"CORS_ORIGINS" envDefault:"http://localhost:5173,http://localhost:5174" envSeparator:","`
	LogLevel    slog.Level    `env:"LOG_LEVEL" envDefault:"INFO"`

	// Admin seed
	AdminEmail    string `env:"ADMIN_EMAIL" envDefault:"admin@ugcboost.kz"`
	AdminPassword string `env:"ADMIN_PASSWORD"`

	// Securityв
	BcryptCost    int           `env:"BCRYPT_COST" envDefault:"12"`
	RefreshExpiry time.Duration `env:"REFRESH_EXPIRY" envDefault:"168h"`
	ResetExpiry   time.Duration `env:"RESET_EXPIRY" envDefault:"1h"`

	// HTTP server
	ReadTimeout     time.Duration `env:"READ_TIMEOUT" envDefault:"10s"`
	WriteTimeout    time.Duration `env:"WRITE_TIMEOUT" envDefault:"30s"`
	IdleTimeout     time.Duration `env:"IDLE_TIMEOUT" envDefault:"60s"`
	BodyLimitBytes  int           `env:"BODY_LIMIT_BYTES" envDefault:"1048576"` // 1 MB
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"10s"`

	// Pagination
	DefaultPerPage int `env:"DEFAULT_PER_PAGE" envDefault:"20"`

	// Build-time version stamped into the binary via Dockerfile ARG→ENV.
	// Surfaced by /healthz so ops can confirm which commit is running.
	Version string `env:"GIT_COMMIT" envDefault:"dev"`

	// Feature flags for mock integrations
	LiveDuneMock bool `env:"LIVEDUNE_MOCK" envDefault:"false"`
	TrustMeMock  bool `env:"TRUSTME_MOCK" envDefault:"false"`
	TelegramMock bool `env:"TELEGRAM_MOCK" envDefault:"false"`
	EmailMock    bool `env:"EMAIL_MOCK" envDefault:"false"`
	StorageMock  bool `env:"STORAGE_MOCK" envDefault:"false"`

	// Telegram bot — used when assembling deep-link returned to creators
	// after a public application is accepted (https://t.me/<username>?start=<application_id>).
	// Different bot per environment.
	TelegramBotUsername string `env:"TELEGRAM_BOT_USERNAME,required"`

	// Versions of the legal documents the user accepts at submission time.
	// Stored alongside each consent row so that a future audit can show
	// exactly which revision was in force.
	LegalAgreementVersion string `env:"LEGAL_AGREEMENT_VERSION,required"`
	LegalPrivacyVersion   string `env:"LEGAL_PRIVACY_VERSION,required"`

	// Derived (not from env vars)
	CookieSecure        bool `env:"-"`
	EnableTestEndpoints bool `env:"-"`
}

// Load reads configuration from environment variables.
// If a .env file exists in the working directory, it is loaded first (env vars take precedence).
// Returns an error if a required variable is missing or a value cannot be parsed.
func Load() (*Config, error) {
	// Load .env if present (ignore if missing — Docker/CI set env vars directly).
	if _, err := os.Stat(".env"); err == nil {
		_ = godotenv.Load()
	}

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
	cfg.EnableTestEndpoints = cfg.Environment != EnvProduction

	// envDefault only applies when the variable is unset; an explicit empty
	// string from the runtime (misconfigured Dokploy entry, empty build arg)
	// bypasses it, so guard against that here.
	if cfg.Version == "" {
		cfg.Version = "dev"
	}

	return cfg, nil
}
