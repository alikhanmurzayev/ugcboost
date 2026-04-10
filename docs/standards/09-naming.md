# Нейминг и стиль [STANDARD]

Консистентный нейминг — основа читаемости. Код читают в 10 раз чаще, чем пишут.

---

## CS-35: Зависимости в struct — с суффиксом типа слоя

**Scope:** backend

**Почему:** `auth Auth` — непонятно, это сервис, репозиторий или что-то ещё. Через месяц при чтении кода приходится лезть в определение интерфейса.

**Плохо:**
```go
type AuthHandler struct {
    auth   Auth     // что это? сервис? репозиторий?
    brands Brands   // тоже непонятно
    audit  AuditLogs
}
```

**Хорошо:**
```go
type AuthHandler struct {
    authService  AuthService
    brandService BrandService
    auditService AuditService
}

type BrandService struct {
    brandRepo BrandRepo
    userRepo  UserRepo
}
```

**Правило:**
- В handler: зависимости с суффиксом `Service` (authService, brandService)
- В service: зависимости с суффиксом `Repo` (brandRepo, userRepo)
- Интерфейсы именуются соответственно: `AuthService`, `BrandRepo`

---

## CS-36: TODO в коде — только с номером issue

**Scope:** both

**Почему:** `authz/authz.go:21` содержит TODO "check real ownership" — никто никогда не вернётся. TODO без трекинга — мёртвый комментарий, который создаёт ложное ощущение "мы это помним".

**Плохо:**
```go
// TODO: check real ownership when creator endpoints exist
// TODO: add rate limiting
// FIXME: this is a hack
```

**Хорошо:**
```go
// TODO(#42): check real ownership when creator endpoints exist
// TODO(#78): add rate limiting for auth endpoints
```

**Правило:**
1. Прежде чем написать TODO — создать issue в GitHub
2. В комментарии указать номер issue: `TODO(#N)`
3. TODO без номера issue запрещены
4. При закрытии issue — удалить TODO из кода

---

## CS-37: Комментарии — на английском, кратко, объясняют "почему"

**Scope:** both

**Почему:** Код и комментарии на одном языке (английском) — единообразие. Комментарии, описывающие "что" делает код, бесполезны — это видно из самого кода.

**Плохо:**
```go
// Check if the user is an admin
if role != api.Admin {

// Create a new brand
brand, err := s.brandRepo.Create(ctx, name, logoURL)

// Создаём запись в аудит-логе (русский в коде)
```

**Хорошо:**
```go
// bcrypt cost 12 gives ~250ms hash time, balancing security vs latency
var bcryptCost = 12

// Atomic delete+return prevents token reuse in concurrent requests
row, err := dbutil.One[RefreshTokenRow](ctx, r.db, q)

// Squirrel doesn't support RETURNING with SetMap, using raw suffix
q = q.Suffix("RETURNING id, created_at")
```

**Правило:**
- Комментарии на английском
- Не комментировать очевидное
- Комментировать **почему** принято такое решение, а не **что** делает код
- Godoc-комментарии для экспортированных функций/типов обязательны
