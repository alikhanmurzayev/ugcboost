---
title: "Reject заявки креатора админом (бэк)"
type: feature
created: "2026-05-04"
status: in-progress
baseline_commit: "3dbafc299200b4f7eb77184ea1b25072d77e670d"
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/creator-onboarding-roadmap.md
  - _bmad-output/planning-artifacts/creator-application-state-machine.md
---

> Перед реализацией обязательно полностью загрузить все файлы `docs/standards/` — это hard rules, спека не дублирует.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** На текущей стейт-машине заявок есть терминал `rejected` (введён в chunk 3), но нет admin-action'а его проставить. Админ не может отклонить заявку ни с экрана `verification` (chunk 6), ни — позже — с экрана `moderation` (chunk 16). Как только pipeline дойдёт до заявок, не подходящих по аудитории/контенту/документам, останавливать их будет нечем.

**Approach:** Admin-only `POST /creators/applications/{id}/reject`. Действие легально на статусах `verification` и `moderation` (см. roadmap chunk 12). Сервис в одной `dbutil.WithTx`: lookup заявки → проверка статуса → `applyTransition(... → rejected, reason=reject_admin)` → audit. Telegram-уведомление креатору в этом чанке **не дёргается** — это chunk 14 (`Бот: уведомление о rejected`), он будет читать сохранённый фидбэк и формировать сообщение. Чанк 12 отвечает только за хранение причины так, чтобы chunk 14 её мог использовать.

## Decision: текст отказа

**MVP:** один универсальный шаблон для всех reject'ов. Без категорий, без шаблон-фабрики, без обещаний, без переменных. Текст — living, итерируем отдельным PR'ом.

**Текущий текст (отправляется в Telegram-боте — chunk 14):**

> Здравствуйте! Благодарим вас за интерес к платформе UGC boost.
>
> Мы внимательно рассмотрели вашу заявку, профиль, контент и текущие показатели аккаунта. К сожалению, на данном этапе ваша заявка не прошла модерацию платформы.
>
> Это не является оценкой вашего потенциала как креатора — просто сейчас ваш профиль не полностью совпадает с критериями отбора для текущих fashion-кампаний и запросов брендов на платформе 🙏
>
> Желаем вам дальнейшего роста и удачи в ваших проектах 🤍

Вшит в `internal/telegram/messages/` (или аналог) как одна константа. Никаких category-template-маппингов / Config-переключателей / placeholder'ов.

**Время-связанная фраза.** Текст упоминает «fashion-кампаний» — это завязано на текущую (Неделя Моды) фазу. После расширения категорий (~14 мая) — текст обновляется отдельным PR'ом, заменой константы. Никакой Config-логики в коде нет.

**Что это даёт chunk 12:**
- Body endpoint'а пустой — `internalNote` / `category` / `creatorMessage` не нужны. Сейчас admin кликает «Отклонить» — и ничего больше.
- Storage rejection — только запись в `creator_application_status_transitions` (`from_status`, `to_status=rejected`, `actor_id`, `reason=reject_admin`, `created_at`) + audit row.
- Никаких Config-флагов в `cmd/api/config.go`.
- Никаких enum'ов категорий, freeform-полей или JSON-metadata payloads.

**На будущее (вне chunk 12):** когда появится потребность категоризации для аналитики или indication причин — body расширим опциональными полями. Сейчас MVP.

## Boundaries & Constraints

**Always:**
- Endpoint в `openapi.yaml`: `POST /creators/applications/{id}/reject`, security: bearerAuth, **без `requestBody`** (POST без body), responses 200/401/403/404/422/default. Path-параметр `id` — uuid.
- Action легален на статусах `verification` и `moderation`. На любом другом — 422 `CREATOR_APPLICATION_NOT_REJECTABLE` (включая повторный reject уже rejected'ой заявки).
- Authz: `AuthzService.CanRejectCreatorApplication(ctx)` — admin-only, шаблон как `CanViewCreatorApplication` / `CanVerifyCreatorApplicationSocialManually` (chunk 10).
- Сервис: `(s *CreatorApplicationService) RejectApplication(ctx, applicationID, actorUserID string) error`. В одной `dbutil.WithTx`: (1) `appRepo.GetByID` — `sql.ErrNoRows` ⇒ `ErrCreatorApplicationNotFound`; (2) `app.Status ∉ {verification, moderation}` ⇒ `ErrCreatorApplicationNotRejectable`; (3) `applyTransition(currentStatus → rejected, actorID=&actor, reason=TransitionReasonReject)`; (4) audit-row.
- Telegram-нотификация **не дёргается** ни в одной ветке (chunk 14 это сделает с фиксированным текстом из § Decision).
- Никаких schema-миграций (см. roadmap chunk 12 — «без миграций»).
- Domain: новый sentinel `ErrCreatorApplicationNotRejectable`. Новый user-facing код (`CodeCreatorApplicationNotRejectable`) с actionable-текстом («Заявку нельзя отклонить в текущем статусе. Допустимые статусы для отклонения — `verification` или `moderation`.»).
- Audit: новая action-константа `creator_application_reject` в `audit_constants.go`. Метаданные: `application_id`, `from_status`, `to_status=rejected`. ActorID = админ.
- Transition reason: `domain.TransitionReasonReject = "reject_admin"`. State-machine map (`creatorApplicationAllowedTransitions`) расширяется парами `(verification → rejected)` и `(moderation → rejected)` — без миграций enum (rejected уже в БД с chunk 3).
- Handler: маппинг sentinel'ов в `respondError` / strict-server. Тело успеха — пустой JSON `{}` (по образцу chunk 10).
- Actor UUID — `middleware.UserIDFromContext(ctx)`, проброс параметром в сервис (captured-input в handler-тестах).
- Idempotency: повторный reject уже rejected'ой → 422 `NotRejectable` (status==rejected уже не входит в allowed). БД не трогается.
- **Admin GET detail (`GET /creators/applications/{id}`) возвращает новое опциональное поле `rejection`** — присутствует ⇔ `status=rejected`. Шейп: `{ fromStatus: CreatorApplicationStatus, rejectedAt: timestamp, rejectedByUserId: uuid }`. Все поля required внутри блока. Нейминг зеркалит chunk 7 verification-блок (`verifiedByUserId` / `verifiedAt`), просто `verified*` → `rejected*`. Источник — последний ряд `creator_application_status_transitions` где `to_status=rejected` (поля `from_status` / `created_at` / `actor_id` соответственно).

**Ask First (BLOCKING до старта Execute):**
- ~~**Применимо ли action на `moderation`-экране admin'а сразу**~~ — RESOLVED 2026-05-04: бэк-эндпоинт открыт на оба статуса с самого начала (`verification` и `moderation`). Фронтовая реализация для `moderation`-экрана будет в chunk 16, не в chunk 13.

**Never:**
- Telegram-нотификация креатору в этом чанке (chunk 14).
- Фронт-админка кнопки «Отклонить» (chunk 13).
- Снятие reject'а / переход `rejected → *` (forward-only).
- Бэкфил уже-rejected заявок (в проде сейчас 0 rejected по новой state-machine).
- Расширение body любыми полями. MVP — пустой POST. Если позже понадобится categorize/note — отдельный PR.
- Параллельный path для бренд-менеджера (admin-only).
- Регенерация / удаление `verification_code` при reject'е.
- **Удаление `creator_application_telegram_link`, social-rows, основной заявки.** На reject'е они сохраняются — нужно для chunk 14 (notifier шлёт сообщение в этот же чат) и для будущего broadcast-механизма, если/когда он появится.
- `internalNote` / `category` / `creatorMessage` / `creatorHint` в body, в `audit_logs.metadata` или в transitions. MVP не хранит структурированную причину.
- Config-флаги типа `CreatorRejectReapplyHint`, `CreatorRejectFAQURL`, `CreatorReapplyCooldownDays`. Текст отказа — статический, без переменных.
- Расширение `CreatorApplicationStatus` enum'а или transitions-схемы — никаких новых статусов / колонок.

## I/O & Edge-Case Matrix

> POST без body. Admin GET detail после reject'а возвращает новое поле `rejection`.

| Сценарий | Состояние | Поведение |
|---|---|---|
| Happy path verification | app `verification`, admin Bearer, POST без body | 200 `{}`; app.status=`rejected`; `creator_application_telegram_link` сохранён; transition row `(from_status=verification, to_status=rejected, actor_id=admin, reason=reject_admin)`; audit row `creator_application_reject`; admin GET detail → `rejection={fromStatus: "verification", rejectedAt: <ts>, rejectedByUserId: <admin>}`; **никакого SendMessage** (chunk 14) |
| Happy path moderation | app `moderation`, admin Bearer | то же, `rejection.fromStatus="moderation"` |
| Application не существует | random UUID | 404 `CREATOR_APPLICATION_NOT_FOUND` |
| Wrong status | app в `rejected` / `withdrawn` / `approved` (после chunk 17) | 422 `CREATOR_APPLICATION_NOT_REJECTABLE`, БД не изменилась |
| Повторный reject | первый reject уже прошёл | 422 `CREATOR_APPLICATION_NOT_REJECTABLE` (status == rejected) |
| Non-admin caller | brand_manager Bearer | 403 `FORBIDDEN` (до любого DB-вызова) |
| Unauthenticated | без Bearer | 401 |
| Body present (невозможно через generated client) | POST с произвольным JSON-телом | 200 — body игнорируется (нет requestBody в openapi). Через generated client отправить нельзя — типы не позволяют. |
| Detail до reject'а | app в `verification` / `moderation` | `rejection` отсутствует (omitempty) |
| Telegram-link сохранён после reject | post-reject GET detail | `telegramLink !== null` (нужен для chunk 14 — notifier шлёт в этот же чат) |

</frozen-after-approval>

## Code Map

> Baseline — `3dbafc2` (chunk 10 backend через PR #54 уже в main, файлы из § Code Map содержат `VerifyApplicationSocialManually` и связанные хелперы; новый код встаёт рядом без конфликтов).

- `backend/api/openapi.yaml` —
  - Новый path `POST /creators/applications/{id}/reject` без `requestBody`. Новый `ErrorCode` `CREATOR_APPLICATION_NOT_REJECTABLE`.
  - Расширение существующей schema `CreatorApplicationDetail` (admin GET detail) полем `rejection?: { fromStatus, rejectedAt, rejectedByUserId }` — все internal-поля required, сам блок optional.
  - После — `make generate-api`.
- `backend/internal/domain/creator_application.go` — новый sentinel `ErrCreatorApplicationNotRejectable`; новый `Code*` с actionable-message; константа `TransitionReasonReject = "reject_admin"`; новые элементы `creatorApplicationAllowedTransitions` для `(verification → rejected)` и `(moderation → rejected)`.
- `backend/internal/service/audit_constants.go` — `AuditActionCreatorApplicationReject = "creator_application_reject"`.
- `backend/internal/authz/creator_application.go` — `CanRejectCreatorApplication(ctx)`.
- `backend/internal/authz/creator_application_test.go` — admin / brand_manager / no-role.
- `backend/internal/repository/creator_application_status_transitions.go` (или где repo транзишенов) — новый метод `GetLatestByApplicationAndToStatus(ctx, applicationID, toStatus) (*StatusTransitionRow, error)`. Возвращает `sql.ErrNoRows` если нет (handler детектит и не включает блок).
- `backend/internal/repository/creator_application_status_transitions_test.go` — unit на новый метод (happy, not-found, multiple → берёт latest by created_at desc).
- `backend/internal/service/creator_application.go` —
  - `RejectApplication(ctx, applicationID, actorUserID string) error`. Использует существующие `appRepo.GetByID`, `applyTransition`, `writeAudit`. Notifier mock в тестах — без EXPECT.
  - Расширение существующего `GetApplicationDetail` (или аналог): если `app.Status == rejected` — fetch latest reject-transition через новый repo-метод, маппит в `rejection` блок.
- `backend/internal/service/creator_application_test.go` — для `RejectApplication`: happy verification, happy moderation, not-found, not-rejectable (3 статуса), повторный reject, audit/transition captured-input. Notifier явно не вызывается. Для `GetApplicationDetail`: rejected app → блок собирается; non-rejected — блок пустой; rejected без transition row (corrupted data) — блок пустой + warn-log (без падения).
- `backend/internal/handler/creator_application.go` — handler `RejectCreatorApplication` (по `operationId`). Тянет actor из `middleware.UserIDFromContext`. Маппит sentinel'ы.
- `backend/internal/handler/creator_application_test.go` — happy (captured actor + applicationID), `not-found → 404`, `not-rejectable → 422`, 403, 401. Для существующего detail-handler'а — extra сценарий: rejected app → response содержит `rejection` блок с правильным маппингом.
- `backend/internal/handler/response.go` — расширить маппинг `respondError` на новый sentinel.
- `backend/e2e/creator_applications/reject_test.go` — `TestRejectCreatorApplication` с `t.Run` по матрице. В happy-path: 5-сек sleep + `/test/telegram/sent?since=before` ассерт «no records for chatID» (notifier из chunk 14 ещё не существует, защищаемся от случайной отправки; ассерт обновится в chunk 14).
- `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` — chunk 12 → `[~]` при старте, `[x]` при merge.

## Tasks & Acceptance

**Pre-execution gates:**
- [x] PR чанка 10 смержен в main (commit `15f3a15`, PR #54), `spec-creator-verification-manual.md` архивирован (commit `5cfb0ed`).

**Execution:**
- [x] OpenAPI: новый path + responses + код + расширение `CreatorApplicationDetail` блоком `rejection`. `make generate-api`.
- [x] Domain: sentinel `ErrCreatorApplicationNotRejectable`, код, `TransitionReasonReject`, расширение transition-map.
- [x] Audit: action-константа.
- [x] Authz: `CanRejectCreatorApplication` + unit-тесты (admin / brand_manager / no-role).
- [x] Repository: `GetLatestByApplicationAndToStatus` на transitions + unit-тесты.
- [x] Service: `RejectApplication` + unit-тесты по матрице (captured-input на `applyTransition` / audit; «notifier не дёрнут» через mockery без EXPECT). Расширение `GetApplicationDetail` маппингом rejection-блока + unit-тесты (rejected → блок собран, non-rejected → пустой).
- [x] Handler: новый handler + маппинг sentinel'а + unit-тесты (captured actor/applicationID; 3 ошибки → коды; 403/401). В тестах existing detail-handler'а — extra сценарий «rejected app → response содержит rejection».
- [x] E2E: `reject_test.go` по матрице, happy-path с silence-window'ом 5 сек на `/test/telegram/sent?since=before` (concurrent race-сценарий снят, см. Spec Change Log).
- [~] Roadmap: chunk 12 → `[~]` проставлен. `[x]` — после merge PR.

**Acceptance Criteria:**
- Given admin Bearer, app в `verification`, when `POST /creators/applications/{id}/reject` (без body), then 200 `{}`; admin GET detail → `status=rejected`, `rejection={fromStatus: "verification", rejectedAt: <ts>, rejectedByUserId: <admin>}`, `telegramLink !== null`; transition row `(from_status=verification, to_status=rejected, actor_id=admin, reason=reject_admin)`; audit row `creator_application_reject`; через `GET /test/telegram/sent?since=before` после `Sleep(5s)` записей с этим `chat_id` нет.
- Given app в `moderation`, when call, then то же что выше с `rejection.fromStatus="moderation"`.
- Given app в `rejected`, when call, then 422 `CREATOR_APPLICATION_NOT_REJECTABLE`, БД не изменилась.
- Given несуществующий applicationID, when call, then 404 `CREATOR_APPLICATION_NOT_FOUND`.
- Given brand_manager Bearer, when call, then 403 `FORBIDDEN` без обращений к БД.
- Given неавторизованный, when call, then 401.
- Given app в `verification` (до reject'а), when admin GET detail, then `rejection` отсутствует в response.
- `make generate-api build-backend lint-backend test-unit-backend-coverage test-e2e-backend` — все зелёные, per-method coverage ≥80%.

## Test Plan

> Производное от `docs/standards/backend-testing-unit.md`, `backend-testing-e2e.md`, `backend-constants.md`, `security.md`, `backend-errors.md`. Перед стартом Execute сверить с актуальной версией стандартов.

### Validation rules (handler-level)

Всё, что можно проверить только по body — в handler, до сервиса (`backend-architecture.md`):
- Body нет — нечего валидировать.
- Path `id`: openapi-валидация uuid через ServerInterfaceWrapper, ручной парсинг запрещён (`backend-codegen.md`).
- Error-message актionable (`backend-errors.md`): «Заявку нельзя отклонить в текущем статусе. Допустимые статусы — `verification` или `moderation`.»

### Security invariants

Сверить перед merge (`security.md` § PII, § Length bounds):
- **Никаких PII в stdout-логах.** В service / handler разрешены только `application_id` (UUID), `actor_id` (UUID), `from_status`, `to_status`. Никаких имён/IIN/handle.
- **Нет PII в `error.Message`** — текст ошибок шаблонный.
- **Нет PII в URL** — uuid в path-параметре.
- **Length bound** — body отсутствует, риск гигантского body отсутствует.
- **Rate-limiting** — admin-only endpoint, не публичный вектор. Не закладываем.

### Unit tests

#### authz/creator_application_test.go — `TestCanRejectCreatorApplication`
- `t.Parallel()`. Шаблон как `CanVerifyCreatorApplicationSocialManually` (chunk 10).
- `t.Run`: `admin → nil`, `brand_manager → ErrForbidden`, `no role in ctx → ErrForbidden`.

#### service/creator_application_test.go — `TestCreatorApplicationService_RejectApplication`
- `t.Parallel()` на функции и на каждом `t.Run`. Новый mock на каждый `t.Run` (`backend-testing-unit.md`).
- Notifier mock получает **0 EXPECT** — mockery упадёт, если service случайно вызовет `Send*` для reject'а.
- Порядок `t.Run` повторяет порядок исполнения кода:
  - `application not found` — `appRepo.GetByID` → `sql.ErrNoRows` → ассерт `errors.Is(err, ErrCreatorApplicationNotFound)`.
  - `not rejectable from rejected` — статус `rejected`, ассерт `errors.Is(err, ErrCreatorApplicationNotRejectable)` + БД-вызовов после lookup нет (mock без `applyTransition`/`writeAudit` EXPECT).
  - `not rejectable from approved` (после chunk 17) — placeholder, помечен skip с TODO(#issue) до landing chunk 17. Альтернатива — табличный `not rejectable` с `[]Status{rejected, withdrawn, approved}`.
  - `not rejectable from withdrawn` — то же.
  - `applyTransition error propagated` — `applyTransition` возвращает ошибку → ассерт `ErrorContains` обёртки + audit НЕ вызван.
  - `audit error propagated` — `applyTransition` ok, `writeAudit` возвращает ошибку → ассерт обёртки.
  - `happy verification` — captured-input на `applyTransition` (`fromStatus=verification`, `toStatus=rejected`, `actorID=&actor`, `reason=TransitionReasonReject`) + captured-input на `writeAudit` (`action=AuditActionCreatorApplicationReject`, `actor=admin`, `entity=app.ID`, metadata = `{application_id, from_status, to_status}` через JSONEq).
  - `happy moderation` — то же, `fromStatus=moderation`.
- Captured-input через `mock.EXPECT().X(...).Run(func(args mock.Arguments) { ... })`.

#### service/creator_application_test.go — extra сценарии для `GetApplicationDetail`
- `rejected app → rejection block populated` — мок `transitionsRepo.GetLatestByApplicationAndToStatus` возвращает row с known `from_status` / `created_at` / `actor_id`. Ассерт detail-output содержит `rejection.{fromStatus, rejectedAt, rejectedByUserId}` 1-в-1.
- `non-rejected app → rejection nil` — мок `transitionsRepo` НЕ вызывается. Ассерт detail-output без блока (nil pointer).
- `rejected without transition row (corrupted data) → rejection nil + warn-log` — `GetLatestByApplicationAndToStatus` → `sql.ErrNoRows`. Ассерт detail-output без блока + warn-сообщение в logger (не падаем — defensive degradation).

#### handler/creator_application_test.go — `TestCreatorApplicationHandler_RejectCreatorApplication`
- `t.Parallel()`. Black-box через HTTP с `httptest`. Body отсутствует.
- `t.Run`:
  - `unauthenticated → 401` (без Bearer).
  - `forbidden non-admin → 403` (brand_manager).
  - `application not found → 404 CREATOR_APPLICATION_NOT_FOUND` — service mock возвращает sentinel.
  - `not rejectable → 422 CREATOR_APPLICATION_NOT_REJECTABLE`.
  - `happy → 200 {}` + **captured-input на service**: `actor=UserIDFromContext`, `applicationID=path`. Захват через `mock.Run`, ассерт через `require.Equal`.
- Middleware-derived поле — actor UUID из `middleware.UserIDFromContext`. Captured-input в test обязательно (`backend-testing-unit.md` § Handler).

#### handler/creator_application_test.go — extra сценарий для existing detail-handler'а
- `rejected app → response.Rejection populated` — service mock возвращает domain-detail с rejection-блоком, handler маппит в response, ассерт через `require.Equal` целиком (динамический `rejectedAt` подменяется после `WithinDuration`).

### E2E tests

#### `backend/e2e/creator_applications/reject_test.go`
- Файл-комментарий — на русском, нарратив (`backend-testing-e2e.md` § Комментарий в начале файла). Заголовок: `// Package creator_applications — E2E тесты HTTP-поверхности /creators/applications/{id}/reject.` + по абзацу на каждый Test* + setup/cleanup абзац.
- Генерированный клиент (`apiclient`), импорт `internal/` запрещён.
- `t.Parallel()` на `TestRejectCreatorApplication`. Cleanup через `testutil.Cleanup` + env `E2E_CLEANUP`.
- Setup: `testutil.SetupAdmin(t)` + `testutil.SetupCreatorApplicationInVerification(t, ...)` (helper из chunk 6.5) + `testutil.LinkTelegramToApplication(t, ...)`. Для moderation-сценария: после verification-setup'а — прогон auto-verify (через webhook чанка 8) или manual-verify (через chunk 10), чтобы статус ушёл в moderation. Helper `SetupCreatorApplicationInModeration(t, ...)` — новый composable, оборачивает оба шага.
- Динамические значения через `crypto/rand` (helper'ы `uniqueIIN` / `uniqueTelegramUserID` уже существуют, см. chunk 6.5). Hardcoded дат не использовать (`backend-testing-e2e.md` § Время).
- `t.Run`-ы по матрице:
  - `happy_verification` — POST reject (без body) → 200 `{}`. Затем admin GET detail → `status=rejected`, `rejection.fromStatus="verification"`, `rejection.rejectedByUserId=adminID`, `rejection.rejectedAt` ≈ now (через `WithinDuration`), **`telegramLink` не null** (инвариант сохранения контакта; нужен для chunk 14). `Sleep(5*time.Second)` + `GET /test/telegram/sent?since=before` → пусто (notifier не дёрнулся; шаблон из chunk 8/10, `SpyOnlySender`). Динамический `rejectedAt` подменяется перед `require.Equal` целиком.
  - `happy_moderation` — прокинуть заявку через verification → moderation (auto-verify webhook chunk 8 или manual-verify chunk 10), затем reject. `rejection.fromStatus="moderation"`.
  - `not_found` — random uuid → 404 `CREATOR_APPLICATION_NOT_FOUND`.
  - `not_rejectable_after_reject` — два POST подряд → второй 422 `CREATOR_APPLICATION_NOT_REJECTABLE`.
  - `not_rejectable_from_withdrawn` — заявка прокинута в `withdrawn` (если на тот момент путь есть; иначе skip с TODO).
  - `forbidden_brand_manager` — brand_manager Bearer → 403 `FORBIDDEN`. БД не изменилась (admin GET detail после — `status=verification`).
  - `unauthenticated` — без Bearer → 401.
  - `detail_before_reject_has_no_rejection_block` — app в `verification`, admin GET detail → `rejection` отсутствует (omitempty). Защищает omitempty-инвариант openapi-схемы.
  - `telegram_link_preserved_after_reject` — отдельный явный ассерт-only тест поверх happy: GET detail → `telegramLink` присутствует с тем же `chat_id` что был до reject'а. Защищает контракт для chunk 14.
- **Concurrent race-сценарий** (опц., но рекомендуется): два goroutine-а одновременно POST reject на одну заявку → один 200, второй 422. Без партиал-unique индекса race не критичен (status-check в service защищает), но тест важен — фиксирует контракт «ровно один reject per application». Ассерт: счётчик 200 == 1, счётчик 422 == 1, в БД ровно одна transition row reject.

### Coverage gate

- `make test-unit-backend-coverage` — per-method ≥ 80% на новых функциях (`backend-testing-unit.md` § Coverage). Покрываемые пакеты: `handler/`, `service/`, `authz/`. Generated-код, `cmd/`, mockery-моки исключены AWK-фильтром в Makefile.
- Не отключать AWK-фильтр и `-race` (`docs/standards/review-checklist.md` § Hard rules).

### Generated/codegen invariants

- `*.gen.go` файлы (`api/server.gen.go`, `e2e/apiclient/*.gen.go`, frontend `schema.ts`) меняются ТОЛЬКО через `make generate-api` после правки `openapi.yaml`. Diff с правкой `*.gen.go` без правки yaml — blocker (`backend-codegen.md` § Что ревьюить).
- Никаких ручных структур request/response в handler — только generated (`backend-codegen.md`).
- Никаких `r.URL.Query().Get(...)` / `chi.URLParam(...)` — параметры через ServerInterfaceWrapper.
- Strict-server response — пустой объект `{}`, одна `Set-Cookie` не нужна (нет cookie).
- mockery с `all: true`. После добавления notifier-интерфейса (если расширяется) — `make generate-mocks`.

### Constants

- Все enum-значения через константы (`backend-constants.md`): `domain.TransitionReasonReject`, `service.AuditActionCreatorApplicationReject`. Литералы запрещены, кроме SQL-литералов в repo-тестах (которых здесь нет — repo не трогаем).

### Race detector

- `make test-unit-backend` и `make test-e2e-backend` запускают с `-race` (Makefile уже включает). Concurrent reject-тест выше обязан проходить с `-race`.

## Verification

**Commands:**
- `make generate-api`
- `make build-backend lint-backend test-unit-backend-coverage test-e2e-backend`

**Manual smoke (локально):**
- `make compose-up && make migrate-up`. Создать заявку через лендос, привязать Telegram, дёрнуть `curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" http://localhost:8082/creators/applications/<id>/reject` (без body) → 200; в БД `app.status=rejected`, transition row есть, audit row есть. Admin GET detail отдаёт `rejection` блок. В Telegram у тестового аккаунта **сообщение НЕ приходит** (notifier для reject — chunk 14).

## Spec Change Log

- **2026-05-04** — chunk 10 merged (PR #54, commit `15f3a15`). Снят WIP / DRAFT баннер; pre-execution gate помечен done; Ask First по `moderation`-экрану зарезолвлен (admin может reject'ить на обоих статусах сразу). Status: `draft` → `ready-for-dev`. Baseline зафиксирован на `3dbafc2`.
- **2026-05-04 (impl)** — реализация в одной сессии. Optional concurrent race-сценарий убран: без явного `SELECT FOR UPDATE` (миграций нет) READ COMMITTED изоляция позволяет двум TX'ам одновременно прочитать `verification` и обоим завершиться 200 (две transition row). Контракт «ровно один reject per application» — на стороне admin-UI (disabled-кнопка после reject); серверный side остаётся идемпотентным relative to status-machine reachability, но не сериализован. Если контракт станет hard-required — отдельный чанк с partial unique index на `(application_id) WHERE to_status='rejected'` или row-locking в WithTx.
