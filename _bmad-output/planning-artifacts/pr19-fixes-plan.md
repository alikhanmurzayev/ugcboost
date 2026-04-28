# План реализации: фиксы по код-ревью PR #19

## Преамбула — что должен сделать агент-исполнитель ПЕРЕД началом

Этот план — самодостаточный. В новой сессии следующий агент должен
выполнить ровно эту последовательность подготовки, прежде чем
притрагиваться к коду:

1. **Полностью прочитать ВСЕ файлы в `docs/standards/`** — целиком, без
   сокращений, не «релевантные» поиском, не grep'ом, а каждый файл
   полностью в контекст. Это hard requirement проекта. Используй skill
   `/standards` (он именно это и делает) или прочитай руками: backend-*,
   frontend-*, общие (`naming.md`, `security.md`).
2. **Прочитать `_bmad-output/planning-artifacts/review-agent-initiative.md`** —
   контекст того, зачем именно эти правки и какие правила из них
   капитализуются в будущий ревьюер-чеклист.
3. **Прочитать треды PR #19** для деталей по конкретным findings:
   `gh pr view 19 --comments` или
   `gh api repos/alikhanmurzayev/ugcboost/pulls/19/comments`. Все 31 тред
   уже разрешён — это источник правды по каждому решению.
4. **Прочитать сами файлы кода**, которые план меняет (см. таблицу
   «Файлы для изменения») — без этого immediate-context'а правки делать
   вслепую нельзя.

Только после этого приступать к Фазе 1.

## Обзор

Реализация всех findings ревью PR #19 (`alikhan/creator-application-submit` → `main`).
Все 31 ревью-тред уже разрешены в GitHub — этот документ описывает
итоговую имплементацию утверждённых правок, не пересказывает обсуждение.
Источник правды по решениям — треды PR #19; источник контекста по
инициативе ревьюер-агента — `_bmad-output/planning-artifacts/review-agent-initiative.md`.

Скоуп — backend код, **новая forward-миграция** (старую не трогаем),
тесты, два стандарта, легал-доки sync, README/.env.example, чистка
артефактов планирования. Внешние интеграции и фронт-сторона не трогаются.

## Hard constraints

- **Миграции, уже прогнанные на staging — не редактировать.** Все
  миграции в `backend/migrations/` уже прогонялись на staging. Любая
  правка поведения схемы — **новой forward-миграцией**, не in-place
  edit'ом старого файла. Это правило также зафиксировано в
  `review-agent-initiative.md` как preview v0 чеклиста.
- **Стандарты `docs/standards/` — обязательны для всего, что попадает в
  код.** Преамбула выше детализирует процедуру их загрузки. Любое
  отступление от стандарта — только с явным согласованием.
- **Не коммитить и не пушить без явной команды** Алихана. `CLAUDE.md`
  и memory это фиксируют.

## Требования

Группировка по областям. Каждое REQ — одна правка из утверждённого треда PR.

### Backend (service / handler / repository)

- **REQ-A1.** `backend/internal/service/audit.go:35-42` — заменить
  `id := actorID; row.ActorID = &id` (и аналог для `entityID`) на
  `pointer.ToString(...)` из `github.com/AlekSi/pointer`. (тред: «зачем
  эта лишняя переменная тут? Можно ведь взять адрес у actorID»)
- **REQ-A2.** `backend/internal/service/creator_application.go::iinErrorToValidation`
  — обе ветки switch возвращают одно сообщение, поэтому свернуть до:
  ```go
  func (s *CreatorApplicationService) iinErrorToValidation(_ error) error {
      return domain.NewValidationError(domain.CodeInvalidIIN, "Невалидный ИИН")
  }
  ```
  Параметр сохранить (caller передаёт ошибку, чтобы в будущем при необходимости
  можно было снова различать варианты), но в теле его не использовать.
- **REQ-A3.** Methodization helpers'ов сервиса. Принципы:
  - Hellpers, привязанные к данным или зависимостям сервиса (repo,
    config, logger) → **методы** `*CreatorApplicationService`.
  - Hellpers, специфичные для сервиса даже без state → **методы**.
  - Чистые generic-утилиты, переиспользуемые между сервисами →
    **package-level free functions**.
  
  По текущему `creator_application.go` это раскладывается так:
  | Helper | До | После |
  |---|---|---|
  | `trimAndValidateRequired` | free function | метод (lowercase) |
  | `validateCategoryOtherText` | free function | метод |
  | `normaliseSocials` | free function | метод |
  | `resolveCategoryIDs(ctx, repo, codes)` | free function с repo-параметром | метод (`repo` через `s.repoFactory.NewDictionaryRepo(tx)`) |
  | `buildConsentRows` | free function | метод |
  | `auditNewValue` | free function | метод |
  | `iinErrorToValidation` | free function | метод |
  | `creatorApplicationDetailFromRows` | free function | метод |
  | `duplicateError` | free function | метод |
  | `trimOptional` | free function | **остаётся** package-level (generic, реально переиспользуема) |
  
  В `audit.go`:
  | Helper | До | После |
  |---|---|---|
  | `contextWithActor(ctx, userID, role)` | free function | **остаётся** package-level (generic, не зависит от state) |
  | `domainFilterToRepo` / `auditRowsToDomain` | free function | **остаются** (generic mappers) |
  | `writeAudit(ctx, repo, ...)` | free function с repo-параметром | **остаётся** package-level (используется из ВСЕХ сервисов, не только AuditService — это утилита уровня пакета `service`) |
  
  Новые методы остаются с lowercase first letter (приватные) — это
  стилистический выбор, а не отговорка для покрытия. Coverage gate
  после REQ-M1 (см. ниже) будет проверять **всё** — публичные и
  приватные одинаково.
- **REQ-A4.** `backend/internal/service/creator_application.go::normaliseSocials` —
  убрать handle из текста validation-ошибки. Сейчас:
  ```go
  return ..., fmt.Sprintf("Дубликат соцсети: %s/%s", a.Platform, handle)
  return ..., fmt.Sprintf("Пустой handle для соцсети %s", a.Platform)
  return ..., fmt.Sprintf("Некорректный handle для соцсети %s: ...", a.Platform)
  ```
  Сейчас сообщения с handle — единственное место, где user-controlled
  handle попадает в текст error-response (а значит и потенциально в
  логи через respondError). Привести к единому виду:
  ```go
  fmt.Sprintf("Дубликат соцсети: %s", a.Platform)
  fmt.Sprintf("Пустой handle для соцсети %s", a.Platform)
  fmt.Sprintf("Некорректный handle для соцсети %s: ...", a.Platform)
  ```
  То есть оставить только `a.Platform` (значение из enum, не PII).
- **REQ-A5.** `backend/internal/service/creator_application.go::duplicateError` —
  добавить actionable hint в текст 409. Сейчас сообщение тупиковое.
  Финальный текст:
  ```
  Заявка по этому ИИН уже находится на рассмотрении или одобрена. Дождитесь решения модератора или, если заявка будет отклонена, подайте новую.
  ```
  Точную формулировку финализировать на месте; принцип — креатор должен
  знать, что делать, а не упереться в стену.
- **REQ-A6.** `backend/internal/service/creator_application.go::Submit` —
  явно проставлять `Status: domain.CreatorApplicationStatusPending` в
  `repository.CreatorApplicationRow{}` при INSERT (строки 101-111).
  Связано с REQ-B1 (insert tag) и REQ-D1 (миграция убирает DEFAULT).
- **REQ-A7.** `backend/internal/handler/testapi.go:115-127` — заменить
  hardcode-сообщение (`"type must be 'user', 'brand' or 'creator_application'"`)
  на проверку через сгенерированный `req.Type.Valid()`. Структура:
  ```go
  switch req.Type {
  case testapi.User:
      // ...
  case testapi.Brand:
      // ...
  case testapi.CreatorApplication:
      // ...
  default:
      if !req.Type.Valid() {
          respondError(w, r, domain.NewValidationError(domain.CodeValidation,
              fmt.Sprintf("unknown type: %q", req.Type)), h.logger)
          return
      }
      // shouldn't happen — switch покрывает все Valid'ные значения
      respondError(w, r, domain.NewValidationError(domain.CodeValidation,
          fmt.Sprintf("unsupported type: %q", req.Type)), h.logger)
      return
  }
  ```

### Build / CI gate (Makefile)

- **REQ-M1.** `Makefile::test-unit-backend-coverage` — починить
  coverage gate, чтобы он проверял ВЕСЬ покрываемый код, а не только
  Capitalized (публичную) поверхность. Сейчас awk-фильтр имеет
  условие `$$2 ~ /^[A-Z]/` — оно отсекает все lowercase методы и
  free functions, и они уходят из проверки. Это баг: lowercase
  поверхность сервисов/репозиториев годами была вне gate'а и могла
  иметь произвольно низкое покрытие.
  
  Текущая awk-секция (`Makefile`, под `test-unit-backend-coverage`):
  ```
  awk '$$1 ~ /\/(handler|service|repository|middleware|authz)\// \
      && $$1 !~ /\.gen\.go:/ \
      && $$1 !~ /\/mocks\// \
      && $$1 !~ /\/cmd\// \
      && $$1 !~ /\/handler\/health\.go:/ \
      && $$1 !~ /\/middleware\/(logging|json)\.go:/ \
      && $$2 ~ /^[A-Z]/ { \
          pct = $$NF; gsub(/%/, "", pct); \
          if (pct + 0 < 80.0) { printf "FAIL %s %s %s%%\n", $$1, $$2, pct; fail=1 } \
      } END { exit fail ? 1 : 0 }'
  ```
  
  Правка — удалить строку `&& $$2 ~ /^[A-Z]/ \`:
  ```
  awk '$$1 ~ /\/(handler|service|repository|middleware|authz)\// \
      && $$1 !~ /\.gen\.go:/ \
      && $$1 !~ /\/mocks\// \
      && $$1 !~ /\/cmd\// \
      && $$1 !~ /\/handler\/health\.go:/ \
      && $$1 !~ /\/middleware\/(logging|json)\.go:/ { \
          pct = $$NF; gsub(/%/, "", pct); \
          if (pct + 0 < 80.0) { printf "FAIL %s %s %s%%\n", $$1, $$2, pct; fail=1 } \
      } END { exit fail ? 1 : 0 }'
  ```
  
  После правки **gate почти наверняка упадёт сразу** — есть
  приватные функции/методы, которые сейчас покрыты <80%.
  Шаг 1.5 в Фазе 1 — поднять покрытие приватной поверхности до 80%
  ДО начала остальных правок (иначе они будут смешиваться с
  фоновым красным состоянием gate'а). Возможные тактики: добавить
  unit-тесты на непокрытые приватные функции, или удалить
  действительно мёртвый приватный код.

- **REQ-M2.** `docs/standards/backend-testing-unit.md` — секция
  «Coverage» сейчас говорит «Целевой порог — 80%. Исключения:
  main.go, DI/wire setup, bootstrap-код.» Это формально соответствует
  REQ-M1, но добавить явное уточнение для будущего читателя:
  ```markdown
  ## Coverage
  
  Целевой порог — 80% **на каждой публичной и приватной
  функции/методе** в покрываемых пакетах (handler/service/repository/
  middleware/authz). Gate в `make test-unit-backend-coverage` падает,
  если хотя бы один identifier ниже 80%. Исключения по файлам:
  generated code (`*.gen.go`), mockery-моки (`*/mocks/`), `cmd/`,
  trivial bootstrap (`handler/health.go`, `middleware/logging.go`,
  `middleware/json.go`).
  ```

### Repository

- **REQ-B1.** `backend/internal/repository/creator_application.go:56` —
  добавить `insert:"status"` тег к полю `Status`:
  ```go
  Status string `db:"status" insert:"status"`
  ```
  Без этого тега `stom` не передаст Status в `INSERT.SetMap(...)`, и
  даже после явного присваивания в сервисе (REQ-A6) колонка останется
  не заполненной → INSERT упадёт (NOT NULL без DEFAULT — см. REQ-D1).

### Migrations (новая forward-миграция, старую не трогаем)

- **REQ-D1.** Создать **новую** миграцию через `make migrate-create
  NAME=creator_applications_relax_constraints`. Goose сгенерирует файл
  `backend/migrations/<timestamp>_creator_applications_relax_constraints.sql`.
  Содержимое:
  ```sql
  -- +goose Up
  -- IIN format CHECK уезжает в backend (domain.ValidateIIN). Миграция
  -- не должна содержать regex'ы — формат может поменяться, и в БД
  -- защита всё равно дублирует backend-валидацию.
  ALTER TABLE creator_applications
      DROP CONSTRAINT IF EXISTS creator_applications_iin_check;

  -- DEFAULT 'pending' уезжает в backend (service.CreatorApplicationService.Submit
  -- теперь явно проставляет domain.CreatorApplicationStatusPending). Бизнес-
  -- defaults определяет код, не БД. CHECK на enum-значения остаётся —
  -- это data integrity, а не business default.
  ALTER TABLE creator_applications
      ALTER COLUMN status DROP DEFAULT;

  -- +goose Down
  ALTER TABLE creator_applications
      ALTER COLUMN status SET DEFAULT 'pending';

  ALTER TABLE creator_applications
      ADD CONSTRAINT creator_applications_iin_check CHECK (iin ~ '^[0-9]{12}$');
  ```
  Имя CHECK'а в исходной миграции — anonymous → Postgres даёт ему
  implicit name `creator_applications_iin_check`. На случай если в
  каком-то окружении имя другое — `IF EXISTS` защищает от падения.
- **REQ-D2.** Старая миграция `backend/migrations/20260420181753_creator_applications.sql`
  **остаётся как есть, не редактировать.** Любая правка in-place
  расходится с тем, что уже прогналось на staging.

### Tests

- **REQ-E1.** `backend/internal/repository/creator_application_test.go::Create`
  — обновить mock SQL (alphabetical порядок insert-колонок согласно
  `sortColumns`) и `WithArgs` под добавленный `status`. После REQ-B1
  колонка `status` встаёт в alphabetical insert-список — между `phone`
  и `last_name` (точный порядок зависит от alphabetical sort всех
  insert-колонок: address, birth_date, category_other_text, city,
  first_name, iin, last_name, middle_name, phone, status). Прогнать
  unit-тесты, поправить SQL-строку и аргументы из happy-path до
  совпадения с реальным.
- **REQ-E2.** `backend/internal/service/creator_application_test.go` —
  если есть happy-path тест `Submit`, в expected mock-call для
  `appRepo.Create` добавить ассерт что `row.Status == "pending"`. Если
  тест использует `mock.AnythingOfType` — добавить `.Run(...)` для
  захвата input + `require.Equal(t, "pending", captured.Status)`.
- **REQ-E3.** `backend/e2e/creator_application/creator_application_test.go` —
  выкинуть shadow-DTO `expectedCreatorApplication` (~строка 430).
  Заменить на сравнение с `apiclient.CreatorApplicationDetailData` напрямую:
  - Резолвить ожидаемые `City` и `Categories` через `GET /dictionaries/{type}`
    (или статический набор, известный в тесте) — превратить codes в
    `{code, name, sortOrder}`.
  - Социалы — нормализовать (lowercase, стрип `@`) согласно сервисной
    логике, чтобы expected совпадало с actual.
  - Подменить динамические поля перед `require.Equal`: `ApplicationId`,
    `CreatedAt`, `UpdatedAt`, `Consents[*].AcceptedAt` — сначала
    проверить отдельно (uuid не пустой, время в `WithinDuration`),
    затем обнулить или подменить на ожидаемое значение, затем
    `require.Equal` структуры целиком.
  - Паттерн соответствует `backend-testing-e2e.md` (assertions через
    сгенерированные типы, динамические поля проверяются отдельно).
- **REQ-E4.** `backend/e2e/dictionary/dictionary_test.go:42` — выкинуть
  `require.NotContains(codes, "gaming")`. Категории — часто меняющийся
  словарь, ассерт на отсутствие конкретного кода хрупкий и не ловит
  реальных багов. Оставить инварианты:
  - формат: `sortOrder >= 0`, `code != ""`,
  - неубывающий `sortOrder` по списку,
  - наличие protected-кода `other` (это контракт submit-схемы — без
    него API ломается, удаление = breaking change).
- **REQ-E5.** Создать `backend/e2e/testutil/audit.go` с двумя экспортами:
  ```go
  // AssertAuditEntry checks that an audit row matching (entityType, entityID,
  // action) exists. Uses the admin token to call GET /audit-logs with the
  // server-side filter, then asserts the action is present.
  func AssertAuditEntry(t *testing.T, c *apiclient.ClientWithResponses,
      adminToken, entityType, entityID, action string) {
      t.Helper()
      resp, err := c.ListAuditLogsWithResponse(context.Background(),
          &apiclient.ListAuditLogsParams{
              EntityType: &entityType,
              EntityId:   &entityID,
          }, WithAuth(adminToken))
      require.NoError(t, err)
      require.Equal(t, http.StatusOK, resp.StatusCode())
      require.NotNil(t, resp.JSON200)
      require.True(t, ContainsAction(resp.JSON200.Data.Logs, action),
          "expected audit entry action=%q for %s/%s", action, entityType, entityID)
  }

  // ContainsAction reports whether logs contain at least one entry with the
  // given action. Exported so audit/audit_test.go (and any future test) can
  // share the same predicate.
  func ContainsAction(logs []apiclient.AuditLog, action string) bool {
      for _, l := range logs {
          if l.Action == action {
              return true
          }
      }
      return false
  }
  ```
  Вынести существующий `containsAction` из `backend/e2e/audit/audit_test.go`
  → `testutil.ContainsAction` и заменить локальное использование там.
- **REQ-E6.** `backend/e2e/creator_application/creator_application_test.go::TestSubmitCreatorApplication`
  — после успешного submit добавить вызов:
  ```go
  testutil.AssertAuditEntry(t, c, adminToken, "creator_application",
      submission.ApplicationID, "creator_application_submit")
  ```
  Создать админский клиент через `testutil.SetupAdminClient`, помимо
  публичного клиента, который выполняет сам submit — `/audit-logs` —
  admin-only ручка. **Скоуп этого PR — только creator_application**;
  расширение audit-ассертов на остальные mutate-handler'ы (auth, brand)
  — отдельная задача, не в этом PR.

### Standards

- **REQ-S1.** `docs/standards/backend-libraries.md` — сжать до **≤3K
  байт**. Сейчас ~3.4K. Что урезать:
  - «Принцип» + «Правило» → один абзац (3-4 строки).
  - «Бизнес-логика» в конце — выкинуть (тривиально, повторяет принцип).
  - В правилах работы с реестром: длинный пункт про `crypto/rand` с
    историей инцидента в `e2e/testutil/seed.go` — сократить до 1-2
    строк, без storytelling. Вся реальная техническая суть — в правиле,
    инцидент уходит в commit message / память как «почему запрещён».
  - Реестр-таблицу не трогать — она и есть main payload документа.
  
  После — `wc -c docs/standards/backend-libraries.md` ≤ 3000.
- **REQ-S2.** `docs/standards/backend-repository.md` — добавить новую
  секцию между «Возвращаемые значения» и «Константы колонок»:
  ```markdown
  ## Целостность данных: БД vs бэк
  
  БД защищается от мусора через `NOT NULL`, `ENUM CHECK`, `FK`, `UNIQUE` —
  это data integrity. Format checks (regex, length) и business defaults
  (`status='pending'` при создании заявки) — на бэке, в БД их нет.
  Если формат меняется — миграция БД не нужна.
  ```
- **REQ-S3.** `docs/standards/backend-repository.md` — добавить пометку
  про идентификатор словаря (рядом с «Константы колонок» или отдельной
  короткой секцией):
  ```markdown
  ## Идентификатор словаря
  
  Текущие словари (`categories`, `cities`) хранят и `id (UUID)`, и
  `code (TEXT UNIQUE)`. Целевая модель — `code` как PK, FK ссылаются
  на code → колонки таблиц-потребителей хранят `<entity>_code`, а не
  `<entity>_id`. Это устраняет лишний JOIN при чтении и indirection
  в коде. Refactor — отдельная задача с миграцией данных; новые словари
  в этой схеме не заводим.
  ```
- **REQ-S4.** `docs/standards/backend-repository.md` — добавить раздел
  про миграции (или отдельный новый стандарт `backend-migrations.md` —
  выбрать на месте; backend-repository.md уже про data layer, в него
  органично):
  ```markdown
  ## Миграции
  
  - Каждая миграция — отдельный goose-файл в `backend/migrations/` с
    `+goose Up` и `+goose Down`. Создание — через `make migrate-create
    NAME=...`.
  - **Миграция, прогнанная в любом не-локальном окружении (staging,
    prod), не редактируется in-place. Никогда.** Любая правка
    поведения схемы — новой forward-миграцией, даже если правка
    тривиальная (drop CHECK, drop DEFAULT, ALTER TYPE и т.п.).
    Причина: миграции применяются one-way, и in-place edit
    рассинхронизирует БД staging/prod с тем, что в репозитории
    (свежий `goose up` на чистой БД даст одну схему, на staging —
    другую).
  - Если не уверен, прогонялась ли миграция — считай что прогонялась.
    Лишняя forward-миграция стоит ничего, разъехавшаяся схема — стоит
    инцидента.
  ```

### Legal docs sync (Variant А)

- **REQ-L1.** `Makefile` — добавить два таргета. Текст:
  ```makefile
  sync-legal:
  	cp 'legal-documents/ПОЛИТИКА ОБРАБОТКИ ПЕРСОНАЛЬНЫХ ДАННЫХ UGCBOOST.md' \
  	   frontend/landing/src/pages/legal/privacy-policy.md
  	cp 'legal-documents/ПОЛЬЗОВАТЕЛЬСКОЕ СОГЛАШЕНИЕ UGCBOOST.md' \
  	   frontend/landing/src/pages/legal/user-agreement.md

  lint-legal:
  	@diff -q 'legal-documents/ПОЛИТИКА ОБРАБОТКИ ПЕРСОНАЛЬНЫХ ДАННЫХ UGCBOOST.md' \
  	   frontend/landing/src/pages/legal/privacy-policy.md > /dev/null \
  	   || (echo 'privacy-policy.md диверговал от legal-documents — выполни make sync-legal' && exit 1)
  	@diff -q 'legal-documents/ПОЛЬЗОВАТЕЛЬСКОЕ СОГЛАШЕНИЕ UGCBOOST.md' \
  	   frontend/landing/src/pages/legal/user-agreement.md > /dev/null \
  	   || (echo 'user-agreement.md диверговал от legal-documents — выполни make sync-legal' && exit 1)
  ```
  Добавить `sync-legal lint-legal` в `.PHONY`. Источник истины —
  `legal-documents/`; копии в `frontend/landing/src/pages/legal/` —
  derived. `ПРИНЯТЫЕ РЕШЕНИЯ.md` (internal) в лендос НЕ копируется.
- **REQ-L2.** `Makefile::lint-landing` — добавить `lint-legal` как
  pre-step. Текущий `lint-landing` — `tsc + eslint`; станет:
  ```makefile
  lint-landing: lint-legal
  	cd frontend/landing && npx tsc --noEmit
  	cd frontend/landing && npx eslint src/
  ```
- **REQ-L3.** `.github/workflows/ci.yml` — добавить `make lint-legal`
  в job, который проверяет лендос (или отдельный шаг до `lint-landing`).
  Точное место — там, где сейчас крутится `npm run lint` или
  `make lint-landing`.
- **REQ-L4.** Один раз вручную прогнать `make sync-legal` — текущие
  копии сейчас отличаются от источников (~130 байт, скорее всего CRLF
  vs LF или ручное редактирование). После sync они совпадут, и
  `make lint-legal` пойдёт зелёным.

### README / .env.example

- **REQ-R1.** `README.md:33-50` — удалить целиком секцию «Backend
  CORS_ORIGINS» с таблицей origins. Это env-конфиг, не onboarding.
- **REQ-R2.** `backend/.env.example` — добавить блок CORS_ORIGINS с
  пояснениями. Текст:
  ```
  # CORS_ORIGINS — список origins, которые backend пропустит через CORS.
  # Локальная dev-среда (vite + docker + playwright):
  #   http://localhost:5173 (web vite dev)
  #   http://localhost:5174 (tma vite dev)
  #   http://localhost:3001 (web docker, make start-web)
  #   http://localhost:3002 (tma docker, make start-tma)
  #   http://localhost:3003 (landing docker, make start-landing)
  #   http://localhost:4321 (landing astro dev / playwright webServer)
  # На staging / prod заменить localhost-entries на реальные публичные origins.
  CORS_ORIGINS=http://localhost:5173,http://localhost:5174,http://localhost:3001,http://localhost:3002,http://localhost:3003,http://localhost:4321
  ```
  Конкретное значение — то же, что в текущем `backend/.env`, чтобы
  никто не упёрся в CORS после `cp .env.example .env`.

### Артефакты планирования

- **REQ-T1.** `_bmad-output/implementation-artifacts/deferred-work.md` —
  триаж 22 пунктов:
  - Закрытые этим PR'ом (DB-state verify, Address NOT NULL, handler
    MatchedBy partial, WithArgs mock) — удалить из документа.
  - Согласованные drop'ы (ServerDeps refactor, i18n) — удалить.
  - Истинно отложенные tech-debt — закрыть GitHub issues по REQ-G*.
  - В конце — удалить файл целиком (`git rm`).
- **REQ-T2.** `_bmad-output/planning-artifacts/landing-form-and-address-cleanup-plan.md`
  — `git mv` в `_bmad-output/planning-artifacts/archive/` (директория
  создаётся при первом mv). Это единственный артефакт, который в этом
  PR физически переезжает в archive (он про другой слайс, landing form
  patch, который уже фактически закрыт).
- **REQ-T3.** Финальный pre-merge коммит (`chore: archive implementation
  artifacts`) — отдельным шагом перед мерджем в main:
  ```
  git mv _bmad-output/implementation-artifacts/spec-creator-application-submit.md \
         _bmad-output/implementation-artifacts/archive/
  git mv _bmad-output/implementation-artifacts/landing-e2e-plan.md \
         _bmad-output/implementation-artifacts/archive/
  git mv _bmad-output/implementation-artifacts/sync-with-landing-plan.md \
         _bmad-output/implementation-artifacts/archive/
  ```
  `deferred-work.md` к этому моменту уже удалён (REQ-T1).
  `pr19-fixes-plan.md` остаётся в `planning-artifacts/`, не в archive
  (это планировочный артефакт, согласовано).

### GitHub issues для tech-debt

- **REQ-G1.** Issue «Migrate handlers to oapi-codegen strict-server
  mode». Скоуп: 8 хендлеров (auth ×3, brand ×3, creator_application ×1,
  testapi ×2). Лейблы: `tech-debt`, `backend`. Описание — краткое
  изложение из соответствующего треда PR #19.
- **REQ-G2.** Issue «Refactor public dictionaries to code-as-PK».
  Включает миграцию данных (UUID id → drop, код становится PK),
  обновление FK в `creator_application_categories` (`category_id` →
  `category_code`), обновление repo-кода (убирается JOIN). Лейблы:
  `tech-debt`, `backend`.
- **REQ-G3.** Issue «Convert landing public/{benefits,hero,logos}/*.{jpg,png}
  → webp». Acceptance: размер каждого изображения уменьшается ≥20%,
  визуально без разницы (subjective check), `<img src=...>` srcs в
  Astro-компонентах обновлены. Лейблы: `tech-debt`, `frontend`.
- **REQ-G4.** Issue «IIN domain edge-cases» — century byte 7/8 → 2100s
  mapping (`backend/internal/domain/iin.go:130-131`), upper-bound
  (`birth.After(now)` / age > 120 в том же файле). Лейблы: `tech-debt`,
  `backend`.
- **REQ-G5.** Issue «`audit_logs_nullable_actor` down-migration
  corner-case» — `SET NOT NULL` в Down-секции упадёт, если в БД есть
  ряды с `actor_id IS NULL` (созданные публичными endpoints вроде
  creator-application submit). Лейблы: `tech-debt`, `backend`.
- **REQ-G6.** Issue «Прочий tech-debt из deferred-work.md» — один
  collapse-issue с чек-листом всех мелких пунктов. Включает (без
  претензий на исчерпывающий список):
  - Clock abstraction для тестирования timing-boundary (handler:44).
  - Content-Type check + `json.NewDecoder.DisallowUnknownFields()` в
    SubmitCreatorApplication.
  - UTF-8 normalisation (RTL-override, ZWJ) в address / names —
    hardening.
  - `length(*) > 0` CHECK на consent NOT NULL TEXT-полях
    (`document_version`, `ip_address`, `user_agent`) — defense-in-depth.
  - TX обёртка вокруг read-операций `HasActiveByIIN` /
    `GetActiveByCodes` в Submit (borderline; lock window короткий).
  - PII-guard test (grep stdout-логов по ИИН/ФИО/телефону → 0
    совпадений) — требует structured logger-assertion helper.
  - `UniqueIIN` counter overflow в e2e (mod 10000 — в одной сессии
    10000 заявок не достигаются, не блокирует MVP).
  Лейбл: `tech-debt`.

## Файлы для изменения

| Файл | REQ | Изменения |
|---|---|---|
| `Makefile` (target `test-unit-backend-coverage`) | M1 | убрать `$$2 ~ /^[A-Z]/` из awk-фильтра — gate ловит всю покрываемую поверхность |
| `docs/standards/backend-testing-unit.md` | M2 | уточнить, что 80% покрытие — для публичных И приватных идентификаторов |
| `backend/internal/service/audit.go` | A1 | `pointer.ToString` для actor_id и entity_id |
| `backend/internal/service/creator_application.go` | A2-A6 | helpers→методы, collapse `iinErrorToValidation`, фикс PII в normaliseSocials, actionable duplicate text, явный Status в Submit |
| `backend/internal/handler/testapi.go` | A7 | `req.Type.Valid()` вместо hardcode-сообщения |
| `backend/internal/repository/creator_application.go` | B1 | `insert:"status"` тег на Status |
| `backend/internal/repository/creator_application_test.go` | E1 | mock SQL + WithArgs под расширенный insert-набор |
| `backend/internal/service/creator_application_test.go` | E2 | ассерт что Submit передаёт Status="pending" |
| `backend/e2e/creator_application/creator_application_test.go` | E3, E6 | убрать shadow-DTO, добавить AssertAuditEntry |
| `backend/e2e/dictionary/dictionary_test.go` | E4 | убрать NotContains gaming, оставить инварианты |
| `backend/e2e/audit/audit_test.go` | E5 | заменить локальный `containsAction` на `testutil.ContainsAction` |
| `docs/standards/backend-libraries.md` | S1 | сжать до ≤3K |
| `docs/standards/backend-repository.md` | S2, S3, S4 | секции «Целостность данных», «Идентификатор словаря», «Миграции» |
| `Makefile` | L1, L2 | sync-legal, lint-legal, lint-landing depends on lint-legal |
| `.github/workflows/ci.yml` | L3 | вызов make lint-legal на CI |
| `frontend/landing/src/pages/legal/privacy-policy.md` | L4 | приведение к sync-копии (через `make sync-legal`) |
| `frontend/landing/src/pages/legal/user-agreement.md` | L4 | приведение к sync-копии |
| `README.md` | R1 | удалить секцию Backend CORS_ORIGINS |
| `backend/.env.example` | R2 | CORS_ORIGINS с комментариями |
| `_bmad-output/implementation-artifacts/deferred-work.md` | T1 | триаж + удаление файла |

## Файлы для создания

| Файл | REQ | Назначение |
|---|---|---|
| `backend/migrations/<timestamp>_creator_applications_relax_constraints.sql` | D1 | новая forward-миграция: drop CHECK на iin format, drop DEFAULT на status |
| `backend/e2e/testutil/audit.go` | E5 | `AssertAuditEntry` хелпер + `ContainsAction` |
| `_bmad-output/implementation-artifacts/archive/` (директория) | T3 | целевая папка для архивации |

## Файлы, которые НЕ трогаем

| Файл | Почему |
|---|---|
| `backend/migrations/20260420181753_creator_applications.sql` | Прогнана на staging — редактирование запрещено (см. Hard constraints) |
| `backend/e2e/{auth,brand}/...` | Расширение audit-ассертов на эти хендлеры — вне скоупа этого PR |
| `frontend/{web,tma,landing}/src/...` | Фронт-сторона не затрагивается ни одним find'ом |
| `legal-documents/ПРИНЯТЫЕ РЕШЕНИЯ.md` | Internal-документ, в лендос не копируется, в этом PR не правится |

## Шаги реализации

Порядок важен. Сначала backend (миграция + код), потом тесты,
потом стандарты и инфра, в конце артефакты + GitHub issues.

### Фаза 0: Подготовка контекста

1. [ ] Выполнить пункты 1-4 из «Преамбула» (стандарты, review-agent-initiative,
       треды PR #19, файлы кода).

### Фаза 1: Coverage gate fix + baseline

Сначала чиним gate, иначе все последующие изменения будут идти на фоне
сломанной метрики.

1.1. [ ] **REQ-M1.** `Makefile` — убрать `&& $$2 ~ /^[A-Z]/ \` из
       awk-фильтра в `test-unit-backend-coverage`.

1.2. [ ] **REQ-M2.** `docs/standards/backend-testing-unit.md` —
       уточнить секцию «Coverage» (см. REQ-M2).

1.3. [ ] Прогнать `make test-unit-backend-coverage`. Получить список
       приватных функций/методов с покрытием < 80% (вывод awk
       печатает `FAIL <file> <name> <pct>%` для каждого).

1.4. [ ] Поднять покрытие приватной поверхности до 80% — ДО начала
       остальных правок:
       - Большинство — за счёт того, что приватная функция вызывается
         из публичного метода → достаточно расширить тест публичного.
       - Если приватная функция действительно мёртвая (никем не
         вызывается) — удалить.
       - Если функция вызывается только из crash-веток (panic recovery,
         shutdown error path) — добавить targeted unit-тест либо
         оформить исключение в awk-фильтре с явным комментарием
         «почему».

1.5. [ ] **Контроль:** `make test-unit-backend-coverage` зелёный.

### Фаза 2: Миграция + backend код

2. [ ] **REQ-D1.** `make migrate-create NAME=creator_applications_relax_constraints`
       → заполнить SQL по тексту из REQ-D1.
3. [ ] **REQ-A1.** `service/audit.go` — `pointer.ToString` для actor_id
       и entity_id. Убедиться что `github.com/AlekSi/pointer` уже в
       `go.mod` (он есть, см. `backend-libraries.md`).
4. [ ] **REQ-A2 + A3.** `service/creator_application.go` — methodization
       helpers'ов одним проходом + collapse `iinErrorToValidation`.
       Перенести каждый helper, обновить call-sites (теперь `s.<name>(...)`
       вместо `<name>(...)`), убрать repo-параметры (доступ через
       `s.repoFactory`). Проверить что `trimOptional` остался
       package-level.
5. [ ] **REQ-A4.** В новом методе `normaliseSocials` — убрать handle
       из всех validation messages, оставить только `a.Platform`.
6. [ ] **REQ-A5.** В методе `duplicateError` — обновить текст ошибки до
       actionable варианта (см. REQ-A5).
7. [ ] **REQ-A7.** `handler/testapi.go` — заменить hardcode на
       `req.Type.Valid()` (см. REQ-A7).
8. [ ] **REQ-B1.** `repository/creator_application.go:56` — добавить
       `insert:"status"` тег.
9. [ ] **REQ-A6.** `service/creator_application.go::Submit` — явно
       проставлять `Status: domain.CreatorApplicationStatusPending` в
       `repository.CreatorApplicationRow{}`.
10. [ ] **Контроль:** `make build-backend` без ошибок. `make lint-backend`
        чисто. `make migrate-reset && make migrate-up` — обе миграции
        (старая и новая forward) применяются успешно.

### Фаза 3: Backend unit-тесты

11. [ ] **REQ-E1.** `repository/creator_application_test.go::Create` —
        обновить SQL-строку и `WithArgs` под расширенный insert-набор
        (status добавился). Прогнать; при mismatch'е — выровнять до
        actual.
12. [ ] **REQ-E2.** `service/creator_application_test.go` — добавить
        ассерт на Status="pending" в happy-path тесте `Submit`.
13. [ ] Все остальные unit-тесты `service`/`handler` — прогнать после
        methodization (REQ-A3). Сигнатуры helpers'ов изменились, но они
        были lowercase free functions, тесты их напрямую не вызывали;
        изменения должны пройти прозрачно. Если что-то падает —
        исправить.
14. [ ] **Контроль:** `make test-unit-backend` чисто.
        `make test-unit-backend-coverage` — все Capitalized методы в
        `handler/service/repository/middleware/authz` ≥ 80% (методизация
        helpers'ов покрытие не меняет: новые методы — lowercase, вне
        gate'а; см. REQ-A3 caveat).

### Фаза 4: Backend e2e-тесты

15. [ ] **REQ-E5.** Создать `backend/e2e/testutil/audit.go` с
        `AssertAuditEntry` и `ContainsAction`. Перевести
        `audit/audit_test.go` на `testutil.ContainsAction`.
16. [ ] **REQ-E3.** `e2e/creator_application/creator_application_test.go` —
        выкинуть `expectedCreatorApplication`, переписать сравнение на
        `apiclient.CreatorApplicationDetailData` напрямую с резолвом
        динамических полей (см. REQ-E3 для паттерна).
17. [ ] **REQ-E6.** В `TestSubmitCreatorApplication` — после успешного
        submit вызвать `testutil.AssertAuditEntry(...,
        "creator_application_submit")`. Создать админский клиент через
        `testutil.SetupAdminClient` (помимо публичного).
18. [ ] **REQ-E4.** `e2e/dictionary/dictionary_test.go` — выкинуть
        `NotContains gaming`, оставить инварианты (формат,
        неубывающий sortOrder, наличие `other`).
19. [ ] **Контроль:** `make test-e2e-backend` чисто.

### Фаза 5: Стандарты

20. [ ] **REQ-S1.** Сжать `backend-libraries.md` до ≤3K. После —
        `wc -c docs/standards/backend-libraries.md` ≤ 3000.
21. [ ] **REQ-S2 + S3 + S4.** Обновить `backend-repository.md` — добавить
        секции «Целостность данных: БД vs бэк», «Идентификатор словаря»,
        «Миграции».

### Фаза 6: Легал-доки sync + CI

22. [ ] **REQ-L1.** Добавить таргеты `sync-legal` и `lint-legal` в
        `Makefile`. Добавить в `.PHONY`.
23. [ ] **REQ-L4.** Один раз прогнать `make sync-legal` — копии
        совпадут с источниками.
24. [ ] **REQ-L2.** Поправить `lint-landing` чтобы зависел от `lint-legal`.
25. [ ] **REQ-L3.** Добавить `make lint-legal` в `.github/workflows/ci.yml`
        в job, который проверяет лендос.
26. [ ] **Контроль:** `make lint-legal` exit 0; искусственно изменить
        одну копию → fail с подсказкой; вернуть.

### Фаза 7: README / .env.example

27. [ ] **REQ-R1.** `README.md` — удалить секцию `Backend CORS_ORIGINS`.
28. [ ] **REQ-R2.** `backend/.env.example` — добавить CORS_ORIGINS блок
        с комментариями.

### Фаза 8: GitHub issues для tech-debt

29. [ ] **REQ-G1..G6.** Создать 6 issues через `gh issue create
        --label tech-debt ...`. Тексты — короткие descriptions из
        соответствующих тредов PR #19. Под каждым issue — короткий
        link на тред (например, `https://github.com/alikhanmurzayev/ugcboost/pull/19#discussion_r<id>`).

### Фаза 9: Артефакты планирования (но НЕ финальный pre-merge коммит)

30. [ ] **REQ-T1.** Триаж `deferred-work.md`. После того как все REQ-G*
        issue созданы — удалить файл целиком (`git rm`).
31. [ ] **REQ-T2.** `git mv _bmad-output/planning-artifacts/landing-form-and-address-cleanup-plan.md
        _bmad-output/planning-artifacts/archive/`.

### Фаза 10: Финальный pre-merge коммит

32. [ ] **REQ-T3.** Отдельный коммит `chore: archive implementation
        artifacts` с тремя `git mv` в `_bmad-output/implementation-artifacts/archive/`
        (см. REQ-T3). Этот коммит создаётся **только перед мерджем в
        main**, не раньше — артефакты остаются в дереве для удобства
        обсуждения PR. `pr19-fixes-plan.md` к этому коммиту НЕ
        прикасается — он остаётся в `planning-artifacts/`.

## Стратегия тестирования

- **Local build/lint** перед фиксацией каждой фазы:
  - Backend: `make build-backend && make lint-backend`.
  - Frontend (для проверки lint-landing после REQ-L2):
    `make lint-landing`.
- **Unit-тесты:** `make test-unit-backend`. Особое внимание — пакеты
  `repository`, `service`, `handler`.
- **Coverage gate:** `make test-unit-backend-coverage` — после REQ-M1
  проверяет ВСЁ покрываемое (публичные И приватные функции/методы) в
  `handler/service/repository/middleware/authz` ≥ 80%. Methodization
  (REQ-A3) метрику затрагивает: новые приватные методы тоже под gate.
  Если methodization обнажает функции, которые раньше пропускал
  фильтр — они должны быть покрыты тестами публичной поверхности.
- **E2E:** `make test-e2e-backend`. Особенно тесты `creator_application/`,
  `dictionary/`, `audit/`.
- **Migration sanity:** `make migrate-reset && make migrate-up` —
  обе миграции (старая `20260420181753_creator_applications.sql` и
  новая forward) применяются последовательно, схема корректная.
- **Legal sync:** `make sync-legal` (idempotent), `make lint-legal`
  (зелёный после sync), искусственный divergence (изменить одну копию)
  → red с подсказкой.
- **Manual smoke:** запустить backend (`make start-backend`), отправить
  POST /creators/applications с дублирующим IIN — проверить что 409
  возвращает обновлённый actionable текст.

### Out-of-scope для этого PR (явно)

- E2E PII-guard test (поиск ИИН/ФИО/телефона в stdout-логах) — REQ-G6.
- Audit-row ассерты во всех mutate-handler'ах вне `creator_application/`
  (auth, brand, и т.п.) — отдельная задача, не в этом PR.
- Strict-server migration (REQ-G1 = отдельный issue).
- Dictionary code-as-PK refactor (REQ-G2).
- WebP-конвертация ассетов лендоса (REQ-G3).

## Оценка рисков

| Риск | Вероятность | Митигация |
|---|---|---|
| Methodization (REQ-A3) ломает существующие service/handler unit-тесты | Средняя | Шаги в Фазе 3: после рефактора прогнать unit перед e2e. Helpers были lowercase free functions → тесты их напрямую не моделировали → должно пройти прозрачно. Если ломается — fix локально. |
| Coverage gate fix (REQ-M1) сразу падает на текущей кодовой базе из-за непокрытых приватных функций | Высокая | Фаза 1 (шаги 1.3-1.4) — выявить непокрытые и поднять до 80% ДО начала остальных правок. Без этого шага все последующие фазы пройдут на фоне красного gate'а, и непонятно будет, что виновато: рефактор или legacy. |
| Удаление DEFAULT 'pending' (REQ-D1) ломает старые INSERT'ы, если где-то Status не передаётся | Низкая | Сейчас единственный INSERT — в `service.Submit`. После REQ-A6 + REQ-B1 Status всегда передаётся. `grep -rn 'appRepo.Create\|CreatorApplicationRow{' backend/internal/` подтверждает один call-site. |
| Удаление CHECK на IIN формат (REQ-D1) пропускает мусор в БД, если бэк не отвалидировал | Низкая | `domain.ValidateIIN` уже включает regex + checksum + 18+. Все INSERT'ы идут через `service.Submit`, который вызывает `ValidateIIN` ПЕРВЫМ. |
| Новая миграция на staging (`creator_applications_relax_constraints`) откатывается через `goose down` — `SET DEFAULT 'pending'` пишет default обратно, но до этого статусы рядов остаются актуальными | Низкая | Down-миграция корректна (восстанавливает CHECK + DEFAULT). Откат не теряет данных. |
| `wc -c backend-libraries.md` после сжатия > 3K | Низкая | Шаг 20 — итеративно сжимать до достижения цели. Реестр (главное содержимое) — 14 строк таблицы, остальное — обвязка, есть много простора. |
| `make sync-legal` (REQ-L1) меняет копии — diff больше ожидаемого | Низкая | После REQ-L4 копии ровно идентичны исходникам. Это и есть инвариант. |
| `lint-legal` в CI добавляет шум при concurrent правках legal-доков | Низкая | `diff -q` за миллисекунды. Если кто-то правит лендос-копию руками без `make sync-legal` — это и должно падать. |
| Финальный pre-merge коммит делается далеко после имплементации; легко забыть какие файлы переехали | Низкая | План явно фиксирует список (3 файла в REQ-T3). Перед мерджем — `git diff main..HEAD --stat -- _bmad-output/` показывает diversification. |

## План отката

Если на staging обнаружена регрессия после мержа:

1. **Регрессия в новой forward-миграции** (`creator_applications_relax_constraints`)
   — `goose down` на staging откатит её до предыдущей (старой
   `20260420181753`). Старая миграция не трогалась → восстановление
   точно то, что было до мержа. Если требуется новая правка — отдельной
   forward-миграцией поверх отката.
2. **Регрессия в backend-коде** (тесты пропустили) — `git revert <merge-commit>`
   на main. Forward-миграция на staging при этом остаётся в применённом
   состоянии (миграция не зависит от кода в смысле синтаксиса), но код
   откатывается.
3. **Регрессия в legal sync** (`make lint-legal` упал в CI на свежей
   ветке после мержа) — попросить разработчика выполнить
   `make sync-legal` локально и закоммитить результат. Никакого hot-fix
   на staging не требуется.
4. **Регрессия в архивации артефактов** — косметика, на работу прода не
   влияет, можно игнорировать до следующего PR.

## Связанные ресурсы

- **PR #19** ([alikhanmurzayev/ugcboost#19](https://github.com/alikhanmurzayev/ugcboost/pull/19))
  — все треды разрешены, источник правды по решениям.
- **`_bmad-output/planning-artifacts/review-agent-initiative.md`** —
  мета-инициатива по построению ревью-агента, для которой PR #19 — v0-сырьё
  чеклиста. Hard rule про миграции зафиксирован там же в разделе
  «Захваченные правила (preview v0 чеклиста)».
- **`docs/standards/`** — обязательные стандарты, на которые опирается
  план. После REQ-S1..S4 в них появятся: усечённый
  `backend-libraries.md`, обновлённый `backend-repository.md` (новые
  секции про data integrity, identifier словаря, миграции).
