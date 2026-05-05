---
title: "GET creator-aggregate + полное E2E через fixture-helper (chunk 18c)"
type: feature
created: "2026-05-05"
status: in-progress
baseline_commit: "467c318"
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/creator-onboarding-roadmap.md
  - _bmad-output/implementation-artifacts/spec-creator-foundation.md (18a)
  - _bmad-output/implementation-artifacts/spec-creator-approve-action.md (18b)
  - _bmad-output/implementation-artifacts/spec-creator-application-approve.md (старая большая спека сохранена как референс — удаляется после merge 18c)
---

> Перед реализацией обязательно полностью загрузить все файлы `docs/standards/` — это hard rules, спека не дублирует.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** После 18b creator пишется в БД через approve, но достать его наружу нельзя — нет admin-endpoint'а для чтения. E2E happy-test 18b проверял только факт записи через прямые SQL-чтения (workaround), но не валидировал API-форму данных. Фронту нужен один запрос, чтобы получить полный профиль креатора со всеми вложенными секциями.

**Approach:** Один admin-endpoint `GET /creators/{id}` возвращает `CreatorAggregate` — большой объект со всеми полями креатора, плоско-вложенным Telegram-блоком, массивами соцсетей с verified-полями и категорий с локализованными именами через словарь. `CreatorService.GetByID` собирает агрегат из 3 repo-вызовов + dictionary hydration по образцу `CreatorApplicationService.GetByID`. На E2E-стороне вводим переиспользуемый fixture-pipeline (`CreatorApplicationFixture` + `SetupCreatorApplicationInModeration` + `AssertCreatorAggregateMatchesSetup`) — он расширяет существующий 18b `approve_test.go` на полную structural-проверку через GET-handler + питает новый `creators/get_test.go`.

## Decisions

### `CreatorAggregate` — большой плоский объект

Один большой response-объект собирает всё, что нужно фронту для отображения профиля. Без вложенных endpoint'ов (`/creators/{id}/socials`, `/creators/{id}/categories`) — это лишние round-trip'ы для UI.

Поля:
- **Identity**: `id`, `iin`, `sourceApplicationId`.
- **PII** (плоско): `lastName`, `firstName`, `middleName?`, `birthDate`, `phone`, `address?`, `categoryOtherText?`.
- **City**: `cityCode` + `cityName` (hydrated через `DictionaryRepo`).
- **Telegram-блок** (плоский): `telegramUserId`, `telegramUsername?`, `telegramFirstName?`, `telegramLastName?`.
- **Socials**: массив объектов `{platform, handle, verified, method?, verifiedByUserId?, verifiedAt?, id, createdAt}`. Порядок детерминированный — `(platform, handle)` из repo (см. 18a).
- **Categories**: массив объектов `{code, name}` (name hydrated через словарь). Порядок — по `code` (18a гарантирует repo).
- **Timestamps**: `createdAt`, `updatedAt`.

`category_other_text` лежит на креаторе плоско, не в массиве категорий — копируется из заявки 1-в-1.

### Authz — admin-only сейчас

`CanViewCreator(ctx)` admin-only, шаблон зеркало `CanViewCreatorApplication`. Расширение для brand_manager'а (когда появится campaign-flow и бренды будут смотреть на каталог креаторов) — отдельным чанком.

### Fixture-pipeline в testutil — переиспользуемый между approve_test и get_test

Чтобы избежать дублирования diff-логики структурного сравнения между двумя тестовыми файлами, вводим helper в `testutil`:

- `CreatorApplicationFixture` — struct с контролируемыми входными данными (см. § Always).
- `SetupCreatorApplicationInModeration(t, in CreatorApplicationFixture) CreatorApplicationFixture` — оборачивает submit → link → auto-/manual-verify заявку до `moderation`. Возвращает обогащённую fixture с `applicationID`, `chatID`, `verifiedByAdminID` для соответствующих соцсетей. Регистрирует cleanup.
- `AssertCreatorAggregateMatchesSetup(t, fx CreatorApplicationFixture, creatorID string, aggregate apiclient.CreatorAggregate)` — two-stage assertion:
  - Stage 1 — динамические поля: `aggregate.Id == creatorID`, `aggregate.SourceApplicationId == fx.ApplicationID`, `aggregate.CreatedAt`/`UpdatedAt` через `WithinDuration`. Каждый `socials[i].Id`, `socials[i].VerifiedAt` (где `verified=true`) — `NotEmpty` + `WithinDuration`.
  - Stage 2 — substitute проверенных dynamic-полей в `actual` на ожидаемые из fixture, потом `require.Equal(expected, actual)` целиком. Сортирует ожидаемые `socials` по `(platform, handle)` и `categories` по `code` перед сборкой `expected`.

Это даёт **одну точку истины** для проверки агрегата. Будущее расширение `CreatorAggregate` (новые поля) обновляет один helper, не два теста.

### `approve_test.go` (18b) расширяется — happy_full + happy_sparse

В 18b happy-test проверял только факт записи через успешный `DeleteCreatorForTests` в cleanup'е (без structural-сверки). В 18c он переписывается:

- **Добавляется** structural-проверка через `GET /creators/{creatorId}` + `AssertCreatorAggregateMatchesSetup`. Прежний минимум (200 + status=approved + WaitForTelegramSent + успешный cleanup) сохраняется как часть happy_full.
- **Раздваивается** на `happy_full` (заполненная заявка с middle_name + address + category_other_text + 3 соцсети по разным verification-веткам) и `happy_sparse` (все nullable=nil, 1 соцсеть). Обе через тот же helper — sparse защищает omitempty/null-семантику openapi.

Локальный `seedApprovableApplication` из 18b удаляется — вместо него `SetupCreatorApplicationInModeration` из testutil. Cleanup-стек продолжает использовать `DeleteCreatorForTests` (уже введён в 18b).

### Cleanup ordering — creator до application

`creators.source_application_id REFERENCES creator_applications(id)` без ON DELETE (default RESTRICT). Cleanup-stack должен сначала удалить creator (ON DELETE CASCADE на socials/categories срабатывает), потом заявку. LIFO в `testutil.Cleanup`: регистрировать `DeleteCreatorForTests` ПОСЛЕ `DeleteCreatorApplicationForTests` (LIFO выполнит creator первым).

`CleanupEntityRequest.type=creator` enum + handler-case в testapi + `DeleteCreatorForTests` testutil-helper — **уже введены в 18b**. В 18c эти примитивы только переиспользуются — новых правок testapi/testutil-cleanup не нужно.

## Boundaries & Constraints

**Always:**

- OpenAPI: `GET /creators/{id}`, security `bearerAuth`, responses 200 / 401 / 403 / 404 / default. Path `id` — uuid. Response 200 — schema `CreatorAggregate`.
- Новый ErrorCode: `CREATOR_NOT_FOUND` (404). Один.
- `CreatorAggregate` schema (см. § Decisions / Поля). Все nullable-поля помечены `nullable: true`. Массивы socials/categories — required (могут быть пустыми, но не отсутствовать в response).
- Authz: `AuthzService.CanViewCreator(ctx)` admin-only, шаблон зеркало `CanViewCreatorApplication`.
- Domain: `domain.CreatorAggregate` — внутренняя структура, плоский объект (не плодить mapping слой). `ErrCreatorNotFound` sentinel + `CodeCreatorNotFound = "CREATOR_NOT_FOUND"` + actionable user-facing message «Креатор не найден».
- Service: `CreatorService` (новый файл `service/creator.go`). Структура: `pool dbutil.Pool`, `repoFactory CreatorRepoFactory`, `logger logger.Logger`. Конструктор `NewCreatorService(pool, repoFactory, log)`. Метод `(s *CreatorService) GetByID(ctx, creatorID) (*domain.CreatorAggregate, error)`:
  1. `creatorRepo.GetByID(s.pool, creatorID)` → `sql.ErrNoRows` ⇒ `ErrCreatorNotFound`.
  2. `socialsRepo.ListByCreatorID(s.pool, creatorID)` (возвращает уже отсортированный по `(platform, handle)` слайс — гарантия 18a).
  3. `categoriesRepo.ListByCreatorID(s.pool, creatorID)` (возвращает `[]string` codes по `category_code ASC` — гарантия 18a).
  4. `dictRepo.ListCitiesByCode([]{creator.CityCode})` + `dictRepo.ListCategoriesByCode(categoryCodes)` — hydration. Деактивированные коды (active=false в словаре) обрабатываются как «name не найден» — fallback на `code`. Зеркало того, как `CreatorApplicationService.GetByID` хидрейтит словари (`creator_application.go:528`).
  5. Композиция → `*CreatorAggregate`.
- Service-side `CreatorRepoFactory` интерфейс в `service/creator.go` — нужны конструкторы `NewCreatorRepo` / `NewCreatorSocialRepo` / `NewCreatorCategoryRepo` / `NewDictionaryRepo`. Не плодим — переиспользуем структуру факторий из 18b (`CreatorApplicationRepoFactory` уже расширен в 18b, но `CreatorService` объявляет свой минимальный интерфейс с теми же 4 методами; зеркало паттерна из стандарта `backend-architecture.md` § RepoFactory).
- Handler: `GetCreator` (по openapi `operationId`). Маппинг `ErrCreatorNotFound` в 404 в `handler/response.go`.
- `handler/creator.go` (новый файл) — handler. Path uuid через ServerInterfaceWrapper. Маппинг domain `CreatorAggregate` → openapi response.
- `wire`-up `CreatorService` в `cmd/api/main.go` (или где сейчас собираются сервисы — там же где `CreatorApplicationService`).
- `testutil`:
  - `CreatorApplicationFixture` struct в `testutil/creator_application.go`. Поля: `IIN string`, `LastName / FirstName string`, `MiddleName *string`, `BirthDate time.Time`, `Phone string`, `CityCode string`, `Address *string`, `CategoryCodes []string`, `CategoryOtherText *string`, `Socials []SocialFixture`. `SocialFixture`: `Platform string` (`instagram` / `tiktok` / `threads`), `Handle string`, `Verification string` (`auto-ig` / `manual` / `none`).
  - На выходе fixture обогащается: `ApplicationID`, `TelegramUserID`, `TelegramUsername / FirstName / LastName *string`, для каждой social — `VerifiedByAdminID *string` + `VerifiedAt *time.Time` (выставляется setup-функцией).
  - `SetupCreatorApplicationInModeration(t *testing.T, in CreatorApplicationFixture) CreatorApplicationFixture`:
    - `SubmitCreatorApplication` → application created, status=`verification`.
    - `LinkTelegramToApplication` → telegram bound (genrate fresh telegram_user_id через `crypto/rand`-helper).
    - Для каждой social с `Verification=auto-ig` (только для IG): дёрнуть SendPulse webhook через testapi → social verified, application transitions to `moderation` (если ещё не).
    - Для каждой social с `Verification=manual`: `manualVerifyApplicationSocial` через admin Bearer → social verified, transitions to `moderation` (если ещё не).
    - Если ни один social не verified → setup ошибается с `t.Fatal` (заявка не дойдёт до `moderation`).
    - Регистрирует cleanup стек.
  - `AssertCreatorAggregateMatchesSetup(t *testing.T, fx CreatorApplicationFixture, creatorID string, aggregate apiclient.CreatorAggregate)` — two-stage (см. § Decisions).
- `DeleteCreatorForTests` testutil-helper, `CleanupEntityRequest.type=creator` enum, и handler-case в testapi — **переиспользуются из 18b** без правок.

**Ask First (BLOCKING до Execute):**
- (нет — все вопросы зарезолвлены)

**Never:**
- Mutate-методы на `CreatorService` (UPDATE / DELETE / CREATE — Create уже в `ApproveApplication`, других сценариев пока нет).
- Wider authz для GET /creators/{id} (admin-only).
- Inline-вложенные endpoint'ы вроде `/creators/{id}/socials` — большой агрегат закрывает все потребности фронта одним запросом.
- Pagination на массивах socials / categories — у одного креатора их максимум единицы, не сотни.
- Кеширование в service / repo (если потребуется — отдельным чанком).
- Search / filter / list endpoint'ы для креаторов (когда понадобится для админки списка креаторов — отдельный чанк).
- Расширение fixture для нет-IG / threads-only кейсов approve (Threads сам по себе нельзя auto-verify, но manual через chunk 10 — можно).
- Менять existing 7 негативных тестов из 18b approve_test.go — они остаются (только happy переписывается на helper + structural-сверка).

## I/O & Edge-Case Matrix

| Сценарий | Состояние | Поведение |
|---|---|---|
| GET creator happy | creator существует | 200 + полный CreatorAggregate (identity / PII / Telegram / socials / categories с локализованными именами) |
| GET creator не существует | random UUID | 404 `CREATOR_NOT_FOUND` |
| GET creator non-admin | brand_manager Bearer | 403 `FORBIDDEN` (до DB-вызова) |
| GET creator unauthenticated | без Bearer | 401 |
| Approve happy_full (расширение 18b) | заявка с заполненными nullable + 3 соцсетями (IG auto / TT manual / Threads non-verified) | 200 + `creatorId` + GET aggregate проходит `AssertCreatorAggregateMatchesSetup` |
| Approve happy_sparse (расширение 18b) | заявка с `middleName=nil`, `address=nil`, `categoryOtherText=nil`, 1 IG-соцсеть | 200 + `creatorId` + agregate проходит helper с null'ами в nullable |
| Деактивированный city_code в словаре | creator с city_code, который был active в момент approve, но потом деактивирован в `cities.active=false` | 200 + agregate с `cityCode` сохраняется + `cityName` либо имя из словаря (если row остался видимым), либо fallback на `code` (если код больше не виден active-only фильтру). Поведение зеркалит `CreatorApplicationService.GetByID`. |
| Деактивированная категория | то же что выше для category_code | то же — code сохраняется, name fallback |

## Local Smoke Acceptance

Автор PR обязан **лично** прогнать после реализации:

1. `make compose-up && make migrate-up && make start-backend`.
2. Через curl — full pipeline: submit → /start link → manual-verify → approve → запомнить `creatorId` из ответа.
3. ✅ `curl -H "Authorization: Bearer $ADMIN_TOKEN" http://localhost:8082/creators/<creatorId>` → 200 JSON. Глазами проверить что:
   - identity (id / iin / sourceApplicationId) совпадают с ожидаемыми.
   - PII-поля 1-в-1 с тем что подавалось при submit (ФИО, birth_date, phone, address, city_code).
   - `cityName` присутствует и совпадает с именем города в словаре.
   - Telegram-блок (`telegramUserId` + 3 nullable метаданных) — корректные значения.
   - Массив `socials` — все добавленные в заявке, `verified` / `method` / `verifiedByUserId` / `verifiedAt` корректны для каждой соцсети.
   - Массив `categories` — все коды + локализованные имена.
4. ✅ `GET /creators/<random-uuid>` → 404 `CREATOR_NOT_FOUND` + actionable message.
5. ✅ `GET /creators/<creatorId>` без Bearer → 401.
6. ✅ `GET /creators/<creatorId>` с brand_manager Bearer → 403.
7. ✅ Прогон полного e2e: `make test-e2e-backend` — `approve_test.go` с happy_full + happy_sparse через helper и `creators/get_test.go` зелёные.

После 7/7 — финал. PR готов к review.

## Code Map

> Baseline — TBD (фиксируется после merge PR 18b).

- `backend/api/openapi.yaml` —
  - Новый path `GET /creators/{id}`.
  - Новая schema `CreatorAggregate` (см. § Decisions).
  - Новая `CreatorAggregateSocial` schema (вложенная).
  - Новая `CreatorAggregateCategory` schema (вложенная).
  - Новый ErrorCode `CREATOR_NOT_FOUND`.
  - После — `make generate-api`.
- `backend/internal/domain/creator.go` —
  - Patch: domain types `CreatorAggregate`, `CreatorAggregateSocial`, `CreatorAggregateCategory` (если они нужны как отдельные структуры, или плоский `Creator` из 18b расширяется + слайсы socials/categories привязаны).
  - Patch: sentinel `ErrCreatorNotFound` + `CodeCreatorNotFound = "CREATOR_NOT_FOUND"` + actionable user-facing message.
- `backend/internal/authz/creator.go` — новый файл: `CanViewCreator(ctx)` admin-only.
- `backend/internal/authz/creator_test.go` — новый: 3 `t.Run` (admin / brand_manager / no-role).
- `backend/internal/service/creator.go` — новый: `CreatorService` + `CreatorRepoFactory` интерфейс + `NewCreatorService` + `GetByID` (3 repo-вызова + dictionary hydration).
- `backend/internal/service/creator_test.go` — новый: `TestCreatorService_GetByID` (см. § Test Plan).
- `backend/internal/handler/creator.go` — новый: `GetCreator` handler. Маппинг domain `CreatorAggregate` → openapi response.
- `backend/internal/handler/creator_test.go` — новый: `TestCreatorHandler_GetCreator` (см. § Test Plan).
- `backend/internal/handler/response.go` — patch: case `ErrCreatorNotFound` → 404 + actionable message.
- `backend/cmd/api/main.go` — patch: wire `CreatorService` + `creatorHandler` + регистрация в роутере.
- `backend/e2e/testutil/creator_application.go` — patch: `CreatorApplicationFixture` struct + `SocialFixture` struct + `SetupCreatorApplicationInModeration(t, in) CreatorApplicationFixture`. Использует существующие `LinkTelegramToApplication`, `submitCreatorApplication`, `manualVerifyApplicationSocial` (или их analogs если они в `e2e/creator_applications/*_test.go`).
- `backend/e2e/testutil/creator.go` — patch (файл создан в 18b с `DeleteCreatorForTests`): добавляется `AssertCreatorAggregateMatchesSetup(t, fx, creatorID, aggregate)` — two-stage helper.
- `backend/e2e/creator_applications/approve_test.go` — patch (расширение 18b):
  - Удалить локальный `seedApprovableApplication`.
  - Удалить SQL-lookup в happy-сценарии.
  - Заменить happy на `happy_full` + `happy_sparse`, оба через `SetupCreatorApplicationInModeration` + `AssertCreatorAggregateMatchesSetup`.
  - 7 негативных + race-сценарий из 18b остаются без изменений (только пере-плагать setup'ы на новый helper).
- `backend/e2e/creators/get_test.go` — новый файл: `TestGetCreator` с 4 `t.Run` (happy через тот же helper / not_found / forbidden / unauthenticated).
- `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` — chunk 18.6 (aggregate read) → `[~]` старт, `[x]` merge.
- `_bmad-output/implementation-artifacts/spec-creator-application-approve.md` — **удаляется** после merge 18c (старая спека-референс отслужила).

## Tasks & Acceptance

**Pre-execution gates:**
- [ ] PR 18b (approve action) смержен в main; baseline_commit зафиксирован.
- [ ] `make test-unit-backend && make test-e2e-backend` зелёные на baseline.

**Execution:**
- [ ] OpenAPI: 1 path + 3 новые schema + 1 ErrorCode. `make generate-api`.
- [ ] Domain: types для агрегата + ErrCreatorNotFound + code + message.
- [ ] Authz: CanViewCreator + unit-тесты.
- [ ] Service: CreatorService.GetByID + dictionary hydration + unit-тесты.
- [ ] Handler: GetCreator + 1 новый case в response.go + unit-тесты.
- [ ] Wiring в `cmd/api/main.go`.
- [ ] testutil: `CreatorApplicationFixture` / `SocialFixture` / `SetupCreatorApplicationInModeration` (в `creator_application.go`) + `AssertCreatorAggregateMatchesSetup` (patch в `creator.go`, файл создан в 18b). `DeleteCreatorForTests` уже есть из 18b — не трогаем.
- [ ] E2E: расширить 18b approve_test.go (happy_full + happy_sparse через helper). Создать `creators/get_test.go`.
- [ ] Manual smoke: 7 шагов из § Local Smoke Acceptance.
- [ ] Удалить старую спеку-референс `spec-creator-application-approve.md` финальным коммитом PR.
- [ ] Roadmap: chunk 18.6 → `[~]` старт, `[x]` merge.

**Acceptance Criteria:**
- Given creator существует, when admin GET `/creators/{id}`, then 200 + `CreatorAggregate` со всеми вложенными секциями (identity / PII / Telegram / socials с verified-полями / categories с локализованными именами).
- Given несуществующий creatorID, then 404 `CREATOR_NOT_FOUND`.
- Given brand_manager / unauthenticated, then 403 / 401.
- Given approve happy_full pipeline через fixture, then approve+GET aggregate структурно совпадают с fixture (поле-в-поле через helper).
- Given approve happy_sparse, then nullable копируются как `null` (а не пустые строки).
- E2E approve_test.go (с расширением) и creators/get_test.go зелёные.
- 7 ручных smoke-шагов отработали.
- Старая спека-референс удалена; в `_bmad-output/implementation-artifacts/` остаются только три рабочие спеки + архив.
- `make generate-api && make build-backend lint-backend test-unit-backend-coverage test-e2e-backend` — зелёные.

## Test Plan

### Validation rules

В handler — только path-uuid. Body отсутствует.

### Security invariants

- PII в stdout-логах запрещена. В service / handler разрешены `creator_id`, `actor_id`. Имена / IIN / handle / phone в логах — нет.
- Нет PII в `error.Message`.
- Нет PII в URL.
- Length bound — body отсутствует.
- Rate-limiting — admin-only, не публичный. Не закладываем.

### Unit tests

#### `authz/creator_test.go` — `TestCanViewCreator`

`t.Parallel()`. Шаблон зеркало `CanViewCreatorApplication`. `t.Run`: admin / brand_manager / no role.

#### `service/creator_test.go` — `TestCreatorService_GetByID`

`t.Parallel()` на функции и каждом `t.Run`. Новый mock на каждый сценарий.

`t.Run`:
- `creator not found` — `creatorRepo.GetByID` → `sql.ErrNoRows` → `errors.Is(ErrCreatorNotFound)`. Mock'и socials/categories/dict не вызваны.
- `socials list error propagated` — `creatorSocialsRepo.ListByCreatorID` → ошибка → `ErrorContains` обёртки.
- `categories list error propagated` — `creatorCategoriesRepo.ListByCreatorID` → ошибка → `ErrorContains` обёртки.
- `dictionary city lookup error` — dict-mock ошибка на `ListCitiesByCode` → `ErrorContains` обёртки.
- `dictionary category lookup error` — dict-mock ошибка на `ListCategoriesByCode` → `ErrorContains` обёртки.
- `happy full` — все mock'и возвращают данные, captured-input на каждый, output-aggregate проверяется через подменённые dynamic + `require.Equal` целиком. socials отсортированы по `(platform, handle)`, categories по code.
- `happy sparse` — `middle_name=nil`, `address=nil`, `category_other_text=nil`, 0 соцсетей (пустой slice), 1 категория. Output reflects nullability.
- `deactivated city code` — dict-mock возвращает пустой slice для city — `cityName` в output это `cityCode` (fallback).
- `deactivated category code` — dict-mock возвращает 1 имя из 2 запрошенных — отсутствующая категория получает name=code (fallback).

#### `handler/creator_test.go` — `TestCreatorHandler_GetCreator`

`t.Parallel()`. Black-box `httptest`.

`t.Run`:
- `unauthenticated → 401`.
- `forbidden non-admin → 403`.
- `not found → 404 CREATOR_NOT_FOUND`.
- `happy → 200 + CreatorAggregate` — service mock возвращает domain-aggregate с известными значениями. Handler маппит в response. Через `require.Equal` на response полностью (с подменой dynamic-полей).

### E2E tests

#### `backend/e2e/creator_applications/approve_test.go` — расширение 18b

Удаляется `seedApprovableApplication` локальный helper. Удаляется SQL-lookup в happy.

happy переписывается:
- `happy_full` — fixture с заполненными nullable + 3 соцсети (IG auto / TT manual / Threads non-verified) + 3 категории + `category_other_text`. POST approve → 200. `GET /creators/{creatorId}` → `AssertCreatorAggregateMatchesSetup(t, fixture, creatorId, aggregate)` — full match. `WaitForTelegramSent` ловит `applicationApprovedText`.
- `happy_sparse` — fixture с `middleName=nil`, `address=nil`, `categoryOtherText=nil`, 1 IG-соцсеть auto-verified, 1 категория без `other`. Тот же helper.

7 негативных + concurrent_race из 18b — остаются. Для каждого setup переключается с локального `seedApprovableApplication` на `SetupCreatorApplicationInModeration` (где применимо — для positive setup'ов; для негативных, где заявка не доходит до moderation, остаются ad-hoc).

#### `backend/e2e/creators/get_test.go` — новый

`TestGetCreator` — `t.Parallel()` + cleanup. Файл-комментарий на русском, нарратив. Setup: `SetupCreatorApplicationInModeration` с full-fixture + admin approve (вызов approve через сгенерированный client).

`t.Run`:
- `happy` — `GET /creators/{creatorId}` → `AssertCreatorAggregateMatchesSetup` (тот же helper что в approve_test.go).
- `not_found` — random uuid → 404 `CREATOR_NOT_FOUND`.
- `forbidden_brand_manager` → 403.
- `unauthenticated` → 401.

### Coverage gate

`make test-unit-backend-coverage` ≥ 80% per-method на новых функциях `CreatorService.GetByID`, `GetCreator` handler, `CanViewCreator`.

### Constants

Все codes / actions через exported константы.

### Race detector

`-race` обязателен; concurrent сценариев в 18c нет, но fixture-helper и parallel `t.Run` должны проходить чисто.

## Verification

**Commands:**
- `make generate-api`
- `make build-backend lint-backend test-unit-backend-coverage test-e2e-backend`

**Manual smoke:** см. § Local Smoke Acceptance — 7 шагов.

## Spec Change Log

- **2026-05-05** — спека создана как 18c в декомпозиции chunk 18 (aggregate read + полное E2E через helper). Status: `draft`. Зависит от 18a и 18b — pre-execution gate проставлен. По merge'у удаляет старую спеку-референс `spec-creator-application-approve.md`.
- **2026-05-05 (cleanup-pipeline переезд)** — `CleanupEntityRequest.type=creator` extension + handler-case + `DeleteCreatorForTests` testutil-helper **переехали в 18b**. В 18c эти примитивы только переиспользуются. `testutil/creator.go` теперь patch (файл создан в 18b с `DeleteCreatorForTests`) — добавляется `AssertCreatorAggregateMatchesSetup`. Удалены упоминания «временного testapi-endpoint'а из 18b» в § Decisions / § Never (его не существовало — 18b factual-чек идёт через cleanup-success).
- **2026-05-05 (approve_test расширение)** — § Decisions для `approve_test.go (18b) расширяется`: уточнена логика — в 18b happy-тест не делал structural-сверку (только cleanup-success), 18c **добавляет** structural через GET aggregate, не «заменяет SQL-lookup».

</frozen-after-approval>
