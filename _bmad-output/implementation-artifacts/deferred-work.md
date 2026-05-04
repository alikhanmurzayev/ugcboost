# Deferred Work

Известные edge cases и tech-debt items, отложенные из конкретных PR'ов. Каждая запись = одна находка с источником, severity и причиной отложения.

## 2026-05-04 — спека `spec-creator-application-reject-frontend.md`

### `frontend/e2e/helpers/telegram.ts:collectTelegramSent` — три edge cases (severity: blocker/major)

**Источник:** edge-case-hunter review chunk-14.

1. **deadline-pass-до-первого-poll** — если `windowMs ≤ pollIntervalMs`, цикл `while (Date.now() < deadline)` может не выполниться ни разу → false-positive empty result. Сейчас все вызовы используют `windowMs=5000` >> 250, безопасно. Fix: do-while с гарантией ≥1 запроса.
2. **silent-on-non-200** — 5xx/4xx/network ответы тихо пропускаются, цикл крутится на пустую seen-set. Тест получит `[]` без диагностики что endpoint недоступен. Fix: console.warn неожиданный status (или throw).
3. **dedup-key-collision** — ключ `chatId|sentAt|text` сольёт две идентичные отправки с миллисекундной точностью в одну. Сейчас тесты только на `>=1` или `=== []`, не критично; станет проблемой если появится тест на точное `length === N`.

**Почему отложено:** функция была inline в `verify-action.spec.ts:343-367`, я её только перенёс в helper без изменений — pre-existing. Чинить отдельным PR'ом для tests-helpers.

### `ApplicationDrawer.apiError` — banner не сбрасывается при prev/next в drawer (severity: minor)

**Источник:** edge-case-hunter review chunk-14.

Кейс: reject заявки A → 422 → banner. Жмём «Next» → drawer показывает заявку B, но banner про A всё ещё висит.

**Почему отложено:** простой fix — `useEffect(() => setApiError(""), [application?.id])` — но ESLint правило `react-hooks/set-state-in-effect` его блокирует. Альтернативы (`key={applicationId}` на OpenDrawer, render-time-pattern с `useState` reset) или ломают существующие тесты, или требуют рефакторинг chunk-11 verify-flow. По пользователю misleading но не блокирующе. Чинится отдельной задачкой.

### `as ApplicationDetail` в test fixture (severity: minor)

**Источник:** acceptance-auditor review chunk-14.

Файл: `frontend/web/src/features/creatorApplications/components/ApplicationActions.test.tsx:56`. Aналогичный паттерн уже есть в `ApplicationDrawer.test.tsx:82`. По `docs/standards/frontend-quality.md` `as` запрещён.

**Почему отложено:** существующий project-wide паттерн в test fixture builder'ах. Замена на `satisfies` или type guards требует прохода по нескольким тестовым файлам — отдельной задачкой.
