# Чеклист ревью PR [REQUIRED]

Источник правды для ревьюер-агента (`/review`). Каждый раздел — обязательная стадия прохождения. Если слой не задет diff'ом — явно отметить "не затронут", не пропускать молча.

Расширяется через "→ Системно:" аннотации в живых ревью PR'ов.

## Принципы

- **VETO Алихана.** Алихан в любой момент может приказать исправить любое finding в текущем PR — без обоснования, поверх любых критериев. Reclassify в `fix-now` без обсуждения. Это правило выше всех принципов и критериев ниже.
- **Default = fix in PR.** Tech-debt issue — исключение, не норма.
- **Каждый слой обязателен.** Все разделы пройти, даже если diff пустой в нём.
- Все стандарты `docs/standards/` — hard rules. Отклонение от стандарта = finding (severity не ниже `major`).

## Hard rules (захваченные из живых ревью)

- **Миграции, прогнанные на staging/prod — НЕ РЕДАКТИРУЮТСЯ in-place. НИКОГДА.** Любая правка = новая forward-миграция. См. `backend-repository.md` § Миграции.
- **Coverage gate — на ВСЕЙ покрываемой поверхности (публичной И приватной).** Awk/grep-фильтр, отсекающий lowercase, — баг gate'а, finding `blocker`. См. `backend-testing-unit.md` § Coverage.
- **PII в стандартных stdout-логах — запрещена.** ИИН, ФИО, телефон, адрес, handle — нельзя ни в `logger.Info`, ни в тексте `error.Message`. См. `security.md`.
- **Аудит-лог — в той же транзакции что и mutate-операция.** Fire-and-forget — finding `blocker`. См. `backend-transactions.md`.
- **Бизнес-defaults — в коде, не в БД.** `DEFAULT 'pending'` в миграции — finding. См. `backend-repository.md` § Целостность данных.
- **Format checks (regex/length) — в коде (domain), не в БД.** CHECK с regex в миграции — finding. Если формат поменяется — миграция БД не нужна.

## Backend (Go)

### handler/

- [ ] Запрос/ответ через сгенерированные типы из `api/`. Ручные struct запрещены. → `backend-codegen.md`
- [ ] Парсинг тела через ServerInterfaceWrapper / strict-server. Ручной `json.NewDecoder(r.Body).Decode` — finding `major`. → `backend-codegen.md`
- [ ] Валидация формата/обязательности/очевидных границ — на handler. До service доходят провалидированные данные. → `backend-architecture.md`
- [ ] Авторизация — один вызов AuthzService. Прямые role-сравнения запрещены. → `backend-architecture.md`
- [ ] Никакого SQL и repository вызовов в handler. → `backend-architecture.md`
- [ ] PII не попадает в `error.Message` и в response body. → `security.md`
- [ ] Все обязательные поля domain-input заполнены при сборке (handler → service). Достать из IP middleware, UA, AgreementVersion и т.п. — обязательно.

### service/

- [ ] Бизнес-логика, требующая данных БД для решения. Дублирование валидации handler — finding `minor`. → `backend-architecture.md`
- [ ] Транзакции — только service. `dbutil.WithTx` — единственный способ. Прямой `pool.Begin()` — `blocker`. → `backend-transactions.md`
- [ ] Аудит-лог — в той же транзакции что и mutate-операция. → `backend-transactions.md`
- [ ] Helper'ы, привязанные к state (repo/config/logger) — методы сервиса. Generic утилиты — package-level. Repo как аргумент helper'а — finding. → захваченные правила
- [ ] PII не пишется в stdout-логи. Только UUID/коды/non-PII context. → `security.md`
- [ ] `error.Message` actionable — даёт пользователю понять что делать. Тупиковые сообщения ("Already exists") — finding `major`. → захваченные правила
- [ ] Business defaults (status, и т.п.) — здесь, не в БД. → `backend-repository.md`
- [ ] Switch-case с одинаковыми ветками — finding `major` (collapsed до одной строки или удалить мёртвую ветку).

### repository/

- [ ] Единственный слой с SQL. → `backend-architecture.md`
- [ ] Прозрачность pool/tx через `dbutil.DB`. Repo не вызывает Begin. → `backend-repository.md`
- [ ] Создание через RepoFactory. Конструкторов на уровне пакета быть не должно. → `backend-repository.md`
- [ ] Row-структуры с тегами `db` (для SELECT) и `insert` (для INSERT). Все business-defaults через insert-tag, не БД. → `backend-repository.md`
- [ ] Константы колонок `{Entity}Column{Field}` и таблиц `Table{Entity}`. Литералы вне `*_test.go` запрещены. → `backend-constants.md`, `naming.md`
- [ ] `sql.ErrNoRows` из stdlib (не `pgx.ErrNoRows`). Прямой импорт `pgx` запрещён (кроме тестов). → `backend-repository.md`
- [ ] Возвращаемые значения — указатели на структуры, не значения. Списки — `[]*Row`. → `backend-repository.md`
- [ ] Pagination: cursor-based для неограниченно растущих таблиц (логи, аудит). → `backend-repository.md`

### middleware / authz

- [ ] Middleware прокидывает context (user_id, role, ip, ua) через `context.WithValue`. → `backend-architecture.md`
- [ ] Authz: одна точка решения через AuthzService. Прямые role-сравнения в handler — `blocker`. → `backend-architecture.md`
- [ ] Logging middleware не пишет PII. → `security.md`

### domain/

- [ ] Бизнес-ошибки, бизнес-константы, бизнес-типы. Ручные дубли API request/response — finding. → `backend-design.md`
- [ ] Ошибочные sentinel'ы определены как `var Err... = errors.New(...)`, обёрнуты в `domain.NewValidationError` / `domain.NewBusinessError` где нужно user-facing. → `backend-errors.md`

### errors

- [ ] Каждая ошибка возвращается или логируется с контекстом. `_ = ...` — `blocker`. → `backend-errors.md`
- [ ] Невалидный ввод = ошибка, не silent fallback. → `backend-errors.md`
- [ ] Ошибка не теряет контекст: оборачивание через `fmt.Errorf("...: %w", err)`. → `backend-errors.md`

### libraries

- [ ] Используется библиотека из реестра `backend-libraries.md`, если задача там упомянута. Велосипеды — finding `major` (с обоснованием в комментарии можно).
- [ ] Поинтеры — через `AlekSi/pointer`. `&local := value; &local` — finding `minor`. → `backend-libraries.md`
- [ ] Случайность — `crypto/rand`. `math/rand[/v2]` — `blocker` (depguard). → `backend-libraries.md`

### naming / стиль

- [ ] Файлы snake_case, пакеты lowercase, структуры/интерфейсы с суффиксом слоя. → `naming.md`
- [ ] Receiver — сокращённое имя типа. → `naming.md`
- [ ] TODO только с номером issue. → `naming.md`
- [ ] Комментарии на английском, объясняют ПОЧЕМУ, не ЧТО. → `naming.md`

## Frontend (web / tma / landing)

### API

- [ ] Все HTTP-запросы через сгенерированный openapi-fetch клиент. Raw `fetch()` — finding `major`. → `frontend-api.md`
- [ ] Query keys — константы с фабриками. Литералы в компонентах — `minor`. → `frontend-api.md`
- [ ] Каждый `useMutation` имеет `onError`. Молчаливые ошибки — `major`. → `frontend-api.md`

### Types

- [ ] API-типы только из `generated/schema.ts`. Ручные `interface`/`type` для API request/response — `major`. → `frontend-types.md`
- [ ] Производные типы через `Pick`/`Omit`/`Partial` от сгенерированных. → `frontend-types.md`
- [ ] OpenAPI enum в рантайм — через const-объект рядом с реэкспортом типов. → `frontend-types.md`

### Components

- [ ] Структура feature-based. → `frontend-components.md`
- [ ] i18n: текст через `react-i18next`. Hardcode JSX-строк — `major`. → `frontend-components.md`
- [ ] Route paths и роли — константы. Литералы — `minor`. → `frontend-components.md`
- [ ] Loading / Error / Empty states обработаны для каждого запроса. Голый текст вместо skeleton — `minor`. → `frontend-components.md`
- [ ] `data-testid` на каждом интерактивном элементе и ключевом контейнере. Отсутствие — `minor`. → `frontend-components.md`
- [ ] Нативные диалоги (`window.confirm/alert/prompt`) — `major`. → `frontend-components.md`
- [ ] Декомпозиция: компонент > 150 строк — `minor`. → `frontend-components.md`

### State

- [ ] Серверное — React Query, глобальное клиентское — Zustand, локальное — useState. Context только для провайдеров. → `frontend-state.md`
- [ ] Auth: access token только в памяти. Refresh — httpOnly cookie. localStorage/sessionStorage для access — `blocker`. → `frontend-state.md`
- [ ] Кнопки мутаций disabled при `isPending`. → `frontend-state.md`
- [ ] Общий код между web/tma — в shared-пакет, не дублирование. → `frontend-state.md`

### Quality

- [ ] TS strict + `noUncheckedIndexedAccess`. → `frontend-quality.md`
- [ ] `any` / `!` / `as` запрещены. → `frontend-quality.md`
- [ ] ESLint: `no-console` (кроме error/warn), `no-explicit-any`, `no-non-null-assertion`. → `frontend-quality.md`
- [ ] Form validation с понятным сообщением. Молчаливый return — `major`. → `frontend-quality.md`
- [ ] Error boundaries настроены. → `frontend-quality.md`
- [ ] Accessibility: labels, aria-label, role="alert", клавиатура. → `frontend-quality.md`
- [ ] Runtime config валидируется при инициализации. → `frontend-quality.md`

## Database / Migrations

- [ ] Каждая миграция = отдельный goose-файл с `+goose Up` и `+goose Down`. Создание через `make migrate-create NAME=...`. → `backend-repository.md`
- [ ] **Миграция, прогнанная на staging/prod, не редактируется in-place.** Любая правка = новая forward-миграция. → `backend-repository.md`
- [ ] БД защищается NOT NULL/CHECK enum/FK/UNIQUE — это data integrity. Format checks (regex) и business defaults в БД — finding `major`. → `backend-repository.md`
- [ ] Down-миграция корректно восстанавливает состояние, либо явно задокументирован prerequisite (например, backfill). → захваченные правила
- [ ] Имена констант таблиц/колонок (`Table{Entity}`, `{Entity}Column{Field}`) согласованы с миграцией. → `backend-constants.md`
- [ ] Индексы для частых WHERE/JOIN/ORDER BY созданы в миграции.

## API contracts (OpenAPI)

- [ ] Контракт = единственный источник истины. Изменения через `make generate-api`. → `backend-codegen.md`, `frontend-types.md`
- [ ] Все 3 фронта (web/tma/landing) пересобраны после изменения yaml.
- [ ] e2e-клиенты пересобраны.
- [ ] Required/optional поля в схеме отражают реальное поведение бэка.
- [ ] Enum'ы перечисляют все возможные значения.

## Tests

### Backend unit

- [ ] Нейминг `Test{Struct}_{Method}`. → `backend-testing-unit.md`
- [ ] `t.Parallel()` везде, новый мок на каждый t.Run. → `backend-testing-unit.md`
- [ ] mockery + testify. Ручные моки — `major`. Точные аргументы в expectations. → `backend-testing-unit.md`
- [ ] Динамические поля проверяются отдельно (`WithinDuration`, `NotEmpty`), подменяются, потом `require.Equal` целиком. → `backend-testing-unit.md`
- [ ] `require.JSONEq` для JSON-полей (порядок ключей не детерминирован). → `backend-testing-unit.md`
- [ ] SQL-литералы в repo-тестах (двойная проверка констант). → `backend-constants.md`
- [ ] Coverage gate: ≥80% на каждой публичной И приватной функции в handler/service/repository/middleware/authz. Падающий gate — `blocker`. → `backend-testing-unit.md`
- [ ] `-race` включён, не отключается. → `backend-testing-unit.md`

### Backend e2e

- [ ] Сравнение через сгенерированные типы из `apiclient`. Shadow-DTO — `major`. → `backend-testing-e2e.md`
- [ ] Динамические поля проверяются отдельно, подменяются, потом `require.Equal` целиком. → `backend-testing-e2e.md`
- [ ] Audit-row для каждого mutate-handler. Отсутствие — `major`. → захваченные правила
- [ ] testutil/ — composable хелперы, переиспользуются. Дубли локальных хелперов — `minor`. → `backend-testing-e2e.md`
- [ ] `t.Parallel()` на всех `Test*`. Уникальные fixture-данные через generated emails/IINs. → `backend-testing-e2e.md`
- [ ] Cleanup через `RegisterCleanup`. Постоянный E2E_CLEANUP=false — `major`. → `backend-testing-e2e.md`
- [ ] Header-комментарий в начале файла на русском, нарратив (не bullet-list). → `backend-testing-e2e.md`
- [ ] Тесты идемпотентны: проходят и на чистой БД, и на накопленной. → `backend-testing-e2e.md`
- [ ] Ассерты `Contains/NotContains` на конкретные значения часто меняющихся словарей — `minor` (хрупкие). Проверять инварианты (формат, сортировка, protected коды).

### Frontend unit

- [ ] Vitest + RTL. Файл теста рядом с тестируемым. → `frontend-testing-unit.md`
- [ ] Loading / Error / Empty states тестируются. → `frontend-testing-unit.md`
- [ ] Snapshot testing запрещён. → `frontend-testing-unit.md`
- [ ] `userEvent` (не `fireEvent`). → `frontend-testing-unit.md`
- [ ] i18n: оборачивание в `I18nextProvider` с реальными переводами, не мок. → `frontend-testing-unit.md`
- [ ] Coverage 80%, `-race` не применимо. → `frontend-testing-unit.md`

### Frontend e2e

- [ ] Playwright по `data-testid`, не по тексту. → `frontend-testing-e2e.md`
- [ ] Composable хелперы (api/ui). → `frontend-testing-e2e.md`
- [ ] Cleanup через `E2E_CLEANUP`. → `frontend-testing-e2e.md`
- [ ] `t.describe` по user flow, не по фиче. → `frontend-testing-e2e.md`
- [ ] Header JSDoc на русском, нарратив. → `frontend-testing-e2e.md`

## Security

- [ ] Логи: без request body, Authorization header, cookies, ПД. → `security.md`
- [ ] Секреты — env vars. Hardcode — `blocker`. → `security.md`
- [ ] Environment явный enum (`local`/`staging`/`production`). Никаких "insecure" / "skip_auth" / "disable_security" флагов. → `security.md`
- [ ] CORS_ORIGINS явный список, не wildcard. → `security.md`
- [ ] CSRF/refresh token в httpOnly cookie. → `security.md`
- [ ] Rate-limiting на критичных публичных endpoint'ах (если применимо).

## Process / Artifacts

- [ ] Стандарты `docs/standards/` обновлены, если поменялись правила (living docs).
- [ ] Артефакты планирования (`_bmad-output/planning-artifacts/`) — без Q&A истории, только итоговое состояние.
- [ ] План/scout содержит преамбулу с требованием полной загрузки `docs/standards/`.
- [ ] Source of truth для legal docs — `legal-documents/`. Копии в лендосе через `make sync-legal`. CI проверяет идентичность.
- [ ] CLAUDE.md обновлён, если поменялся workflow.
- [ ] Коммит-сообщения concise, English, описывают "почему".
- [ ] CI gate для нового инварианта добавлен (lint/test/sync-проверка).

## Расширение чеклиста

Чеклист — living document. Каждый PR с системным ревью пополняет его.

**Источники для пополнения:**
1. **`docs/standards/`** — формализованные правила проекта. Любое новое правило в стандарте автоматически становится кандидатом на пункт чеклиста.
2. **«→ Системно:» аннотации в PR-обсуждениях.** В живом ревью, когда выявлен повторяющийся pattern, Claude в ответе на ревью-коммент помечает решение как **"→ Системно: <правило>"**. Каждая такая аннотация = кандидат на пункт чеклиста.
3. **`_bmad-output/decisions/`** — архитектурные ADR (когда появятся).

**Процесс пополнения после PR:**
1. Извлечь все «→ Системно:» через `gh api repos/.../pulls/<N>/comments --paginate | jq '.[] | select(.body | contains("→ Системно"))'`.
2. Дедуп, группировка по слоям.
3. Дописать в этот файл в соответствующий раздел или в «Hard rules», если правило сквозное.
4. Если правило широкое и не вписывается в один раздел — обновить соответствующий стандарт в `docs/standards/`.
