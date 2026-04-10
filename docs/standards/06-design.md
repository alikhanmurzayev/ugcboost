# Архитектура и проектирование [REQUIRED]

Принципы проектирования, которые обеспечивают поддерживаемость, тестируемость и надёжность кода.

---

## CS-24: Все зависимости — через конструктор, никаких Set*

**Scope:** backend

**Почему:** `SetAuditor()` (`handler/auth.go:39`, `handler/brand.go:40`) создаёт temporal coupling — порядок вызовов важен, зависимость может быть nil, нужны проверки `if h.auditor != nil` в каждом месте использования.

**Плохо:**
```go
// handler/auth.go
type AuthHandler struct {
    auth    Auth
    auditor Auditor  // может быть nil!
    secure  bool
}

func NewAuthHandler(auth Auth, secure bool) *AuthHandler {
    return &AuthHandler{auth: auth, secure: secure}
}

func (h *AuthHandler) SetAuditor(a Auditor) { h.auditor = a }

// В каждом методе — проверка на nil:
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
    // ...
    if h.auditor != nil {  // забыл проверить = panic
        h.auditor.Log(ctx, entry)
    }
}
```

**Хорошо:**
```go
type AuthHandler struct {
    authService  AuthService
    auditService AuditService
    secure       bool
}

// ВСЕ зависимости — в конструкторе. Структура иммутабельна после создания.
func NewAuthHandler(authService AuthService, auditService AuditService, secure bool) *AuthHandler {
    return &AuthHandler{
        authService:  authService,
        auditService: auditService,
        secure:       secure,
    }
}

// Никаких проверок на nil — зависимость гарантированно есть
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
    // ...
    h.auditService.Log(ctx, entry)
}
```

**Правило:** все зависимости инжектируются через `New*()`. Метод `Set*()` для зависимостей запрещён. Структура иммутабельна после создания.

---

## CS-25: Конфигурируемые значения — в Config

**Scope:** backend

**Почему:** Hardcoded значения, которые могут отличаться между окружениями или которые нужно тюнить:
- bcrypt cost = 12 (`service/auth.go:15`)
- shutdown timeout = 10s (`main.go:194`)
- refresh token TTL = 7 days (`token.go:25`)

**Плохо:**
```go
// service/auth.go:15
var bcryptCost = 12  // hardcoded, var только для тестов

// main.go:194
shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)  // hardcoded

// token.go:25
const refreshTokenTTL = 7 * 24 * time.Hour  // hardcoded
```

**Хорошо:**
```go
type Config struct {
    BcryptCost      int           `env:"BCRYPT_COST" envDefault:"12"`
    ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"10s"`
    RefreshTokenTTL time.Duration `env:"REFRESH_TOKEN_TTL" envDefault:"168h"` // 7 days
    // ...
}
```

**Правило:** всё, что может отличаться между local/staging/production или что может потребовать тюнинга без релиза, должно быть в Config и управляться через env vars.

---

## CS-26: Environment — явный enum вместо вычисления из CORS

**Scope:** backend

**Почему:** `main.go:99-102` вычисляет `isSecure` по первому CORS origin — хрупкая логика. Ошибка в CORS_ORIGINS → cookie без Secure → утечка refresh token.

**Плохо:**
```go
isSecure := true
if len(cfg.CORSOrigins) > 0 {
    isSecure = !strings.HasPrefix(cfg.CORSOrigins[0], "http://localhost")
}
```

**Хорошо:**
```go
type Environment string

const (
    EnvLocal      Environment = "local"
    EnvStaging    Environment = "staging"
    EnvProduction Environment = "production"
)

type Config struct {
    Environment Environment `env:"ENVIRONMENT" envDefault:"local"`
    // ...
}

// Поведение определяется явно от окружения:
isSecure := cfg.Environment != EnvLocal
```

**Правило:** окружение определяется явным `ENVIRONMENT` env var. Поведение (secure cookies, debug logging, test endpoints) зависит от него напрямую, а не вычисляется косвенно.

---

## CS-27: domain пакет — только бизнес-ошибки и бизнес-константы

**Scope:** backend

**Почему:** `domain/brand.go` содержит структуры с JSON-тегами — это модели API, не доменные сущности. Смешение приводит к неясности: кто владеет типами?

**Плохо:**
```go
// domain/brand.go — структура с JSON-тегами в доменном пакете
type Brand struct {
    ID        string  `json:"id"`
    Name      string  `json:"name"`
    LogoURL   *string `json:"logoUrl"`
}
```

**Хорошо — разделение ответственности:**
```
api/server.gen.go     → API-модели (request/response), сгенерированные из OpenAPI
domain/errors.go      → бизнес-ошибки (ErrNotFound, ErrForbidden, ValidationError)
domain/constants.go   → бизнес-константы (коды ошибок, статусы)
repository/user.go    → DB-модели (UserRow с тегами db:"...")
```

**Правило:** 
- `domain/` = ошибки + константы + бизнес-типы без привязки к HTTP или SQL
- API-модели (JSON) → `api/` (кодогенерация из OpenAPI)
- DB-модели (Row structs) → `repository/` (маппинг на таблицы)

---

## CS-28: DatabaseURL → DatabaseDSN

**Scope:** backend

**Почему:** `pgxpool.New()` принимает connection string (DSN), а не URL. Неточное именование вводит в заблуждение.

**Плохо:**
```go
DatabaseURL string  // config.go:13 — выглядит как http://... URL
```

**Хорошо:**
```go
DatabaseDSN string `env:"DATABASE_DSN,required"` // точное название
```

**Правило:** именование переменных точно отражает содержимое. DSN — это DSN, не URL. Token — это token, не key.
