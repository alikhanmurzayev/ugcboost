# Слои архитектуры [REQUIRED]

Бэкенд организован по слоям: **handler → service → repository**. Каждый слой имеет чёткую ответственность. Нарушение границ слоёв приводит к дублированию логики, невозможности тестирования и цепным багам.

---

## CS-15: Авторизация — отдельный сервис, не размазывать по хендлерам

**Scope:** backend

**Почему:** `if role != "admin"` встречается в 6+ хендлерах. Это дублирование: забудут добавить проверку в новый хендлер — дыра безопасности. Плюс авторизационная логика смешивается с бизнес-логикой хендлера.

**Плохо** (handler/brand.go:46, handler/audit.go:32, и ещё 4 места):
```go
func (h *BrandHandler) CreateBrand(w http.ResponseWriter, r *http.Request) {
    role, _ := middleware.RoleFromContext(r.Context())
    if role != "admin" {  // дублирование + строковый литерал
        respondError(w, http.StatusForbidden, "FORBIDDEN", "admin only")
        return
    }
    // бизнес-логика...
}
```

**Ещё хуже** (authz/authz.go:26-42):
```go
// authz пакет ходит напрямую в БД, минуя repository
func CanManageBrand(ctx context.Context, db dbutil.DB, userID, role, brandID string) error {
    if role == "admin" { return nil }
    q := dbutil.Psql.Select("1").From("brand_managers").
        Where("user_id = ? AND brand_id = ?", userID, brandID)
    _, err := dbutil.Val[int](ctx, db, q)  // прямой SQL!
    // ...
}
```

**Хорошо:**
```go
// internal/service/authz.go — отдельный сервис авторизации
type AuthzService struct {
    brandRepo BrandRepository  // зависимость через интерфейс
}

func (s *AuthzService) RequireAdmin(ctx context.Context) error {
    role := middleware.RoleFromContext(ctx)
    if role != api.Admin {
        return domain.ErrForbidden
    }
    return nil
}

func (s *AuthzService) RequireBrandAccess(ctx context.Context, brandID string) error {
    userID := middleware.UserIDFromContext(ctx)
    role := middleware.RoleFromContext(ctx)
    if role == api.Admin {
        return nil
    }
    isManager, err := s.brandRepo.IsManager(ctx, userID, brandID)
    if err != nil {
        return fmt.Errorf("authz check: %w", err)
    }
    if !isManager {
        return domain.ErrForbidden
    }
    return nil
}

// В хендлере — одна строка:
func (h *BrandHandler) CreateBrand(w http.ResponseWriter, r *http.Request) {
    if err := h.authzService.RequireAdmin(r.Context()); err != nil {
        respondError(w, err)
        return
    }
    // бизнес-логика...
}
```

**Правило:** вся авторизационная логика инкапсулирована в `AuthzService`. Хендлеры вызывают один метод авторизации. Прямые сравнения ролей в хендлерах запрещены.

---

## CS-16: Репозиторий — единственный слой, который работает с БД

**Scope:** backend

**Почему:** `authz/authz.go:32` строит SQL запрос напрямую через `dbutil.DB`, дублируя `repository/brand.go:162` (`IsManager()`). Два места для одного запроса = два места для бага.

**Правило:** 
- **ТОЛЬКО** пакет `repository` имеет право строить SQL-запросы и обращаться к `dbutil.DB`
- Все остальные пакеты (service, authz, handler) работают с данными через интерфейсы репозиториев
- Если сервису нужны данные из БД — он вызывает метод репозитория

---

## CS-17: Направление зависимостей: handler → service → repository

**Scope:** backend

**Почему:** Нарушение направления зависимостей приводит к циклическим импортам, невозможности мокать слои в тестах, и спагетти-коду.

```
handler  →  service  →  repository  →  dbutil/DB
   ↓           ↓            ↓
 (HTTP)   (бизнес)     (SQL/data)
```

**Правила:**
- handler зависит от service interfaces — **никогда** от repository напрямую
- service зависит от repository interfaces — **никогда** от handler
- repository зависит от `dbutil.DB` interface — **никогда** от service или handler
- Интерфейсы определяются в пакете-потребителе (Go convention: "accept interfaces, return structs")

---

## CS-18: Валидация — один раз на уровне хендлера

**Scope:** backend

**Почему:** Проверка `name == ""` и в хендлере, и в сервисе (`service/brand.go:51`) — дублирование. При изменении правила валидации надо менять в двух местах.

**Плохо:**
```go
// handler/brand.go — валидация в хендлере
if req.Name == "" {
    respondError(w, 422, "VALIDATION_ERROR", "name is required")
    return
}
brand, err := h.brandService.Create(ctx, req.Name, req.LogoURL)

// service/brand.go:51 — та же валидация в сервисе (дублирование!)
func (s *BrandService) Create(ctx context.Context, name string, ...) (*Brand, error) {
    if name == "" {
        return nil, domain.NewValidationError("VALIDATION_ERROR", "brand name is required")
    }
}
```

**Хорошо:**
```go
// handler — валидирует входные данные (формат, обязательность, границы)
func (h *BrandHandler) CreateBrand(w http.ResponseWriter, r *http.Request) {
    var req api.CreateBrandJSONRequestBody
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil { ... }
    if req.Name == "" { respondError(...) }  // валидация здесь

    brand, err := h.brandService.Create(ctx, req)  // сервис доверяет входу
}

// service — только бизнес-логика, входные данные уже провалидированы
func (s *BrandService) Create(ctx context.Context, req api.CreateBrandRequest) (*Brand, error) {
    // Никакой валидации формата! Только бизнес-правила:
    // - уникальность имени (проверка в БД)
    // - лимиты (не более N брендов)
}
```

**Правило:** хендлер валидирует формат и обязательность. Сервис содержит только бизнес-правила (уникальность, лимиты, бизнес-ограничения). Дублирование валидации запрещено.
