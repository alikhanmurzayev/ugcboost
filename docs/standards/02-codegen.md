# Использование кодогенерации [REQUIRED]

Мы используем contract-first подход: OpenAPI YAML → кодогенерация для Go (oapi-codegen), TypeScript (openapi-typescript), E2E-клиентов. Весь сгенерированный код **должен использоваться**. Если мы генерируем типы и не используем их — зачем генерируем?

---

## CS-06: Регистрация роутов — через сгенерированный HandlerFromMux

**Scope:** backend

**Почему:** Ручная регистрация 15+ роутов в main.go дублирует пути из OpenAPI spec. При 100+ эндпоинтах файл станет нечитаемым, а рассинхрон путей неизбежен.

**Плохо** (main.go:119-146):
```go
// Каждый роут вручную — дублирование OpenAPI spec
r.Post("/auth/login", authHandler.Login)
r.Post("/auth/refresh", authHandler.Refresh)
r.Post("/auth/password-reset-request", authHandler.RequestPasswordReset)
r.Route("/brands", func(r chi.Router) {
    r.Post("/", brandHandler.CreateBrand)
    r.Get("/", brandHandler.ListBrands)
    r.Get("/{brandID}", brandHandler.GetBrand)
    // ... ещё 10 строк
})
```

**Хорошо:**
```go
// Композитная структура реализует api.ServerInterface
type Server struct {
    auth   *handler.AuthHandler
    brand  *handler.BrandHandler
    audit  *handler.AuditHandler
    health *handler.HealthHandler
}

// Делегирует вызовы в конкретные хендлеры
func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
    s.auth.Login(w, r)
}

// Одна строка вместо 30:
r.Mount("/api", api.HandlerFromMux(server, r))
```

**Правило:** роуты регистрируются ТОЛЬКО через `api.HandlerFromMux()`. Ручная регистрация через `r.Get/Post/Route` запрещена для API-эндпоинтов. Исключение: health check и test endpoints.

---

## CS-07: Типы запросов/ответов — только из кодогенерации

**Scope:** backend + frontend

**Почему:** Анонимная структура в хендлере может разойтись с контрактом. Фронтенд отправит одно, бэкенд ожидает другое — 500 или молчаливый баг.

**Плохо** (найдено в 8 местах handler/*.go):
```go
// handler/auth.go:44-47
var req struct {
    Email    string `json:"email"`
    Password string `json:"password"`
}

// handler/brand.go:51-54
var req struct {
    Name    string  `json:"name"`
    LogoURL *string `json:"logoUrl"`
}
```

**Хорошо:**
```go
// Использовать сгенерированные типы из api/server.gen.go
var req api.LoginJSONRequestBody
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    // ...
}

var req api.CreateBrandJSONRequestBody
```

**Для фронтенда** (web/src/api/auth.ts:3-7 — ручные интерфейсы вместо generated):
```typescript
// ПЛОХО: ручное определение
interface User {
    id: string;
    email: string;
    role: "admin" | "brand_manager";
}

// ХОРОШО: импорт из сгенерированных типов
import type { components } from './generated/schema';
type User = components['schemas']['User'];
```

**Правило:** запрещено объявлять структуры/интерфейсы для API request/response вручную. Только сгенерированные типы.

---

## CS-08: Query-параметры — через сгенерированные структуры

**Scope:** backend

**Почему:** Ручной парсинг `r.URL.Query().Get("page")` + `strconv.Atoi` — 10 строк boilerplate на каждый query param. Каждая строка — потенциальная опечатка или пропущенная валидация.

**Плохо** (handler/audit.go:37-66):
```go
q := r.URL.Query()
actorID := q.Get("actor_id")
entityType := q.Get("entity_type")
page, _ := strconv.Atoi(q.Get("page"))       // ошибка парсинга игнорируется!
perPage, _ := strconv.Atoi(q.Get("per_page")) // ошибка парсинга игнорируется!
if page < 1 { page = 1 }                      // тихий fallback вместо ошибки
```

**Хорошо:**
```go
// При использовании api.ServerInterface, wrapper автоматически парсит params:
func (s *Server) ListAuditLogs(w http.ResponseWriter, r *http.Request, params api.ListAuditLogsParams) {
    // params.Page, params.PerPage, params.ActorId и т.д. уже распарсены и типизированы
    // Валидация — на уровне хендлера, но парсинг автоматический
}
```

**Правило:** query-параметры парсятся ServerInterfaceWrapper автоматически. Ручной `r.URL.Query().Get()` запрещён для API-эндпоинтов.

---

## CS-09: Path-параметры — через сгенерированный wrapper

**Scope:** backend

**Почему:** `chi.URLParam(r, "brandID")` — hardcoded строка. Если в OpenAPI параметр переименуют в `brand_id`, код молча сломается.

**Плохо** (handler/brand.go:109):
```go
brandID := chi.URLParam(r, "brandID")
```

**Хорошо:**
```go
// ServerInterface метод уже получает brandID как аргумент:
func (s *Server) GetBrand(w http.ResponseWriter, r *http.Request, brandID string) {
    // brandID извлечён wrapper'ом, валидация в одном месте
}
```

**Правило:** path-параметры извлекаются ServerInterfaceWrapper и передаются как аргументы методов. Ручной `chi.URLParam()` запрещён для API-эндпоинтов.

---

## CS-10: Фронтенд — импортировать типы из generated/schema.ts

**Scope:** frontend

**Почему:** `web/src/api/auth.ts:3-7` вручную определяет `interface User`, который уже сгенерирован в `web/src/api/generated/schema.ts`. Два источника правды → рассинхрон.

**Плохо:**
```typescript
// web/src/api/auth.ts — ручные интерфейсы
interface LoginResponse {
    data: {
        accessToken: string;
        user: User;
    };
}
```

**Хорошо:**
```typescript
// Импорт из сгенерированных типов
import type { components, paths } from './generated/schema';

type LoginResponse = paths['/auth/login']['post']['responses']['200']['content']['application/json'];
type User = components['schemas']['User'];
```

**Правило:** все API-типы на фронтенде импортируются из `generated/schema.ts`. Ручные интерфейсы для API request/response запрещены.

---

## CS-11: mockery — автоматическое обнаружение интерфейсов

**Scope:** backend

**Почему:** Ручной список интерфейсов в `.mockery.yaml` забудут обновить при добавлении нового интерфейса. Тесты будут писаться без моков или с ручными моками.

**Плохо** (backend/.mockery.yaml):
```yaml
packages:
  github.com/alikhanmurzayev/ugcboost/backend/internal/handler:
    interfaces:
      Auth: {}       # Добавлять вручную каждый новый интерфейс?
      Brands: {}
```

**Хорошо:**
```yaml
packages:
  github.com/alikhanmurzayev/ugcboost/backend/internal/handler:
    config:
      all: true    # Генерировать моки для всех интерфейсов в пакете
  github.com/alikhanmurzayev/ugcboost/backend/internal/service:
    config:
      all: true
```

**Правило:** mockery настроен на автообнаружение (`all: true`) для пакетов handler, service, repository. Ручной список интерфейсов не нужен.
