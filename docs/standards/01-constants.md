# Константы вместо строковых литералов [CRITICAL]

Строковые литералы для enum-подобных значений — **критическая ошибка**. Опечатка в строке — это silent bug: компилятор не поймает, тесты могут не покрыть, баг всплывёт в проде.

---

## CS-01: Роли пользователей — только через константы из кодогенерации

**Scope:** backend

**Почему:** Опечатка `"admim"` вместо `"admin"` — пользователь с ролью admin не пройдёт проверку. Компилятор молчит, IDE не подсветит.

**Плохо** (найдено в 12+ местах):
```go
// handler/brand.go:46, handler/audit.go:32, authz/authz.go:12,
// service/brand.go:65, service/auth.go:239...
if role != "admin" {
    respondError(w, http.StatusForbidden, "FORBIDDEN", "admin only")
    return
}
```

**Хорошо:**
```go
// Константы уже сгенерированы в api/server.gen.go:23-25
// api.Admin = "admin", api.BrandManager = "brand_manager"
if role != api.Admin {
    respondError(w, http.StatusForbidden, domain.CodeForbidden, "admin only")
    return
}
```

**Правило:** все сравнения с ролями используют константы из `api/server.gen.go`. Если роль не определена в OpenAPI — добавить в спецификацию и перегенерировать.

---

## CS-02: Коды ошибок — константы в пакете domain

**Scope:** backend

**Почему:** `"VALIDATION_ERROR"` встречается 15+ раз как строковый литерал. Одна опечатка — фронтенд не распознает код ошибки и не покажет правильное сообщение пользователю.

**Плохо** (найдено в 25+ местах):
```go
// handler/auth.go:49, handler/brand.go:56, middleware/auth.go:30,
// service/brand.go:53, handler/test.go:49...
respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "email is required")
```

**Хорошо:**
```go
// domain/errors.go — единое место определения
const (
    CodeValidation  = "VALIDATION_ERROR"
    CodeNotFound    = "NOT_FOUND"
    CodeForbidden   = "FORBIDDEN"
    CodeUnauthorized = "UNAUTHORIZED"
    CodeConflict    = "CONFLICT"
    CodeInternal    = "INTERNAL_ERROR"
)

// Использование:
respondError(w, http.StatusUnprocessableEntity, domain.CodeValidation, "email is required")
```

**Правило:** коды ошибок определяются один раз в `domain/errors.go` как константы. Все хендлеры, middleware, сервисы используют эти константы. Строковые литералы для кодов ошибок запрещены.

---

## CS-03: HTTP-заголовки и cookie — константы

**Scope:** backend

**Почему:** Имя cookie `"refresh_token"` используется в 3 местах (`handler/auth.go:88,131,233`). Переименовали в одном — забыли в другом = refresh не работает.

**Плохо:**
```go
// handler/auth.go:88
http.SetCookie(w, &http.Cookie{Name: "refresh_token", ...})

// handler/auth.go:131
cookie, err := r.Cookie("refresh_token")

// handler/auditor.go:19
ip := r.Header.Get("X-Forwarded-For")
```

**Хорошо:**
```go
// Определить в одном месте (например, internal/httputil/constants.go)
const (
    CookieRefreshToken  = "refresh_token"
    HeaderXForwardedFor = "X-Forwarded-For"
    HeaderXRealIP       = "X-Real-IP"
    HeaderAuthorization = "Authorization"
)

// Использование:
http.SetCookie(w, &http.Cookie{Name: httputil.CookieRefreshToken, ...})
cookie, err := r.Cookie(httputil.CookieRefreshToken)
ip := r.Header.Get(httputil.HeaderXForwardedFor)
```

**Правило:** все имена HTTP-заголовков, cookie и подобные строки-идентификаторы — через константы. Стандартные заголовки (Content-Type) допустимо использовать через `net/http` или `textproto`.

---

## CS-04: Названия таблиц и колонок в SQL — константы

**Scope:** backend

**Почему:** Опечатка `"audit_logss"` или `"actorid"` — runtime ошибка при запросе к БД, а не compile-time.

**Плохо** (найдено во всех файлах repository/):
```go
// repository/audit.go:50
q := dbutil.Psql.Insert("audit_logs").
    Columns("actor_id", "actor_role", "action", "entity_type").
    Values(e.ActorID, e.ActorRole, e.Action, e.EntityType)

// repository/user.go:55
q := dbutil.Psql.Select("id", "email", "password_hash", "role").
    From("users").Where("email = ?", email)
```

**Хорошо:**
```go
// repository/audit.go — константы в начале файла
const (
    TableAuditLogs       = "audit_logs"
    ColAuditID           = "id"
    ColAuditActorID      = "actor_id"
    ColAuditActorRole    = "actor_role"
    ColAuditAction       = "action"
    ColAuditEntityType   = "entity_type"
    ColAuditEntityID     = "entity_id"
    ColAuditOldValue     = "old_value"
    ColAuditNewValue     = "new_value"
    ColAuditIPAddress    = "ip_address"
    ColAuditCreatedAt    = "created_at"
)

q := dbutil.Psql.Insert(TableAuditLogs).SetMap(map[string]any{
    ColAuditActorID:    e.ActorID,
    ColAuditActorRole:  e.ActorRole,
    ColAuditAction:     e.Action,
    ColAuditEntityType: e.EntityType,
})
```

**Исключение для тестов (CS-33):** в тестах репозиториев строковые литералы ОБЯЗАТЕЛЬНЫ — это двойная проверка, что константы в коде корректны.

**Правило:** каждый репозиторий определяет экспортированные константы для своей таблицы и колонок. Названия колонок извлекать из struct tags через `stom` или аналогичный пакет (см. CS-30).

---

## CS-05: Значения конфигурации с перечислением — типизированные константы

**Scope:** backend

**Почему:** Передал `"deubg"` вместо `"debug"` в LOG_LEVEL — молча работает на дефолтном уровне. Никакой ошибки.

**Плохо:**
```go
// config/config.go — log level как обычная строка без валидации
LogLevel string
```

**Хорошо:**
```go
type LogLevel string

const (
    LogDebug LogLevel = "debug"
    LogInfo  LogLevel = "info"
    LogWarn  LogLevel = "warn"
    LogError LogLevel = "error"
)

// Валидация при загрузке конфига:
func parseLogLevel(s string) (LogLevel, error) {
    switch LogLevel(s) {
    case LogDebug, LogInfo, LogWarn, LogError:
        return LogLevel(s), nil
    default:
        return "", fmt.Errorf("invalid log level: %q, allowed: debug, info, warn, error", s)
    }
}
```

**Правило:** любое значение с конечным набором допустимых вариантов — типизированная константа + валидация при загрузке.
