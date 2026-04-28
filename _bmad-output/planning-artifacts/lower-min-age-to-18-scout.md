# Scout: понизить минимальный возраст подачи заявки креатора с 21 до 18

## Контекст задачи

Сейчас лендинг (форма заявки креатора) принимает только тех, кому 21+. Нужно вернуть порог к 18 — чтобы собирать заявки с более широкой аудитории. Текущая ветка `alikhan/creator-application-submit` уже замёржена в main, поэтому работа ведётся с новой ветки от свежего main.

История константы: изначально была 18 (FR3 в PRD), 2026-04-25 подняли до 21 как «бизнес-фильтр под EFW» (см. комментарий в `domain/iin.go:26-29` и спеку `_bmad-output/implementation-artifacts/spec-creator-application-submit.md:204`). Сейчас откатываем обратно к 18 — ровно то значение, которое держит PRD §FR3, FR/NRD §«Только 18+» и legal user-agreement (п. 6.3.4 «совершеннолетним и полностью дееспособным»).

**Перед стартом загрузить полный набор стандартов из `docs/standards/`** (hard rules, см. review-checklist.md) и сверить точечно: `backend-design.md` (domain/константы), `backend-testing-unit.md` (Coverage, перепиcка тестов под новое значение), `frontend-quality.md`, `naming.md`.

## Затронутые области

### Backend (Go) — главная константа

| # | Файл:строка | Текущее | Целевое |
|---|---|---|---|
| 1 | `backend/internal/domain/iin.go:29` | `const MinCreatorAge = 21` | `18` |
| 2 | `backend/internal/domain/iin.go:26-29` | Комментарий «Originally 18 per FR3; raised to 21 on 2026-04-25 as the EFW business filter…» | Перефразировать: убрать след о подъёме до 21, оставить указание на FR3. |

`service/creator_application.go:68-71` использует константу динамически (`fmt.Sprintf("Возраст менее %d лет", domain.MinCreatorAge)`) и `EnsureAdult()` — **изменений не нужно**, сообщение само станет «Возраст менее 18 лет».

### Backend — тесты, привязанные к значению 21

| # | Файл:строка | Что сделать |
|---|---|---|
| 3 | `backend/internal/domain/iin_test.go:133-146` (TestEnsureAdult) | `under by one day`: `birth=2005-04-21`, `now=2026-04-20` → возраст 20. После смены константы на 18 (20 ≥ 18) тест падает: ошибки больше нет. **Привязать даты к константе** (`time.Date(2005+MinCreatorAge, …)` и т.п.) либо перевыставить под 18 вручную. Аналогично сосед `exactly MinCreatorAge today`. |
| 4 | `backend/e2e/creator_application/creator_application_test.go:580` | `const minAge = 21` — поменять на `18`. Логика `birth = now - (minAge - 2)` остаётся валидной (16 лет назад < 18). |

`backend/internal/service/creator_application_test.go:140-153` (тест `under MinCreatorAge fails before tx`) **уже** строит `in.Now` через `1995+domain.MinCreatorAge` — менять не надо, при `MinCreatorAge=18` он продолжит ловить под-возраст (1995-05-15 vs 2013-05-14 = 17 лет).

`backend/internal/handler/creator_application_test.go` — упоминаний возраста по факту нет; в diff-tested файлах попадаются только числа `18, 21` в `time.Date(2026, 4, 20, 18, 0…)` (часы/дни), эти тесты не трогаем.

### Frontend (landing) — user-facing текст

| # | Файл:строка | Текущее | Целевое |
|---|---|---|---|
| 5 | `frontend/landing/src/content.ts:94` | `bold: "Тебе 21+"` | `"Тебе 18+"` |
| 6 | `frontend/landing/src/content.ts:97` | `{ bold: "Тебе 21+" }` (parts[0]) | `{ bold: "Тебе 18+" }` |
| 7 | `frontend/landing/src/api/client.ts:24-25` | Комментарий с примером `"Возраст менее 21 лет"` | Поменять пример на «менее 18 лет» (нужно для согласованности с фактическим серверным сообщением). |

`frontend/landing/src/pages/index.astro:443` — фраза «Нужен для проверки возраста и подписания договора» возраст не называет, **не трогаем**. Клиентской JS-валидации возраста нет (форма шлёт ИИН на сервер, ошибка `UNDER_AGE` приходит с message — рендерится через `ApiError.serverMessage`). E2E-Playwright тестов на форму в landing пока нет (`placeholder.test.ts` — заглушка, реальных spec-файлов в репо нет).

### Спека и документация (artifacts)

| # | Файл:строка | Что сделать |
|---|---|---|
| 8 | `_bmad-output/implementation-artifacts/spec-creator-application-submit.md:204` | Запись «MinCreatorAge поднят с 18 до 21. Бизнес-фильтр EFW…» — пометить «откатили обратно к 18 на 2026-04-28» либо переписать. |
| 9 | `_bmad-output/implementation-artifacts/spec-creator-application-submit.md:228` | `domain.MinCreatorAge = 21` → `18`. |

PRD `_bmad-output/planning-artifacts/prd.md` уже говорит «18+» (FR3, §«Только 18+ — проверка возраста по ИИН») — **трогать не нужно**, наоборот: возвращение к 18 синхронизирует код со спекой PRD.

Legal-документы (`legal-documents/*.md` + копии в `frontend/landing/src/pages/legal/`) говорят про «совершеннолетие» (≥18 РК) — никаких правок не требуется.

Backend OpenAPI-спека (`backend/api/openapi.yaml`) и SQL-миграции возраст не материализуют — **не трогаем**.

## Паттерны реализации

- **Константа в `domain/`.** `MinCreatorAge` живёт в `backend/internal/domain/iin.go` рядом с алгоритмом валидации ИИН и парой `EnsureAdult/AgeYearsOn`. Сообщения и e2e должны ссылаться на эту константу — ровно как уже делает service (`fmt.Sprintf("…%d…", domain.MinCreatorAge)`). Хардкод в e2e (`const minAge = 21`) — нарушение этого паттерна, его сейчас и фиксим.
- **Тесты `EnsureAdult`.** Текущие тесты используют фиксированные годы (`2005`, `2026`). Это сознательный выбор (дата гарантированно прошлая), но он завязан на конкретное значение `MinCreatorAge`. Лучшее решение — привязать `now`/`birth` к самой константе (`1995+MinCreatorAge` уже делают service-тесты), чтобы будущие изменения не требовали ручной правки таблиц.
- **Frontend.** UI текст — статика в `content.ts`, бизнес-валидация — на сервере. Меняем только тексты + комментарий-пример в `client.ts`.

## Риски и соображения

- **Coverage gate.** `make test-unit-backend-coverage` (≥80% per-method на handler/service/repo/middleware/authz) уже зелёный — изменение константы и пары дат не должно снизить покрытие, но проверить надо обязательно (один из обязательных гейтов перед PR).
- **Е2Е флакость.** `buildUnderageIIN()` использует `time.Now().UTC().AddDate(-(minAge-2)…)`. С `minAge=18` отрезаем 16 лет — этой подушки хватает (тест собирает заявку явно под-возрастного человека). Для `exactly`-edge кейсов в domain-тесте — даты уйдут на 2008-04-21 (или, если перепривязать к константе, рассчитаются автоматически).
- **PII в логах.** Менять константу — без работы с stdout-логами или audit_logs; рисков по `security.md` § PII нет.
- **Спека-living-doc.** Запись «поднят до 21» в spec-creator-application-submit.md — исторический контекст. Можно (а) удалить, (б) дописать «откатили обратно к 18 — собираем 18+». Предпочтительнее (б): при следующем изменении не потеряется контекст.
- **Нет breaking change для DB/API.** В БД лежат `birth_date` (а не возраст) — перепроверка существующих заявок не требуется, новые 18-20-летние просто перестанут получать `UNDER_AGE` 422.
- **Нет миграции.** Константа — чисто доменная, БД не задействует.
- **Согласованность сообщения.** После смены константы service вернёт «Возраст менее 18 лет» — пример в комментарии `client.ts` обновляем синхронно, чтобы будущий читатель не путался.
- **Edge cases по тестам.** Перед коммитом прогнать локально:
  - `cd backend && go test ./internal/domain/... ./internal/service/... -count=1 -race`
  - `make test-unit-backend-coverage`
  - `make test-e2e-backend` (включает `creator_application` пакет, проверит `under MinCreatorAge` сценарий end-to-end)
  - `make build-landing` + `cd frontend/landing && npx tsc --noEmit && npx eslint src/`

## Рекомендуемый подход

1. Создать ветку от свежего `main`: `git fetch origin && git checkout -b alikhan/lower-creator-min-age main`.
2. **Backend, доменное ядро:** `iin.go` — `MinCreatorAge = 18` + переписать комментарий (без следа про 21/EFW).
3. **Backend, доменные тесты:** `iin_test.go:133-146` — пересчитать (или, лучше, привязать) даты `under by one day` / `exactly MinCreatorAge today` к новому значению.
4. **Backend, e2e тест:** `creator_application_test.go:580` — `const minAge = 18`.
5. **Frontend:** `content.ts:94,97` — «Тебе 18+»; `client.ts:24-25` — пример в комментарии.
6. **Спека:** `spec-creator-application-submit.md:204,228` — отметить откат к 18 и обновить значение константы.
7. Локальные гейты: `make lint-backend`, `make test-unit-backend`, `make test-unit-backend-coverage`, `make test-e2e-backend`, `make lint-landing`, `make build-landing`. Сторонним фронтам (`web`, `tma`) изменения не касаются — отдельные команды можно не запускать.
8. PR с фокусом «откат MinCreatorAge с 21 до 18 — возвращаем PRD-FR3 поведение, расширяем воронку заявок». Перед открытием — `/review` если хочется multi-agent ревью.

**Альтернатива (overkill для текущего MVP, не рекомендую):** вынести `MinCreatorAge` в конфиг (env / DB-config) — позволит менять без релиза. Не надо: сейчас задача одна, конфиг-флаг = новые точки отказа и тесты. Если в будущем потребуется крутить порог часто (>2 раз) — тогда вернёмся.
