# Deferred work

Не доделанное в рамках конкретных PR'ов: pre-existing issues, обнаруженные ревью, и edge case'ы за пределами текущего scope. Каждая запись — `<дата> <откуда> <severity> — <что>`.

## 2026-05-02 — review chunk 6.5 (admin verification e2e)

### Тестовая инфраструктура (cross-cutting)

- **[major] e2e cleanup-функции в afterEach молча проглатывают ошибки.** В `frontend/e2e/web/auth.spec.ts` и в новом `admin-creator-applications-verification.spec.ts` cleanup в `afterEach` обёрнут в `try/catch{}` без логирования. Поломанный `/test/cleanup-entity` приведёт к захламлению БД и flaky-collisions в следующих прогонах. Минимально: `console.warn` в catch (не fail'ит тест, но видно). Кандидат в стандарт `frontend-testing-e2e.md` § Cleanup.
- **[minor] Cleanup без per-call timeout.** While-loop sequentially await'ит cleanup-функции; один залипший backend-вызов блокирует afterEach до Playwright global timeout (60s). Решение: `Promise.race` с 5s per-call.
- **[minor] e2e helper'ы кидают без diagnostic при backend down.** `seedUser` / `seedCreatorApplication` / `linkTelegramToApplication` бросают на `resp.status() !== 201`, но `request.post()` при connection refused бросает раньше с менее информативным stack. Кандидат: добавить wrap'ы с явным "[seedUser] backend at API_URL=… не отвечает".
- **[minor] `auth.spec.ts` cleanup сломан (DELETE /test/users/:email — эндпоинта нет).** При прогоне auth.spec возвращает 404, тест ловит молча, user остаётся в БД. Заменить на `POST /test/cleanup-entity {type:"user", id}` — pattern уже использован в новых helper'ах.

### Генераторы синтетических данных

- **[major] `uniqueIIN()` жёстко привязан к birth=1995-05-15 — flaky-окно ±1 день вокруг 15 мая каждого года.** UTC-midnight race между Node-clock тестового процесса и браузерным `new Date()` может разойтись на 1 в age, что ломает ассерт `${age} ${pluralYears(age)}`. Решение: либо параметризовать дату (`now - 30 years`, не на границе midnight), либо рандомизировать день рождения в +/-365 дней с сохранением возраста ≥18.
- **[minor] `uniqueIIN()` collision-risk при N>1 воркерах.** Birth date фиксирован, century byte fixed, valid checksum-ниши ~9000. Birthday-paradox: при ~15+ заявках в одной партии вероятность collision ~1-5%. Сейчас CI использует `workers=1`, но при включении параллелизма всплывёт. Решение: расширить пространство до случайных дат +/-365 дней.
- **[minor] `uniqueTelegramUserId()` — counter per-process, при параллельных воркерах могут совпасть.** Формула `epoch + (Date.now()%(1<<20))*1024 + counter`; counter сбрасывается на новый Node-процесс. UNIQUE на `telegram_user_id` снят миграцией `20260430203000`, поэтому constraint violation не падает, но семантика "один TG = одна заявка" рушится. Решение: добавить `process.pid` или `crypto.randomBytes` в формулу.

### AC2 timing

- **[minor] AC2 опирается на детерминированный `created_at desc` для трёх sequential POST'ов.** Postgres `now()` имеет microsecond-precision; sequential await'ы разделены RTT 10-50ms — collision маловероятен. Тай-брейк по `id ASC` (UUID rand) — non-deterministic при равных timestamp'ах. На медленной CI с заморожённым clock возможен false-fail. Решение: `setTimeout(10ms)` между сидами в `seedCreatorApplication` либо явный `sort=updated_at`.

### Defensive coding в helper'ах

- **[minor] `parseBirthDateFromIin` не валидирует длину/цифры IIN.** Для невалидного input'а возвращает Invalid Date, что даёт непонятный fail'ed-assert вместо явной ошибки. Сейчас все callers (helper и opts.iin) гарантируют 12-цифровый IIN, но stronger contract — добавить `/^\d{12}$/` проверку до парсинга.
- **[minor] `seedCreatorApplication.middleName=""` дропается из requestBody, но остаётся в SeededCreatorApplication.middleName.** Контракт типизирован как `string | null`; пустая строка ломает консистентность между API call и helper return value. Сейчас тест защищён `filter(Boolean)` в fullName-ассерте, но helper'у стоит нормализовать `"" → null` либо явно сохранить пустую строку в обе стороны.
