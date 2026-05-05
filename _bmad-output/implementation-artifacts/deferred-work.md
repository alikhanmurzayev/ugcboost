# Deferred Work

Findings из review-цикла, отложенные на отдельные итерации (pre-existing patterns / out-of-scope для текущего PR).

## Из spec-admin-creator-applications-moderation-e2e.md (review 2026-05-05)

### Reject "without TG" assert тавтологичен
- **Где**: `frontend/e2e/web/admin-creator-applications-moderation-reject-action.spec.ts:188-260` и (источник паттерна) `frontend/e2e/web/admin-creator-applications-reject-action.spec.ts:180-254`.
- **Что**: Ассерт «notifier ничего не отправил» через polling по `syntheticChatId = uniqueTelegramUserId()` — рандомный id, который никогда не записан ни в одной таблице `telegram_links`. Notifier не имеет повода писать на этот chatId никогда → `collectTelegramSent` возвращает пустой массив даже если notifier полностью сломан.
- **Почему отложили**: Паттерн установлен в верификационном spec'е и копируется без вопросов; исправление требует side-channel test endpoint'а (например, `/test/telegram/sent?since=...&applicationId=...` или log inspection), что выходит за рамки этого PR. Сейчас отрицательный кейс доказывает только что мы не получаем чужих сообщений на наш random id.
- **Исправление**: добавить тестовую ручку, индексирующую попытки send'а по `applicationId`/`actorId`, и переписать оба assertion'а на неё.

### Cleanup-стек fail-fast останавливает оставшийся drain
- **Где**: каждый e2e spec в `frontend/e2e/web/` (паттерн локальный во всех файлах).
- **Что**: `await withTimeout(fn(), CLEANUP_TIMEOUT_MS, "cleanup")` бросает на первой failed cleanup-функции → afterEach throws → остальные функции в стеке не вызываются → orphan rows остаются в БД.
- **Почему отложили**: Системный паттерн репо. Спека описывает поведение как «fail-fast», но реализация = best-effort first failure (drain прерывается). Исправление требует AggregateError-стратегии или per-call try/catch — затрагивает 5+ spec-файлов разом.
- **Исправление**: вынести cleanup-loop в shared helper `helpers/cleanup.ts` с `Promise.allSettled` или AggregateError, обновить все spec'и одним PR.

### Дублированные `loginAs` и `withTimeout` (5+ копий)
- **Где**: `auth.spec.ts`, `verification.spec.ts`, `verify-action.spec.ts`, `reject-action.spec.ts`, `moderation.spec.ts`, `moderation-reject-action.spec.ts`.
- **Что**: одна и та же `async function loginAs(page, email, password)` и `async function withTimeout(promise, ms, label)` скопированы по 6 файлам.
- **Почему отложили**: Уже системный паттерн в репо; PR добавляет ещё 2 копии, но stop-the-line ради рефакторинга задержал бы основной скоп.
- **Исправление**: вынести в `frontend/e2e/helpers/ui-web.ts::loginAsAdmin` и `helpers/promise.ts::withTimeout`, обновить все 6 spec'ов одним PR.

### `filters-search.fill(uuid)` без debounce генерирует 36 refetch'ей
- **Где**: `ApplicationFilters.tsx` (production code) и каждый e2e тест, использующий filters-search.
- **Что**: При вводе uuid в search-инпут каждый keystroke триггерит `setSearchParams` → `useQuery` refetches → 36 запросов на 36-символьный UUID. На медленном CI это 36×100ms = 3.6s промежуточных запросов.
- **Почему отложили**: Production-level concern (debounce в самом ApplicationFilters), не e2e-фикс. Текущие тесты идемпотентны и зелёные.
- **Исправление**: ввести `useDebounce` (~250ms) в `ApplicationFilters` для search-input; убрать промежуточные refetch'и.

### Reject submit assert не дожидается network response
- **Где**: `frontend/e2e/web/admin-creator-applications-moderation-reject-action.spec.ts:152-156` и в верификационном spec'е тот же паттерн.
- **Что**: после `submit.click()` сразу идут `expect(drawer).toHaveCount(0)` + `not.toHaveURL(/?id=/)`. Если backend ответит 5xx, mutation onError → drawer не закроется, тест fails с timeout 30s вместо понятного error.
- **Почему отложили**: Pre-existing паттерн; правильное решение — assertion на `reject-dialog-error` testid или explicit `await expect(submit).toBeDisabled()` для pending-state.
- **Исправление**: добавить promise-based wait на конкретный success-signal от mutation (например, query-invalidation event).

## Кандидаты в стандарты

Список ниже — предложения для `docs/standards/frontend-testing-e2e.md` § Что ревьюить (от ревьюеров standards-auditor / blind-hunter / edge-case-hunter):

- **e2e-ассерты на «X не произошло»** должны делаться через side channel, который точно бы зарегистрировал X — не через random-id, на который X физически не мог быть направлен.
- **Helpers, форматирующие даты для e2e-assertion'ов** обязаны использовать `timeZone: "UTC"` в `Intl.DateTimeFormat` options ИЛИ строить строку через manual UTC math (getUTCDate / getUTCMonth / getUTCFullYear) — `test.use({ timezoneId: "UTC" })` пинит TZ только для browser, не для Node-runner'а.
- **Composable seed-helpers с multi-step setup** (seed → link → promote) должны принимать cleanupStack и push'ить cleanup после каждого успешного шага, либо делать try/catch с rollback внутри.
- **e2e-spec'и, опирающиеся на промоушн заявки через webhook** обязаны проверять достигнутый статус через GET-detail перед UI-сценарием — webhook returns 200 на no-op.
- **e2e-тесты, проверяющие порядок строк по `updated_at`/`created_at`** от последовательных HTTP-запросов, обязаны включать sleep ≥ 50ms между запросами либо ассертить нестрогий порядок.
- **e2e-spec'и для admin-страниц** должны резолвить i18n-копи через ApiResponse (dictionary endpoint) или testid'ы, не сравнивать с hardcoded русским текстом.
- **Дубликаты helper-функций между spec'ами e2e** обязаны быть подняты в `helpers/*` с первого появления второй копии (DRY-2).
- **JSDoc-нарратив** должен соответствовать коду spec'а; расхождение нарратив↔код — finding.
- **Local helper в spec'е, дублирующий функцию из соседнего spec'а** — поднять в helpers/*.
