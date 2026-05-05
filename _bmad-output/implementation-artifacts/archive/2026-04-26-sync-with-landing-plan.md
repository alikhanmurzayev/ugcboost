# Plan: Синхронизация бэка с лендингом + Dictionary system

**Контекст:** Айдана сделала PR #20 (`aidana/landing-efw`) с лендингом для EFW 13-14 мая. Лендинг готов, но 11 структурных расхождений с бэк-API (`POST /creators/applications` из PR #19). Этот план описывает синхронизацию и параллельное внедрение системы словарей (categories + cities, общий API `GET /dictionaries/{type}`).

**Дата:** 2026-04-25
**Ветка:** `alikhan/creator-application-submit` (продолжаем PR #19)
**Связанные документы:**
- `_bmad-output/implementation-artifacts/spec-creator-application-submit.md` — текущий контракт
- `_bmad-output/implementation-artifacts/deferred-work.md` — backlog

---

## Решения (выбраны через /interview)

| # | Тема | Решение |
|---|---|---|
| 1 | Согласия | 1 чекбокс на лендинге → API принимает `acceptedAll: bool` → бэк пишет 4 ряда в `creator_application_consents` под одной версией Политики |
| 2 | Threads | Добавить `threads` в `social_platform` ENUM (миграция) + `domain.SocialPlatformThreads` |
| 3 | Соцсеть-инпут | Лендинг: `type=url` → `type=text` + placeholder=`@username`. Бэк не трогаем — `TrimLeft('@')` уже есть |
| 4 | Категории | Публичный `GET /dictionaries/categories`, лендинг тянет при загрузке |
| 5 | «Другое» free-text | Добавить `category_other_text` в API + колонку в `creator_applications` |
| 6 | Gaming | DELETE из `categories` через миграцию |
| 7 | MaxCategories | Лимит 3 на бэке (`domain.MaxCategoriesPerApplication`) → 422 `VALIDATION_ERROR` |
| 8 | Возраст | `MinCreatorAge=21` на бэке (поднимаем с 18) |
| 9 | middleName | Делаем optional на лендинге |
| 10 | Слияние | `git merge aidana/landing-efw` в локальную `alikhan/creator-application-submit` (один PR #19) |
| 11 | Submit-интеграция | Я подхватываю — fetch + error states + success-screen с динамическим `data.telegram_bot_url` |

**Бонус:** Dictionary system. Общий `DictionaryEntryRow` + 1 `DictionaryRepo` с параметром `table`. `DictionaryService` мапит `type → table` через константы `repository.TableX`. Лендинг тянет и categories, и cities через единый `GET /dictionaries/{type}`.

---

## Решённые открытые вопросы (зафиксировано 2026-04-25 через /interview)

| ID | Вопрос | Решение |
|---|---|---|
| Q1 | Обновлять ли имена категорий под лендинг-формат | **Да, в одной миграции** (`categories_sync.sql`) — UPDATE имён + sort_order одним проходом |
| Q2 | Sort order городов | **Топ-3 метро сначала** (Алматы=10/Астана=20/Шымкент=30), остальное по алфавиту с шагом 100 |
| Q3 | `other` без `categoryOtherText` | **422 VALIDATION_ERROR** с сообщением "Укажите название категории в поле «Другое»" |
| Q4 | Лимит длины `categoryOtherText` | **200 символов** (maxLength в OpenAPI + валидация в service) |
| Q5 | Лендинг при `telegram_bot_url=""` | **Success-screen без CTA-кнопки** — просто «Заявка принята» без deep-link. Бэк env-check на старте делает этот кейс почти невозможным, но если случится — фейлим тихо без поддержки-fallback |

---

## Pre-flight checks

**P1.** Локальный baseline зелёный:
```bash
cd backend && make test    # build + lint + unit + coverage gate ≥80% + E2E
cd frontend/landing && npm run build
```
Если что-то красное на текущей ветке — остановка, разбираемся почему до старта.

**P2.** Ветка Айданы доступна:
```bash
git fetch origin aidana/landing-efw
git log --oneline origin/aidana/landing-efw | head -5
```
Должны увидеть коммиты лендинга. Если нет — `git fetch --all`.

**P3.** Текущая ветка чистая:
```bash
git status
```
Только `?? .claude/commands/notes_do_not_commit.txt` (gitignored по сути) — допустимо.

**P4.** Запомнить baseline coverage из вывода `make test` для сравнения после.

---

## Шаг 1: Spec обновление

**Файл:** `_bmad-output/implementation-artifacts/spec-creator-application-submit.md`

**Что добавить (внизу):**

```markdown
## Sync with landing 2026-04-25

После анализа PR #20 (aidana/landing-efw) и /interview-сессии приняты следующие изменения контракта (см. также sync-with-landing-plan.md):

1. **Consents API:** ConsentsInput из 4 полей → одно поле `acceptedAll: bool`. Бэк по-прежнему пишет 4 ряда в creator_application_consents (legal/privacy-policy.md п.9.2 — принятие Политики покрывает все 4 типа).
2. **CategoryOtherText:** новое поле в DTO. Required если в `categories` присутствует `other`, max 200 chars. Хранится в `creator_applications.category_other_text TEXT NULL`.
3. **MaxCategoriesPerApplication = 3.** При >3 → 422 `VALIDATION_ERROR`.
4. **MinCreatorAge поднят с 18 до 21** (бизнес-фильтр EFW; legal min остаётся 18 для будущих воронок).
5. **SocialPlatform += `threads`.** Handle-regex тот же (`^[a-z0-9._]{1,30}$`).
6. **Categories seed обновлён:** DELETE `gaming`, INSERT `home_diy/animals/other`. Имена обновлены под формат лендинга (Бьюти → "Бьюти (макияж, уход)" и т.д.).
7. **Cities — новая таблица** + сид (17 городов) + публичный API.
8. **Dictionary system:** новый GET /dictionaries/{type} с DictionaryEntry/DictionaryListResult. Используется лендингом для подгрузки categories и cities.

Удалено из API: ConsentsInput.{Processing,ThirdParty,CrossBorder,Terms}.
Добавлено в API: CreatorApplicationCreate.acceptedAll, .categoryOtherText; ListDictionary endpoint.
```

**Verification:** `head -40 _bmad-output/implementation-artifacts/spec-creator-application-submit.md` — таблицы валидны, заголовки на русском (по правилу `feedback_russian_headers`).

---

## Шаг 2: Слияние ветки Айданы

```bash
git fetch origin aidana/landing-efw
git merge --no-ff origin/aidana/landing-efw -m "merge: include landing from Aidana (PR #20)"
git status   # working tree clean
git log --oneline -5   # видим merge-коммит
```

**Проверки после merge:**
- `ls frontend/landing/src/` — есть `content.ts`, `pages/index.astro`, `pages/legal/`
- `cd backend && go build ./...` — фронт-мерж не должен ломать бэк
- `cd frontend/landing && npm install && npm run build` — фронт собирается

**Если конфликты:**
- STOP. Зафиксировать конфликты в этом документе как блокер.
- Не делать `git merge --abort` молча — сообщить пользователю.

**Что НЕ делаем:**
- НЕ закрываем PR #20 на GitHub. Айдана увидит, что её код вошёл через мой PR — этого достаточно.
- НЕ комментируем PR #20.

---

## Шаг 3: Backend — пошаговая реализация

### 3.1 OpenAPI обновление

**Файл:** `backend/api/openapi.yaml`

**Изменения:**
1. Добавить путь `/dictionaries/{type}`:
   ```yaml
   /dictionaries/{type}:
     get:
       operationId: ListDictionary
       parameters:
         - name: type
           in: path
           required: true
           schema:
             type: string
             enum: [categories, cities]
       responses:
         '200': { $ref: '#/components/responses/DictionaryListResult' }
         '404': { $ref: '#/components/responses/ErrorResponse' }
   ```
2. Добавить схемы:
   ```yaml
   DictionaryEntry:
     type: object
     required: [code, name, sortOrder]
     properties:
       code: { type: string }
       name: { type: string }
       sortOrder: { type: integer }
   ListDictionaryData:
     type: object
     required: [type, items]
     properties:
       type: { type: string }
       items: { type: array, items: { $ref: '#/components/schemas/DictionaryEntry' } }
   DictionaryListResult:
     type: object
     required: [data]
     properties:
       data: { $ref: '#/components/schemas/ListDictionaryData' }
   ```
3. В `SocialPlatform` enum добавить `threads`.
4. В `CreatorApplicationCreate`:
   - Удалить `consents` (старый объект из 4 полей)
   - Добавить `acceptedAll: { type: boolean }` в required
   - Добавить `categoryOtherText: { type: string, maxLength: 200, nullable: true }`
5. В `ConsentsInput` schema удалить из required `processing/thirdParty/crossBorder/terms` (или удалить схему целиком если больше не используется).

**Codegen:**
```bash
cd backend && make generate
```

**Verification:**
- `git diff backend/internal/api/server.go` — есть `ListDictionary`
- `git diff backend/internal/api/types.go` — есть `DictionaryEntry`, `CreatorApplicationCreate.AcceptedAll`, `CategoryOtherText`
- `go build ./...` зелёный

### 3.2 Миграции

Создать 4 файла в `backend/migrations/` (timestamps 20260425XXXXNN):

**3.2.1 `20260425XXXX01_categories_sync.sql`**
```sql
-- +goose Up
ALTER TABLE categories ADD COLUMN sort_order INT NOT NULL DEFAULT 0;

-- Update names to match landing format
UPDATE categories SET name = 'Бьюти (макияж, уход)', sort_order = 20 WHERE code = 'beauty';
UPDATE categories SET name = 'Мода / Стиль',         sort_order = 10 WHERE code = 'fashion';
UPDATE categories SET name = 'Еда / Рестораны',      sort_order = 40 WHERE code = 'food';
UPDATE categories SET name = 'Фитнес / Здоровье / ЗОЖ', sort_order = 60 WHERE code = 'fitness';
UPDATE categories SET name = 'Лайфстайл',            sort_order = 30 WHERE code = 'lifestyle';
UPDATE categories SET name = 'Тех / Гаджеты',        sort_order = 90 WHERE code = 'tech';
UPDATE categories SET name = 'Путешествия',          sort_order = 50 WHERE code = 'travel';
UPDATE categories SET name = 'Мама и дети / Семья',  sort_order = 70 WHERE code = 'parenting';
UPDATE categories SET name = 'Авто',                 sort_order = 80 WHERE code = 'auto';

-- Remove gaming (not on landing)
DELETE FROM categories WHERE code = 'gaming';

-- Add new categories from landing
INSERT INTO categories (code, name, sort_order) VALUES
    ('home_diy', 'Дом / Интерьер / DIY', 100),
    ('animals',  'Животные',             110),
    ('other',    'Другое',               999);

-- +goose Down
ALTER TABLE categories DROP COLUMN sort_order;
DELETE FROM categories WHERE code IN ('home_diy', 'animals', 'other');
INSERT INTO categories (code, name) VALUES ('gaming', 'Игры') ON CONFLICT DO NOTHING;
UPDATE categories SET name = 'Бьюти'         WHERE code = 'beauty';
UPDATE categories SET name = 'Мода'          WHERE code = 'fashion';
UPDATE categories SET name = 'Еда'           WHERE code = 'food';
UPDATE categories SET name = 'Фитнес'        WHERE code = 'fitness';
UPDATE categories SET name = 'Технологии'    WHERE code = 'tech';
UPDATE categories SET name = 'Родительство'  WHERE code = 'parenting';
```

**3.2.2 `20260425XXXX02_threads_platform.sql`**
```sql
-- +goose Up
-- Goose handles ENUM ALTER outside of transactions
-- +goose NO TRANSACTION
ALTER TYPE social_platform ADD VALUE IF NOT EXISTS 'threads';

-- +goose Down
-- Postgres does not support DROP VALUE on ENUM. Down requires recreating enum.
-- For MVP we accept that down-migration leaves 'threads' in the enum but unused.
SELECT 1;
```

**3.2.3 `20260425XXXX03_cities.sql`**
```sql
-- +goose Up
CREATE TABLE cities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_cities_active ON cities(active) WHERE active = TRUE;

-- 17 cities from landing content.ts. Sort order: top-3 metros first, rest alphabetical.
INSERT INTO cities (code, name, sort_order) VALUES
    ('almaty',          'Алматы',          10),
    ('astana',          'Астана',          20),
    ('shymkent',        'Шымкент',         30),
    ('aktau',           'Актау',           100),
    ('aktobe',          'Актобе',          110),
    ('atyrau',          'Атырау',          120),
    ('karaganda',       'Караганда',       130),
    ('kostanay',        'Костанай',        140),
    ('kyzylorda',       'Кызылорда',       150),
    ('oral',            'Уральск',         160),
    ('oskemen',         'Усть-Каменогорск',170),
    ('pavlodar',        'Павлодар',        180),
    ('petropavlovsk',   'Петропавловск',   190),
    ('semey',           'Семей',           200),
    ('taldykorgan',     'Талдыкорган',     210),
    ('taraz',           'Тараз',           220),
    ('turkistan',       'Туркестан',       230);

-- +goose Down
DROP TABLE IF EXISTS cities;
```
**Note:** Финальный список из 17 городов содержащийся в `frontend/landing/src/content.ts` сверить **после мержа Шага 2**, до запуска миграции. Если расходится — исправить cities sql.

**3.2.4 `20260425XXXX04_creator_application_category_other.sql`**
```sql
-- +goose Up
ALTER TABLE creator_applications ADD COLUMN category_other_text TEXT NULL;

-- +goose Down
ALTER TABLE creator_applications DROP COLUMN category_other_text;
```

**Verification после миграций:**
```bash
cd backend && make migrate-up
psql $DATABASE_URL -c "SELECT code, name, sort_order FROM categories ORDER BY sort_order;"
psql $DATABASE_URL -c "SELECT code, name FROM cities ORDER BY sort_order;"
psql $DATABASE_URL -c "\d+ creator_applications" | grep category_other
psql $DATABASE_URL -c "SELECT unnest(enum_range(NULL::social_platform));"
```

Down/Up cycle тест:
```bash
make migrate-down  # последняя
make migrate-down  # ещё
make migrate-up    # обе обратно
```

### 3.3 Domain layer

**Создать `backend/internal/domain/dictionary.go`:**
```go
package domain

import "errors"

type DictionaryType string

const (
    DictionaryTypeCategories DictionaryType = "categories"
    DictionaryTypeCities     DictionaryType = "cities"
)

var DictionaryTypeValues = []DictionaryType{
    DictionaryTypeCategories,
    DictionaryTypeCities,
}

type DictionaryEntry struct {
    Code      string
    Name      string
    SortOrder int
}

var ErrDictionaryUnknownType = errors.New("unknown dictionary type")
```

**Обновить `backend/internal/domain/creator_application.go`:**
- Добавить `SocialPlatformThreads = "threads"` и в `SocialPlatformValues`
- Добавить `MinCreatorAge = 21` (если ещё нет; искать существующую константу — если она в другом месте, переименовать/обновить)
- Добавить `MaxCategoriesPerApplication = 3`
- Заменить `ConsentsInput { Processing, ThirdParty, CrossBorder, Terms bool }` на `ConsentsInput { AcceptedAll bool }`
- Удалить метод `AsMap` (больше не нужен — все 4 типа всегда true)
- Добавить в `CreatorApplicationInput` поле `CategoryOtherText *string`
- Добавить `CodeCategoryOtherRequired = "CATEGORY_OTHER_REQUIRED"` (или использовать `CodeValidation` — выбираем VALIDATION_ERROR для согласованности)

**Verification:**
```bash
go build ./internal/domain/...
go test ./internal/domain/... -run TestIIN -count=1   # smoke что не сломали ничего
```

### 3.4 Repository layer

**Удалить:**
- `backend/internal/repository/category.go`
- `backend/internal/repository/category_test.go`
- `backend/internal/repository/mocks/mock_category_repo.go`
- Из `backend/internal/repository/factory.go`: метод `NewCategoryRepo` и его реализацию.

**Создать `backend/internal/repository/dictionary.go`:**
```go
package repository

import (
    "context"
    "time"

    sq "github.com/Masterminds/squirrel"
    "github.com/elgris/stom"

    "github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
)

const (
    TableCategories = "categories"
    TableCities     = "cities"
)

const (
    DictColumnID        = "id"
    DictColumnCode      = "code"
    DictColumnName      = "name"
    DictColumnActive    = "active"
    DictColumnSortOrder = "sort_order"
    DictColumnCreatedAt = "created_at"
)

type DictionaryEntryRow struct {
    ID        string    `db:"id"`
    Code      string    `db:"code"`
    Name      string    `db:"name"`
    Active    bool      `db:"active"`
    SortOrder int       `db:"sort_order"`
    CreatedAt time.Time `db:"created_at"`
}

var dictionarySelectColumns = sortColumns(stom.MustNewStom(DictionaryEntryRow{}).SetTag(string(tagSelect)).TagValues())

type DictionaryRepo interface {
    ListActive(ctx context.Context, table string) ([]*DictionaryEntryRow, error)
    GetActiveByCodes(ctx context.Context, table string, codes []string) ([]*DictionaryEntryRow, error)
}

type dictionaryRepository struct {
    db dbutil.DB
}

func (r *dictionaryRepository) ListActive(ctx context.Context, table string) ([]*DictionaryEntryRow, error) {
    q := sq.Select(dictionarySelectColumns...).
        From(table).
        Where(sq.Eq{DictColumnActive: true}).
        OrderBy(DictColumnSortOrder, DictColumnCode)
    return dbutil.Many[DictionaryEntryRow](ctx, r.db, q)
}

func (r *dictionaryRepository) GetActiveByCodes(ctx context.Context, table string, codes []string) ([]*DictionaryEntryRow, error) {
    if len(codes) == 0 {
        return nil, nil
    }
    q := sq.Select(dictionarySelectColumns...).
        From(table).
        Where(sq.Eq{DictColumnCode: codes}).
        Where(sq.Eq{DictColumnActive: true})
    return dbutil.Many[DictionaryEntryRow](ctx, r.db, q)
}
```

**Обновить `factory.go`:** добавить `NewDictionaryRepo(db dbutil.DB) DictionaryRepo` возвращающий `&dictionaryRepository{db: db}`.

**Создать `backend/internal/repository/dictionary_test.go`:**
- Test ListActive по таблице categories — возвращает упорядоченные строки (sort_order, code)
- Test ListActive фильтрует active=false
- Test GetActiveByCodes по таблице cities — фильтрует по кодам
- Test GetActiveByCodes пустой codes input — возвращает nil без хита БД
- Test GetActiveByCodes неизвестный код — отсутствует в результате
- Используем pgxmock как в `category_test.go` (pattern уже есть)

**Mockery regen:**
```bash
cd backend && go run github.com/vektra/mockery/v3 ...   # или make mock-gen
```

**Verification:**
```bash
cd backend
go build ./internal/repository/...
go test ./internal/repository/... -count=1 -race
```

### 3.5 Service layer

**Создать `backend/internal/service/dictionary.go`:**
```go
package service

import (
    "context"

    "github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
    "github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
    "github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
    "github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

type DictionaryRepoFactory interface {
    NewDictionaryRepo(db dbutil.DB) repository.DictionaryRepo
}

// dictionaryTables maps user-facing dictionary type onto the underlying DB table.
// Adding a new dictionary = new entry here + new DB table + update domain enum.
var dictionaryTables = map[domain.DictionaryType]string{
    domain.DictionaryTypeCategories: repository.TableCategories,
    domain.DictionaryTypeCities:     repository.TableCities,
}

type DictionaryService struct {
    db          dbutil.DB
    repoFactory DictionaryRepoFactory
    logger      logger.Logger
}

func NewDictionaryService(db dbutil.DB, repoFactory DictionaryRepoFactory, log logger.Logger) *DictionaryService {
    return &DictionaryService{db: db, repoFactory: repoFactory, logger: log}
}

func (s *DictionaryService) List(ctx context.Context, t domain.DictionaryType) ([]domain.DictionaryEntry, error) {
    table, ok := dictionaryTables[t]
    if !ok {
        return nil, domain.ErrDictionaryUnknownType
    }
    rows, err := s.repoFactory.NewDictionaryRepo(s.db).ListActive(ctx, table)
    if err != nil {
        return nil, err
    }
    out := make([]domain.DictionaryEntry, len(rows))
    for i, r := range rows {
        out[i] = domain.DictionaryEntry{Code: r.Code, Name: r.Name, SortOrder: r.SortOrder}
    }
    return out, nil
}
```

**Создать `backend/internal/service/dictionary_test.go`:**
- Unknown type → `ErrDictionaryUnknownType`
- Categories type → mock factory вернул repo → repo вернул rows → mapped корректно
- Cities type → аналогично
- Repo error пропагируется

**Обновить `creator_application.go` service:**
- В `CreatorApplicationRepoFactory` заменить `NewCategoryRepo` на `NewDictionaryRepo`
- В `Submit`: `categoryRepo := s.repoFactory.NewDictionaryRepo(tx)`
- `resolveCategoryIDs(ctx, categoryRepo, repository.TableCategories, in.CategoryCodes)` — добавить параметр table
- `requireAllConsents(in.Consents)` → заменить на:
  ```go
  if !in.Consents.AcceptedAll {
      return nil, domain.NewValidationError(domain.CodeMissingConsent, "Требуется согласие со всеми условиями")
  }
  ```
- Удалить `consentLabelRU`
- В `buildConsentRows`: цикл по `domain.ConsentTypeValues` остаётся, но без проверки AsMap — всё равно все true (валидация уже прошла)
- Добавить валидацию `MaxCategoriesPerApplication`:
  ```go
  if len(in.CategoryCodes) > domain.MaxCategoriesPerApplication {
      return nil, domain.NewValidationError(domain.CodeValidation,
          fmt.Sprintf("Максимум %d категории", domain.MaxCategoriesPerApplication))
  }
  ```
- Добавить валидацию `category_other_text` (в `trimAndValidateRequired` или отдельной функции):
  ```go
  hasOther := false
  for _, c := range in.CategoryCodes {
      if c == "other" { hasOther = true; break }
  }
  if hasOther {
      txt := strings.TrimSpace(stringPtrValue(in.CategoryOtherText))
      if txt == "" {
          return nil, domain.NewValidationError(domain.CodeValidation,
              "Укажите название категории в поле «Другое»")
      }
      if len(txt) > 200 {
          return nil, domain.NewValidationError(domain.CodeValidation,
              "Текст категории «Другое» слишком длинный (макс. 200 символов)")
      }
      trimmed.CategoryOtherText = &txt
  }
  ```
- Сохранение: добавить `CategoryOtherText: trimOptional(in.CategoryOtherText)` в `appRepo.Create(...)` payload
- Поднять MinCreatorAge: проверить где `EnsureAdult` живёт. Если константа `domain.MinCreatorAge=18` — обновить на 21. Сообщение `"Возраст менее 21 года"`.

**Обновить `creator_application_test.go`:**
- Все тесты с consents — заменить 4 поля на `AcceptedAll: true`
- Новый тест: `acceptedAll=false` → `MISSING_CONSENT`
- Новый тест: 4 категории → `VALIDATION_ERROR`
- Новый тест: «other» в кодах + пустой `CategoryOtherText` → `VALIDATION_ERROR`
- Новый тест: «other» + длинный `CategoryOtherText` (201 char) → `VALIDATION_ERROR`
- Новый тест: «other» + валидный текст → запись с `CategoryOtherText` в creator_application
- Новый тест: возраст 19 (валидный ИИН) → `UNDER_AGE`
- Новый тест: соцсеть `threads` принимается
- Обновить ВСЕ существующие тесты которые passed `acceptedAll=false` (старые consents=4xtrue)

**Verification:**
```bash
cd backend
go build ./internal/service/...
go test ./internal/service/... -count=1 -race
```

### 3.6 Handler layer

**Создать `backend/internal/handler/dictionary.go`:**
```go
package handler

import (
    "errors"
    "net/http"

    "github.com/alikhanmurzayev/ugcboost/backend/internal/api"
    "github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

func (s *Server) ListDictionary(w http.ResponseWriter, r *http.Request, typeParam string) {
    entries, err := s.dictionaryService.List(r.Context(), domain.DictionaryType(typeParam))
    if err != nil {
        if errors.Is(err, domain.ErrDictionaryUnknownType) {
            respondError(w, r,
                domain.NewValidationError(domain.CodeNotFound, "Unknown dictionary type"),
                s.logger)
            return
        }
        respondError(w, r, err, s.logger)
        return
    }
    items := make([]api.DictionaryEntry, len(entries))
    for i, e := range entries {
        items[i] = api.DictionaryEntry{Code: e.Code, Name: e.Name, SortOrder: e.SortOrder}
    }
    respondJSON(w, r, http.StatusOK, api.DictionaryListResult{
        Data: api.ListDictionaryData{Type: typeParam, Items: items},
    }, s.logger)
}
```

**Note:** Точная сигнатура `ListDictionary` зависит от того, как oapi-codegen сгенерил параметры — может быть `(w, r, params api.ListDictionaryParams)` или прямой param. Сверить с `api/server.go` после codegen.

**Обновить Server constructor (`backend/internal/handler/server.go`):**
- Добавить `dictionaryService DictionaryService` в struct и в `NewServer(...)` сигнатуру
- Обновить порядок параметров — посмотреть существующую конвенцию

**Обновить wire-up (`backend/cmd/server/main.go` или где собирается Server):**
- Создать `DictionaryService` и передать в `NewServer`

**Создать `backend/internal/handler/dictionary_test.go`:**
- GET /dictionaries/categories → 200, корректный JSON
- GET /dictionaries/cities → 200
- GET /dictionaries/unknown → 404 (с кодом NOT_FOUND или VALIDATION_ERROR — зависит от того, как codegen валидирует enum)
- Service error → 500

**Обновить `creator_application_test.go` handler:**
- Все тесты с request body — `consents` → `acceptedAll`
- Добавить test: request с `categoryOtherText` проходит до service (handler передаёт корректно)
- Все упоминания `apiConsentsToDomain` обновить под новую схему

**Обновить `creator_application.go` handler:**
- `apiConsentsToDomain` → упростить:
  ```go
  func apiConsentsToDomain(in api.CreatorApplicationCreate) domain.ConsentsInput {
      return domain.ConsentsInput{AcceptedAll: in.AcceptedAll}
  }
  ```
  (или inline в input := { ... })
- Добавить `CategoryOtherText: req.CategoryOtherText` в `domain.CreatorApplicationInput`

**Verification:**
```bash
cd backend
go build ./internal/handler/...
go test ./internal/handler/... -count=1 -race
```

### 3.7 E2E

**Обновить `backend/e2e/creator_application/creator_application_test.go`:**
- Все тесты — `consents` объект → `acceptedAll: true`
- Новый тест: «other» + `categoryOtherText` → 201 + запись в БД (`SELECT category_other_text FROM creator_applications WHERE id = ...`)
- Новый тест: 4 категории → 422 `VALIDATION_ERROR`
- Новый тест: соцсеть `threads` → 201
- Тест `under-21`: `birth = time.Now().UTC().AddDate(-19, 0, 0)` — должен быть UNDER_AGE
- Тест `under-18` → переименовать в `under-21` или оставить оба

**Создать `backend/e2e/dictionary/dictionary_test.go`:**
- `GET /dictionaries/categories` → 200, в items есть `home_diy/animals/other`, нет `gaming`, `sortOrder` отсортирован по возрастанию
- `GET /dictionaries/cities` → 200, 17 items, top-3 = Алматы/Астана/Шымкент
- `GET /dictionaries/unknown` → 4xx (зависит от codegen-валидации path enum — может быть 400 от codegen-роутера до достижения handler)

**Verification:**
```bash
cd backend && make test-e2e
```

### 3.8 Очистка dangling references

```bash
cd backend
grep -rn "CategoryRepo\|categoryRepo\|NewCategoryRepo" internal/ cmd/ e2e/ | grep -v "_test\|mocks/"
# Должно быть пусто или только в обновлённых файлах

grep -rn "AsMap\|consentLabelRU\|requireAllConsents" internal/ | grep -v "_test"
# Должно быть пусто

grep -rn "ConsentsInput.*Processing\|ThirdParty\|CrossBorder" internal/ | grep -v "_test"
# Должно быть пусто
```

### 3.9 Сводный backend verification

```bash
cd backend
make test
# Ожидаем: build OK, lint OK, all unit tests pass, coverage ≥ baseline (80%), E2E pass
```

Coverage проверка: финальный % должен быть ≥ baseline (зафиксированный в P4). Если упал — дописать тесты на новый код (DictionaryService, рефакторинг creator_application service).

---

## Шаг 4: Frontend — лендинг

### 4.1 `frontend/landing/src/content.ts`

- Удалить hardcoded `categories` array (теперь из API)
- Удалить hardcoded `cities` array (теперь из API)
- Оставить hardcoded `socials` (по решению — ENUM)
- Оставить остальное (`successScreen`, `criteria`, тексты)

### 4.2 `frontend/landing/src/pages/index.astro`

**Подгрузка словарей:**
- Добавить inline `<script>` (или Astro client island) который при загрузке формы делает:
  ```js
  const [catRes, cityRes] = await Promise.all([
    fetch(`${window.UGCBOOST_CONFIG.apiUrl}/dictionaries/categories`),
    fetch(`${window.UGCBOOST_CONFIG.apiUrl}/dictionaries/cities`),
  ]);
  ```
- Дёргать перед открытием модалки/формы или при `DOMContentLoaded`
- Использовать ответ для рендера `<select name="city">` опций и `<input type="checkbox" data-category="{code}">` категорий
- Loading state: пока загружаются — показать spinner или disable submit
- Error state: если API упал — показать сообщение «попробуйте позже», disable форму

**Form changes:**
- 1 чекбокс согласий вместо 2: `<input type="checkbox" name="acceptedAll" required>` с лейблом ссылающимся и на Соглашение, и на Политику
- Соцсеть-input: `<input type="text" placeholder="@username">` вместо `type="url"`
- middleName: убрать `required` атрибут
- При выборе категории `other` (через `data-category="other"`) — показать `<input name="categoryOtherText" maxlength="200">`

**Submit handler:**
```js
form.addEventListener('submit', async (e) => {
  e.preventDefault();
  const data = collectFormData(form);
  try {
    const res = await fetch(`${window.UGCBOOST_CONFIG.apiUrl}/creators/applications`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    const body = await res.json();
    if (res.ok) {
      showSuccessScreen(body.data.telegram_bot_url);
    } else {
      showError(body.error?.message || 'Ошибка отправки заявки');
    }
  } catch (err) {
    showError('Сеть недоступна. Попробуйте позже.');
  }
});
```

**Success screen:**
- Заменить hardcoded `successScreen.botLink` на динамический `data.telegram_bot_url` из ответа
- Если `botLink` пустой — показываем success-screen без CTA-кнопки (только текст «Заявка принята»). Не падаем, не шумим — env-check на старте бэка предотвращает кейс (см. Q5)

**Удалить:** `alert("Заявка отправлена (демо)...")` и весь демо-код

### 4.3 Конфиг

- Проверить `frontend/landing/public/config.js`:
  ```js
  window.UGCBOOST_CONFIG = { apiUrl: '...' };
  ```
- Если файла нет — создать с placeholder, в README/docs указать что прод подменяет config.js при деплое

### 4.4 Verification

```bash
cd frontend/landing
npm install
npm run build
# ожидаем: успешная сборка, без warnings о неразрешённых импортах
npm run check  # если есть astro check — type errors
```

Manual: открыть локально (`npm run dev`), убедиться что:
- Категории и города подгружаются (Network tab — 2 GET-запроса к /dictionaries)
- Submit отправляет JSON (Network tab — POST с правильным телом)

---

## Шаг 5: Финальная верификация

### 5.1 Локальная сборка end-to-end

```bash
cd backend && make test          # build + lint + unit + coverage + E2E
cd frontend/landing && npm run build && npm run check
```

### 5.2 Playwright golden-path smoke

**Setup:** убить старые Playwright процессы (per memory `feedback_playwright_cleanup`):
```bash
pkill -f playwright || true
```

**Поднять backend локально:**
```bash
cd backend && make run-local &
# ждём пока поднимется (curl /health)
```

**Поднять лендинг локально:**
```bash
cd frontend/landing && npm run dev &
# ждём
```

**Через Playwright MCP** прогнать сценарии:

**Сценарий 1 (golden path):**
1. `browser_navigate` → `http://localhost:4321`
2. `browser_snapshot` — убедиться что форма видна, категории подгрузились
3. `browser_fill_form` со всеми валидными полями (включая 2 категории, 1 соцсеть)
4. `browser_click` submit
5. `browser_wait_for` text=«Заявка отправлена» (или success-screen marker)
6. Проверить через psql: `SELECT id, last_name, status FROM creator_applications ORDER BY created_at DESC LIMIT 1;`

**Сценарий 2 (other category):**
1. Заполнить форму, выбрать «Другое» категорию → input для category_other_text появился
2. Заполнить, submit
3. psql: `SELECT category_other_text FROM creator_applications WHERE id = ?;` — не NULL

**Сценарий 3 (validation):**
1. Заполнить форму с возрастом 19 (валидный ИИН) → submit
2. Дождаться error message на лендинге, должно быть «Возраст менее 21 года»

**Cleanup:**
```bash
pkill -f playwright || true
# kill backend и фронт процессы
```

### 5.3 Diff обзор

```bash
git status                                  # working tree clean
git log --oneline main..HEAD                # видим все коммиты
git diff main..HEAD --stat | head -50       # список изменённых файлов
```

Сверить `git diff --stat` с этим планом — нет ли неожиданных файлов.

---

## Definition of Done (чеклист для Alikhan-revisit)

- [ ] Все unit-тесты бэка зелёные (`make test`)
- [ ] Все E2E зелёные
- [ ] Coverage ≥ baseline (зафиксированный на P4)
- [ ] Frontend build зелёный
- [ ] Spec обновлена (раздел Sync 2026-04-25 присутствует)
- [ ] PR #20 (aidana) НЕ помечен как merged через GitHub UI (только локальный merge)
- [ ] Все 11 решений отражены в коде:
  - [ ] 1 чекбокс на лендинге, `acceptedAll` в API, 4 ряда в БД
  - [ ] `threads` в social_platform enum + handle-regex принимает
  - [ ] Лендинг шлёт handle (не URL), placeholder=`@username`
  - [ ] Лендинг тянет categories из API
  - [ ] `category_other_text` поле в API+БД, валидация required при `other`
  - [ ] `gaming` удалён, `home_diy/animals/other` добавлены
  - [ ] MaxCategoriesPerApplication=3 валидируется
  - [ ] MinCreatorAge=21 валидируется
  - [ ] middleName optional на лендинге
  - [ ] aidana/landing-efw merged локально
  - [ ] Submit делает реальный fetch + success screen с динамическим botLink
- [ ] Dictionary system работает:
  - [ ] `GET /dictionaries/categories` — 200, отсортированный список
  - [ ] `GET /dictionaries/cities` — 200, 17 городов
  - [ ] `GET /dictionaries/unknown` — 4xx
- [ ] Playwright golden-path сценарий 1 проходит (запись в БД появилась)
- [ ] Playwright сценарий 2 (other) проходит
- [ ] Playwright сценарий 3 (validation) проходит
- [ ] Нет dangling references (grep по `CategoryRepo`, `AsMap`, etc.)
- [ ] `git status` чист или показывает только ожидаемые файлы
- [ ] **НЕ закоммичено и НЕ запушено** — Alikhan делает review в working tree (per memory `feedback_no_commits`)
- [ ] Этот документ обновлён по факту выполнения — deviations, решения по Q1-Q5, заметки в `## Execution log` ниже

---

## Risks & rollback

| Риск | Митигация |
|---|---|
| Codegen упал на новой OpenAPI | Ревьюнуть YAML, поправить, запустить ещё раз. Если упало на конкретном схеме — упростить |
| ALTER TYPE social_platform требует non-tx | Использовать `-- +goose NO TRANSACTION` (см. 3.2.2) |
| Конфликты при `git merge aidana/landing-efw` | STOP, сообщить, не делать `--abort` молча |
| Лендинг не может достучаться до API (CORS) | Проверить `backend/internal/middleware/cors.go` (если есть), добавить landing-origin в whitelist |
| Coverage упал ниже 80% | Дописать тесты на DictionaryService и новые ветки creator_application service. НЕ снижать gate |
| Playwright процессы зависают | `pkill -f playwright` перед каждым прогоном (memory `feedback_playwright_cleanup`) |
| Миграция категорий рушит существующие FK (creator_application_categories.category_id → categories.id для `gaming`) | На MVP в БД нет реальных заявок с `gaming` (продакта нет). Если в локальной/staging БД есть — TRUNCATE creator_application_categories перед миграцией. Проверить через SELECT перед DELETE |

---

## Execution log

*Обновляется по ходу работы — что сделано, deviations от плана.*

- _2026-04-25_ — план составлен, /interview по Q1-Q5 завершено (см. таблицу в начале).
- _2026-04-25_ — старт реализации. Pre-flight зелёный (lint, unit, coverage, E2E, build лендинга, lint лендинга).
- _2026-04-25_ — Шаг 1 spec обновлён, раздел "Sync with landing 2026-04-25" добавлен в `spec-creator-application-submit.md`.
- _2026-04-25_ — Шаг 2: `git merge --no-ff origin/aidana/landing-efw` — merge-коммит cdffc69, конфликтов нет, backend/landing собираются.
- _2026-04-25_ — Шаг 3.1: OpenAPI обновлён (Dictionary endpoint+схемы, threads enum, acceptedAll, categoryOtherText). `make generate-api` зелёный.
- _2026-04-25_ — Шаг 3.2: 4 миграции созданы и накатаны. Внесена правка vs план: `social_platform` оказался TEXT+CHECK constraint, а не ENUM — миграция меняет CHECK constraint, не `ALTER TYPE`. Down/up cycle проходит. Migrations накатаны через локальный `goose` напрямую (Docker migrations контейнер пересобрался уже после фикса handler).
- _2026-04-25_ — Шаг 3.3: domain/dictionary.go создан. domain/creator_application.go обновлён (SocialPlatformThreads, MaxCategoriesPerApplication=3, CategoryCodeOther, упрощённый ConsentsInput.AcceptedAll, CategoryOtherText в Input). domain/iin.go: MinCreatorAge 18→21, ErrIINUnderAge18 → ErrIINUnderAge.
- _2026-04-25_ — Шаг 3.4: category.go/test/mock удалены. dictionary.go (DictionaryEntryRow + DictionaryRepo + 2 метода с table param) и dictionary_test.go созданы. factory.go: NewDictionaryRepo вместо NewCategoryRepo.
- _2026-04-25_ — Шаг 3.5: service/dictionary.go (DictionaryService с map type→table). Полный refactor service/creator_application.go: NewDictionaryRepo, упрощённый consents check, MaxCategories валидация, validateCategoryOtherText helper, MinCreatorAge=21 message. Тесты обновлены, dictionary_test.go добавлен.
- _2026-04-25_ — Шаг 3.6: handler/dictionary.go + handler/dictionary_test.go. Server расширен 6-м параметром DictionaryService (NewServer 6 сервисов вместо 5). Все 56 тестовых вызовов NewServer обновлены через sed, main.go тоже. ListDictionary мапит ErrDictionaryUnknownType → 404 через domain.ErrNotFound.
- _2026-04-25_ — Шаг 3.7: e2e/creator_application обновлён (acceptedAll, threads test, other test, max-categories test, переименование под21). e2e/dictionary новый: GET categories/cities. Все E2E green.
- _2026-04-25_ — Шаг 3.8-3.9: dangling refs grep пуст. Финальный сводный test зелёный (lint 0 issues, coverage handler 95%/service 94.6%/repo 98.9%/middleware 100%/authz 100%, E2E 13+2 PASS).
- _2026-04-25_ — Шаг 4: content.ts чистится от cities/categories (оставлены socials + socialPlatformCodes mapping). index.astro: 1 чекбокс, type=text + placeholder=`@username в X`, middleName без required, dynamic city select + category checkboxes из API, success-screen, fetch handler. public/config.js (dev fallback). Использует существующую конвенцию `window.__RUNTIME_CONFIG__`.
- _2026-04-25_ — Шаг 5: финальный build + lint лендинга зелёные. **Browser smoke не выполнен** — Playwright MCP отключился во время сессии. Сделана ручная проверка через curl: `/dictionaries/cities` → 17 городов, `/dictionaries/categories` → 12 категорий с правильным sort_order, `/dictionaries/unicorns` → 404, лендинг рендерит правильный HTML с id=application-form/city-select/category-options/consent-all/success-screen, сервит config.js с правильным apiUrl. Полный submit-flow в браузере **не проверен автоматически** — нужно ручное smoke-тестирование Alikhan.

### Deviations / заметки

- **CORS_ORIGINS в backend/.env** дополнен `http://localhost:3003,http://localhost:4321` — нужно для local landing+astro dev. .env в gitignore — Alikhan повторит правку у себя локально (или через staging .env у Dokploy).
- **Migrations для local dev** были применены локальным `goose`, не через Docker `make migrate-up` — это связано с тем, что Docker `migrations` строит из тех же исходников что backend, а backend временно не собирался во время рефакторинга. После завершения рефакторинга Docker-сборка прошла успешно (через test-e2e-backend → start-backend → migrations → backend). На staging/prod миграции прокатятся стандартным путём через CI.
- **Browser smoke deferred** — Playwright MCP стал недоступен в середине сессии. API-сторона проверена через curl и E2E. UI-сторона требует ручного smoke от Alikhan: пройти golden path через браузер, проверить успех-экран и редирект на бот.

### Что Alikhan получит при ревью

- Working tree содержит ~30 modified + 10 new + 3 deleted файла, разбитые по слоям как в плане.
- 7 коммитов уже существующих в ветке (PR #19 + merge Айданы), новых коммитов **не сделано** (per memory `feedback_no_commits`).
- Все автотесты зелёные. Coverage по gated пакетам выше baseline.
- Spec и plan живы, отражают финальное состояние.
