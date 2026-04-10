# Обработка ошибок [CRITICAL]

Игнорирование ошибок — одна из самых опасных практик. Молчаливый баг хуже громкого — его находят в проде, когда данные уже потеряны.

---

## CS-19: Ошибки НИКОГДА не игнорируются

**Scope:** both

**Почему:** `_ = h.auth.Logout(r.Context(), userID)` (`handler/auth.go:120`) — если logout упал (БД недоступна), клиент думает что вышел, а refresh token жив. Это дыра безопасности.

**Плохо:**
```go
// handler/auth.go:120 — ошибка logout проигнорирована
_ = h.auth.Logout(r.Context(), userID)

// middleware/json.go:11 — ошибка encode проигнорирована
json.NewEncoder(w).Encode(v) //nolint:errcheck
```

**Хорошо:**
```go
// Вариант 1: вернуть ошибку клиенту
if err := h.authService.Logout(r.Context(), userID); err != nil {
    respondError(w, err)
    return
}

// Вариант 2: залогировать, если не можем вернуть
if err := h.authService.Logout(r.Context(), userID); err != nil {
    slog.Error("logout failed", "error", err, "user_id", userID)
}
```

**Правило:** каждая ошибка либо возвращается вызывающему коду, либо логируется с контекстом. Допустимое исключение — см. CS-23.

---

## CS-20: Невалидный ввод = ошибка валидации, не тихий fallback

**Scope:** backend

**Почему:** `page=-5` молча становится `page=1`, `perPage=999` молча становится `perPage=20`. Фронтенд-разработчик не узнает о своём баге, пока пользователь не пожалуется.

**Плохо** (handler/audit.go:59-65):
```go
page, _ := strconv.Atoi(q.Get("page"))       // ошибка парсинга игнорируется
perPage, _ := strconv.Atoi(q.Get("per_page")) // ошибка парсинга игнорируется
if page < 1 {
    page = 1          // тихий fallback
}
if perPage < 1 || perPage > 100 {
    perPage = 20      // тихий fallback
}
```

**Хорошо:**
```go
// Строгая валидация: невалидный ввод = ошибка
pageStr := q.Get("page")
if pageStr != "" {
    page, err := strconv.Atoi(pageStr)
    if err != nil || page < 1 {
        respondError(w, http.StatusUnprocessableEntity, domain.CodeValidation,
            "page must be a positive integer")
        return
    }
}

perPageStr := q.Get("per_page")
if perPageStr != "" {
    perPage, err := strconv.Atoi(perPageStr)
    if err != nil || perPage < 1 || perPage > 100 {
        respondError(w, http.StatusUnprocessableEntity, domain.CodeValidation,
            "per_page must be between 1 and 100")
        return
    }
}
```

**Правило:** бэкенд **строгий**. Любой невалидный ввод → 422 Unprocessable Entity с понятным описанием ошибки. Тихие fallback-значения запрещены. Значения по умолчанию допустимы только когда параметр **не передан** (пустая строка), а не когда передан невалидно.

---

## CS-21: Транзакции — корректный rollback с multi-error

**Scope:** backend

**Почему:** `dbutil/db.go:115` — `defer tx.Rollback(ctx) //nolint:errcheck`. Если callback упал И rollback упал (например, соединение с БД потеряно) — мы теряем информацию о второй ошибке.

**Плохо:**
```go
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(tx DB) error) error {
    tx, err := pool.Begin(ctx)
    if err != nil {
        return fmt.Errorf("begin: %w", err)
    }
    defer tx.Rollback(ctx) //nolint:errcheck  // ошибка rollback потеряна!

    if err := fn(tx); err != nil {
        return err  // rollback через defer, но его ошибка проигнорирована
    }
    return tx.Commit(ctx)
}
```

**Хорошо:**
```go
// Принимать интерфейс, не конкретный тип
type TxStarter interface {
    Begin(ctx context.Context) (pgx.Tx, error)
}

func WithTx(ctx context.Context, db TxStarter, fn func(tx DB) error) error {
    tx, err := db.Begin(ctx)
    if err != nil {
        return fmt.Errorf("begin tx: %w", err)
    }

    if err := fn(tx); err != nil {
        if rbErr := tx.Rollback(ctx); rbErr != nil {
            return errors.Join(err, fmt.Errorf("rollback failed: %w", rbErr))
        }
        return err
    }

    return tx.Commit(ctx)
}
```

**Правило:**
1. `WithTx` принимает интерфейс `TxStarter`, не конкретный `*pgxpool.Pool`
2. При ошибке callback: явный rollback + `errors.Join` если rollback тоже упал
3. Defer для rollback не используется — явный control flow

---

## CS-22: Аудит-сервис — возвращать ошибки, а не глотать

**Scope:** backend

**Почему:** `service/audit.go:70` — `Log()` глотает ошибку `repo.Create()` и только логирует. Аудит — compliance requirement. Потеря записей аудита означает, что мы не можем доказать кто и когда что менял.

**Плохо:**
```go
// service/audit.go
func (s *AuditService) Log(ctx context.Context, e AuditEntry) {
    // ...
    if err := s.repo.Create(ctx, row); err != nil {
        slog.Error("audit log failed", "error", err)  // проглотил ошибку!
    }
}
```

**Хорошо:**
```go
func (s *AuditService) Log(ctx context.Context, e AuditEntry) error {
    // ...
    if err := s.repo.Create(ctx, row); err != nil {
        return fmt.Errorf("audit log: %w", err)
    }
    return nil
}

// Вызывающий код решает, что делать:
if err := h.auditService.Log(ctx, entry); err != nil {
    // Для критических операций — откатить всю операцию
    // Для некритических — залогировать и продолжить (но решение за вызывающим)
    slog.Error("audit log failed", "error", err)
}
```

**Правило:** сервисы возвращают ошибки. Решение о том, как обрабатывать ошибку (откатить, залогировать, проигнорировать) — принимает вызывающий код, не сервис.

---

## CS-23: JSON encode в response — допустимое исключение

**Scope:** backend

**Почему:** `json.NewEncoder(w).Encode(v)` может вернуть ошибку, если клиент разорвал соединение. Мы не можем отправить клиенту ошибку, потому что соединения уже нет.

**Правило:**
```go
// Допустимо с обязательным комментарием:
json.NewEncoder(w).Encode(v) //nolint:errcheck // response write: client may have disconnected
```

**Это единственное допустимое место для `//nolint:errcheck`** в проекте. Для всех остальных ошибок — возвращать или логировать.
