# Паттерны репозитория [REQUIRED]

Репозиторий — единственный слой, который работает с SQL.

## Структуры с двумя тегами

Row-структуры используют два тега:
- `db` — все колонки, которые поддерживает текущая версия кода (для SELECT)
- `insert` — только колонки, которые передаются при стандартном INSERT (исключает auto-generated: ID, created_at и т.п.)

## Предвычисленные списки колонок

Для каждой Row-структуры на уровне пакета определяются приватные переменные через stom:

```go
var userSelectColumns = sortColumns(stom.MustNewStom(UserRow{}).SetTag(string(tagSelect)).TagValues())
var userInsertMapper  = stom.MustNewStom(UserRow{}).SetTag(string(tagInsert))
var userInsertColumns = sortColumns(userInsertMapper.TagValues())
```

- `selectColumns` — используется вместо `SELECT *`. Причина: новая колонка в БД после миграции сломает код, где нет поля для этой колонки. Когда нужна только часть колонок — допустимо перечислять конкретные константы
- `insertColumns` + `insertMapper` — используется для стандартного INSERT всей сущности. Для нестандартных INSERT (часть полей, специфичные таблицы) — допустимы другие подходы

## Хелперы

Приватные хелперы на уровне пакета:

```go
import (
    "slices"

    "github.com/Masterminds/squirrel"
    "github.com/elgris/stom"
)

type tagMap string

const (
    tagSelect tagMap = "db"
    tagInsert tagMap = "insert"
)

// toMap конвертирует структуру в map по тегам
func toMap(row any, st stom.ToMapper) map[string]any {
    m, err := st.ToMap(row)
    if err != nil {
        panic(err)
    }
    return m
}

// insertEntities добавляет значения строки в INSERT query builder
func insertEntities(qb squirrel.InsertBuilder, st stom.ToMapper, cols []string, row any) squirrel.InsertBuilder {
    m, err := st.ToMap(row)
    if err != nil {
        panic(err)
    }
    values := make([]any, len(cols))
    for i, c := range cols {
        values[i] = m[c]
    }
    return qb.Values(values...)
}

func sortColumns(columns []string) []string {
    slices.Sort(columns)
    return columns
}
```

Использование:
- Single insert: `squirrel.Insert(TableUsers).SetMap(toMap(row, userInsertMapper))`
- Batch insert: `insertEntities(qb, userInsertMapper, userInsertColumns, row)` в цикле

Query builder создаётся через `squirrel` напрямую. Формат плейсхолдеров устанавливается внутри хелперов `dbutil` (`Val`, `Row` и т.д.), которые принимают `squirrel.Sqlizer`.

## Возвращаемые значения

Методы репозитория возвращают указатели на структуры, не структуры по значению. Для списков — слайс указателей `[]*EntityRow`, не `[]EntityRow`.

## Константы колонок

Строковые литералы для имён колонок в коде запрещены (в тестах — наоборот, обязательны, см. 01-constants). Для каждой колонки — экспортированная константа. Формат: `{Entity}Column{Field}`.

```go
const (
    UserColumnID           = "id"
    UserColumnEmail        = "email"
    UserColumnPasswordHash = "password_hash"
    UserColumnRole         = "role"
    UserColumnCreatedAt    = "created_at"
)
```

Константы используются везде, где нужна конкретная колонка: WHERE, ORDER BY, JOIN, частичный SELECT, нестандартный INSERT и т.д.

## Пагинация

- **Offset** — допустим для таблиц с предсказуемо малым размером (справочники, пользователи)
- **Cursor-based** — обязателен для таблиц с неограниченным ростом (логи, уведомления, история операций)
