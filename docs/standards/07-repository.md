# Паттерны репозитория [REQUIRED]

Репозиторий — единственный слой, который работает с SQL. Здесь особенно важны надёжность, читаемость и защита от опечаток.

---

## CS-29: Squirrel — использовать SetMap для INSERT

**Scope:** backend

**Почему:** `Columns("a", "b", "c").Values(x, y, z)` при 10+ полях — легко перепутать порядок. Вставил `actorID` в колонку `action` — данные испорчены, ошибки нет.

**Плохо** (repository/audit.go:50):
```go
q := dbutil.Psql.Insert("audit_logs").
    Columns("actor_id", "actor_role", "action", "entity_type", "entity_id",
        "old_value", "new_value", "ip_address").
    Values(e.ActorID, e.ActorRole, e.Action, e.EntityType, e.EntityID,
        oldJSON, newJSON, e.IPAddress)
// 8 колонок и 8 значений — легко перепутать порядок
```

**Хорошо:**
```go
q := dbutil.Psql.Insert(TableAuditLogs).SetMap(map[string]any{
    ColAuditActorID:    e.ActorID,
    ColAuditActorRole:  e.ActorRole,
    ColAuditAction:     e.Action,
    ColAuditEntityType: e.EntityType,
    ColAuditEntityID:   e.EntityID,
    ColAuditOldValue:   oldJSON,
    ColAuditNewValue:   newJSON,
    ColAuditIPAddress:  e.IPAddress,
}).Suffix("RETURNING " + ColAuditID + ", " + ColAuditCreatedAt)
// Каждое значение явно привязано к колонке — порядок не важен
```

**Правило:** INSERT-запросы через `SetMap()` (ключ-значение), не через `Columns().Values()`. Это исключает ошибки порядка аргументов.

---

## CS-30: Struct tags → column names через stom или аналог

**Scope:** backend

**Почему:** Без автоизвлечения у нас три места для одного имени колонки:
1. Struct tag `db:"actor_id"`
2. Константа `ColAuditActorID = "actor_id"`
3. Строка в SQL-запросе

Это тройное дублирование. Пакет типа `stom` или аналог позволяет извлекать имена колонок из struct tags автоматически.

**Хорошо:**
```go
// AuditLogRow — struct с db-тегами
type AuditLogRow struct {
    ID         string     `db:"id"`
    ActorID    string     `db:"actor_id"`
    ActorRole  string     `db:"actor_role"`
    Action     string     `db:"action"`
    // ...
}

// Автоматическое извлечение для SELECT:
var auditLogColumns = stom.MustColumns(AuditLogRow{}) // ["id", "actor_id", "actor_role", "action", ...]

q := dbutil.Psql.Select(auditLogColumns...).From(TableAuditLogs)
```

**Константы всё ещё нужны** для WHERE, ORDER BY, JOIN — там мы используем отдельные колонки:
```go
q = q.Where(sq.Eq{ColAuditActorID: actorID})
q = q.OrderBy(ColAuditCreatedAt + " DESC")
```

**Правило:** для SELECT — извлекать список колонок из struct tags (stom или аналог). Для WHERE/ORDER/JOIN — использовать константы колонок. Строковые литералы для имён колонок в production-коде запрещены.

---

## CS-31: Курсорная пагинация для растущих таблиц

**Scope:** backend

**Почему:** OFFSET-пагинация деградирует на больших таблицах. `OFFSET 1000000` = БД сканирует миллион строк и выкидывает их. Для `audit_logs`, где каждый чих записывается, это критично.

**Offset-пагинация допустима для:**
- Небольших справочников: `brands`, `users` (десятки-сотни записей)
- Таблицы с предсказуемым небольшим размером

**Курсорная пагинация обязательна для:**
- `audit_logs` — растёт неограниченно
- `notifications` — растёт неограниченно
- Любая таблица, размер которой пропорционален количеству операций в системе

**Пример курсорной пагинации:**
```go
// Вместо page=5&per_page=20:
// cursor=2024-01-15T10:30:00Z&limit=20

type CursorParams struct {
    Cursor string // значение created_at последнего элемента предыдущей страницы
    Limit  int
}

q := dbutil.Psql.Select(auditLogColumns...).
    From(TableAuditLogs).
    OrderBy(ColAuditCreatedAt + " DESC").
    Limit(uint64(params.Limit))

if params.Cursor != "" {
    q = q.Where(sq.Lt{ColAuditCreatedAt: params.Cursor})
}
```

**Правило:** при проектировании нового эндпоинта с пагинацией — оценить рост таблицы. Если рост неограничен → cursor-based. Если таблица маленькая → offset допустим.
