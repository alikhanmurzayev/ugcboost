---
title: "Реализация целевой стейт-машины заявки креатора"
type: feature
created: "2026-05-01"
status: done
baseline_commit: 39567d8
context:
  - docs/standards/backend-repository.md
  - docs/standards/backend-transactions.md
  - _bmad-output/planning-artifacts/creator-application-state-machine.md
---

## Преамбула для агента

Перед любой правкой полностью загрузи `docs/standards/` (все 20 файлов) — это hard rules. Источник правды по статусам — `creator-application-state-machine.md`. Чанк 3 roadmap'а: один PR, без таблицы истории переходов и сервиса переходов (будущие чанки).

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** enum статусов заявки в БД, openapi и domain (`pending/approved/rejected/blocked`) не соответствует целевой 7-статусной модели; partial unique index по ИИН и initial status в `service.Submit` тоже завязаны на старые значения. Без перехода следующие чанки (админка, модерация, договор, TMA) не имеют легальной поверхности.

**Approach:** одной миграцией перевести БД на 7 целевых значений (`verification`, `moderation`, `awaiting_contract`, `contract_sent`, `signed`, `rejected`, `withdrawn`), бэкфилить `pending → verification`, перестроить partial unique index по 4 активным. Параллельно: `domain.CreatorApplication*` константы, openapi enum, initial status в `Submit`, регенерация клиентов (`make generate-api`), unit/e2e тесты.

## Boundaries & Constraints

**Always:**
- Источник правды по списку статусов — `domain.CreatorApplication*` константы. OpenAPI enum и CHECK в миграции зеркалят их.
- `CreatorApplicationActiveStatuses` = ровно 4 значения: `verification`, `moderation`, `awaiting_contract`, `contract_sent`. Partial unique index фильтруется по ним же.
- Initial status новой заявки = `verification`, ставится явно в `service.CreatorApplicationService.Submit` (DEFAULT в БД нет — business default остаётся в коде, см. `backend-repository.md` § Целостность данных).
- SQL-литералы в `*_test.go` намеренно строки (`backend-constants.md` § Исключение: тесты), но значения обновляются.
- `t.Parallel()`, `-race`, mockery, точные аргументы в expectations. Coverage gate ≥ 80% по handler/service/repository.
- Миграция — новая forward, не in-place edit (`backend-repository.md` § Миграции).

**Ask First:**
- Если в коде нашлось место использования старых значений вне настоящей спеки — HALT.
- Если в openapi.yaml кроме `CreatorApplicationDetailData.status` обнаружится ещё один enum — HALT.
- Если в dev-БД обнаружатся ряды со статусом отличным от `pending` — HALT (по умолчанию мап только `pending → verification`).
- Если в `_prototype/` или TMA окажется код, привязанный к старым значениям — HALT.

**Never:**
- Никакой таблицы истории переходов и сервиса переходов в этом PR — будущие чанки.
- Никаких модераторских эндпоинтов / ручек смены статуса / webhook'ов.
- Не трогать `frontend/web/src/_prototype/` (другая модель «заявок на кампанию»).
- Не переходить на PG `ENUM TYPE` — оставить CHECK.
- Никаких backward-compat shims для старых значений.
- Не редактировать существующие миграции in-place.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Behavior |
|----------|--------------|-------------------|
| Submit happy path | Валидный POST `/creators/applications` | 201; ряд `status='verification'`; audit-row с тем же статусом в той же tx |
| Submit duplicate IIN (race) | Два concurrent POST с одинаковым ИИН | Один 201; второй 23505 → `domain.ErrCreatorApplicationDuplicate` → 409 `CREATOR_APPLICATION_DUPLICATE` |
| Admin GET application | Существующая заявка | 200; `status` ∈ 7 новых значений |
| Migrate Up (чистая БД) | Пустая `creator_applications` | CHECK на 7 значений; partial unique index по 4 активным |
| Migrate Up (есть pending) | Ряд `status='pending'` | Ряд → `status='verification'`, проходит финальный CHECK |
| Migrate Up (не-pending старое) | Ряд `status='approved'`/`'blocked'` | Миграция упадёт на финальном CHECK (намеренно — сигнал расхождения, см. roadmap chunk 3 / вариант бэкфила) |
| Migrate Down | После Up на чистой БД | CHECK откатывается на 4 старых; index — на 3 старых; ряды `verification → pending` |

</frozen-after-approval>

## Code Map

- `backend/migrations/<ts>_creator_applications_state_machine.sql` -- новая goose-миграция: транзитный CHECK → backfill → финальный CHECK + перестройка partial unique index
- `backend/internal/domain/creator_application.go:12-26` -- константы статусов и `CreatorApplicationActiveStatuses`
- `backend/internal/service/creator_application.go:118` -- initial status в `Submit`
- `backend/api/openapi.yaml:1248-1251` -- enum `CreatorApplicationDetailData.status`
- `backend/internal/repository/creator_application.go:18-22` -- комментарий `CreatorApplicationsIINActiveIdx` (ссылка на миграцию + список активных)
- Тесты: `backend/internal/repository/creator_application_test.go`, `backend/internal/handler/creator_application_test.go:385,471`, `backend/internal/service/creator_application_test.go`, `backend/internal/service/creator_application_telegram_test.go`, `backend/e2e/creator_application/creator_application_test.go` (header + ассерты), `backend/e2e/telegram/telegram_test.go` (если есть)
- Generated (`make generate-api`): `backend/internal/api/server.gen.go`, `backend/internal/testapi/server.gen.go`, `backend/e2e/{apiclient,testclient}/types.gen.go`, `frontend/{web,tma,landing}/src/api/generated/schema.ts`

## Tasks & Acceptance

**Execution:**
- [x] Создать миграцию через `make migrate-create NAME=creator_applications_state_machine`. Up: drop текущий CHECK → add transitional CHECK (старые + 7 целевых) → `UPDATE ... SET status='verification' WHERE status='pending'` → drop transitional CHECK → add финальный CHECK (только 7 целевых) → DROP partial unique index → CREATE новый по 4 активным. Down: обратная последовательность с маппингом `verification → pending`. В Down-комментарии указать: «работает только до первого реального перехода; ряды в `moderation/awaiting_contract/contract_sent/signed/rejected/withdrawn` валят Down».
- [x] `domain/creator_application.go` -- удалить старые `Status{Pending,Approved,Rejected,Blocked}`; добавить `Status{Verification,Moderation,AwaitingContract,ContractSent,Signed,Rejected,Withdrawn}`; пересобрать `CreatorApplicationActiveStatuses` (4 элемента); обновить godoc-ссылку на новое имя миграции.
- [x] `service/creator_application.go:118` -- initial status → `domain.CreatorApplicationStatusVerification`.
- [x] `openapi.yaml` -- enum в `CreatorApplicationDetailData.status` → 7 целевых; description: «Application status; см. creator-application-state-machine.md».
- [x] `make generate-api` -- регенерация; ручных правок в `*.gen.go` / `schema.ts` быть не должно.
- [x] `repository/creator_application.go` -- обновить комментарий у `CreatorApplicationsIINActiveIdx` (имя новой миграции, новый список активных).
- [x] `repository/creator_application_test.go` -- SQL-литерал `status IN ($2,$3,$4)` → `status IN ($2,$3,$4,$5)`; `WithArgs` cortege для `HasActiveByIIN` (4 активных); `Status:` в Row-структурах и insert-row `WithArgs`.
- [x] `handler/creator_application_test.go`, `service/creator_application_test.go`, `service/creator_application_telegram_test.go` -- replace `StatusPending` → `StatusVerification`.
- [x] `e2e/creator_application/creator_application_test.go` -- header-комментарий (`(pending)` → `(verification)`); ассерты на статус.
- [x] `e2e/telegram/telegram_test.go` -- проверить grep'ом, обновить если есть.
- [x] Локальный полный gate (Verification ниже).

**Acceptance Criteria:**
- Given чистая БД, when applied миграция Up, then `\d+ creator_applications` показывает CHECK на 7 значений и partial unique index по 4 активным; миграция Down успешно откатывает структуру.
- Given БД с рядом `status='pending'`, when applied миграция Up, then ряд `status='verification'` и проходит финальный CHECK.
- Given успешный POST, when заявка сохранена, then в БД `status='verification'`, audit-row в той же tx содержит тот же статус.
- Given два concurrent POST с одинаковым ИИН, when второй доходит до Create, then 409 `CREATOR_APPLICATION_DUPLICATE` (existing race-test остаётся зелёным).
- Given admin GET, when заявка `status='verification'`, then ответ содержит `status: "verification"` и проходит openapi-валидацию сгенерированного клиента.
- Given `make generate-api` на чистом diff, when запущен, then `git status` показывает изменения только в generated-файлах, согласованных с openapi.yaml.
- Given `make lint-backend && make test-unit-backend-coverage && make test-e2e-backend`, when выполнено, then 0 failures, coverage gate зелёный.

## Spec Change Log

### 2026-05-01 — review iteration 1 (review-time patches)

**Trigger:** review findings от blind hunter / edge-case hunter / acceptance auditor.

**Patches applied (в коде, не в спеке):**
- Миграция `20260501222829_creator_applications_state_machine.sql`:
  - Добавлен fail-fast guard (`DO $$ ... RAISE EXCEPTION ... $$`) на `approved`/`blocked`
    ряды до изменения схемы. Дает оператору понятное сообщение вместо CHECK violation.
  - `DROP CONSTRAINT/INDEX` всюду с `IF EXISTS` — согласуется с соседней миграцией
    `relax_constraints` и делает retry безопаснее.
  - В шапке явно прописан маппинг старых значений: `pending→verification`,
    `rejected→rejected (1:1)`, `approved/blocked → ABORT`.
  - Добавлено предупреждение «не добавлять goose-аннотацию NO TRANSACTION».
- `domain.CreatorApplicationDuplicate` godoc — ссылается на `CreatorApplicationActiveStatuses`
  вместо литерального списка значений.
- `service.auditNewValue` — добавлено поле `"status": domain.CreatorApplicationStatusVerification`
  в audit-payload (поддерживает буквальный текст AC#3).
- Unit-тест happy-path расширен: ассерт через `MatchedBy` парсит `row.NewValue`
  и проверяет `status == verification`.

**Spec corrections (Down comment):**
- В Tasks Execution: «ряды в moderation/awaiting_contract/contract_sent/signed/rejected/withdrawn
  валят Down» — буквально некорректно. `rejected` есть в обеих моделях и Down
  его не валит. Корректный список «валящих» Down — 5 значений без `rejected`:
  moderation/awaiting_contract/contract_sent/signed/withdrawn. Реализация
  миграции уже отражает корректный вариант.

**KEEP (что должно сохраниться при любых будущих re-derive):**
- 4-step Up (transitional CHECK → backfill → final CHECK → index rebuild) —
  единственно безопасный путь без CONCURRENTLY.
- Partial unique index по 4 активным статусам (не 7).
- `CreatorApplicationActiveStatuses` = source of truth, openapi и CHECK зеркалят.
- Fail-fast guard на `approved/blocked` (review-time addition).
- Audit-payload содержит `status` (review-time addition).

### 2026-05-01 — review iteration 2 (PR review feedback)

**Trigger:** комментарии Alikhan на PR #46.

**Patches applied:**
- `handler/creator_application.go` — заменил прямой каст `api.CreatorApplicationDetailDataStatus(d.Status)` на explicit switch-case mapper `mapCreatorApplicationStatusToAPI` с `(value, error)` сигнатурой. `domainCreatorApplicationDetailToAPI` теперь возвращает `(api.CreatorApplicationDetailData, error)`; caller `GetCreatorApplication` пробрасывает.
- Удалён `_bmad-output/implementation-artifacts/deferred-work.md` (отложенные пункты по фидбеку не нужны).
- `openapi.yaml` — description у `status` без ссылки на md-файл.
- `domain/creator_application.go`, `repository/creator_application.go` — лишние godoc-комменты удалены (имена констант говорят сами за себя).

## Design Notes

**Почему 4 шага в Up, а не 2.** Единый ALTER CHECK ломается о существующие `pending`-ряды: Postgres валидирует CHECK сразу. Транзитный CHECK (старое + новое) → бэкфил → финальный CHECK без старых.

**Почему Down не пытается 1:1 мапить новые в старые.** Целевая модель (см. design-doc) не имеет 1:1 со старой 4-статусной. `verification → pending` — единственный осмысленный обратный шаг для рядов, созданных до первого реального перехода. Для остальных Down валится — это намеренно.

## Verification

**Commands:**
- `make migrate-reset && make migrate-up` -- Up проходит, схема содержит новый CHECK и index.
- `make generate-api` затем `git status` -- generated-файлы согласованы с openapi.yaml; ручных правок нет.
- `make lint-backend` -- 0 issues (включая gofmt).
- `make test-unit-backend-coverage` -- все pass; coverage gate зелёный.
- `make test-e2e-backend` -- creator_application и telegram e2e зелёные; race-detector тих.

**Manual checks:**
- `psql -c "\d+ creator_applications"` -- CHECK содержит 7 значений; `creator_applications_iin_active_idx` имеет `WHERE status IN ('verification','moderation','awaiting_contract','contract_sent')`.
- `grep -nE '\"pending\"|\"approved\"|\"blocked\"|StatusPending|StatusApproved|StatusBlocked' backend/ frontend/src` -- 0 совпадений вне `_prototype/` и архивных миграций.

## Suggested Review Order

**Источник правды по статусам**

- 7 целевых констант + 4 активных, godoc указывает на новую миграцию.
  [`creator_application.go:13`](../../backend/internal/domain/creator_application.go#L13)

- Initial status новой заявки + добавлен в audit-payload для AC#3.
  [`creator_application.go:118`](../../backend/internal/service/creator_application.go#L118)

**Миграция БД (data integrity)**

- 4-step Up + fail-fast guard + IF EXISTS + комментарий-маппинг старых значений.
  [`20260501222829_creator_applications_state_machine.sql:1`](../../backend/migrations/20260501222829_creator_applications_state_machine.sql#L1)

- Down с обратным backfill verification → pending + комментарий о границах применимости.
  [`20260501222829_creator_applications_state_machine.sql:62`](../../backend/migrations/20260501222829_creator_applications_state_machine.sql#L62)

**API и сериализация**

- OpenAPI enum `CreatorApplicationDetailData.status` → 7 целевых.
  [`openapi.yaml:1250`](../../backend/api/openapi.yaml#L1250)

- Switch-case mapper `domain.Status → api.Status` с pass-through ошибки.
  [`creator_application.go:212`](../../backend/internal/handler/creator_application.go#L212)

**Тесты**

- Unit-тест happy-path расширен ассертом на `status` в audit-payload.
  [`creator_application_test.go:541`](../../backend/internal/service/creator_application_test.go#L541)

- Repo-тест: SQL-литерал `IN ($2,$3,$4,$5)` + `WithArgs` для 4 активных.
  [`creator_application_test.go:22`](../../backend/internal/repository/creator_application_test.go#L22)

- E2E happy-path ассертит `apiclient.Verification` после Submit.
  [`creator_application_test.go:495`](../../backend/e2e/creator_application/creator_application_test.go#L495)
