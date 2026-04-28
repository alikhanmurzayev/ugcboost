# План реализации: понизить минимальный возраст подачи заявки креатора с 21 до 18

> Перед стартом: загрузить полный набор стандартов из `docs/standards/` (`/standards`) — особенно `backend-design.md`, `backend-testing-unit.md`, `frontend-quality.md`, `naming.md`. Все правила оттуда — hard rules.

## Обзор

Откатываем `domain.MinCreatorAge` обратно к 18 — возвращаем поведение, описанное в PRD §FR3, и расширяем воронку заявок (сейчас 18-20-летним прилетает 422 `UNDER_AGE`). Скоуп: одна доменная константа в backend, два теста, две статические строки на лендинге, плюс синхронизация спеки.

## Требования

- **REQ-1 (must):** `domain.MinCreatorAge` = 18. Сообщение валидации становится «Возраст менее 18 лет» автоматически (service использует `domain.MinCreatorAge` в `fmt.Sprintf`).
- **REQ-2 (must):** Backend unit-тесты, ловившие `under MinCreatorAge` сценарий, продолжают ловить ошибку. `under by one day` в `iin_test.go` после смены константы не должен молча пройти (20 ≥ 18) — отвязать даты от конкретного значения, привязав к самой константе.
- **REQ-3 (must):** E2E backend (`creator_application_test.go`) — `const minAge` синхронизирован с domain (поменять хардкод 21 → 18; формула `now - (minAge - 2)` остаётся валидной).
- **REQ-4 (must):** User-facing текст лендинга «Тебе 21+» → «Тебе 18+» (две строки в `content.ts`).
- **REQ-5 (must):** Комментарий-пример в `frontend/landing/src/api/client.ts` — синхронизирован с фактическим серверным сообщением («Возраст менее 18 лет»).
- **REQ-6 (should):** Спека `spec-creator-application-submit.md` — отметить откат на 18 (living document, исторический контекст сохранить).
- **REQ-7 (must, comment hygiene):** Комментарий в `domain/iin.go` после правки — на английском, объясняет «почему 18», без упоминаний прошлого подъёма до 21 (комментарии — это про принятое решение, не журнал изменений; источник правды истории — git log + спека).

### Nice-to-have

- Не выявлены. Конфигурация значения через env/Config — out of scope (см. альтернативу в scout: overkill для одноразового изменения).

### Вне скоупа

- PRD/legal documents — уже говорят «18+», не трогаем.
- Backend OpenAPI / SQL миграции — возраст не материализуют.
- `frontend/web`, `frontend/tma` — константа их не касается.
- Client-side JS-валидация возраста — её нет (форма шлёт ИИН, ошибка приходит с сервера) и добавлять её сейчас не требуется.

### Критерии успеха

1. `cd backend && go test ./internal/domain/... ./internal/service/... -count=1 -race` — зелёный.
2. `make test-unit-backend-coverage` — зелёный (per-method ≥80%).
3. `make test-e2e-backend` — зелёный, включая `creator_application` пакет, сценарий `under MinCreatorAge rejected with UNDER_AGE`.
4. `make lint-backend`, `make lint-landing`, `make build-landing` — зелёные.
5. Ручная проверка не нужна (UI-текст — статика, бизнес-логика покрыта e2e). Но если хотим — `make start-landing` и убедиться, что в блоке «Кого мы ищем» теперь «Тебе 18+».

## Файлы для изменения

| Файл | Изменения |
|------|-----------|
| `backend/internal/domain/iin.go` | `MinCreatorAge = 21` → `18`. Перефразировать godoc-комментарий (строки 26-29): убрать упоминание подъёма до 21 / EFW, оставить лаконичное описание «minimum age required to submit a creator application» с указанием источника требования (PRD FR3 / 18+ в РК). |
| `backend/internal/domain/iin_test.go` | TestEnsureAdult: подтесты `under MinCreatorAge by one day` и `exactly MinCreatorAge today` (строки 133-146) — даты привязать к `MinCreatorAge`, чтобы тесты не зависели от конкретного значения. Конкретно: `birth := time.Date(2005, time.April, 21, …)` и `now := time.Date(2005+MinCreatorAge, time.April, 20, …)` (для `under by one day` — возраст = MinCreatorAge-1); `birth := time.Date(2005, time.April, 20, …)` и `now := time.Date(2005+MinCreatorAge, time.April, 20, …)` (для `exactly today` — возраст = MinCreatorAge ровно). Третий подтест `comfortably adult` (строки 148-153) трогать не надо. |
| `backend/e2e/creator_application/creator_application_test.go` | Строка 580 `const minAge = 21 // mirrors domain.MinCreatorAge` → `18`. Комментарий «mirrors domain.MinCreatorAge» оставить — он подсвечивает связь с константой. |
| `frontend/landing/src/content.ts` | Строки 94 и 97: `bold: "Тебе 21+"` → `"Тебе 18+"` (обе вхождения в `criteria.items[0]`). |
| `frontend/landing/src/api/client.ts` | Комментарий в строках 24-25: пример `"Возраст менее 21 лет"` → `"Возраст менее 18 лет"` (синхронизация с фактическим сообщением сервера). |
| `_bmad-output/implementation-artifacts/spec-creator-application-submit.md` | Строка 204: запись «MinCreatorAge поднят с 18 до 21. Бизнес-фильтр EFW…» — дописать «откатили обратно к 18 на 2026-04-28: расширяем воронку заявок, EFW-фильтр снимаем». Строка 228: `domain.MinCreatorAge = 21` → `18`. |

## Файлы для создания

Никаких новых файлов не нужно.

## Шаги реализации

1. [ ] **Создать ветку.** `git fetch origin && git checkout main && git pull && git checkout -b alikhan/lower-creator-min-age`. Перед этим убедиться через `git status`, что текущее рабочее дерево чистое (есть untracked `infra/_step.sh` — это сторонний файл, его не трогаем; `git stash --include-untracked` если мешает, потом восстановить).
2. [ ] **Backend, доменная константа.** В `backend/internal/domain/iin.go`: `MinCreatorAge = 18`. Переписать комментарий 26-29 на английском, без истории про 21/EFW. Пример: `// MinCreatorAge is the minimum age required to submit a creator application. Mirrors PRD FR3 ("18+ via IIN birth date").`
3. [ ] **Backend, доменные тесты.** В `backend/internal/domain/iin_test.go` для подтестов `under by one day` и `exactly today`: даты завязать на `MinCreatorAge`. После правки — `cd backend && go test ./internal/domain/... -count=1 -race -run TestEnsureAdult` локально, убедиться зелёный.
4. [ ] **Backend, e2e константа.** В `backend/e2e/creator_application/creator_application_test.go:580` — `const minAge = 18`. Запустить локально `make test-e2e-backend` (длительно — поднимет docker compose; делать в фоне другой команды можно).
5. [ ] **Frontend, текст лендинга.** В `frontend/landing/src/content.ts` обе строки `"Тебе 21+"` → `"Тебе 18+"`.
6. [ ] **Frontend, комментарий-пример.** В `frontend/landing/src/api/client.ts:24-25` обновить пример сообщения.
7. [ ] **Спека.** В `_bmad-output/implementation-artifacts/spec-creator-application-submit.md`: отметить откат и поправить значение константы.
8. [ ] **Локальные гейты (обязательны до push, см. memory `feedback_local_build_first`).** Запустить:
   - `make lint-backend`
   - `make test-unit-backend`
   - `make test-unit-backend-coverage`
   - `make test-e2e-backend`
   - `make lint-landing`
   - `make build-landing`
   
   Если что-то падает — фиксим и перепроверяем тот же таргет, потом запускаем дальше.
9. [ ] **Передать на ревью Alikhan.** Согласно memory — Claude НЕ коммитит и НЕ мержит сам. Изменения остаются в working tree до ручного ревью. После апрува — Alikhan сам делает коммит, push, PR.

## Стратегия тестирования

- **Unit-тесты (Go):**
  - `domain.TestEnsureAdult` — три подтеста (под-возраст, ровно по возрасту, comfortably adult). Перепривязка к константе делает их робастными к будущим изменениям.
  - `service.TestCreatorApplicationService_Submit` (`under MinCreatorAge fails before tx`) — уже использует `domain.MinCreatorAge`, **изменений не требуется**, но прогнать обязательно: должен остаться зелёным.
- **Coverage gate:** `make test-unit-backend-coverage` обязателен — ни одна публичная/приватная функция в покрываемых пакетах не должна упасть ниже 80%. Текущая константа меняется без изменения сигнатур, риск минимальный, но проверить надо.
- **E2E backend:** `make test-e2e-backend` — обязателен. Сценарий `under MinCreatorAge rejected with UNDER_AGE` должен пройти с новой константой (`buildUnderageIIN()` строит ИИН на 16 лет назад — гарантированно под 18).
- **Frontend:** статические правки текста + комментарий — поверхность тестов не появляется. `make build-landing` + `lint-landing` (tsc + eslint) перекрывают регресс.
- **Ручная проверка лендинга (опционально):** `make start-landing` → http://localhost:3003, в блоке «Кого мы ищем» — «Тебе 18+».

## Оценка рисков

| Риск | Вероятность | Митигация |
|------|-------------|-----------|
| `iin_test.go` `under by one day` молча пройдёт после смены константы (если забыли перепривязать даты) | Средняя | Шаг 3 явно требует перепривязки. Локальный прогон `go test -run TestEnsureAdult` ловит регрессию мгновенно. |
| Coverage упал из-за того, что какая-то ветка стала недостижимой | Низкая | Изменения только в значении константы и в тестах. Сигнатуры/ветки не трогаются. `test-unit-backend-coverage` ловит, если что. |
| E2E `buildUnderageIIN` хуже работает с минимальным возрастом 18 (ИИН на 16 лет назад — нет валидной комбинации checksum) | Очень низкая | Логика поиска checksum брутфорсит digit, в исходной реализации panic если не нашёл — но с millions комбинаций сидов checksum всегда находится. Дополнительная проверка — прогон `make test-e2e-backend`. |
| Спека и код «разбегутся» — забыли обновить spec | Низкая | Шаг 7 — отдельный пункт. Спека `_bmad-output/implementation-artifacts/spec-creator-application-submit.md` упоминается явно. |
| Изменение «возраст менее N» в ApiError-сообщении сломает фронтенд-парсинг | Очень низкая | Лендинг показывает `serverMessage` verbatim, не парсит число. Комментарий в client.ts — только пояснительный, не использует значение. |
| Уйдёт незамеченное user-facing упоминание «21+» в каком-то ассете/строке | Низкая | Сделана сквозная проверка `grep -rn "21" frontend/landing/src/` — единственные user-facing вхождения это `content.ts:94,97`. Остальные «21» — даты, законы РК, ID Apple Store. После правки сделать финальный `grep -n "21+" frontend/landing/src/` для подтверждения нуля совпадений. |

## План отката

Изменения локальны и не оставляют следов в БД/инфре, поэтому откат тривиален.

- **До коммита:** `git checkout -- backend/internal/domain/iin.go backend/internal/domain/iin_test.go backend/e2e/creator_application/creator_application_test.go frontend/landing/src/content.ts frontend/landing/src/api/client.ts _bmad-output/implementation-artifacts/spec-creator-application-submit.md` (либо `git restore .`).
- **После мержа PR в main и деплоя:** `git revert <merge-commit>` + повторный деплой. БД не трогаем — `birth_date` уже хранится для каждой заявки, существующие записи валидны независимо от значения `MinCreatorAge`. Никаких миграций откатывать не нужно.
- **Hot-fix через env-флаг:** не предусмотрен — константа доменная, не конфиг. Если в будущем потребуется — выносить в Config (см. альтернативу в scout-артефакте), но это отдельная задача.
