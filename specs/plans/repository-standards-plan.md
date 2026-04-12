# План реализации: Приведение репозитория к стандартам

## Обзор

Добавить недостающие константы колонок, убрать строковые литералы из production-кода, исправить паттерн Create в UserRepository и дописать тесты для AuditRepository.List.

## Требования

- REQ-1: Для каждой колонки каждой таблицы — экспортированная константа `{Entity}Column{Field}` (backend-constants.md [CRITICAL])
- REQ-2: Строковые литералы колонок в production-коде запрещены (backend-constants.md [CRITICAL])
- REQ-3: Single insert через `SetMap(toMap(row, mapper))`, не через позиционные Values (backend-repository.md)
- REQ-4: Каждый метод репозитория покрыт unit-тестом с SQL assertion (backend-testing-unit.md)

## Файлы для изменения

| Файл | Изменения |
|------|-----------|
| `backend/internal/repository/user.go` | +4 константы (RefreshToken: ID, CreatedAt; ResetToken: ID, CreatedAt). Переписать `Create` на `SetMap` |
| `backend/internal/repository/brand.go` | +2 константы (BrandManager: ID, CreatedAt). Заменить 6 строковых литералов на константы |
| `backend/internal/repository/user_test.go` | Убрать `captureQuery` и `captureExec` (переезжают в helpers_test.go) |
| `backend/internal/repository/helpers_test.go` | **Новый**. `captureQuery`, `captureExec` (из user_test.go) + новый `scalarRows` хелпер |
| `backend/internal/repository/brand_test.go` | Добавить `TestBrandRepository_GetBrandIDsForUser` — SQL assertion |
| `backend/internal/repository/audit_test.go` | Добавить `TestAuditRepository_List` — count-запрос, data-запрос, фильтры, пагинация |

## Шаги реализации

### 1. [ ] Добавить константы в `user.go`

Добавить в блок `RefreshTokens`:
```go
RefreshTokenColumnID        = "id"
RefreshTokenColumnCreatedAt = "created_at"
```

Добавить в блок `PasswordResetTokens`:
```go
ResetTokenColumnID        = "id"
ResetTokenColumnCreatedAt = "created_at"
```

### 2. [ ] Добавить константы в `brand.go`

Добавить в блок `BrandManagers`:
```go
BrandManagerColumnID        = "id"
BrandManagerColumnCreatedAt = "created_at"
```

### 3. [ ] Заменить строковые литералы в `brand.go`

6 замен:

| Метод | Было | Стало |
|-------|------|-------|
| `List` (строка 95) | `"COUNT(bm.id) AS manager_count"` | `"COUNT(bm."+BrandManagerColumnID+") AS manager_count"` |
| `List` (строка 100) | `GroupBy("b.id")` | `GroupBy("b."+BrandColumnID)` |
| `List` (строка 101) | `OrderBy("b.created_at DESC")` | `OrderBy("b."+BrandColumnCreatedAt+" DESC")` |
| `ListByUser` (строка 114) | `OrderBy("b.created_at DESC")` | `OrderBy("b."+BrandColumnCreatedAt+" DESC")` |
| `ListManagers` (строка 167) | `"bm."+BrandColumnCreatedAt` | `"bm."+BrandManagerColumnCreatedAt` |
| `ListManagers` (строка 171) | `OrderBy("bm.created_at ASC")` | `OrderBy("bm."+BrandManagerColumnCreatedAt+" ASC")` |

Строка 167 — ошибка семантики: используется `BrandColumnCreatedAt` вместо `BrandManagerColumnCreatedAt`. Значения совпадают (`"created_at"`), поэтому SQL не менялся, но после добавления правильной константы — исправляем.

### 4. [ ] Исправить `UserRepository.Create` на `SetMap`

Было:
```go
q := dbutil.Psql.Insert(TableUsers).
    Columns(userInsertColumns...).
    Values(email, passwordHash, role).
    Suffix(returningClause(userSelectColumns))
```

Стало:
```go
q := dbutil.Psql.Insert(TableUsers).
    SetMap(toMap(UserRow{Email: email, PasswordHash: passwordHash, Role: role}, userInsertMapper)).
    Suffix(returningClause(userSelectColumns))
```

**Тест не меняется**: SetMap сортирует ключи алфавитно → `email, password_hash, role` — тот же порядок, что и `userInsertColumns`.

### 5. [ ] Создать `helpers_test.go` и перенести хелперы

Стандарт `backend-testing-unit.md`: "Общие хелперы для тестов пакета — в `helpers_test.go`."

Сейчас `captureQuery` и `captureExec` живут в `user_test.go` — нарушение стандарта.

**Действия:**
1. Создать `backend/internal/repository/helpers_test.go`
2. Перенести `captureQuery` и `captureExec` из `user_test.go`
3. Добавить новый хелпер `scalarRows` — минимальная реализация `pgx.Rows` для scalar-значений

```go
type scalarRows struct {
    val    int64
    called bool
}
```

Реализует `pgx.Rows`: `Next()` возвращает true один раз, `Scan()` записывает значение, остальные методы — no-op.

Нужен для тестирования методов с двумя SQL-запросами (COUNT + SELECT), где первый запрос должен вернуть результат, чтобы метод дошёл до второго. Первый `Query` возвращает `scalarRows{val: N}`, второй — стандартный `captureQuery`.

### 6. [ ] Добавить `TestBrandRepository_GetBrandIDsForUser` в `brand_test.go`

Метод `GetBrandIDsForUser` (brand.go:176) не покрыт тестом — нарушение REQ-4.

```go
t.Run("SQL", func(t *testing.T) {
    // captureQuery, verify:
    // SELECT brand_id FROM brand_managers WHERE user_id = $1
})
```

### 7. [ ] Добавить `TestAuditRepository_List` в `audit_test.go`

Три сценария:

**`count SQL no filters`** — verify базовый COUNT:
```sql
SELECT COUNT(*) FROM audit_logs
```

**`count SQL with all filters`** — verify COUNT + все фильтры + аргументы:
```sql
SELECT COUNT(*) FROM audit_logs WHERE actor_id = $1 AND entity_type = $2 AND entity_id = $3 AND action = $4 AND created_at >= $5 AND created_at <= $6
```

**`data SQL with pagination`** — scalarRows для count, captureQuery для data:
```sql
SELECT action, actor_id, actor_role, created_at, entity_id, entity_type, id, ip_address, new_value, old_value FROM audit_logs WHERE actor_id = $1 ORDER BY created_at DESC LIMIT 20 OFFSET 20
```
Проверяем: колонки (auditSelectColumns), ORDER BY, LIMIT, OFFSET, filter args.

### 8. [ ] Прогнать `make build-backend` — компиляция без ошибок

### 9. [ ] Прогнать `make test-unit-backend` — все тесты зелёные

### 10. [ ] Прогнать `make test-e2e-backend` — E2E зелёные

Изменения внутренние, SQL семантически не менялся — но E2E подтвердит, что ничего не сломалось на реальной БД.

## Стратегия тестирования

- **Unit-тесты**: SQL assertion через `captureQuery`/`captureExec` для AuditRepository.List. Новый хелпер `scalarRows` для multi-query тестов
- **Существующие тесты**: не ломаются — SQL семантически не меняется (значения констант совпадают с литералами)
- **E2E-тесты**: не затронуты — изменения внутренние

## Оценка рисков

| Риск | Вероятность | Митигация |
|------|-------------|-----------|
| `scalarRows` не совместим с pgx.CollectOneRow | Средняя | Если pgx.RowTo[int64] ожидает другой Scan — fallback на раздельные тесты count/data |
| Забыть литерал в brand.go | Низкая | grep по строковым литералам после всех замен |
| SetMap меняет порядок колонок | Низкая | stom сортирует по тегам, squirrel SetMap сортирует ключи — оба алфавитно |

## План отката

Все изменения — рефакторинг без изменения SQL-семантики. Если что-то пойдёт не так: `git checkout -- backend/internal/repository/`.
