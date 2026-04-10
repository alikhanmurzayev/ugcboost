# Безопасность [CRITICAL]

Безопасность — не фича, а свойство системы. Secure by default, не secure by configuration.

---

## CS-38: Никаких "insecure" флагов — безопасность через Environment enum

**Scope:** backend

**Почему:** `main.go:99-102` вычисляет `isSecure` по первому CORS origin. Ошибка в `CORS_ORIGINS` → cookie без `Secure` flag → refresh token утекает по HTTP.

**Плохо:**
```go
// main.go:99-102 — хрупкая логика
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

// Поведение определяется от окружения явно:
cookieSecure := cfg.Environment != EnvLocal
enableTestEndpoints := cfg.Environment == EnvLocal
debugLogging := cfg.Environment == EnvLocal
```

**Правило:**
- Окружение задаётся через `ENVIRONMENT` env var (`local`, `staging`, `production`)
- Поведение, зависящее от окружения, определяется от этого enum
- Никаких `insecure`, `disable_security`, `skip_auth` флагов
- Production — всегда максимально secure, без возможности ослабить через конфиг

---

## CS-39: Логирование — никогда не логировать чувствительные данные

**Scope:** backend

**Почему:** Пароли, JWT-токены, ИИН, персональные данные в логах = утечка. Логи часто попадают в системы мониторинга (Grafana, ELK) с широким доступом.

**Текущее состояние:** logging middleware (`middleware/logging.go`) логирует только метод, путь, статус, длительность — это корректно.

**Запрещено логировать:**
- Request body (может содержать пароли, токены)
- Response body (может содержать access tokens)
- HTTP-заголовок `Authorization`
- Cookie `refresh_token`
- Персональные данные: ИИН, телефон, email (в связке с другими данными)
- Любые секреты: JWT_SECRET, DATABASE_DSN и т.д.

**Допустимо логировать:**
- HTTP method, path, status code, duration
- User ID (UUID, не email)
- Request ID / trace ID
- IP-адрес (для аудита)
- Количество результатов (не сами данные)
- Error messages (без sensitive context)

**Правило:** при добавлении логирования — проверить что ни один `slog.*` вызов не содержит чувствительных данных. В code review обращать особое внимание на `slog.Any`, `slog.String` с аргументами из request body.

---

## CS-40: Пароли и секреты — только в env vars, не в коде

**Scope:** both

**Текущее состояние:** уже соблюдается (JWT_SECRET, DATABASE_URL из env vars). Правило зафиксировано для полноты.

**Запрещено:**
```go
// НИКОГДА не делать:
const jwtSecret = "my-secret-key"
const dbPassword = "postgres123"
```

**Правило:**
- Все секреты — через env vars
- В `.env` файлах — только для локальной разработки, `.env` в `.gitignore`
- В CI/CD — через GitHub Secrets
- В production — через Dokploy env vars
- При обнаружении секрета в коде — немедленно ротировать и исправить
