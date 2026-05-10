# Deferred work

Накопительный список находок, которые review-цикл собрал, но они не блокируют
текущий слайс. Каждая запись — это либо pre-existing проблема, surfaced
incidentally, либо follow-up к завершённой фиче, требующий отдельного решения.

## Из ревью UTM-меток (2026-05-10)

### PII-policy для UTM-полей
- **Severity**: major
- **Контекст**: спека и комментарий `auditNewValue` декларируют UTM как «non-PII
  tracking metadata», но никакой backend-валидации формата нет. Атакующий или
  маркетолог может вложить телефон / email / IIN в `utm_term` через прямой
  API-вызов — значение попадёт в БД и в `audit_logs.new_value`.
- **Что нужно решить**: либо добавить charset whitelist в OpenAPI / handler
  (`^[A-Za-z0-9_.\-]+$` — стандартный UTM формат), либо обновить
  `docs/standards/security.md` записью «UTM-поля считаются user-controlled и
  могут содержать произвольные данные до cap'а».
- **Why deferred**: требует policy-decision на уровне продукта (Aidana) и
  потенциального обновления стандарта — это не зона текущего dev-слайса.

### Дублирующиеся UTM-ключи в URL не ассертятся
- **Severity**: minor
- **Контекст**: `?utm_source=A&utm_source=B` → `URLSearchParams.get` берёт
  первое значение. Поведение разумное, но не покрыто тестом и не
  документировано.
- **Что нужно**: добавить кейс в `frontend/landing/src/lib/utm.test.ts` либо
  явный `getAll().length > 1` warning в `captureUTM()`.
- **Why deferred**: маркетинг сейчас не генерирует ссылки с дублями, риск
  низкий.

### `noUncheckedIndexedAccess` отключён в landing tsconfig
- **Severity**: minor
- **Контекст**: `frontend/landing/tsconfig.json` extends `astro/tsconfigs/strict`,
  который не включает `noUncheckedIndexedAccess` (web/tma его включают). По
  стандарту `frontend-quality.md` флаг обязателен.
- **Что нужно**: добавить `"compilerOptions": {"noUncheckedIndexedAccess": true}`
  в `frontend/landing/tsconfig.json` и пройтись по landing-исходникам, починив
  возникшие type errors.
- **Why deferred**: правка cross-cutting, может вскрыть несвязанные с UTM
  места — отдельный slice.

## Из дополнительного ревью UTM-меток (2026-05-10, второй раунд)

### UTM charset whitelist (Unicode bidi/zero-width)
- **Severity**: major
- **Контекст**: `validateUTMField` пропускает любые printable Unicode-символы
  до 256 рун. U+202E (RTL OVERRIDE), U+200B (ZERO WIDTH SPACE), U+2028/U+2029
  (LINE/PARAGRAPH SEPARATOR) проходят. React текст-эскейпит, но визуально
  bidi-trick переворачивает направление, а ZWSP ломает аналитические count'ы
  (`telegram` ≠ `telegram​`).
- **Что нужно**: расширить `validateUTMField` allow-list'ом
  `^[A-Za-z0-9._\-~+%]+$` (RFC 3986 unreserved + UTM-стандарт). Реальные UTM
  от GA / Google Ads / Facebook Ads вписываются. Или задокументировать
  обратное в `docs/standards/security.md`.
- **Why deferred**: расширяет уже-deferred PII-policy item; требует policy
  decision продакта (Aidana) одновременно с PII-вопросом — единый раунд.

### captureUTM глотает sessionStorage errors молча
- **Severity**: minor
- **Контекст**: `frontend/landing/src/lib/utm.ts:42-45` — try/catch вокруг
  `setItem` без логирования. В Safari private mode quota-exceeded заглушится;
  маркетинг получит «0 заявок с UTM» и не поймёт почему.
- **Что нужно**: `console.warn("[utm] sessionStorage unavailable", err)` в
  catch'е (или Sentry hook когда подключим).
- **Why deferred**: low priority — UTM tracking задумано best-effort, но
  observability стоит дёшево, добавим следующим.

### Service UTM tests: refactor на mock.Run + require.JSONEq
- **Severity**: minor
- **Контекст**: `backend/internal/service/creator_application_test.go:1267-1409`
  — три UTM-кейса используют `mock.MatchedBy(func(row) bool {...})` с
  custom `jsonEqRaw` хелпером. На падении предикат возвращает false, и
  testify печатает «no matching call expected» без диффа.
- **Что нужно**: заменить на `mock.Run` + capture + `require.JSONEq`
  снаружи матчера, как в `creator_application_test.go:2841-2849`. Capture
  всю строку Row, потом ассертить и UTM, и остальные поля единым
  `require.Equal` с динамическими полями, скопированными из captured.
- **Why deferred**: текущие тесты работают, диагностика — это improvement,
  не block.

### Negative e2e: oversize/control-char UTM на wire
- **Severity**: minor
- **Контекст**: `validateUTMField` unit-тестируется, но через wire ни одна
  e2e-проверка не подтверждает, что 422 фактически возвращается. Если
  generated handler-route когда-нибудь оторвётся от validator'а, регрессия
  пройдёт незамеченной.
- **Что нужно**: один e2e-кейс в `backend/e2e/creator_application/...`:
  POST с `utmSource = strings.Repeat("a", 257)` → expect 422 + body.code =
  VALIDATION_ERROR. Аналогично для `utmCampaign = "spring\x00"`.
- **Why deferred**: low risk — handler-цепочка стабильная, validator
  unit-тесты ловят регрессию контракта.

### UTM round-trip coverage в GetByID / GetByIDForUpdate / GetByVerificationCodeAndStatus
- **Severity**: minor
- **Контекст**: repo unit-тесты `Create` ассертят non-nil UTM в RETURNING,
  но `success maps row to struct` для трёх SELECT-методов передают `nil` в
  `AddRow` для UTM-колонок. Mapping non-nil utm-пойнтеров в этих методах не
  покрыт — регрессия в stom-биндинге для select-only paths не сработает.
- **Что нужно**: расширить existing «success maps row to struct» кейсы или
  добавить отдельный `t.Run("utm round-trip")` в каждом методе с
  `pointer.ToString("chat")`-значениями в `AddRow` и `require.Equal` всей
  Row.
- **Why deferred**: low risk — все три метода используют одну и ту же
  stom-конфигурацию; если Create read-back проходит, остальные тоже.

### SSR-branch для captureUTM/readUTM
- **Severity**: minor
- **Контекст**: оба метода имеют `if (typeof window === "undefined") return ...`
  для SSR. Тесты не покрывают этот путь (vitest всегда в jsdom-окружении с
  `window` определённым). Если кто-то импортирует `lib/utm.ts` из Astro
  build pipeline (SSR-фаза) — компилятор пропускает, regressions silent.
- **Что нужно**: один кейс с `vi.stubGlobal("window", undefined)` в начале —
  `expect(captureUTM()).toBeUndefined()` и `expect(readUTM()).toEqual({})`.
- **Why deferred**: текущий index.astro вызывает оба метода только в
  client-script (`<script>` без `is:server`), SSR-путь сейчас никогда не
  выполняется.

### UTM detail wire-contract: omitempty vs always-present
- **Severity**: minor
- **Контекст**: спека `creator-application-utm` § Boundaries декларирует «5
  полей возвращаются всегда (null если пусто)», но oapi-codegen с
  `nullable: true` генерит `*string \`json:"utmSource,omitempty"\``. Nil
  pointer → ключ выпадает из JSON, не пишется как `null`. Drawer + e2e
  справляются (`?? null` / `typeof v === "string"`), но контракт между
  спекой и wire-форматом расходится.
- **Что нужно**: либо убрать `omitempty` через oapi-codegen overrides
  (per-field setting), либо обновить спеку, что null-поля **опускаются**
  на wire. Затем разрешить e2e ассерт `expect(detail.utmMedium).toBeNull()`
  без `?? null`-fallback.
- **Why deferred**: функционально работает; решение требует понимания, как
  TMA/web реагируют на missing keys — отдельный аудит.
