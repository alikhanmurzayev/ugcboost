---
title: "Бэк-фундамент верификации: storage соцсетей + verification-код заявки"
type: feature
created: "2026-05-02"
status: done
baseline_commit: "0e7a5d0"
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/creator-onboarding-roadmap.md
  - _bmad-output/planning-artifacts/creator-application-state-machine.md
  - _bmad-output/planning-artifacts/creator-verification-concept.md
---

> Перед реализацией обязательно полностью загрузить все файлы `docs/standards/` — это hard rules, спека не дублирует их.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Chunk 7 roadmap'а. Концепция верификации зафиксирована в `creator-verification-concept.md`, но storage'а нет — соцсети заявки не имеют признака верификации, заявка не имеет идентификатора-кода, который креатор отправит в IG DM. Без этого webhook (chunk 8) и manual-verify (chunk 9) не на чем строить.

**Approach:** Одна миграция добавляет 4 поля верификации на `creator_application_socials` и колонку `verification_code` на `creator_applications` с partial unique index по `WHERE status = 'verification'`. Существующие 20 prod-заявок бэкфилятся в этой же миграции (DO-блок, retry на коллизию). Сервис генерирует код при подаче заявки (`crypto/rand` 6 цифр + префикс `UGC-`), на конфликт по индексу — retry через `cenkalti/backoff/v5` (constant 0, max 20). Admin-detail и admin-list автоматически получают новые verified-поля через расширение shared schema `CreatorApplicationDetailSocial`. Creator-detail-ручка (где код будет виден креатору) откладывается до chunk 11 — потребителя в TMA пока нет.

## Boundaries & Constraints

**Always:**
- Миграция в одной транзакции goose: `ALTER ADD verification_code TEXT` (nullable) → `ALTER ADD` 4 поля на socials → `CREATE UNIQUE INDEX ... WHERE status = 'verification'` → DO-блок бэкфила (random 6 цифр + префикс, retry до 10 на `unique_violation`, на 11-й RAISE EXCEPTION) → `ALTER SET NOT NULL` на verification_code.
- Колонка `verification_code TEXT NOT NULL` хранит полное `UGC-NNNNNN`. Никаких CHECK на формат. Partial unique только `WHERE status = 'verification'` — после перехода в `moderation` код перестаёт быть constraint'ом (может позже переиспользоваться новой заявкой в `verification`).
- Поля верификации соцсети: `verified BOOLEAN NOT NULL DEFAULT false`, `method TEXT`, `verified_by_user_id UUID REFERENCES users(id)`, `verified_at TIMESTAMPTZ`. Никаких CHECK на согласованность — целостность держит сервис.
- Constants в `domain/creator_application.go`: `VerificationCodePrefix = "UGC-"`, `VerificationCodeDigits = 6`, `VerificationCodeMaxGenerationAttempts = 20`. Метод-enum: `SocialVerificationMethodAuto` / `SocialVerificationMethodManual` + `SocialVerificationMethodValues`.
- `domain.GenerateVerificationCode()` — pure helper на `crypto/rand`, возвращает `UGC-NNNNNN`.
- Repo `Create` различает pgErr 23505 по `ConstraintName`: IIN-индекс → `ErrCreatorApplicationDuplicate` (409), verification_code-индекс → `ErrCreatorApplicationVerificationCodeConflict` (sentinel для retry). Новая константа `CreatorApplicationsVerificationCodeVerificationIdx`.
- `Service.Submit` оборачивается в `backoff.Retry` (cenkalti/backoff v5) с constant 0 backoff и max 20 tries. Только `ErrCreatorApplicationVerificationCodeConflict` retry-able, всё остальное (валидации, IIN-конфликт, dictionary lookups) заворачивается в `backoff.Permanent`. Каждая попытка — новая транзакция `WithTx`. Исчерпание лимита → generic ошибка вида `failed to generate unique verification code after 20 attempts` (5xx).
- Admin-detail (`GET /creators/applications/{id}`) и admin-list (`POST /creators/applications/list`) автоматически отдают новые поля через расширение `CreatorApplicationDetailSocial`. `verification_code` НЕ выводится ни в одну admin-ручку.
- Audit submit-row не пополняется `verification_code`.
- `creator-verification-concept.md` уточняется в этом же PR: «код активен пока заявка в `verification`» (а не «в активном статусе»), creator-detail-ручка вынесена в chunk 11.

**Ask First:**
- CHECK-constraints на согласованность 4 полей верификации (default — не делаем).
- Любые UPDATE-методы на verified-поля (это chunk 9 — manual verify, и chunk 8 — webhook).
- Создание creator-detail-ручки сейчас (default — отложено в chunk 11).

**Never:**
- Webhook от SendPulse, manual-verify endpoint, reject/withdraw, state-history таблица — отдельные чанки.
- Изменения в TMA / админ-фронте — отдельные следующие чанки (10, 11, 12, 14, 16).
- Удаление partial unique index по IIN.
- Регенерация verification_code на живой заявке (концепцией не предусмотрена).
- Audit-rows бэкфила.

## I/O & Edge-Case Matrix

| Сценарий | State | Behavior |
|---|---|---|
| Submit happy path | новый IIN | 201 + заявка `verification` + `verification_code` формата `^UGC-[0-9]{6}$` + соцсети с `verified=false` |
| Submit duplicate IIN | active заявка по IIN | 409 `CREATOR_APPLICATION_DUPLICATE` (без изменений в поведении) |
| Submit code conflict (race) | репо вернул `ErrCreatorApplicationVerificationCodeConflict` | backoff retry с новым кодом |
| Submit code exhausted | 20 попыток подряд в конфликте | 5xx generic, no rows persisted |
| Migration backfill | 20 prod-заявок без кода | каждая получает уникальный `UGC-NNNNNN`, миграция атомарна |
| Migration backfill exhausted | DO-блок не сгенерил уникальный за 10 попыток для одной заявки | RAISE EXCEPTION → миграция rollback |
| Admin-detail на свежей заявке | сразу после submit | `socials[].verified=false`, `method=null`, `verifiedByUserId=null`, `verifiedAt=null` |
| Admin-list | `statuses=["verification"]` | `items[].socials[]` имеют те же default verified-поля |
| Admin-detail response shape | любая заявка | `verificationCode` отсутствует в JSON (не в schema) |

</frozen-after-approval>

## Code Map

- `backend/migrations/{timestamp}_creator_application_verification_storage.sql` — новая миграция: ALTER ADD nullable + 4 поля на socials + CREATE partial unique index + DO-блок бэкфила + ALTER SET NOT NULL. Down — DROP INDEX + DROP COLUMNS (безопасен до первого реального использования кода).
- `backend/internal/domain/creator_application.go` — константы verification-кода, метод-enum (`SocialVerificationMethod*`), `GenerateVerificationCode()`, `ErrCreatorApplicationVerificationCodeConflict`, расширение `CreatorApplicationDetailSocial` (4 новых поля).
- `backend/internal/repository/creator_application.go` — `CreatorApplicationRow.VerificationCode` (insert-тег), различение constraint'ов в `Create` по `ConstraintName`, новая константа `CreatorApplicationsVerificationCodeVerificationIdx`.
- `backend/internal/repository/creator_application_social.go` — расширение `CreatorApplicationSocialRow` (4 поля, только select-теги — INSERT происходит без них, верификация наполняет UPDATE'ом из chunks 8/9).
- `backend/internal/service/creator_application.go` — `Submit` обёрнут в `backoff.Retry` v5; код генерируется в каждой попытке; `creatorApplicationDetailFromRows` и маппер в `List` копируют новые поля в `CreatorApplicationDetailSocial`.
- `backend/api/openapi.yaml` — расширение `CreatorApplicationDetailSocial` (verified, method, verifiedByUserId, verifiedAt) + новый shared enum `SocialVerificationMethod` (`auto`/`manual`).
- `backend/internal/handler/creator_application.go` — маппер domain → API DTO socials добавляет 4 новых поля.
- `backend/go.mod` / `go.sum` — `github.com/cenkalti/backoff/v5`.
- `_bmad-output/planning-artifacts/creator-verification-concept.md` — уточнение про жизненный цикл кода и creator-detail-ручку (см. Always).
- `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` — `[ ]` → `[~]` на chunk 7, при merge → `[x]`.

## Tasks & Acceptance

**Execution:**
- [x] Миграция: ALTER + 4 поля socials + CREATE partial unique index + DO-блок бэкфила + ALTER SET NOT NULL.
- [x] Domain: константы, метод-enum, `GenerateVerificationCode()`, sentinel, расширение `CreatorApplicationDetailSocial`.
- [x] Repo: insert-тег `VerificationCode`, различение constraint'ов в `Create`, расширение `CreatorApplicationSocialRow`.
- [x] Service: `backoff.Retry` v5 на `Submit`; маппинг verified-полей в `creatorApplicationDetailFromRows` и в `List`.
- [x] OpenAPI: расширение схемы socials + enum, `make generate-api`.
- [x] Handler-маппер: 4 новых поля в socials response.
- [x] Unit-тесты: формат `GenerateVerificationCode`; `Submit` retry (мок репо отдаёт `ErrCreatorApplicationVerificationCodeConflict` на N-й попытке, success на N+1); `Submit` exhausted (20 conflict подряд → ошибка). Coverage ≥80% per-method.
- [x] E2E-расширения: `TestSubmitCreatorApplication` после `GET /creators/applications/{id}` ассертит default verified-поля; `list_verification_test` — то же на `items[].socials[]`.
- [x] Уточнение `creator-verification-concept.md` (жизнь кода активна пока статус = `verification`; creator-detail-ручка — chunk 11).
- [x] `creator-onboarding-roadmap.md`: chunk 7 → `[~]` сейчас, при merge — `[x]`.

**Acceptance Criteria:**
- Given валидный submit, when `POST /creators/applications`, then 201 и в БД одна строка `creator_applications` с `status='verification'` и непустым `verification_code` matching `^UGC-[0-9]{6}$`, плюс N социалок с `verified=false` / `method=null` / `verified_by_user_id=null` / `verified_at=null`.
- Given admin Bearer и id свежей заявки, when `GET /creators/applications/{id}`, then `response.socials[].{verified=false, method=null, verifiedByUserId=null, verifiedAt=null}`. Поле `verificationCode` отсутствует в JSON (не в схеме).
- Given admin Bearer и `statuses=["verification"]`, when `POST /creators/applications/list`, then `response.items[].socials[]` имеют default verified-поля.
- Given мок-репо возвращает `ErrCreatorApplicationVerificationCodeConflict` на 1-й попытке и success на 2-й, when `Submit`, then 201 (retry сработал ровно один раз).
- Given мок-репо возвращает conflict 20 раз подряд, when `Submit`, then ошибка вида `failed to generate unique verification code after 20 attempts`, ничего не персистится.
- Given миграция применяется на dev-БД с 20 заявками без `verification_code`, when goose up, then все 20 имеют непустой `verification_code` формата `UGC-NNNNNN`, никаких дублей среди `status='verification'`.
- `make build-backend lint-backend test-unit-backend-coverage test-e2e-backend generate-api` — все зелёные.

## Verification

**Commands:**
- `make generate-api` — codegen openapi пересобран.
- `make build-backend lint-backend test-unit-backend-coverage test-e2e-backend` — все зелёные, per-method coverage ≥80%.
- `make migrate-up` (dev/staging) — миграция применилась без ошибок.

**Manual smoke на staging (e2e на verification_code не пишем — random 6-digit space делает race-тест never-fail; стабильность кода у конкретной заявки покрывается chunk 11, когда появится creator-detail ручка):**
- `psql`: `SELECT id, status, verification_code FROM creator_applications;` — все строки имеют непустой `verification_code` формата `UGC-[0-9]{6}`; среди `status='verification'` дублей нет.
- `psql`: `SELECT application_id, platform, verified, method, verified_by_user_id, verified_at FROM creator_application_socials LIMIT 10;` — все `verified=false`, остальные 3 поля `NULL`.
- Подать новую заявку через лендос → проверить в БД, что `verification_code` непустой и в формате `UGC-NNNNNN`.

## Suggested Review Order

**Спецификация и контракт**

- Источник правды для DB-формы и сериализации поля `method` — `nullable: true` рядом с `$ref` (TS-codegen quirk вынесён в deferred-work).
  `backend/api/openapi.yaml:1042`

- Миграция: ALTER ADD nullable + 4 поля socials + partial unique index + DO-блок бэкфила (gen_random_bytes — крипто) + ALTER SET NOT NULL.
  `backend/migrations/20260502204252_creator_application_verification_storage.sql:1`

**Domain слой**

- Константы кода + sentinel + `GenerateVerificationCode()` — pure helper на crypto/rand, формат проверяется детерминированно.
  `backend/internal/domain/creator_application.go:55`

- `CreatorApplicationDetailSocial` расширён 4 verified-полями (Verified/Method/VerifiedByUserID/VerifiedAt).
  `backend/internal/domain/creator_application.go:228`

**Repository слой**

- INSERT-tag на `VerificationCode` + точный switch по ConstraintName (а не Contains) для маппинга 23505 → правильный sentinel.
  `backend/internal/repository/creator_application.go:200`

- `CreatorApplicationSocialRow` пополнен select-only полями — INSERT остаётся прежним, верификация наполняется UPDATE'ом из chunks 8/9.
  `backend/internal/repository/creator_application_social.go:31`

**Service слой**

- Submit обёрнут в `backoff.Retry` v5 с constant 0 + WithMaxTries(20) + WithMaxElapsedTime(0) (отключает дефолт 15min).
  `backend/internal/service/creator_application.go:117`

- Каждая retry-итерация — свежая транзакция + новый код через `submitOnce`; verification_code conflict — единственный retryable case.
  `backend/internal/service/creator_application.go:140`

**Handler слой**

- Маппер domain → API DTO: 4 verified-поля + UUID-парсинг `verifiedByUserId` (corruption → 500 с wrapped error).
  `backend/internal/handler/creator_application.go:238`

**Тесты**

- Unit: формат + variance `GenerateVerificationCode`.
  `backend/internal/domain/creator_application_test.go:11`

- Unit: retry на 1-й коллизии и exhaust на 20 — двух новых сценариях `Submit`.
  `backend/internal/service/creator_application_test.go:732`

- Unit: repo различает IIN-23505 vs verification_code-23505 на уровне ConstraintName.
  `backend/internal/repository/creator_application_test.go:144`

- Unit: handler-маппер на default + manual + auto + corrupt UUID.
  `backend/internal/handler/creator_application_test.go:1066`

- E2E: default verified-fields ассертятся в admin-detail после submit.
  `backend/e2e/creator_application/creator_application_test.go:454`

- E2E: те же ассерты на `items[].socials[]` в admin-list.
  `backend/e2e/creator_applications/list_verification_test.go:680`

**Documentation / planning**

- Концепт уточнён: жизнь кода = пока статус `verification`; creator-detail-ручка перенесена на chunk 11.
  `_bmad-output/planning-artifacts/creator-verification-concept.md:26`

- Roadmap: chunk 7 → `[~]` (in-progress).
  `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md:53`

- Реестр библиотек пополнен `cenkalti/backoff/v5`.
  `docs/standards/backend-libraries.md:24`

- Deferred backlog: 4 finding'а из ревью оставлены на будущие чанки (TS-codegen nullable+$ref, retry-hoist, NO TRANSACTION migration, graceful UUID degradation).
  `_bmad-output/implementation-artifacts/deferred-work.md:1`
