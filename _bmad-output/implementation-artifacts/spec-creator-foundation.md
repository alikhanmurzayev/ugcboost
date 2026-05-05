---
title: "Бэк-фундамент сущности креатора + retry в Telegram-нотификаторе (chunk 18a)"
type: feature
created: "2026-05-05"
status: ready-for-dev
baseline_commit: "19da316"
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/creator-onboarding-roadmap.md
  - _bmad-output/implementation-artifacts/spec-creator-application-approve.md (старая большая спека сохранена как референс; декомпозирована на 18a/18b/18c)
---

> Перед реализацией обязательно полностью загрузить все файлы `docs/standards/` — это hard rules, спека не дублирует.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Chunk 18 целиком (approve action + GET creator-aggregate + retry-рефактор + новый notify-метод + e2e) — ~5K LoC в одном PR, риск drift'а между OpenAPI ↔ service ↔ handler. Декомпозируем на три ровных под-чанка.

**18a (этот PR): инфраструктура без эффекта.** Закладываем DB-таблицы и 3 repo, рефакторим `Notifier.fire()` под retry. Никакого OpenAPI / domain types / service / handler / authz / e2e. После merge 18a в проде остаются 3 пустых таблицы и более надёжная отправка существующих 3 нотификаций (linked / verification_approved / rejected) — никаких поведенческих изменений для пользователя.

**Approach:** Одна forward-only миграция с 3 таблицами. 3 новых repo с минимально-достаточным набором методов. 23505-mapping в `CreatorRepo.Create` на 3 constraint'а. Retry-обёртка в `Notifier.fire()` через `cenkalti/backoff/v5` (двойной context: outer max-elapsed + per-attempt sub-ctx). Локальный smoke прямо в `cmd/api/main.go` подтверждает работоспособность всех методов всех 3 repo с захардкоженными данными — финальным коммитом PR удаляется, main.go возвращается в чистое состояние.

## Decisions

### Сущность креатора в БД

`creators` — основная строка, плоский снимок заявки в момент approve (PII + Telegram-блок + ссылка на исходную заявку). `creator_socials` / `creator_categories` — snapshot M2M с заявкой по той же конвенции `{owner-singular}_{children-plural}` что и существующие `creator_application_*`. Уникальности: `iin`, `telegram_user_id`, `source_application_id` — все три играют race-protect и detection через `pgErr.ConstraintName` в repo. Ничего из заявки про verification_code / consents / status_transitions сюда не уезжает — это артефакты процесса подачи.

В 18a таблицы создаются пустыми, никакой код их не читает. `RepoFactory` получает 3 новых конструктора, но service-side `CreatorApplicationRepoFactory` / `CreatorRepoFactory` интерфейсы пока не расширяются — это сделают 18b/18c, где появятся потребители.

### Sentinel'ы без user-facing codes пока

В 18a в `domain/creator.go` (новый файл) живут только три sentinel'а — `ErrCreatorAlreadyExists`, `ErrCreatorTelegramAlreadyTaken`, `ErrCreatorApplicationNotApprovable` — нужны для 23505-switch в repo. **User-facing коды и actionable-message** для них появятся в 18b, где handler начнёт их маппить в HTTP-ответ. До тех пор sentinel'ы достаточно с обычным `errors.New("...")` сообщением.

Domain-types для самого `Creator` / `CreatorSocial` / `CreatorCategory` (не Row, а domain) **не вводятся** в 18a — потребителя нет. Появятся в 18b (`ApproveApplication` собирает `Creator` из `CreatorApplicationRow`+link для INSERT'ов в repo) и в 18c (`CreatorAggregate` для GET-handler'а).

### Retry в notifier — orthogonal

Рефактор `Notifier.fire()` через `cenkalti/backoff/v5` (уже в `go.mod`). Применяется ко всем 3 существующим notify-методам (`NotifyApplicationLinked` / `NotifyVerificationApproved` / `NotifyApplicationRejected`) — каждый получает retry бесплатно.

**Архитектура context'ов** (правится из-за того, что нынешний `fire()` ставит один `notifyCtx` с `n.timeout=10s` — этого мало для retry):

- **Outer context** — `context.WithTimeout(context.WithoutCancel(ctx), n.maxElapsed)` где `maxElapsed=30s`. Хранит весь retry-loop. `WithoutCancel` сохраняет существующий инвариант `caller cancellation does not abort the notify` (тест в `notifier_test.go:314`).
- **Per-attempt sub-context** — внутри `operation`: `context.WithTimeout(outerCtx, n.timeout)` где `timeout=10s` (как сейчас). Передаётся в `sender.SendMessage`.
- Retry чтит `outerCtx.Done()` — closer-shutdown прерывает sleep между попытками.

**Параметры retry:**
- Initial interval 1s, multiplier 2.0, max interval 8s → backoff'ы 1s/2s/4s.
- Max attempts 4 (1 initial + 3 retry).
- Max elapsed time 30s (hard cap).

**Классификация ошибок:**
- Retryable: HTTP 5xx, 429 Too Many Requests, network errors (timeout, dial error, connection refused).
- Non-retryable (terminal, обёртывается `backoff.Permanent`): HTTP 4xx кроме 429 (bad request, forbidden / бот забанен, chat not found), `ctx.Err()`.

**Логи:**
- Промежуточные неудачи — `log.Warn` с `op`, `chat_id`, `attempt`, `error`.
- Финальная неудача (исчерпан retry или terminal) — `log.Error` (как сейчас).
- Успех — без отдельного лога.
- PII — никаких имён / IIN / handle. Только uuid / chat_id / op / attempt / error.

`Notifier` получает дополнительные параметры через расширение конструктора: `NewNotifier(sender, log)` остаётся для production, добавляется `NewNotifierWithBackoff(sender, log, initial, max, multiplier, maxElapsed time.Duration, maxAttempts int)` — используется тестами для миллисекундных backoff'ов. Set-методы запрещены стандартом.

### Smoke в `cmd/api/main.go` — без env-флагов

Функция `runCreatorRepoSmoke(ctx, pool, log)` вызывается **безусловно** из `main.go` после bootstrap'а `*pgxpool.Pool`, ДО `srv.ListenAndServe`. Прогоняет полный набор операций над всеми тремя repo с захардкоженными данными, печатает результат каждого шага через `fmt.Println` / `log.Info`, при любой ошибке — `log.Fatal` с понятным сообщением и `os.Exit(1)`. По завершении successful-прогона — `os.Exit(0)`.

После того как автор PR убедился локально, что все 13 проверок (см. § Local Smoke Acceptance) пройдены — функция и её вызов из `main.go` **удаляются финальным коммитом** в том же PR. Перед merge'ем `git diff main..HEAD -- backend/cmd/api/main.go` показывает только структурные/тривиальные изменения (например, импорт логера для notify-конструктора, если он там нужен), но не smoke-код.

**PR с smoke-кодом в финальном состоянии main.go merge'у не подлежит.** Это hard-инвариант (см. § Never).

## Boundaries & Constraints

**Always:**

- Migration: одна forward-only goose-миграция `<timestamp>_creators.sql`. Создаёт 3 таблицы + UNIQUE/FK/CHECK. Никаких regex / length CHECK. Constraint names — exported константы в repo (зеркало `CreatorApplicationTelegramLinksPK` / `_application_id_fkey`):
  - `creators_iin_unique`
  - `creators_telegram_user_id_unique`
  - `creators_source_application_id_unique`
  - `creator_socials_creator_platform_handle_unique`
  - `creator_categories_creator_category_code_unique`
- Down-миграция — DROP в обратном порядке (нет prod-данных, бэкфила нет).
- `creators` поля: `id` UUID PK, `iin` TEXT NOT NULL UNIQUE, `last_name` / `first_name` TEXT NOT NULL, `middle_name` TEXT NULL, `birth_date` DATE NOT NULL, `phone` TEXT NOT NULL, `city_code` TEXT NOT NULL → `cities(code)`, `address` TEXT NULL, `category_other_text` TEXT NULL, `telegram_user_id` BIGINT NOT NULL UNIQUE CHECK > 0, `telegram_username` / `telegram_first_name` / `telegram_last_name` TEXT NULL, `source_application_id` UUID NOT NULL UNIQUE → `creator_applications(id)`, `created_at` / `updated_at` TIMESTAMPTZ DEFAULT now().
- `creator_socials` поля: `id` UUID PK, `creator_id` UUID NOT NULL → `creators(id)` ON DELETE CASCADE, `platform` TEXT CHECK IN (`instagram`, `tiktok`, `threads`), `handle` TEXT NOT NULL, `verified` BOOLEAN NOT NULL DEFAULT false, `method` TEXT NULL, `verified_by_user_id` UUID NULL → `users(id)` ON DELETE SET NULL, `verified_at` TIMESTAMPTZ NULL, `created_at` TIMESTAMPTZ DEFAULT now(), UNIQUE `(creator_id, platform, handle)`. Index на `creator_id`.
- `creator_categories` поля: `id` UUID PK, `creator_id` UUID NOT NULL → `creators(id)` ON DELETE CASCADE, `category_code` TEXT NOT NULL → `categories(code)`, `created_at` TIMESTAMPTZ DEFAULT now(), UNIQUE `(creator_id, category_code)`. Indexes на оба FK-столбца.
- 3 новых repo (`backend/internal/repository/creator.go`, `creator_social.go`, `creator_category.go`). Stom-теги, `sortColumns`, паттерн зеркало `creator_application_*.go`. Имя receiver'а: `r *creatorRepository` / `r *creatorSocialRepository` / `r *creatorCategoryRepository`.
- `CreatorRepo` интерфейс: `Create(ctx, row) (*CreatorRow, error)`, `GetByID(ctx, id) (*CreatorRow, error)`, `DeleteForTests(ctx, id) error`. 23505 в `Create` транслируется по `pgErr.ConstraintName` в один из трёх sentinel'ов.
- `CreatorSocialRepo` интерфейс: `InsertMany(ctx, rows) error`, `ListByCreatorID(ctx, creatorID) ([]*CreatorSocialRow, error)` (`ORDER BY platform ASC, handle ASC` — детерминированный порядок для будущих E2E ассертов).
- `CreatorCategoryRepo` интерфейс: `InsertMany(ctx, rows) error`, `ListByCreatorID(ctx, creatorID) ([]string, error)` (возвращает `[]category_code`, `ORDER BY category_code ASC`).
- `RepoFactory` (`repository/factory.go`) расширяется тремя конструкторами `NewCreatorRepo` / `NewCreatorSocialRepo` / `NewCreatorCategoryRepo`. Все принимают `dbutil.DB`.
- `domain/creator.go` (новый): 3 sentinel'а через `errors.New(...)` (без user-facing codes). Файл не содержит domain types — они появятся в 18b/18c.
- Notifier retry: outer-ctx `WithTimeout(WithoutCancel(ctx), maxElapsed=30s)` + per-attempt sub-ctx `WithTimeout(outerCtx, timeout=10s)`. Backoff initial 1s / multiplier 2.0 / maxInterval 8s. Max attempts 4. Retryable классификация — функция `isRetryable(err error) bool` в notifier.go.
- `NewNotifier(sender, log)` сохраняет существующую поверхность; новый `NewNotifierWithBackoff(...)` принимает все retry-параметры, используется тестами.
- Smoke-функция `runCreatorRepoSmoke(ctx, pool, log)` в `cmd/api/main.go` (или в новом `cmd/api/smoke.go` рядом с main.go в том же пакете). Вызов безусловный, перед `srv.ListenAndServe`. После прогона — `os.Exit(0)` чтобы сервер не стартовал в smoke-режиме.

**Ask First (BLOCKING до Execute):**
- (нет — все вопросы зарезолвлены)

**Never:**
- OpenAPI changes.
- Domain types для `Creator` / `CreatorSocial` / `CreatorCategory` (потребителей нет).
- User-facing codes / actionable messages для трёх новых sentinel'ов (привяжем в 18b).
- Service-методы (`ApproveApplication`, `CreatorService.GetByID` и т.п.).
- Handler-эндпоинты.
- Authz-методы.
- E2E тесты.
- Notifier `NotifyApplicationApproved` (новый метод) и текст `applicationApprovedText`.
- Расширение `creatorAppNotifier` интерфейса в `service/creator_application.go`.
- Расширение service-side factory-интерфейсов (`CreatorApplicationRepoFactory` и т.п.).
- DDL business defaults / regex-CHECK / length-CHECK / бэкфил.
- env-флаги вокруг smoke-функции (хардкод-вызов; удалять смок-кодом, не флагом).
- Final-коммит PR со smoke-кодом в `cmd/api/main.go`. Перед merge — main.go без smoke-следов.
- `Set*`-методы в `Notifier` для DI backoff-параметров (только конструктор-расширение).
- Новые миграции внутри 18a для добавления audit-row или transition-row (приходят в 18b).

## Local Smoke Acceptance

Автор PR обязан **лично** прогнать `make migrate-reset && make migrate-up && go run ./cmd/api` локально (свежая БД ⇒ предсказуемый baseline) и убедиться, что все 14 шагов отработали успешно. Только после этого smoke-код удаляется финальным коммитом и PR идёт в review.

Каждый шаг печатает результат через `log.Info` / `fmt.Println` (короткое сообщение `<step-name>: OK`), при ошибке — `log.Fatal` с понятным контекстом и stop.

**Setup-зависимости** (FK creators → creator_applications, cities, categories):
- `creators.source_application_id → creator_applications(id)` — обязателен parent-row. Smoke сначала создаёт 4 заявки в `creator_applications` (по одной на каждый race-сценарий шагов 3/5/6/7).
- `creators.city_code → cities(code)` — используем `'almaty'` (есть в seed `20260425205626_cities.sql`).
- `creator_categories.category_code → categories(code)` — используем `'fashion'`, `'lifestyle'`, `'food'` (все в seed `20260420181745_categories.sql`).
- `creator_socials.verified_by_user_id → users(id)` — все 3 социалки smoke'а вставляются с `verified=false`, `verified_by_user_id=NULL`, `method=NULL`, `verified_at=NULL`. Verification-ветки тестируются на сервисном уровне в 18b (snapshot из заявки), здесь достаточно репо-канала.

Setup-заявки создаются прямым SQL `INSERT INTO creator_applications (id, iin, last_name, first_name, birth_date, phone, city_code, status, ...) VALUES (...)` (4 уникальных UUID + 4 уникальных ИИН: `123456789012/13/14/15`, статус — любой валидный, важен лишь FK). Локальный helper `seedSmokeApplications(ctx, pool) (appID1, appID2, appID3, appID4 string, err error)` рядом со smoke-функцией.

| # | Действие | Ожидание | Печать |
|---|---|---|---|
| 0 | `seedSmokeApplications(ctx, pool)` — 4 заявки | 4 row'а в `creator_applications` | `setup.applications: OK count=4 ids=<a1,a2,a3,a4>` |
| 1 | `creatorSocialsRepo.InsertMany([])` | nil error (no-op) | `socials.empty-insert: OK` |
| 2 | `creatorCategoriesRepo.InsertMany([])` | nil error (no-op) | `categories.empty-insert: OK` |
| 3 | `creatorRepo.Create(rowFull)` где `source_application_id=appID1`, `iin='123456789012'`, `telegram_user_id=9999999991`, `city_code='almaty'`, ФИО / phone / birth_date — синтетика | row с непустым `id`, `created_at`, `updated_at` | `creator.create: OK id=<uuid>` |
| 4 | `creatorRepo.GetByID(creator1.ID)` | row, **все поля 1-в-1** с `rowFull` (`reflect.DeepEqual` на структуре с подменёнными dynamic-полями) | `creator.get-by-id: OK fields-match` |
| 5 | `creatorRepo.Create(rowSameIIN)` где `source_application_id=appID2`, `iin='123456789012'` (тот же что в #3), `telegram_user_id=9999999992` | `errors.Is(err, ErrCreatorAlreadyExists)` | `creator.iin-conflict: OK` |
| 6 | `creatorRepo.Create(rowSameTelegram)` где `source_application_id=appID3`, `iin='999999999999'` (другой), `telegram_user_id=9999999991` (тот же что в #3) | `errors.Is(err, ErrCreatorTelegramAlreadyTaken)` | `creator.telegram-conflict: OK` |
| 7 | `creatorRepo.Create(rowSameSourceApp)` где `source_application_id=appID1` (тот же что в #3), `iin='888888888888'`, `telegram_user_id=9999999993` | `errors.Is(err, ErrCreatorApplicationNotApprovable)` | `creator.source-app-conflict: OK` |
| 8 | `creatorSocialsRepo.InsertMany([{instagram,h1,verified:false}, {tiktok,h2,verified:false}, {threads,h3,verified:false}])` — 3 социалки, **все `verified=false`, `verified_by_user_id=NULL`, `method=NULL`, `verified_at=NULL`** | nil error | `socials.insert: OK rows=3` |
| 9 | `creatorSocialsRepo.ListByCreatorID(creator1.ID)` | 3 ряда, порядок `instagram → threads → tiktok` (по `platform ASC`); все verified-поля = nil/false | `socials.list: OK count=3 order=ig,threads,tt` |
| 10 | `creatorCategoriesRepo.InsertMany([fashion, lifestyle, food])` | nil error | `categories.insert: OK rows=3` |
| 11 | `creatorCategoriesRepo.ListByCreatorID(creator1.ID)` | `[fashion, food, lifestyle]` (`ORDER BY category_code ASC`) | `categories.list: OK count=3` |
| 12 | `creatorRepo.DeleteForTests(creator1.ID)` | nil error | `creator.delete: OK` |
| 13 | `creatorRepo.GetByID(creator1.ID)` + `socials.ListByCreatorID(creator1.ID)` + `categories.ListByCreatorID(creator1.ID)` | первый — `errors.Is(err, sql.ErrNoRows)`; второй и третий — пустые слайсы (ON DELETE CASCADE) | `creator.cascade: OK get=NotFound socials=0 categories=0` |

Промежуточные cleanup'ы для шагов 5-7 не нужны — race-проверки идут на свежий creator1 без побочных INSERT'ов.

Setup-заявки (4 row'а) останутся в БД после прогона smoke'а — это **не проблема**: smoke предполагает свежий baseline `migrate-reset`, после следующего `migrate-reset` заявки уйдут вместе со всем. Чистить их вручную не надо.

После 14/14 — функция и её вызов из main.go удаляются финальным коммитом. `git log --oneline -2` в PR показывает: `feat(creator): foundation tables + repo + notifier retry` затем `chore(creator): drop smoke from main.go`.

## Code Map

> Baseline — TBD (зафиксируется после merge PR chunk 17).

- `backend/migrations/<timestamp>_creators.sql` — миграция 3 таблиц + индексы + constraint names.
- `backend/internal/domain/creator.go` — новый: 3 sentinel'а через `errors.New(...)`. Без types, без codes.
- `backend/internal/repository/creator.go` — новый: `CreatorRow`, `CreatorRepo` (Create / GetByID / DeleteForTests), 5 exported constraint-name константа (см. § Always), 23505-switch.
- `backend/internal/repository/creator_social.go` — новый: `CreatorSocialRow`, `CreatorSocialRepo` (InsertMany / ListByCreatorID).
- `backend/internal/repository/creator_category.go` — новый: `CreatorCategoryRow`, `CreatorCategoryRepo` (InsertMany / ListByCreatorID).
- `backend/internal/repository/factory.go` — patch: 3 новых конструктора.
- `backend/internal/repository/creator_test.go`, `creator_social_test.go`, `creator_category_test.go` — pgxmock-tests.
- `backend/internal/repository/factory_test.go` — patch: тесты на 3 новых конструктора (зеркало существующих кейсов).
- `backend/internal/telegram/notifier.go` — patch: переработка `fire()` (двойной context + retry-обёртка через `cenkalti/backoff/v5`). Хелпер `isRetryable(err) bool`. Новая функция `NewNotifierWithBackoff(...)` для DI таймингов в тестах. `Notifier` struct получает поля `initial`, `multiplier`, `maxInterval`, `maxElapsed`, `maxAttempts`. Существующий `NewNotifier(sender, log)` дефолтит их к prod-значениям.
- `backend/internal/telegram/notifier_test.go` — patch: новые retry-сценарии (см. § Test Plan). Существующие 4 теста продолжают проходить без правок.
- `backend/cmd/api/main.go` — patch: вызов `runCreatorRepoSmoke(ctx, pool, log)` после bootstrap'а pool, перед `srv.ListenAndServe`. Финальный коммит PR удаляет вызов. Smoke-функцию положить в новый файл `backend/cmd/api/smoke.go` (тот же `package main`) — вычистить файл целиком финальным коммитом.
- `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` — chunk 18 разбить на `[~] 18 (foundation)` / `[ ] 18.5 (approve)` / `[ ] 18.6 (aggregate)`. Текущий 18 → `[~]` при старте, `[x]` при merge этого PR.

## Tasks & Acceptance

**Pre-execution gates:**
- [ ] PR chunk 17 смержен в main; `baseline_commit` проставлен в frontmatter.

**Execution:**
- [ ] Migration 3 таблиц + индексы + constraint names. Smoke `make migrate-up` локально → 3 пустые таблицы видны в БД.
- [ ] Domain: 3 sentinel'а в `domain/creator.go`.
- [ ] Repository: 3 новых repo + расширение `RepoFactory` + unit-тесты + расширение `factory_test.go`.
- [ ] Telegram retry: `notifier.go` с двойным context'ом + retry + `isRetryable` + `NewNotifierWithBackoff` + расширение `notifier_test.go` retry-сценариями.
- [ ] Smoke: `cmd/api/smoke.go` + вызов из `main.go`. Локальный прогон 13/13 — все шаги OK.
- [ ] Финальный коммит PR: удалить `cmd/api/smoke.go` и вызов из `main.go`. `git diff main..HEAD -- backend/cmd/api/main.go` чист от smoke-следов.
- [ ] Roadmap: chunks 18 (foundation) → `[~]` при старте, `[x]` при merge.

**Acceptance Criteria:**
- Given свежий локальный pgsql после `make migrate-up`, when автор запускает `go run ./cmd/api`, then 13/13 smoke-шагов отрабатывают, программа завершается с кодом 0.
- Given smoke удалён финальным коммитом PR, when автор запускает `go run ./cmd/api`, then сервер стартует штатно (smoke-кода не существует, поведение не отличается от main).
- Given existing notifier-тесты, when запустить `go test ./internal/telegram/...`, then все существующие сценарии (`TestNotifier_FireAndForget` / `_NotifyApplicationLinked` / `_NotifyVerificationApproved` / `_NotifyApplicationRejected`) проходят без правок.
- Given новые retry-сценарии, when запустить тесты, then 5xx/429/network errors retry'ятся, 4xx (кроме 429) — нет, exhausted retry даёт final Error-лог, ctx-cancel прерывает loop.
- Given `make build-backend lint-backend test-unit-backend-coverage` после удаления smoke-кода, then всё зелёное, per-method coverage ≥ 80% на новых repo + retry-функциях.

## Test Plan

> Производное от `docs/standards/backend-testing-unit.md`, `backend-constants.md`, `security.md`, `backend-libraries.md`.

### Validation rules

В 18a нет handler / service-уровней — валидация не применяется. Repo трактует все входные данные как trusted (валидирует service в 18b).

### Security invariants

- PII в stdout-логах запрещена (`security.md` § PII). В smoke-функции при `log.Fatal` печатать только `creator_id` (UUID) / `iin-conflict` / `telegram-conflict` без значений ИИН / handle / phone. В retry-логах notifier — `op`, `chat_id`, `attempt`, `error` (классификация: `error.Error()` без PII там, где Telegram-API возвращает текст).
- Smoke-данные хардкодные — синтетический IIN (`123456789012`), синтетический telegram_user_id (`9999999999`). Это не реальная PII — допустимо в коде временно. После удаления финальным коммитом следов не остаётся.
- Notifier `NewNotifierWithBackoff` принимает `time.Duration`-параметры — никаких `int seconds` / unsafe-преобразований.

### Unit tests

#### `repository/creator_test.go` — `TestCreatorRepository_*`

Pgxmock (`backend-testing-unit.md` § Repository).

- `TestCreatorRepository_Create`:
  - Happy: точная SQL `INSERT INTO creators (...) RETURNING ...` через `captureQuery`, ассерт всех `insert`-tagged значений. Returned row: dynamic поля (`id`, `created_at`, `updated_at`) проверяются `NotEmpty` + `WithinDuration`, потом substitute + `require.Equal` целиком.
  - 23505 на `creators_iin_unique` ⇒ `errors.Is(err, ErrCreatorAlreadyExists)`.
  - 23505 на `creators_telegram_user_id_unique` ⇒ `errors.Is(err, ErrCreatorTelegramAlreadyTaken)`.
  - 23505 на `creators_source_application_id_unique` ⇒ `errors.Is(err, ErrCreatorApplicationNotApprovable)`.
  - 23505 с неизвестным `ConstraintName` ⇒ raw error пробрасывается с обёрткой.
  - Нерелевантный pgErr (например, 23503 FK violation) ⇒ обёртка с контекстом.
- `TestCreatorRepository_GetByID`:
  - Happy: `SELECT ... FROM creators WHERE id=$1`, маппинг row → struct.
  - `sql.ErrNoRows` пробрасывается wrapped.
- `TestCreatorRepository_DeleteForTests`:
  - n=1 → nil.
  - n=0 → `sql.ErrNoRows`.

#### `repository/creator_social_test.go`

- `TestCreatorSocialRepository_InsertMany`:
  - Happy: multi-row INSERT, точный SQL + аргументы. Empty input → no-op без SQL.
  - БД-ошибка обёрнута.
- `TestCreatorSocialRepository_ListByCreatorID`:
  - Happy: `SELECT ... ORDER BY platform ASC, handle ASC`. Маппинг 3 row'ов.
  - Empty result → nil слайс, nil error.

#### `repository/creator_category_test.go`

- `TestCreatorCategoryRepository_InsertMany`: то же что соцсети.
- `TestCreatorCategoryRepository_ListByCreatorID`: возвращает `[]string` codes, `ORDER BY category_code ASC`.

#### `repository/factory_test.go` — patch

3 новых `t.Run`: `NewCreatorRepo` / `NewCreatorSocialRepo` / `NewCreatorCategoryRepo` возвращают соответствующий интерфейс.

#### `telegram/notifier_test.go` — новые retry-сценарии

Все используют `NewNotifierWithBackoff` с миллисекундными таймингами (`initial=1ms`, `maxInterval=4ms`, `maxElapsed=20ms`, `maxAttempts=4`) — тесты быстрые, race-detector чистый.

- `TestNotifier_Retry_TransientThenSuccess`:
  - sender mock: 1st call → `&statusError{code: 503}`; 2nd → success.
  - Ожидание: ровно 2 вызова `SendMessage`. `log.Warn` ровно 1 раз с `attempt=1`. Финального `log.Error` нет.
- `TestNotifier_Retry_TransientThenSuccess_429`:
  - То же, но 429.
- `TestNotifier_Retry_NetworkError_Success`:
  - sender mock: 1st → `&net.OpError{...}`; 2nd → success.
  - Ровно 2 вызова, 1 Warn, 0 Error.
- `TestNotifier_Retry_Terminal_400_NoRetry`:
  - sender mock: 400 на первой.
  - Ровно 1 вызов. 0 Warn. 1 Error финальный.
- `TestNotifier_Retry_Terminal_403_NoRetry`:
  - То же с 403 (бот забанен).
- `TestNotifier_Retry_Exhausted`:
  - sender mock: 503 на каждой попытке.
  - Ровно 4 вызова. 3 Warn (`attempt=1, 2, 3`). 1 Error финальный с `attempts=4` и финальной error.
- `TestNotifier_Retry_CtxCancelledBreaksLoop`:
  - sender mock: 503 на первой попытке (transient). Параллельно через `time.AfterFunc(2ms, cancel)` отменяем outer-ctx.
  - Финальный `log.Error` с `ctx canceled`. Количество вызовов sender — 1 или 2 (race-зависимо, но тест не должен фейлить от этой неопределённости — ассертится только финальный лог).

Существующие 4 теста (`TestNotifier_FireAndForget`, `TestNotifier_NotifyApplicationLinked`, `_NotifyVerificationApproved`, `_NotifyApplicationRejected`) продолжают проходить без правок — `NewNotifier(sender, log)` сохраняет нынешнее поведение через дефолтные backoff-параметры (но в success-кейсах retry не активируется, в error-кейсах sender mock возвращает один и тот же error на любой попытке → exhausted retry → финальный Error).

> **Нюанс:** существующий тест `TestNotifier_NotifyApplicationLinked / sender error logged, Wait still drains` — sender mock возвращает error один раз. После рефактора с retry он будет вызван 4 раза. Тест надо адаптировать: либо ожидать 4 вызова `sender.SendMessage` (через `mock.EXPECT().Times(4)`), либо использовать `NewNotifierWithBackoff` с `maxAttempts=1` чтобы сохранить семантику single-shot.

### Coverage gate

`make test-unit-backend-coverage` — per-method ≥ 80% на 3 новых repo (`creator.go`, `creator_social.go`, `creator_category.go`).

`internal/telegram/` **в awk-фильтре gate'а отсутствует** (`Makefile:120` — фильтр `/(handler|service|repository|middleware|authz)/`). Поэтому новые retry-функции (`fire`, `isRetryable`, `NewNotifierWithBackoff`) под автоматический gate не попадают, **но** обязаны иметь покрытие через retry-сценарии в § Test Plan / Unit tests (transient/terminal/exhausted/ctx-cancelled). Расширение фильтра под `telegram/` — за рамки 18a (отдельная инициатива, если потребуется).

### Constants

Все имена constraint'ов через exported константы (`backend-constants.md`). Имена столбцов / таблиц — exported константы. Литералы только в SQL-asserts тестов.

### Race detector

`-race` обязателен; concurrent-сценариев в 18a нет, но retry-loop с goroutine + WaitGroup должен проходить чисто.

## Verification

**Commands:**
- `make compose-up && make migrate-up`
- `go run ./cmd/api` (smoke прогон 13/13)
- После удаления smoke финальным коммитом: `make build-backend lint-backend test-unit-backend-coverage`

**Manual smoke:**
- См. § Local Smoke Acceptance — 13 шагов, каждый с печатью результата. Все 13 должны быть OK перед финальным удалением smoke-кода.

## Spec Change Log

- **2026-05-05** — спека создана как 18a в декомпозиции chunk 18 (foundation + retry + smoke). Status: `draft`. Старая большая спека `spec-creator-application-approve.md` сохранена как референс до merge всех трёх под-чанков.
- **2026-05-05 (smoke hardening)** — четыре правки § Local Smoke Acceptance после анализа FK-зависимостей:
  1. Добавлен step-0 `seedSmokeApplications`: 4 заявки в `creator_applications` через прямой SQL — без них creator-INSERT'ы упадут на FK `source_application_id`. 13 шагов → 14.
  2. Шаг 8 упрощён: все 3 социалки `verified=false` / `verified_by_user_id=NULL` (verification-ветки тестируются на сервисном уровне в 18b). Снимает зависимость от users-row для FK `verified_by_user_id`.
  3. Зафиксированы конкретные seed-коды: `city_code='almaty'`, категории `[fashion, lifestyle, food]` — все из миграций `20260425205626_cities.sql` / `20260420181745_categories.sql`.
  4. Шаги 3/5/6/7 явно используют разные `source_application_id` (`appID1..4` из step-0) и разные telegram_user_id для изоляции race-сценариев.
- **2026-05-05 (coverage-gate clarity)** — § Test Plan / Coverage gate: явно прописано, что `internal/telegram/` не входит в awk-фильтр `Makefile:120` (только `handler|service|repository|middleware|authz`). Retry-функции notifier'а не под автоматический gate, но обязаны иметь покрытие через retry-сценарии тестов. Расширение фильтра — за рамки 18a.

</frozen-after-approval>
