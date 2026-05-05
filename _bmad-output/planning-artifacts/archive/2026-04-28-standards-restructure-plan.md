# План реструктуризации стандартов и чеклиста ревью

## Преамбула — что должен сделать агент-исполнитель ПЕРЕД началом

Этот план самодостаточный. В новой сессии следующий агент должен выполнить
ровно эту последовательность подготовки, прежде чем притрагиваться к
файлам:

1. **Полностью прочитать ВСЕ файлы в `docs/standards/`** — целиком, без
   сокращений, не «релевантные» поиском, не grep'ом, а каждый файл
   полностью в контекст. Это hard requirement проекта. Используй skill
   `/standards` или прочитай руками: backend-*, frontend-*, общие
   (`naming.md`, `security.md`, `review-checklist.md`).
2. **Прочитать `_bmad-output/planning-artifacts/review-agent-initiative.md`** —
   контекст инициативы по ревью-агенту, объясняет почему эта
   реструктуризация делается.
3. **Прочитать `_bmad-output/planning-artifacts/pr19-fixes-plan.md`** —
   источник 15+ захваченных правил из живого ревью PR #19, которые этот
   план распределяет по стандартам.
4. **Прочитать `.claude/commands/review.md` и `.claude/agents/standards-auditor.md`** —
   текущая модель оркестратора и subagent'а-аудитора стандартов. После
   реструктуризации `standards-auditor.md` обновляется (Фаза 4).

## Контекст

Сейчас:
- `review-checklist.md` (219 строк) **дублирует формулировки** из
  стандартов. Изменение правила требует синхронной правки в двух местах,
  и неминуемо рассинхронизируется.
- 19 стандартов содержательно разной плотности. Ни в одном **нет
  секции `## Что ревьюить`** — ревьюер-агент `standards-auditor` читает
  весь стандарт целиком и сам решает что найти.
- 15+ захваченных правил из PR #19 распределены по `pr19-fixes-plan.md`,
  но **не попали в стандарты как hard rules** — их нет в чеклисте, и
  следующий PR ревьюер пропустит без подсказки.

После Пути B:
- Каждый стандарт имеет секцию `## Что ревьюить` (3-10 буллетов с
  severity-меткой). Это **прямой чеклист** для ревьюер-агента, когда
  diff задевает соответствующий слой.
- `review-checklist.md` превращается в **роутинг**: слой → набор
  стандартов + Hard rules. Никаких дублирующих формулировок правил.
- Hard rules (cross-cutting) остаются в чеклисте однострочниками со
  ссылкой на стандарт ("См. backend-repository.md § Миграции").
- Пополнение чеклиста новым правилом = пополнение конкретного
  стандарта. Чеклист подтягивается автоматом через роутинг.
- `standards-auditor.md` обновляется: при ревью смотрит сначала в
  чеклист (роутинг → набор стандартов), затем в `## Что ревьюить`
  каждого релевантного стандарта.

## Целевой формат секции «Что ревьюить» в стандарте

Пример (security.md):

```markdown
## Что ревьюить

- [blocker] Hardcoded секрет в коде (не env var).
- [blocker] PII (ИИН, ФИО, телефон, адрес, handle) в `logger.Info`/`logger.Debug`
  или в тексте `error.Message`.
- [blocker] CORS_ORIGINS включает `*` (wildcard).
- [major] Anti-fingerprinting нарушение: разный текст ошибки для login
  user-not-found vs wrong-password.
- [major] User-controlled string в построении внешних URL без
  `url.QueryEscape` / `url.PathEscape`.
- [major] Free-text поле без length bound (UA, address — DoS-риск через
  гигантский body).
- [minor] Rate-limiting на критичной публичной ручке отсутствует.
```

Принципы:

- **3-10 буллетов** на стандарт. Плотные стандарты (`backend-repository.md`,
  `backend-testing-e2e.md`) могут иметь 7-10, минимальные — 3-5.
- **Severity-метка обязательна** (`[blocker]`/`[major]`/`[minor]`/`[nitpick]`).
  Severity = что это значит для ревьюер-агента, не "насколько важно
  правило в принципе". См. open question #1 ниже.
- **Каждый буллет — actionable recognition pattern**: то, что ревьюер
  **видит в diff'е** и опознаёт как нарушение. Не "правильно делать X",
  а "неправильно — X в коде".
- **Без дублирования основной части стандарта.** Подробности (как это
  работает, почему, варианты реализации) живут в основной части. `## Что
  ревьюить` — только распознавание, ссылается на основную часть стандарта
  через структуру (читатель смотрит ближайший раздел выше).
- **Раздел в самом конце стандарта** (после всего описательного
  материала). Логика: сначала прочитал стандарт, потом получил чеклист
  для применения.

## Куда какое правило переезжает

### A. Дубли — правило живёт в стандарте, чеклист повторяет → удалить из чеклиста

После Фазы 3 эти строки чеклиста заменяются роутингом ("слой handler/ →
стандарты X, Y, Z" — без перечисления конкретных правил). Сами правила
остаются в стандартах, плюс попадают в их секцию `## Что ревьюить`.

| Чеклист (строка) | Удалить, остаётся в стандарте |
|---|---|
| L27 handler/ Запрос/ответ через generated | `backend-codegen.md` |
| L28 handler/ ServerInterfaceWrapper, no manual decode | `backend-codegen.md` |
| L30 handler/ Авторизация одна точка AuthzService | `backend-architecture.md` |
| L31 handler/ Никакого SQL и repo в handler | `backend-architecture.md` |
| L37 service/ Бизнес-логика которая требует БД | `backend-architecture.md` |
| L38 service/ Транзакции — service, dbutil.WithTx | `backend-transactions.md` |
| L39 service/ Аудит-лог в той же tx | `backend-transactions.md` |
| L43 service/ Business defaults в коде | `backend-repository.md` |
| L48 repository/ Единственный слой с SQL | `backend-architecture.md`, `backend-repository.md` |
| L49 repository/ Прозрачность pool/tx | `backend-repository.md`, `backend-transactions.md` |
| L50 repository/ Создание через RepoFactory | `backend-repository.md`, `backend-transactions.md` |
| L51 repository/ Row с тегами db/insert | `backend-repository.md` |
| L52 repository/ Константы колонок | `backend-repository.md`, `backend-constants.md`, `naming.md` |
| L53 repository/ sql.ErrNoRows из stdlib | `backend-repository.md` |
| L54 repository/ Указатели, не значения | `backend-repository.md` |
| L55 repository/ Pagination cursor для растущих таблиц | `backend-repository.md` |
| L60 middleware/authz одна точка | `backend-architecture.md` |
| L61 middleware/authz logging без PII | `security.md` |
| L65 domain/ Бизнес-ошибки, не дубли API | `backend-design.md` |
| L66 domain/ Sentinel-ошибки + NewValidationError | `backend-errors.md` |
| L70 errors/ Каждая ошибка возвращается или логируется | `backend-errors.md` |
| L71 errors/ Невалидный ввод = ошибка | `backend-errors.md` |
| L72 errors/ Ошибка не теряет контекст: %w | `backend-errors.md` |
| L77 libraries/ AlekSi/pointer | `backend-libraries.md` |
| L78 libraries/ crypto/rand | `backend-libraries.md` |
| L82-85 naming | `naming.md` |
| L91 API/ openapi-fetch, raw fetch finding | `frontend-api.md` |
| L92 API/ Query keys константы | `frontend-api.md` |
| L93 API/ useMutation onError | `frontend-api.md` |
| L97 Types/ generated/schema.ts | `frontend-types.md` |
| L98 Types/ Pick/Omit/Partial | `frontend-types.md` |
| L99 Types/ enum const-объект | `frontend-types.md` |
| L103 Components/ feature-based | `frontend-components.md` |
| L104 Components/ i18n react-i18next | `frontend-components.md` |
| L105 Components/ Route paths константы | `frontend-components.md` |
| L106 Components/ Loading/Error/Empty | `frontend-components.md` |
| L107 Components/ data-testid | `frontend-components.md` |
| L108 Components/ Нативные диалоги | `frontend-components.md` |
| L109 Components/ Декомпозиция > 150 строк | `frontend-components.md` |
| L113 State/ React Query/Zustand/useState/Context | `frontend-state.md` |
| L114 State/ Access token в памяти | `frontend-state.md`, `security.md` |
| L115 State/ Disabled при isPending | `frontend-state.md` |
| L116 State/ Shared между web/tma | `frontend-state.md` |
| L120-126 Quality | `frontend-quality.md` |
| L130-135 DB/Migrations | `backend-repository.md` |
| L139-143 API contracts OpenAPI | `backend-codegen.md`, `frontend-types.md` |
| L149-156 Backend unit | `backend-testing-unit.md`, `backend-constants.md` |
| L160-168 Backend e2e | `backend-testing-e2e.md` |
| L172-177 Frontend unit | `frontend-testing-unit.md` |
| L181-185 Frontend e2e | `frontend-testing-e2e.md` |
| L189-194 Security | `security.md` |

### B. Только в чеклисте → перенести в стандарт + добавить в `## Что ревьюить`

| Чеклист (строка) | Куда перенести |
|---|---|
| L29 handler/ валидация формата/обязательности на handler | `backend-architecture.md` (раздел «Ответственности слоёв» уже описывает; добавить в `## Что ревьюить`) |
| L33 handler/ Все обязательные поля domain-input заполнены (IP, UA, AgreementVersion из middleware) | `backend-architecture.md` (новое правило в секции про handler) |
| L40 service/ Helpers привязанные к state — методы; generic — package-level | `backend-architecture.md` (раздел про service) |
| L42 service/ error.Message actionable | `backend-errors.md` |
| L44 service/ Switch-case с одинаковыми ветками | `backend-errors.md` (или `backend-design.md` — выбрать) |
| L76 libraries/ Используется библиотека из реестра | `backend-libraries.md` (есть в принципе, усилить в `## Что ревьюить`) |
| L162 e2e/ Audit-row для каждого mutate-handler | `backend-testing-e2e.md` |
| L168 e2e/ Contains/NotContains на конкретные значения хрупкие | `backend-testing-e2e.md` |
| L195 Security/ Rate-limiting на критичных публичных endpoint'ах | `security.md` |
| L41 service/ PII в stdout-логах | `security.md` (есть, усилить) |
| L32 handler/ PII в response body / error.Message | `security.md` (есть, усилить) |

### C. Захваченные из PR #19 → распределение по стандартам

Не в чеклисте, не в стандартах. Это новые правила, которые вносим в
стандарты в Фазе 2.

| Правило | Куда добавить (в основную часть + `## Что ревьюить`) |
|---|---|
| URL-escape user-strings в построении внешних URL (Telegram deep-link) | `security.md` |
| `pgconn 23505` (UNIQUE violation) → domain-error на concurrent race | `backend-repository.md` (раздел «Ошибки») |
| `logger.Info("success...")` ПОСЛЕ WithTx commit, не внутри callback | `backend-transactions.md` (раздел про логи или новый «Side effects вокруг tx») |
| Trim + non-empty validation для required string полей | `backend-architecture.md` (handler) или `backend-errors.md` (валидация ввода) |
| Length bounds для free-text (UA truncate to 1024 и т.п.) | `security.md` |
| Granular error codes (`CodeInvalidIIN`, `CodeUnderAge`) vs generic `CodeValidation` | `backend-errors.md` |
| `req.Type.Valid()` вместо хардкод-списка enum | `backend-codegen.md` |
| Captured-input test для middleware-derived полей (IP, UA) | `backend-testing-unit.md` (раздел про handler-тесты) |
| Helpers-методы (привязанные к state) vs free functions (generic) | `backend-architecture.md` (раздел про service) — см. B |
| Switch-case с одинаковыми ветками — collapse | `backend-errors.md` или `backend-design.md` — см. B |
| PII в URL params (query string) | `security.md` |
| Anti-fingerprinting в auth/validation messages | `security.md` |
| Public endpoints требуют actor_id NULLABLE в audit | `backend-transactions.md` (раздел «Аудит-лог») |
| `SET NOT NULL` в Down-migration corner case | `backend-repository.md` (раздел «Миграции») |
| Format-CHECK даже как defense-in-depth — finding | `backend-repository.md` (раздел «Целостность данных» — есть в общей формулировке, усилить) |
| Документ "что хранится и почему" для каждой PII/legal колонки (PII inventory) | `security.md` (новый раздел «PII inventory») |
| `error.Message` actionable hint для пользователя | `backend-errors.md` — см. B |

Landing-specific:

| Правило | Куда |
|---|---|
| Form input masks (phone +7-формат, IIN 12 цифр) | `frontend-quality.md` (раздел «Landing-specific» или расширить «Валидация форм») |
| Submit-lock пока async prereqs не загружены | `frontend-state.md` (расширить «Кнопки мутаций») |
| Double-submit guard (external flag + disabled) | `frontend-state.md` (там же) |
| WebP по дефолту для ассетов | `frontend-quality.md` (раздел «Ассеты» новый) |
| E2E не привязан к конкретным seed-значениям | `frontend-testing-e2e.md` (раздел «Assertions» расширить) |

Tests (общие):

| Правило | Куда |
|---|---|
| Время в e2e относительное (`now - 16years`), не hardcoded | `backend-testing-e2e.md` (раздел «Создание тестовых данных») |
| Race-тест для partial unique index | `backend-testing-e2e.md` (раздел «Сценарии») |
| PII guard test (grep stdout по ИИН/ФИО → 0 совпадений) | `backend-testing-e2e.md` или `security.md` (тестовая секция) |

## Карта стандарт → секция «Что ревьюить»

Каждый блок ниже — драфт `## Что ревьюить` для конкретного стандарта.
Это **то, что попадёт в стандарт** после Фазы 2. На этом этапе плана
формулировки можно и нужно править.

### backend-architecture.md (5-7 буллетов)

```markdown
## Что ревьюить

- [blocker] Прямой SQL вне `repository/` (в handler или service).
- [blocker] Прямые role-сравнения в handler (вместо AuthzService).
- [major] handler не валидирует формат / обязательность / тривиальные
  границы — невалидированные данные доходят до service.
- [major] handler не заполняет обязательные поля domain-input из
  middleware-context (IP, UserAgent, AgreementVersion).
- [major] service дублирует валидацию handler (которая возможна без
  обращения к БД).
- [major] Helpers сервиса, привязанные к state (`s.repoFactory`,
  `s.config`, `s.logger`), оставлены package-level free functions —
  должны быть методами `*XService`.
- [minor] Generic утилиты (`trim`, `parseDate`) сделаны методами
  сервиса — должны быть package-level.
```

### backend-codegen.md (3-5)

```markdown
## Что ревьюить

- [blocker] Ручной struct для API request/response в handler (вместо
  типов из `api/`).
- [blocker] Ручной `json.NewDecoder(r.Body).Decode` в handler (вместо
  ServerInterfaceWrapper / strict-server).
- [major] Ручной `r.URL.Query().Get(...)` / `chi.URLParam(...)` для
  API-эндпоинта (должно идти через сгенерированные параметры).
- [major] Хардкод-список enum-значений в switch / message вместо
  `req.X.Valid()` от сгенерированного типа.
- [minor] Ручной мок (вместо mockery с `all: true`).
```

### backend-constants.md (3-5)

```markdown
## Что ревьюить

- [blocker] Строковый литерал enum-значения в коде (роли, коды ошибок,
  статусы) — должен быть константой/импортом из generated.
- [blocker] Литерал имени колонки/таблицы в коде вне `*_test.go` (в
  тестах — наоборот, обязательны).
- [major] HTTP-заголовок / cookie name как литерал.
- [minor] Конфигурационное значение из конечного набора (log level,
  environment) как литерал.
```

### backend-design.md (4-6)

```markdown
## Что ревьюить

- [blocker] `Set*`-метод для зависимости (после конструктора). Структура
  должна быть иммутабельна после `New*`.
- [blocker] Hardcoded таймаут / TTL / лимит вне Config.
- [major] Косвенное определение окружения (через "если есть VAR_X — это
  prod") вместо явного `ENVIRONMENT` enum.
- [major] Ручной дубль API request/response в `domain/` (для этого есть
  `api/` codegen).
- [major] `RepoFactory` с полями / хранением соединения (должна быть
  stateless).
- [major] Сервис-структура с `Set*` для swap зависимости в тестах
  (моки идут через интерфейс в конструкторе).
```

### backend-errors.md (6-8)

```markdown
## Что ревьюить

- [blocker] `_ = someFunc()` (игнор ошибки).
- [blocker] Молчаливый fallback на default при невалидном вводе
  (`page=-5` → молча `page=1` вместо 422).
- [major] Сервис управляет lifecycle транзакции напрямую (`pool.Begin()`,
  `tx.Commit()`, `tx.Rollback()`) — должен через `dbutil.WithTx`.
- [major] Ошибка пробрасывается без контекста (`return err` вместо
  `fmt.Errorf("xCreate: %w", err)`).
- [major] Sentinel-ошибка не обёрнута в `domain.NewValidationError` /
  `domain.NewBusinessError` там, где она user-facing.
- [major] Generic `CodeValidation` вместо granular кода (`CodeInvalidIIN`,
  `CodeUnderAge`) для известного класса валидационных ошибок.
- [major] `error.Message` тупиковый ("Already exists") — без actionable
  hint что делать пользователю.
- [minor] Switch-case с одинаковыми ветками — collapse в одну строку
  или удалить мёртвую ветку.
```

### backend-libraries.md (3-5)

```markdown
## Что ревьюить

- [blocker] `math/rand` или `math/rand/v2` (depguard banned —
  использовать `crypto/rand`).
- [major] Самописная утилита для задачи, для которой есть библиотека в
  реестре (без обоснования в комментарии).
- [major] Ручной `&local := value; &local` или `ptrTo[T]` вместо
  `pointer.ToString` / `pointer.GetString`.
- [minor] Зависимость удалена из `go.mod`, но реестр в
  `backend-libraries.md` не обновлён тем же PR'ом.
```

### backend-repository.md (8-10)

```markdown
## Что ревьюить

- [blocker] Прямой `pool.Begin()` в repo (lifecycle транзакций — service).
- [blocker] `DEFAULT 'pending'` (или другой business default) для
  business-колонки в миграции — должно быть в коде сервиса.
- [blocker] CHECK с regex/length для format в миграции — должно быть в
  domain.Validate*.
- [blocker] Редактирование уже прогнанной миграции in-place (любая
  правка = новая forward-миграция).
- [major] `pgx.ErrNoRows` вместо `sql.ErrNoRows` из stdlib.
- [major] Прямой импорт `pgx` в `repository/` (вне тестов с pgxmock).
- [major] Repo возвращает структуру по значению (не указатель) или
  `[]EntityRow` (не `[]*EntityRow`).
- [major] Конструктор repo на уровне пакета (вместо `RepoFactory`).
- [major] `SELECT *` или ручной список колонок-литералов вместо
  предвычисленного `entitySelectColumns` через stom.
- [major] Литерал имени колонки в WHERE / ORDER BY / JOIN / частичном
  SELECT вместо константы `{Entity}Column{Field}`.
- [major] Offset-pagination для неограниченно растущей таблицы (логи,
  аудит) — должен быть cursor-based.
- [major] `pgconn 23505` (UNIQUE violation) пробрасывается raw, не
  транслирован в domain-error на concurrent race.
- [major] Down-миграция содержит `SET NOT NULL` без backfill — упадёт,
  если в БД есть ряды с NULL (созданные новой моделью).
```

### backend-testing-e2e.md (8-10)

```markdown
## Что ревьюить

- [blocker] Импорт `internal/` пакетов в `e2e/` (e2e — отдельный module).
- [major] Shadow-DTO для сравнения вместо `apiclient.X` напрямую.
- [major] Динамическое поле (UUID, время) сравнивается через
  `require.Equal` без отдельной проверки + подмены.
- [major] Mutate-handler без `testutil.AssertAuditEntry` (audit-row
  не проверен).
- [major] Hardcoded дата (`birth_date = "2008-01-01"`) — поломается
  через год; использовать `now - 16years`.
- [major] `t.Parallel()` отсутствует в `Test*`.
- [major] `E2E_CLEANUP=false` оставлен в коммите (постоянный, не
  локальный override).
- [major] Дублирующий локальный helper, который должен быть в
  `testutil/` (composable shared).
- [minor] `Contains` / `NotContains` на конкретное значение часто
  меняющегося словаря (хрупкий, не ловит реальных багов).
- [minor] Race-тест для partial unique index не написан.
- [minor] Header-комментарий не на русском, не нарратив (bullet-list,
  HTTP-коды без контекста).
```

### backend-testing-unit.md (8-10)

```markdown
## Что ревьюить

- [blocker] `t.Parallel()` отсутствует в `Test*` или `t.Run`.
- [blocker] Coverage gate отключён или фильтрует часть поверхности
  (например, `$$2 ~ /^[A-Z]/` режет lowercase).
- [blocker] `-race` отключён в make-таргете или в CI.
- [major] Ручной мок (вместо mockery).
- [major] Не проверены точные аргументы в mock expectations
  (`mock.Anything` для аргумента, который должен иметь конкретное
  значение).
- [major] Динамическое поле сравнивается через `require.Equal` без
  `WithinDuration` / отдельной проверки + подмены.
- [major] JSON-поле сравнивается через `require.Equal` (вместо
  `JSONEq` — порядок ключей `json.Marshal` не детерминирован).
- [major] Captured-input проверка не сделана для middleware-derived
  полей (IP, UserAgent, AgreementVersion из context).
- [major] Один мок переиспользуется между `t.Run` (вместо нового на
  каждый сценарий).
- [major] Имя теста не `Test{Struct}_{Method}`.
```

### backend-transactions.md (5-7)

```markdown
## Что ревьюить

- [blocker] Прямой `pool.Begin()` в сервисе (вместо `dbutil.WithTx`).
- [blocker] Аудит-лог fire-and-forget (вне tx с mutate-операцией) —
  если mutate откатится, audit останется.
- [blocker] Nested transaction / savepoint.
- [major] Сервис передаёт `tx` напрямую в `Repo` (вместо
  `s.repoFactory.NewXRepo(tx)`).
- [major] `logger.Info("success...")` ВНУТРИ `WithTx` callback — если
  callback откатится после лога, лог соврёт. Логи успеха пишутся ПОСЛЕ
  `WithTx` (когда commit гарантирован).
- [major] Public endpoint, который пишет audit от анонимного actor'а,
  но колонка `actor_id` в `audit_logs` — `NOT NULL`. Миграция должна
  сделать колонку nullable.
```

### frontend-api.md (3-5)

```markdown
## Что ревьюить

- [major] Raw `fetch()` (вне самого API-клиента, где это инфраструктура
  — refresh-token и т.п.).
- [major] Литерал query key в компоненте (`["brands", "list"]`) вместо
  константы из фабрики.
- [major] `useMutation` без `onError` handler (молчаливая ошибка
  мутации).
```

### frontend-components.md (5-7)

```markdown
## Что ревьюить

- [major] Hardcoded JSX-строка для пользовательского текста (вместо
  `t("...")`).
- [major] `window.confirm` / `window.alert` / `window.prompt`.
- [major] Литерал route path в компоненте/роутере (вместо константы).
- [major] Литерал роли в компоненте (вместо константы или generated
  type).
- [minor] Loading / Error / Empty state не обработан для запроса
  данных (голый текст или пусто).
- [minor] `data-testid` отсутствует на интерактивном элементе или
  ключевом контейнере.
- [minor] Компонент > 150 строк без декомпозиции.
```

### frontend-quality.md (5-8)

```markdown
## Что ревьюить

- [blocker] Runtime config не валидируется при инициализации в
  production — приложение запустится с пропавшим обязательным конфигом.
- [major] `any` / `!` (non-null assertion) / `as` (type assertion) в
  коде. Исключение `document.getElementById('root')!` — единственное.
- [major] `console.log` в коде (только `console.error` / `console.warn`
  допустимы).
- [major] Молчаливый return формы без error message.
- [major] `<input>` / `<select>` / `<textarea>` без связанного
  `<label>` / `aria-label`.
- [major] Ошибка валидации без `role="alert"` / без `aria-describedby`
  на поле.
- [minor] Form input без mask для известного формата (phone, IIN) —
  landing-specific.
- [minor] Image не WebP (jpg/png без webp-fallback) — landing-specific.
```

### frontend-state.md (5-7)

```markdown
## Что ревьюить

- [blocker] Access token в `localStorage` / `sessionStorage` (XSS
  уязвимость — должен быть только в памяти Zustand).
- [major] Refresh token не httpOnly cookie (доступен из JS).
- [major] Глобальный store для локального UI-state (модалка, тоггл).
- [major] Context для часто обновляемого state (re-render всего дерева).
- [major] Кнопка мутации не disabled при `isPending` (двойной submit).
- [major] Submit-lock не учитывает async prereqs (form отправляется до
  загрузки dictionaries / async данных) — landing-specific.
- [major] Дублирование одного и того же файла между web/tma (вместо
  `@ugcboost/shared`).
```

### frontend-testing-e2e.md (5-7)

```markdown
## Что ревьюить

- [major] Локатор по тексту (`getByText("Войти")`) вместо `data-testid`
  — ломается при смене копирайта / i18n.
- [major] `t.describe` по фиче (а не user flow), когда flow естественен.
- [major] Hardcoded зависимость от seed-значения (тест ожидает
  `admin@example.com` — поломается если admin изменился) —
  landing-specific.
- [major] `E2E_CLEANUP=false` в коммите (только локальный override).
- [minor] Header JSDoc не на русском, не нарратив (нумерованный список
  шагов).
- [minor] Edge case дублируется из backend e2e (frontend e2e — только
  критические user flows).
```

### frontend-testing-unit.md (5-7)

```markdown
## Что ревьюить

- [blocker] Snapshot testing.
- [major] `fireEvent` (вместо `userEvent` — ближе к реальному
  поведению).
- [major] i18n замокан (вместо реального `I18nextProvider` с
  переводами).
- [major] MSW для unit-теста (overhead, мокать API-клиент напрямую).
- [major] Loading / Error / Empty state не тестируется для компонента
  с запросом данных.
- [minor] Style / layout / CSS-class в ассерте (только behavior + data).
```

### frontend-types.md (3-5)

```markdown
## Что ревьюить

- [blocker] Ручной `interface` / `type` для API request / response.
- [major] Ручная обёртка `{ data: { ... } }` (вместо извлечения из
  `paths` schema).
- [major] OpenAPI enum используется в рантайме без const-объекта (only
  type — пропадёт при компиляции).
```

### naming.md (5-8)

```markdown
## Что ревьюить

- [major] Файл `.go` не snake_case.
- [major] Receiver — full name типа (вместо сокращённого `s`/`r`/`h`).
- [major] Интерфейс без суффикса слоя (`Brand` вместо `BrandService`).
- [major] TODO без `(#issue-N)`.
- [major] Комментарий объясняет ЧТО (а не ПОЧЕМУ).
- [minor] Компонент-файл не PascalCase (`brandList.tsx` вместо
  `BrandList.tsx`).
- [minor] Хук без префикса `use`.
- [minor] Event handler не `handle{Action}` / Callback prop не
  `on{Action}`.
- [minor] Булева переменная без префикса `is`/`has`/`can`.
```

### security.md (8-10)

```markdown
## Что ревьюить

- [blocker] Hardcoded секрет в коде (token, password, API key) — не
  env var.
- [blocker] PII (ИИН, ФИО, телефон, адрес, handle) в `logger.Info` /
  `logger.Debug` / `logger.Warn`.
- [blocker] PII в тексте `error.Message` (попадает в response body).
- [blocker] PII в URL params (query string или path segment).
- [blocker] CORS_ORIGINS включает `*` (wildcard).
- [blocker] `insecure_*` / `disable_*` / `skip_*` флаг конфигурации
  для production-поведения.
- [major] Anti-fingerprinting нарушение: разный текст ошибки для login
  user-not-found vs wrong-password vs short-password.
- [major] Free-text поле без length bound (UA, address, comment) —
  DoS-риск через гигантский body.
- [major] User-controlled string в построении внешних URL (Telegram
  deep-link, redirect URL) без `url.QueryEscape` / `url.PathEscape`.
- [major] Refresh token не httpOnly cookie.
- [minor] Rate-limiting на критичной публичной ручке отсутствует.
- [minor] PII / legal колонка в БД без записи в "PII inventory"
  документе (что хранится и почему).
```

## Карта чеклиста → роутинг (новая структура `review-checklist.md`)

После Фазы 3 чеклист не содержит формулировок правил. Только роутинг.

```markdown
## Backend (Go)

- handler/ → `backend-architecture.md`, `backend-codegen.md`,
  `backend-errors.md`, `naming.md`, `security.md`
- service/ → `backend-architecture.md`, `backend-design.md`,
  `backend-transactions.md`, `backend-errors.md`, `backend-libraries.md`,
  `security.md`
- repository/ → `backend-architecture.md`, `backend-repository.md`,
  `backend-constants.md`, `backend-libraries.md`
- middleware/ + authz → `backend-architecture.md`, `security.md`
- domain/ → `backend-design.md`, `backend-errors.md`
- libraries/ → `backend-libraries.md`

## Frontend (web/tma/landing)

- API → `frontend-api.md`
- Types → `frontend-types.md`
- Components → `frontend-components.md`, `naming.md`
- State → `frontend-state.md`, `security.md`
- Quality → `frontend-quality.md`

## Database / Migrations

- → `backend-repository.md` (разделы «Миграции», «Целостность данных»)

## API contracts (OpenAPI)

- → `backend-codegen.md`, `frontend-types.md`

## Tests

- Backend unit → `backend-testing-unit.md`, `backend-constants.md`
- Backend e2e → `backend-testing-e2e.md`
- Frontend unit → `frontend-testing-unit.md`
- Frontend e2e → `frontend-testing-e2e.md`

## Security

- → `security.md`

## Process / Artifacts

- → `CLAUDE.md` (общие правила про артефакты, sync-legal)
```

## Hard rules (cross-cutting, остаются в чеклисте)

Эти правила сквозные — задействуют несколько слоёв и стандартов
одновременно. В чеклисте остаются как handle + ссылка, без
дублирования формулировки правила. Полный текст и `## Что ревьюить` —
в соответствующем стандарте.

Принципы (не дублируются нигде в стандартах) остаются формулировками:

```markdown
## Принципы

- **VETO Алихана.** Может приказать reclassify любого finding в `fix-now`
  без обоснования, поверх любых критериев. Выше всех правил.
- **Default = fix-in-PR.** Tech-debt issue — исключение, см. 5
  закрытых критериев в `.claude/commands/review.md`.
- **Каждый слой обязателен.** Все разделы чеклиста пройти, даже если
  diff пустой в нём.
- **Все стандарты `docs/standards/` — hard rules.** Отклонение от
  стандарта = finding, severity не ниже `major`.
```

Hard rules — только handle + ссылка:

```markdown
## Hard rules (cross-cutting)

- Миграции на staging/prod — не in-place → `backend-repository.md` § Миграции
- Coverage gate на всю поверхность → `backend-testing-unit.md` § Coverage
- PII в stdout / error.Message → `security.md` § Что ревьюить
- Аудит-лог в той же tx что и mutate → `backend-transactions.md` § Аудит-лог
- Business defaults в коде → `backend-repository.md` § Целостность данных
- Format checks в коде → `backend-repository.md` § Целостность данных
```

## Решение по landing

**Не создавать `landing-*.md`.** Landing — один из 3 фронтов
(web/tma/landing), активной разработки сейчас нет (после PR #19 —
точечные правки). Создание отдельных стандартов для landing
сейчас = лишняя сложность без выгоды.

Landing-specific правила добавляются короткими секциями внутри
существующих `frontend-*.md`:

- `frontend-quality.md` — раздел «Landing (Astro) specifics»: form
  masks (phone, IIN), WebP по дефолту для ассетов.
- `frontend-state.md` — раздел «Submit-lock с async prereqs»:
  блокировка submit пока async-данные (dictionaries) не загружены.
- `frontend-testing-e2e.md` — раздел «Не привязывать assertions к
  seed-значениям»: тесты не зависят от конкретного admin/seed-record.

Когда landing-разработки станет больше (новый flow, отдельная команда,
свои практики) — выделим `landing-*.md`. Сейчас — не upfront.

## Closed decisions (закрыты Алиханом)

1. **Severity-метка в стандартах** — ДА. `[blocker]` / `[major]` /
   `[minor]` / `[nitpick]` рядом с каждым буллетом в `## Что ревьюить`.
   Severity = severity по умолчанию; ревьюер-агент / оркестратор могут
   escalate/downgrade на основе контекста с обоснованием.

2. **Hard rules — без дублей.** Полный текст правила и `## Что
   ревьюить` живут в соответствующем стандарте. В чеклисте — только
   handle (3-7 слов «что искать») + ссылка на стандарт. Это не дубль
   формулировки.

3. **Process / Artifacts остаётся в чеклисте** отдельным разделом.
   Ревьюер-агент проверяет: стандарты обновлены, артефакты без Q&A
   истории, sync-legal, CLAUDE.md актуален, коммит-сообщения,
   CI gate для нового инварианта.

4. **Короткие стандарты** — расширять основную часть **только там, где
   новое правило требует объяснения** (например, «PII inventory» в
   `security.md` — нужен короткий раздел про инвентаризацию). Остальное
   остаётся минимальным; `## Что ревьюить` = checklist.

5. **Landing-specific** — секции внутри `frontend-*.md`. Не отдельные
   `landing-*.md`.

6. **`error.Message` actionable** → `backend-errors.md`.

7. **Switch-case с одинаковыми ветками** → `backend-design.md`.

## Что Фаза 2 будет делать (для понимания scope)

После одобрения этого плана:

1. **18 правок стандартов** (security.md и backend-repository.md —
   получают больше всего нового; backend-errors.md, frontend-quality.md,
   frontend-state.md — заметные расширения; остальные — добавление
   `## Что ревьюить`).
2. **Новый раздел "PII inventory"** в `security.md` (если open question
   #4 закрыт «расширять»).
3. **Landing-specific подсекции** в 3 `frontend-*.md` (см. выше).
4. **18 секций `## Что ревьюить`** — по одной на каждый стандарт, в
   формате выше.

Объём: ~18 файлов под правки, +200-400 строк суммарно. Можно одним
коммитом «docs(standards): add per-standard review checklists,
distribute captured rules», или разбить по слоям (backend / frontend /
testing). На усмотрение исполнителя — оба варианта приемлемы.

## Что Фаза 3 будет делать

Полный rewrite `docs/standards/review-checklist.md`:

- Удалить все секции с дублирующими формулировками правил
  (handler/ / service/ / repository/ / Components/ / State/ / Quality
  и т.п.).
- Заменить на **роутинг** (см. карту выше).
- Hard rules — оставить, переписать однострочниками со ссылками.
- Принципы (VETO, default fix-in-PR) — оставить.
- Раздел «Расширение чеклиста» — переписать под новую модель: правило
  пополняет стандарт, чеклист подтягивается через роутинг.

Объём: 1 файл, было 219 строк → станет ~80-100 строк.

## Что Фаза 4 будет делать

Обновить `.claude/agents/standards-auditor.md`:

- Раздел «Что обязательно прочитать перед ревью»: уточнить, что
  `review-checklist.md` теперь роутинг, а суть правил — в `## Что
  ревьюить` каждого стандарта.
- Добавить уточнение: при ревью diff'а сначала смотреть в чеклист
  (роутинг → набор стандартов), затем в `## Что ревьюить` каждого
  релевантного стандарта.
- Severity-меткa в стандартах = severity finding'а по умолчанию (если
  open question #1 закрыт «вписывать»).

Объём: 1 файл, минорные правки.

## Связанные ресурсы

- `_bmad-output/planning-artifacts/review-agent-initiative.md` — мета-
  инициатива по ревью-агенту, объясняет принцип «ревьюер ≠ кодер».
- `_bmad-output/planning-artifacts/pr19-fixes-plan.md` — источник 15+
  захваченных правил из живого ревью PR #19.
- `.claude/commands/review.md` — оркестратор ревью.
- `.claude/agents/standards-auditor.md` — субагент-аудитор стандартов
  (обновляется в Фазе 4).
- `.claude/agents/{blind-hunter,edge-case-hunter,acceptance-auditor}.md`
  — другие субагенты (этой реструктуризацией не задеваются).
