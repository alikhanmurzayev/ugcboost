# Паттерны репозитория [REQUIRED]

Репозиторий — единственный слой, который работает с SQL.

## Прозрачность pool/tx

Repo — приватная структура, хранит `db dbutil.DB`. Этот интерфейс реализуется и `pgxpool.Pool`, и `pgx.Tx` — repo не знает и не должен знать, выполняется ли он внутри транзакции. Repo никогда не вызывает `Begin()` — управление транзакциями не его ответственность.

Конструкторов на уровне пакета нет. Все repos создаются через `RepoFactory`, который принимает `dbutil.DB` и возвращает экспортируемый интерфейс repo. Рядом с каждой приватной repo-структурой лежит экспортируемый интерфейс, перечисляющий все её публичные методы.

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

Query builder создаётся через чистый `squirrel` (`squirrel.Select(...)`, `squirrel.Insert(...)` и т.д.). Репозитории **не** используют `dbutil.Psql` и не знают про формат плейсхолдеров — `$1, $2, ...` подставляется автоматически внутри хелперов `dbutil` (`One`, `Many`, `Val`, `Vals`, `Exec`).

## Условия WHERE

Для условий используется `squirrel.Eq`, `squirrel.Gt`, `squirrel.Lt` и т.д. вместо строковых литералов. Цепочка `.Where()` вызовов объединяется через AND:

```go
q := squirrel.Select(cols...).
    From(TableUsers).
    Where(squirrel.Eq{UserColumnEmail: email}).
    Where(squirrel.Gt{UserColumnCreatedAt: since})
```

Для OR — `squirrel.Or{...}`.

## Ошибки

Репозитории используют `sql.ErrNoRows` из стандартной библиотеки, не `pgx.ErrNoRows`. Прямой импорт `pgx` в пакете `repository` запрещён (кроме тестов, где мокается `pgx.Rows`).

## Возвращаемые значения

Методы репозитория возвращают указатели на структуры, не структуры по значению. Для списков — слайс указателей `[]*EntityRow`, не `[]EntityRow`.

## Целостность данных: БД vs бэк

БД защищается от мусора через `NOT NULL`, `ENUM CHECK`, `FK`, `UNIQUE` — это data integrity. Format checks (regex, length) и business defaults (`status='pending'` при создании заявки) — на бэке, в БД их нет. Если формат меняется — миграция БД не нужна.

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

## Идентификатор словаря

Текущие словари (`categories`, `cities`) хранят и `id (UUID)`, и `code (TEXT UNIQUE)`. Целевая модель — `code` как PK, FK ссылаются на code → колонки таблиц-потребителей хранят `<entity>_code`, а не `<entity>_id`. Это устраняет лишний JOIN при чтении и indirection в коде. Refactor — отдельная задача с миграцией данных; новые словари в этой схеме не заводим.

## Пагинация

- **Offset** — допустим для таблиц с предсказуемо малым размером (справочники, пользователи)
- **Cursor-based** — обязателен для таблиц с неограниченным ростом (логи, уведомления, история операций)

## Миграции

- Каждая миграция — отдельный goose-файл в `backend/migrations/` с `+goose Up` и `+goose Down`. Создание — через `make migrate-create NAME=...`.
- **Миграция, прогнанная в любом не-локальном окружении (staging, prod), не редактируется in-place. Никогда.** Любая правка поведения схемы — новой forward-миграцией, даже если правка тривиальная (drop CHECK, drop DEFAULT, ALTER TYPE и т.п.). Причина: миграции применяются one-way, и in-place edit рассинхронизирует БД staging/prod с тем, что в репозитории (свежий `goose up` на чистой БД даст одну схему, на staging — другую).
- Если не уверен, прогонялась ли миграция — считай что прогонялась. Лишняя forward-миграция стоит ничего, разъехавшаяся схема — стоит инцидента.
