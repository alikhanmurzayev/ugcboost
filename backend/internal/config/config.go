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
	BodyLimitBytes  int           `env:"BODY_LIMIT_BYTES" envDefault:"5242880"` // 5 MB — accommodates raw-PDF uploads on PUT /campaigns/{id}/contract-template (Google-Docs export typical 100 KB – 2 MB; legal-style scans up to ~3 MB).
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"10s"`

	// Pagination
	DefaultPerPage int `env:"DEFAULT_PER_PAGE" envDefault:"20"`

	// Build-time version stamped into the binary via Dockerfile ARG→ENV.
	// Surfaced by /healthz so ops can confirm which commit is running.
	Version string `env:"GIT_COMMIT" envDefault:"dev"`

	// Feature flags for mock integrations
	LiveDuneMock bool `env:"LIVEDUNE_MOCK" envDefault:"false"`
	// TrustMeMock — true → SpyOnlyClient (default local + staging). false →
	// RealClient (default prod). На staging вручную меняем для one-off
	// real-теста с прод-токеном (см. project_trustme_no_sandbox memory).
	TrustMeMock bool `env:"TRUSTME_MOCK" envDefault:"true"`
	EmailMock   bool `env:"EMAIL_MOCK" envDefault:"false"`
	StorageMock bool `env:"STORAGE_MOCK" envDefault:"false"`

	// TrustMe HTTP integration. BaseURL без trailing slash. Token — статичный
	// per-окружение (см. blueprint.apib §2: «Authorization: {token}» без
	// Bearer-префикса). Для local/staging значения не обязательны (spy не
	// ходит в сеть); для прода RealClient их валидирует при старте.
	TrustMeBaseURL string `env:"TRUSTME_BASE_URL" envDefault:"https://test.trustme.kz/trust_contract_public_apis"`
	TrustMeToken   string `env:"TRUSTME_TOKEN" envDefault:""`
	// TrustMeWebhookToken — статичный токен, который TrustMe шлёт в
	// заголовке `Authorization: <token>` (raw, без Bearer-префикса) при
	// доставке webhook'а — формат жёстко прописан в blueprint § «Установка
	// хуков». Constant-time compare в middleware.TrustMeWebhookAuth. Для
	// staging/prod обязателен (Load() валидирует не-локальные окружения),
	// локально — допустим пустой (e2e через test-окружение задаёт явно).
	TrustMeWebhookToken string `env:"TRUSTME_WEBHOOK_TOKEN" envDefault:""`
	// TrustMeKzBmg — feature-flag для prod-trial проверки TrustMe «База
	// мобильных граждан»: если true, в payload SendToSign передаётся
	// `KzBmg=true`, и TrustMe сверяет связку ИИН↔Phone по госреестру до
	// отправки SMS. Default false: фича требует подключения у TrustMe и
	// прогоняется one-off на проде, не должна ломать local/staging.
	TrustMeKzBmg bool `env:"TRUSTME_KZ_BMG" envDefault:"false"`
	// TrustMeRetryBackoffSeconds — пауза перед повторной попыткой Phase 0
	// recovery, если предыдущий SendToSign упал. Константный, не
	// экспоненциальный. Default 300 (5 минут): достаточно длинно, чтобы
	// кривой orphan (плохой ИИН, 1219 и т.п.) не блокировал слоты для
	// свежих контрактов; достаточно коротко, чтобы транзиентные сбои
	// сами догонялись в обозримое время.
	TrustMeRetryBackoffSeconds int `env:"TRUSTME_RETRY_BACKOFF_SECONDS" envDefault:"300"`

	// Telegram bot — used when assembling deep-link returned to creators
	// after a public application is accepted (https://t.me/<username>?start=<application_id>).
	// Different bot per environment.
	TelegramBotUsername string `env:"TELEGRAM_BOT_USERNAME,required"`

	// Bot token from @BotFather. When set, the runtime starts long-polling
	// against Telegram and the outbound sender delivers to real users; when
	// empty, both are skipped — useful for local/CI where EnableTestEndpoints
	// drives a parallel fake bot. Doubles as the HMAC key for /tma/* initData
	// verification — same secret, different scope.
	TelegramBotToken string `env:"TELEGRAM_BOT_TOKEN" envDefault:""`

	// Freshness window for Telegram WebApp initData. Older / future-dated
	// signatures get a generic 401 (anti-fingerprint) before any DB lookup.
	// Default 24h matches the official WebApp expiration; staging may dial
	// it down for faster manual replay-attack drills.
	//
	// Use Go time.Duration syntax — `24h`, `1h30m`, `90s`. Bare integers
	// like `86400` are NOT valid (caarlos0/env parses through
	// time.ParseDuration which would treat them as nanoseconds).
	TMAInitDataTTL time.Duration `env:"TMA_INITDATA_TTL" envDefault:"24h"`

	// Bearer secret SendPulse signs the IG webhook with. Constant-time
	// compared in middleware before the handler runs.
	SendPulseWebhookSecret string `env:"SENDPULSE_WEBHOOK_SECRET,required"`

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

	// envconfig's `required` tag treats `KEY=` (set to empty) as satisfied;
	// for security-critical secrets that's an open-auth bypass on misconfig.
	// Re-validate non-empty here so a deploy with KEY= refuses to start.
	if cfg.SendPulseWebhookSecret == "" {
		return nil, fmt.Errorf("SENDPULSE_WEBHOOK_SECRET must be a non-empty value")
	}
	// Refuse to boot a deployment with no Telegram surface at all: without
	// either a real bot token or test endpoints, services that reach for the
	// notifier would fall back to a no-op silently. Surfacing the misconfig
	// up front is cheaper than chasing a "creator never got the message"
	// ticket in production.
	if cfg.TelegramBotToken == "" && cfg.Environment == EnvProduction {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN must be set in production")
	}
	// TrustMe webhook token guard — required outside local окружения. На
	// staging/prod TrustMe доставляет webhook'и с этим токеном; пустой
	// токен дал бы открытый endpoint без auth. Локально пустой допустим
	// (e2e тесты задают явно через docker env).
	if cfg.Environment != EnvLocal && cfg.TrustMeWebhookToken == "" {
		return nil, fmt.Errorf("TRUSTME_WEBHOOK_TOKEN must be a non-empty value outside local environment")
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
