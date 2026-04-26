# План реализации: UX-фиксы лендинг-формы и nullable address

## Перед началом работы (для агента-исполнителя)

Этот план рассчитан на исполнение в новой сессии (например, через `/build`). Прежде чем трогать любой файл — **полностью прочитай в контекст все стандарты проекта** из `docs/standards/`. Не выборочно, не по grep, не «только релевантные» — все файлы целиком, в исходном виде. На момент написания плана это:

- `backend-architecture.md`, `backend-codegen.md`, `backend-constants.md`, `backend-design.md`, `backend-errors.md`, `backend-libraries.md`, `backend-repository.md`, `backend-testing-e2e.md`, `backend-testing-unit.md`, `backend-transactions.md`
- `frontend-api.md`, `frontend-components.md`, `frontend-quality.md`, `frontend-state.md`, `frontend-testing-e2e.md`, `frontend-testing-unit.md`, `frontend-types.md`
- `naming.md`, `security.md`

Если в `docs/standards/` появились новые файлы после написания плана — их тоже. Прочитай папку директорийным листингом и убедись, что взял весь набор. Стандарты — единственный источник истины по конвенциям проекта; без полной загрузки нечем проверять корректность изменений.

Также прочитай `CLAUDE.md` в корне репо (project instructions) — там дополнительные правила работы с этим проектом.

## Обзор

Исправляем подачу заявки на лендинге одним PR: убираем рассинхрон по полю `address` через все слои (миграция → domain → service → openapi → frontend → unit/e2e), добавляем клиентские маски и валидацию (телефон, IIN, никнеймы соцсетей), блокируем submit пока словари не загрузились или упали, добавляем e2e smoke-тест на загрузку словарей.

## Требования

### Must-have

- **REQ-1.** `address` становится nullable в БД и опциональным в API. Старые записи с `address = city` остаются как есть.
- **REQ-2.** `domain.CreatorApplicationInput.Address` и `domain.CreatorApplicationDetail.Address` — `*string`. Сервисная валидация «address не пустой» удалена.
- **REQ-3.** `openapi.yaml` помечает `address` как опциональный (нет в `required`, без `minLength`). Сгенерированные клиенты пересобраны через `make generate-api`.
- **REQ-4.** Лендинг **не отправляет** поле `address` в payload (или отправляет `null` — определяется тем, что разрешает сгенерированная типизация).
- **REQ-5.** Поле `phone` имеет input-маску `+7 (XXX) XXX-XX-XX`; перед отправкой нормализуется в `+7XXXXXXXXXX` (10 цифр после +7). Префикс `+7` фиксированный, переписать его пользователь не может.
- **REQ-6.** Поле `iin` принимает только цифры (12 ровно). Реализация: `pattern="[0-9]{12}"` + on-input filter, который вырезает не-цифры.
- **REQ-7.** Никнейм соцсети нормализуется на blur и перед submit: trim, ведущий `@` удалён, URL-префиксы `https?://(www\.)?(instagram|tiktok|youtube)\.com/` удалены.
- **REQ-8.** Пока `bootDictionaries` не завершилась, кнопка submit `disabled`, label «Загрузка…». При ошибке `bootDictionaries` submit остаётся `disabled` навсегда, текст ошибки сообщает о необходимости перезагрузить страницу.
- **REQ-9.** Защита от двойного submit: внешний флаг `submitting`. Хендлер делает ранний `return`, если флаг `true`.
- **REQ-10.** Перед submit проверяется `socials.length > 0`; при нуле — form-error с понятным текстом, реквест не уходит.
- **REQ-11.** Новый e2e-тест в `frontend/e2e/landing/submit.spec.ts`: smoke на отрисовку словарей (`city-select` имеет >1 option, `.category-checkbox` рендерится >0 раз).
- **REQ-12.** Хелпер `waitForDictionaries` ослаблен — больше не зависит от наличия `category-checkbox-beauty`. Использует «есть хотя бы один категориальный чекбокс».
- **REQ-13.** Все юнит и e2e тесты бэкенда обновлены под опциональный address (там где он передавался — либо убран, либо помечен как опциональный сценарий).

### Nice-to-have

Не входит в этот PR — отдельные задачи в backlog:

- Field-level error display (показывать ошибку под конкретным полем, а не общим блоком сверху).
- Сбор адреса в админке/боте после одобрения заявки.
- Кириллица-онли в ФИО (отвергнуто: ФИО может быть на казахском, должно совпадать с документами).
- Очистка существующих `address = city` строк в БД.

### Вне скоупа

- Миграция данных `address` (тестовые данные на staging).
- Изменения в TMA или web-кабинете.
- Любые product-changes по UX лендинга кроме перечисленных REQ.

### Критерии успеха

- `make lint-backend lint-web lint-tma lint-landing` зелёные.
- `make test-unit-backend test-unit-web test-unit-tma test-unit-landing` зелёные.
- `make test-e2e-backend test-e2e-frontend test-e2e-landing` зелёные локально.
- CI проходит по всем стадиям, включая Staging E2E Landing.
- На staging форма принимает реальную заявку: маска телефона работает (нельзя ввести буквы / нельзя удалить +7), IIN не принимает буквы, никнейм с `@` сохраняется без `@`, без выбранной соцсети submit показывает ошибку.
- В БД: колонка `creator_applications.address` — `NULL`-able. Новые заявки с лендинга — `address = NULL`.

## Файлы для изменения

| Файл | Изменения |
|------|-----------|
| `backend/api/openapi.yaml` | Убрать `address` из `required` у `CreatorApplicationSubmitRequest` и у одного-двух response-схем где он присутствует; убрать `minLength: 1`. После — `make generate-api`. |
| `backend/internal/domain/creator_application.go` | `Address string` → `*string` в `CreatorApplicationInput` (стр. 105) и `CreatorApplicationDetail` (стр. 157). |
| `backend/internal/service/creator_application.go` | Убрать ветку `case out.Address == "": return out, missing("address")` (стр. 216–217). Адаптировать места где `out.Address` или `app.Address` присваиваются — теперь это `*string`. |
| `backend/internal/handler/creator_application.go` | Конвертация request→domain и domain→response для `Address` через указатель. |
| `backend/internal/repository/creator_application.go` | Поле `Address string` (стр. 54) → `*string` (DB-маппинг pgx сам справится с NULL→nil). Если есть SQL-запросы с явным `address` — оставить как есть, БД примет NULL. |
| `backend/internal/service/creator_application_test.go` | В тестовых fixtures address → указатель или nil. Добавить t.Run-сценарий «address отсутствует — успех». |
| `backend/internal/handler/creator_application_test.go` | Тоже самое: указатели в fixtures, плюс отсутствие `address` в payload. |
| `backend/e2e/creator_application/creator_application_test.go` | Убрать `Address: "ул. Абая 1"` из payload'ов; адаптировать ожидаемые response-структуры под `*string`. |
| `frontend/landing/src/api/creator-applications.ts` | После `make generate-api` тип `CreatorApplicationSubmitRequest` обновится; убедиться что landing не строит payload с `address`. |
| `frontend/landing/src/pages/index.astro` | (1) убрать строку 813 `address: String(fd.get("city") ?? "").trim()` и связанный комментарий; (2) маска телефона — добавить input-listener, нормализатор перед сабмитом; (3) IIN on-input filter + `pattern`; (4) social handle normalize в `collectFormData`; (5) wire submit-lock на bootDictionaries; (6) флаг `submitting`; (7) проверка `socials.length > 0` перед submit. |
| `frontend/e2e/landing/submit.spec.ts` | Ослабить `waitForDictionaries` (счёт опций вместо проверки `category-checkbox-beauty`). Добавить smoke-тест в начале describe-блока. |

## Файлы для создания

| Файл | Назначение |
|------|------------|
| `backend/migrations/<ts>_creator_applications_address_nullable.sql` | `ALTER TABLE creator_applications ALTER COLUMN address DROP NOT NULL`. Down-миграция в `-- +goose Down` пытается выставить NOT NULL обратно — `UPDATE ... SET address = ''` для NULL-строк, потом `ALTER ... SET NOT NULL`. |

## Шаги реализации

### Этап 1 — Бэкенд (миграция + domain + сервис + контракт)

1. [ ] Создать миграцию: `make migrate-create NAME=creator_applications_address_nullable`. Заполнить Up/Down.
2. [ ] Применить локально: `make migrate-up`. Убедиться, что схема обновлена (`\d creator_applications` в psql или интроспекция через тест).
3. [ ] Поправить `domain/creator_application.go`: `Address` → `*string` в `CreatorApplicationInput` и `CreatorApplicationDetail`.
4. [ ] Поправить `service/creator_application.go`: убрать валидацию пустого address, обновить присвоения.
5. [ ] Поправить `repository/creator_application.go`: `Address *string` в Row struct, проверить что SQL-запросы корректно работают с NULL (pgx маппит NULL в `nil` указатель).
6. [ ] Поправить `handler/creator_application.go`: конвертация поля в обе стороны (request→domain, domain→response).
7. [ ] `backend/api/openapi.yaml`: убрать `address` из `required` массивов, убрать `minLength: 1`.
8. [ ] `make generate-api` — пересобрать `backend/internal/api/server.gen.go`, e2e клиенты, и openapi schemas во всех трёх фронтах.
9. [ ] Прогнать `make build-backend` — должен скомпилироваться.

### Этап 2 — Тесты бэкенда

10. [ ] Поправить `service/creator_application_test.go`: указатели в fixtures, добавить сценарий «`Address == nil` — успех».
11. [ ] Поправить `handler/creator_application_test.go`: указатели в fixtures, добавить сценарий submit без поля address в JSON-теле.
12. [ ] Поправить `e2e/creator_application/creator_application_test.go`: убрать `Address: "ул. Абая 1"` из всех payload'ов, адаптировать ожидания (`got.Address` теперь `*string`).
13. [ ] Прогнать `make test-unit-backend` — зелёные.
14. [ ] Прогнать `make test-e2e-backend` — зелёные.
15. [ ] Прогнать `make lint-backend test-unit-backend-coverage` — без issues, покрытие в `service/handler/repository/middleware/authz` ≥ 80% по каждому методу.

### Этап 3 — Лендинг (UX-фиксы)

16. [ ] `index.astro:803-818`: убрать `address` из payload, удалить связанный комментарий.
17. [ ] Маска телефона: написать функцию `formatPhoneInput(value: string)` → возвращает `+7 (XXX) XXX-XX-XX`. Прицепить input-listener, на `keydown`/`paste` блокировать удаление префикса `+7`. Перед submit — нормализатор `normalizePhoneForSubmit(value)` → `+7XXXXXXXXXX`.
18. [ ] IIN: input-listener убирает не-цифры; добавить `pattern="[0-9]{12}"` в HTML.
19. [ ] Social handle: функция `normalizeSocialHandle(raw: string, platform: string)` — strip whitespace + ведущий `@` + URL-prefix per platform. Применить в `collectFormData` к каждому handle.
20. [ ] Submit-lock: в `bootDictionaries` до старта — `submitBtn.disabled = true; submitBtn.textContent = "Загрузка..."`. После успеха — снять disabled и вернуть оригинальный текст. После ошибки — оставить disabled, обновить error message.
21. [ ] Double-submit: внешняя `let submitting = false`. В начале handler — если `submitting`, `return` без действий. Установить `true` сразу после `clearFormError()`, обнулять в `finally`.
22. [ ] Проверка соцсетей: в начале try-блока внутри handler — если `payload.socials.length === 0`, `showFormError("Укажи хотя бы одну соцсеть")` и `return`.
23. [ ] Локальный smoke: `make build-landing && make lint-landing` — зелёные.

### Этап 4 — E2E лендинга

24. [ ] `frontend/e2e/landing/submit.spec.ts`:
   - Переписать `waitForDictionaries`: ждать `expect.poll(() => citySelect.locator('option').count() > 1)` и `expect.poll(() => container.locator('.category-checkbox').count() > 0)` с timeout 5–10s.
   - Добавить новый `test("dictionaries загружены — селекты не пустые", ...)` в начале describe.
   - Адаптировать happy path к маске телефона (заполнять `+7 (700) 123-45-67`, проверять что value стало `+77001234567` в network request — или просто продолжать заполнять без скобок если маска нормализует).
   - Адаптировать к новому требованию `socials.length > 0` (тест уже выбирает Instagram — проходит).
25. [ ] Прогнать `make test-e2e-landing` локально — все 4 теста зелёные.

### Этап 5 — Финал

26. [ ] `make lint-backend lint-web lint-tma lint-landing` — все зелёные.
27. [ ] `make test-unit-backend test-unit-landing test-e2e-backend test-e2e-landing` — все зелёные.
28. [ ] Оставить изменения в working tree, доложить результат, **не коммитить** до явного approval Alikhan.
29. [ ] После approval — коммит + пуш одной серией коммитов (либо одним large commit, либо логическими частями: «backend: address nullable», «landing: form UX fixes», «e2e: dictionary smoke»).
30. [ ] Дождаться CI, посмотреть Staging E2E Landing — должны пройти.

## Стратегия валидации

Каждый этап заканчивается прогоном линта/тестов. Финальная валидация — снизу вверх:

| Уровень | Что проверяет | Чем проверяется |
|---|---|---|
| Schema | DB-колонка nullable, нет NOT NULL violations | `make migrate-up` + ручная проверка в psql или e2e тест с `address = nil` |
| Domain/Service | Бизнес-логика: пустой address принимается, не валится | `make test-unit-backend` (новые сценарии) |
| Contract | OpenAPI совпадает с реальным поведением | `make test-e2e-backend` (e2e использует сгенерированный клиент) |
| Frontend logic | Маски и валидаторы работают на пустых, валидных и невалидных входах | `make test-unit-landing` (если есть юнит-тесты на normalize-функции — иначе только через e2e) |
| Integration | Лендинг → бэк end-to-end, словари грузятся, submit проходит | `make test-e2e-landing` локально |
| Production-like | Лендинг работает за CF Access на staging | CI: Staging E2E Landing |

## Оценка рисков

| Риск | Вероятность | Митигация |
|------|-------------|-----------|
| Миграция Down не сработает на проде, если кто-то накатит NULL'ы | Низкая (staging-only сейчас) | В Down — `UPDATE ... SET address = '' WHERE address IS NULL` перед `SET NOT NULL`. Документировать что Down не восстанавливает данные. |
| Existing e2e тесты ломаются массово после `address` → `*string` (компиляция Go упадёт сразу) | Высокая | Делается за раз: domain + handler + e2e тесты + service тесты в одном этапе. Сборка на каждом шаге. |
| Маска телефона ломает paste из реальных номеров (`8 7771234567`, `+7-700-123-45-67`) | Средняя | Нормализатор сначала вырезает все не-цифры, оставляет последние 10 цифр после `+7` (или преобразует ведущую `8` в `7`). На вводе — символы вне `[0-9 ()+-]` блокируются. Тест на paste обязателен в e2e. |
| Pattern на IIN может затруднить вставку с пробелами | Низкая | On-input listener сначала вырезает не-цифры, потом `pattern` валидирует. Браузер не блокирует ввод по pattern, только submit — поэтому pattern сам по себе не мешает. |
| Лендинг с `credentials: include` теперь шлёт CF cookie — на проде CF Access не пропустит анонимных юзеров | Низкая, не в скоупе | В REQ заявлено: на проде нужен bypass. Открытая задача в backlog (см. предыдущий разговор). Этот PR — стабилизация staging. |
| Сгенерированные openapi-схемы не пересобраны во фронтах | Средняя | Шаг 8 явно требует `make generate-api` и проверки git-diff'а в `frontend/{web,tma,landing}/src/api/generated/schema.ts`. |

## План отката

Если на staging что-то сломается после деплоя:

1. **Откат CD только**: `git revert <merge-commit>` + push → CI пересоберёт и задеплоит предыдущий стейт. Ничего не нужно делать с БД (миграция оставила колонку nullable — это совместимо с предыдущим кодом, который писал туда городы).
2. **Если и БД нужно вернуть**: `goose down 1` через Dokploy migrations job — вернёт NOT NULL. Перед этим: `UPDATE creator_applications SET address = '' WHERE address IS NULL;` (иначе `SET NOT NULL` упадёт на новых записях с лендинга).
3. Локальный откат во время разработки — стандартный `git reset --hard origin/<branch>` и `make migrate-reset`.
