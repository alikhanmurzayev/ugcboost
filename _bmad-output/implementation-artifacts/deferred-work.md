# Deferred work

Не доделанное в рамках конкретных PR'ов: pre-existing issues, обнаруженные ревью, и edge case'ы за пределами текущего scope. Каждая запись — `<дата> <откуда> <severity> — <что>`.

## 2026-05-02 — review chunk 6.5 (admin verification e2e), вторая итерация

После ревью PR #50 первой итерацией исправлено в этом же PR (см. Spec Change Log в `spec-admin-creator-applications-verification-e2e.md`):

- cleanup в afterEach падает на первой ошибке + per-call 5s таймаут (`Promise.race`)
- `auth.spec.ts` переехал на `seedAdmin` helper (`/test/cleanup-entity`), DELETE /test/users больше нет
- `uniqueIIN` рандомизирует год/месяц/день через `crypto.randomBytes` — пул ~70M, как у backend `testutil.UniqueIIN`
- `uniqueTelegramUserId` на `crypto.randomBytes(6)` — параллельно-безопасно без atomic counter
- `seedCreatorApplication` делает `sleep(10ms)` перед каждым POST для детерминированного `created_at desc`
- `middleName=""` больше не дропается из API-call; backend trimOptional нормализует к nil; добавлен спец-тест на `middleName=null` для покрытия "creator without отчества"

### Открытые пункты

- **[minor] e2e helper'ы кидают без diagnostic при backend down.** `seedUser` / `seedCreatorApplication` / `linkTelegramToApplication` бросают на `resp.status() !== 201`, но `request.post()` при connection refused бросает раньше с менее информативным stack. Кандидат: добавить wrap'ы с явным "[seedUser] backend at API_URL=… не отвечает".
- **[minor] `parseBirthDateFromIin` не валидирует длину/цифры IIN.** Для невалидного input'а возвращает Invalid Date, что даёт непонятный fail'ed-assert вместо явной ошибки. Сейчас все callers (helper и opts.iin) гарантируют 12-цифровый IIN, но stronger contract — добавить `/^\d{12}$/` проверку до парсинга.

### Кандидаты в стандарты

- **`docs/standards/frontend-testing-e2e.md` § Cleanup.** Зафиксировать: cleanup падает на первой ошибке (fail-fast) + per-call timeout. Поломанный backend-вызов или висящий cleanup → flaky-collisions в следующих прогонах. Поведение реализовано в обоих spec'ах, нужно поднять до правила.
- **`docs/standards/frontend-types.md`.** Добавить упоминание `frontend/e2e/types/{schema.ts, test-schema.ts}` как production/test OpenAPI sources for e2e helpers. Сейчас правило про "только generated API-типы" применимо к web/tma/landing, но e2e-модуль — отдельный, и явная ссылка снимает двусмысленность.
- **`docs/standards/frontend-testing-e2e.md` § Header JSDoc.** Стандарт сейчас требует русский нарратив только для spec-файлов. Helper-модули (`frontend/e2e/helpers/*.ts`) не оговорены — на практике у них английский header. Решить: оставить английский (test infrastructure ближе к коду) или распространить русское правило.
- **`docs/standards/frontend-testing-unit.md`.** `pluralYears` и `ageInYearsUTC` дублируются между `frontend/e2e/web/admin-creator-applications-verification.spec.ts` и `frontend/web/src/.../ApplicationDrawer.tsx`. По стандарту "frontend e2e — изолированный модуль" это допустимо, но рассинхрон логики при изменении plural-правил → silent fail. Кандидат: вынести `pluralYears` в `shared/utils/plural.ts` + unit-тест и использовать в spec через локальную копию (или переписать assert в spec через regex `год|года|лет`).
