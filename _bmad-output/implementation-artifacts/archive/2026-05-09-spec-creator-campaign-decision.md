---
title: 'TMA приём решения креатора + secret_token integrity'
type: 'feature'
created: '2026-05-08'
status: 'done'
baseline_commit: '556fb09382f71eb04ea2e0d27db976719aa1b529'
context:
  - docs/standards/security.md
  - docs/standards/backend-transactions.md
  - _bmad-output/planning-artifacts/design-campaign-creator-flow.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Креатор не может через TMA подтвердить участие или отказаться (T1/T2 не реализованы). Запланированный lookup кампании по `tma_url` через `LIKE '%/'||token` уязвим к suffix-attack: однобуквенный токен сматчит чужую кампанию.

**Approach:** Добавить колонку `campaigns.secret_token` (real, NULLABLE) с partial UNIQUE + backfill валидных tma_url; ввести `domain.ValidateTmaURL`/`ExtractSecretToken` для POST/PATCH `/campaigns`; реализовать middleware `tma_initdata` (HMAC + lookup creator → ctx), `AuthzService.AuthorizeTMACampaignDecision`, идемпотентные T1/T2 с `already_decided` флагом. На TMA-фронте — wire через openapi-fetch middleware с `Authorization: tma <init-data>`. Полный концепт: `_bmad-output/implementation-artifacts/intent-creator-campaign-decision.md`.

## Boundaries & Constraints

**Always:**
- `secret_token` — real NULLABLE column; lookup `WHERE secret_token = $1 AND is_deleted = false` (точное `=`, без LIKE); partial UNIQUE INDEX `WHERE secret_token IS NOT NULL AND is_deleted = false`; формат `^[A-Za-z0-9_-]{16,}$` валидируется в domain.
- Middleware `tma_initdata` валидирует HMAC (`HMAC_SHA256(bot_token,"WebAppData")` → key, `subtle.ConstantTimeCompare`) + `auth_date ∈ [now-TTL, now]` (config `TMA_INITDATA_TTL_SECONDS`, default 86400); кладёт в ctx `telegram_user_id`, и при наличии в creators — `creator_id`+`role`. Любой провал → 401 generic anti-fingerprint, без обращения к БД для авторизации.
- AuthzService.AuthorizeTMACampaignDecision выполняет ВСЮ авторизацию: проверка role в ctx, lookup campaign по secret_token (404), lookup campaign_creator (403 — anti-fingerprint между «не зарегистрирован» и «не в кампании»).
- Service.ApplyDecision внутри `dbutil.WithTx`: `SELECT FOR UPDATE` row → switch по `(status, decision)`; audit в той же tx с `actor_id=NULL`, payload `{campaign_id, creator_id}`. Логи успеха — после `WithTx`.
- Handler: тонкий; regex-reject path-param `secret_token` → 404 до БД; AuthzService → Service. Никаких прямых repo-вызовов и сравнений ролей.
- Идемпотентность: `agreed`+agree, `declined`+decline → 200 + `already_decided=true`, без UPDATE и audit. `agreed`+decline / `declined`+agree / `planned`+любое → 422 granular code.
- TMA-фронт: openapi-fetch middleware инжектит `Authorization: tma <init-data>` через `@telegram-apps/sdk-react`; UI без status-проверок; `AcceptedView`/`DeclinedView` принимают `alreadyDecided: boolean`.
- PII в stdout запрещена (`init_data`, `secret_token`, `telegram_user_id` — никогда). `actor_id=NULL` в audit для T1/T2.
- Стандарты `docs/standards/` — hard rules. Coverage gate ≥80% per-method (handler/service/repo/middleware/authz).

**Ask First:**
- Если backfill SQL потребует Postgres-фич недоступных в проде/staging.
- Если в проде есть кампании с tma_url last-segment <16 chars и админам нужны их ссылки рабочими (вместо «обновите URL»).
- Любые отступления от response shape (`{status, already_decided}`) или header-name (`Authorization: tma`).

**Never:**
- Computed column `GENERATED ALWAYS AS … STORED` — extraction строго на бэке.
- LIKE для lookup secret_token.
- Auto-генерация tma_url/secret_token на бэке.
- Изменения admin-flow (chunks 11/13/15).
- TrustMe-триггер при `agreed` (chunks 16/17).
- Bypass-флаги middleware в production; HMAC-валидация всегда настоящая. Test-helper `sign-init-data` доступен только при `EnableTestEndpoints=true`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected | Error Handling |
|---|---|---|---|
| agree happy | invited + valid initData | 200 `{status:"agreed", already_decided:false}`; UPDATE + audit | — |
| decline happy | invited + valid initData | 200 `{status:"declined", already_decided:false}`; UPDATE + audit | — |
| agree/decline idempotent | terminal=decision + valid initData | 200 + `already_decided:true` | без UPDATE и audit |
| agree из declined | declined + valid | 422 | `CodeCampaignCreatorDeclinedNeedReinvite`, actionable «попросите переприглашение» |
| decline из agreed | agreed + valid | 422 | `CodeCampaignCreatorAlreadyAgreed`, «обратитесь к менеджеру» |
| любое из planned | planned + valid | 422 | `CodeCampaignCreatorNotInvited`, «дождитесь приглашения» |
| campaign soft-deleted / unknown | secret_token не найден | 404 | `CodeCampaignNotFound` |
| creator не в creators / не в campaign_creators | mismatch | 403 | `CodeTMAForbidden`, anti-fingerprint один текст |
| invalid initData | header missing/HMAC mismatch/auth_date истёк или >now | 401 | `CodeUnauthorized` generic |
| ранний reject token | path не matches `^[A-Za-z0-9_-]{16,}$` | 404 без БД | `CodeCampaignNotFound` |
| POST campaign valid | новый url с валидным last-segment | 201; secret_token в БД | — |
| POST campaign empty | TmaURL = "" | 201; secret_token=NULL | legacy/draft режим |
| POST/PATCH invalid | last-segment <16 / не URL-safe / битый URL | 422 | `CodeInvalidTmaURL`, actionable формат |
| POST/PATCH conflict | secret_token совпадает с живым | 422 | `CodeTmaURLConflict` через EAFP 23505 |

</frozen-after-approval>

## Code Map

- `backend/migrations/<ts>_campaigns_secret_token.sql` -- новая миграция: ADD column NULLABLE + backfill regex-фильтрованных last-segments + partial UNIQUE INDEX.
- `backend/internal/domain/campaign.go:114` -- расширить `ValidateCampaignTmaURL` + добавить `ExtractSecretToken`, `secretTokenRe`.
- `backend/internal/domain/errors.go:9-87` -- добавить коды (`CodeInvalidTmaURL`, `CodeTmaURLConflict`, `CodeCampaignCreatorNotInvited`, `CodeCampaignCreatorAlreadyAgreed`, `CodeCampaignCreatorDeclinedNeedReinvite`, `CodeTMAForbidden`, `CodeCampaignNotFound`, `CodeUnauthorized`) + sentinel errors.
- `backend/internal/domain/campaign_creator.go:12` -- `CampaignCreatorDecision`, `CampaignCreatorDecisionResult`.
- `backend/internal/repository/campaign.go:38,83-96` -- расширить `CampaignRow` (+ `SecretToken sql.NullString` с `db:"secret_token" insert:"secret_token"`); добавить `GetBySecretToken`; EAFP по новому constraint (`campaigns_secret_token_uniq`) в Create/Update.
- `backend/internal/repository/campaign_creator.go:66-225` -- `GetByIDForUpdate` (`SELECT … FOR UPDATE`) + `ApplyDecision` (UPDATE status/decided_at/updated_at).
- `backend/internal/repository/creator.go` -- проверить наличие `GetByTelegramUserID`; добавить если нет.
- `backend/internal/service/campaign.go:41-128` -- вызов `ValidateTmaURL` + `ExtractSecretToken` перед repo.Create/Update; пробрасывать secret_token (расширить сигнатуру repo).
- `backend/internal/service/tma_campaign_creator.go` -- новый: `TmaCampaignCreatorService.ApplyDecision`.
- `backend/internal/service/audit_constants.go:22-25` -- `AuditActionCampaignCreatorAgree`, `AuditActionCampaignCreatorDecline`.
- `backend/internal/authz/authz.go:1-22` -- `AuthorizeTMACampaignDecision(ctx, secretToken)`; расширить deps (campaign-repo, campaign-creator-repo).
- `backend/internal/middleware/tma_initdata.go` -- новый: HMAC + creator lookup; ctx-keys `TelegramUserIDKey`, `CreatorIDKey`, `CreatorRoleKey` + extractors.
- `backend/internal/handler/tma_campaign_creator.go` -- новый: `TmaAgree`, `TmaDecline`; regex-reject token.
- `backend/internal/handler/testapi.go:48-138` -- добавить `SignTMAInitData` (HMAC по `cfg.TelegramBotToken`).
- `backend/internal/config/config.go:66-73` -- `TMAInitDataTTL time.Duration` env (default 86400s).
- `backend/api/openapi.yaml` -- `/tma/campaigns/{secret_token}/agree|decline`, securityScheme `tmaInitData`, `TmaDecisionResult` schema, новые error codes.
- `backend/api/openapi-test.yaml` -- `POST /test/tma/sign-init-data` (`{telegram_user_id, auth_date?}` → `{init_data}`).
- `backend/cmd/api/main.go:91,143-157` -- DI wiring: `TmaCampaignCreatorService`, `tma_initdata` middleware на chi-group `/tma/*`, AuthzService получает campaign + campaign-creator repos, testapi handler include.
- `backend/e2e/testutil/tma.go` -- новый: `SignInitData(t, tgID, opts)`, `SetupCampaignWithInvitedCreator(t, ...)`.
- `backend/e2e/tma/tma_test.go` -- новый: agree/decline/idempotent/re-invite/422 granular/404/403/401.
- `backend/e2e/campaign/campaign_test.go` -- расширить: secret_token validation/conflict/empty (POST + PATCH).
- `frontend/tma/src/api/client.ts` + `middleware.ts` -- новые: openapi-fetch instance + initData injection через `@telegram-apps/sdk-react`.
- `frontend/tma/src/api/generated/schema.ts` -- regenerated.
- `frontend/tma/src/features/campaign/hooks/useDecision.ts` -- новые хуки `useAgreeDecision`/`useDeclineDecision` (react-query, double-submit guard).
- `frontend/tma/src/features/campaign/CampaignBriefPage.tsx:18-47` -- wire backend; убрать mock-only flow; ТЗ-hardcode остаётся.
- `frontend/tma/src/features/campaign/AcceptedView.tsx`, `DeclinedView.tsx` -- prop `alreadyDecided: boolean` + плашка `tma-already-decided-banner`.
- `frontend/tma/src/features/campaign/campaigns.ts:79-81` -- удалить mock `getCampaignByToken`; ТЗ-mapping остаётся.
- `frontend/tma/src/shared/i18n/ru.json` + `errors.ts` -- ru-translations campaign-decision + error mappings.
- `frontend/e2e/helpers/api.ts:706+` -- добавить `signInitDataForCreator`, `mockTelegramWebApp` (`page.addInitScript`).
- `frontend/e2e/tma/decision.spec.ts` -- новый: happy/repeat/422/401/404 flows.

## Tasks & Acceptance

**Execution** (порядок строгий: implementation → unit → self-check gate → e2e; backend и frontend идут независимо, каждый со своим gate):

*Backend implementation:*
- [ ] `backend/migrations/<ts>_campaigns_secret_token.sql` -- `make migrate-create NAME=campaigns_secret_token`; ADD column NULLABLE + regex-фильтрованный backfill + partial UNIQUE INDEX; Down: drop index + column.
- [ ] `backend/internal/domain/campaign.go` -- `ValidateTmaURL` (пустая ok; иначе `url.Parse` + last-segment regex), `ExtractSecretToken`, `lastSegment`, `secretTokenRe`.
- [ ] `backend/internal/domain/errors.go` -- новые коды + sentinel errors per Boundaries.
- [ ] `backend/internal/domain/campaign_creator.go` -- Decision/Result типы.
- [ ] `backend/internal/repository/campaign.go` -- `SecretToken sql.NullString` в Row; `GetBySecretToken`; EAFP secret_token UNIQUE в Create/Update; расширить сигнатуры.
- [ ] `backend/internal/repository/campaign_creator.go` -- `GetByIDForUpdate`, `ApplyDecision`.
- [ ] `backend/internal/repository/creator.go` -- `GetByTelegramUserID` (если нет).
- [ ] `backend/internal/service/campaign.go` -- инжект ValidateTmaURL + ExtractSecretToken; пробрасывать secret_token.
- [ ] `backend/internal/service/tma_campaign_creator.go` -- новый сервис; ApplyDecision (WithTx + SELECT FOR UPDATE + transition + audit).
- [ ] `backend/internal/service/audit_constants.go` -- 2 константы.
- [ ] `backend/internal/authz/authz.go` -- AuthorizeTMACampaignDecision; deps на campaign и campaign-creator repos.
- [ ] `backend/internal/middleware/tma_initdata.go` -- HMAC + creator lookup; ctx-keys + extractors; debug-логи без PII.
- [ ] `backend/internal/handler/tma_campaign_creator.go` -- TmaAgree/TmaDecline; regex-reject; 200/401/403/404/422.
- [ ] `backend/internal/handler/testapi.go` -- SignTMAInitData.
- [ ] `backend/internal/config/config.go` -- TMAInitDataTTL.
- [ ] `backend/api/openapi.yaml` + `openapi-test.yaml` -- описанные пути и схемы.
- [ ] `backend/cmd/api/main.go` -- DI wiring; chi-group `/tma/*` с middleware.
- [ ] `make generate-api` -- регенерация.

*Backend unit-тесты:*
- [ ] domain (`ValidateTmaURL`, `ExtractSecretToken` все edge), middleware (HMAC valid/invalid/expired/future, creator found/missing, db-error), authz (role missing, campaign 404, cc 403, happy), service.tma (вся таблица transitions, idempotent, audit-row only on real transition), service.campaign (validation/conflict пути), repo (`GetBySecretToken` SQL, EAFP 23505 specific constraint, `GetByIDForUpdate`, `ApplyDecision`), handler (regex-reject, captured-input ctx, response shape). Coverage gate ≥80% per-method (`make test-unit-backend-coverage`).

*Backend self-check gate — ОБЯЗАТЕЛЬНО перед e2e:*
- [ ] Поднять стек: `make compose-up && make migrate-up && make start-backend`. Через curl + `psql` пройти ключевые строки I/O matrix живой системой:
  - `POST /campaigns` с валидным tma_url → 201 + secret_token в БД (`SELECT secret_token FROM campaigns WHERE id=...`); пустой → 201 + NULL; невалидный (last-segment <16) → 422 `CodeInvalidTmaURL`; повторный с тем же URL → 422 `CodeTmaURLConflict`.
  - Setup: создать campaign + approved creator с известным `telegram_user_id` + A1 add + A4 notify через стандартные admin-ручки.
  - Получить initData через `POST /test/tma/sign-init-data`; передать в `Authorization: tma <init-data>` для T1.
  - T1 happy → 200 + `{status:"agreed", already_decided:false}`; в БД `psql`'ом проверить `status=agreed`, `decided_at IS NOT NULL`; в `audit_logs` ровно один row `campaign_creator_agree` c `actor_id=NULL` и payload `{campaign_id, creator_id}`.
  - T1 повтор → 200 + `already_decided=true`; в `audit_logs` всё ещё один matching row (не два).
  - T2 из agreed → 422 `CodeCampaignCreatorAlreadyAgreed`.
  - Header без `tma `-префикса / битый HMAC / истёкший `auth_date` / `auth_date > now` → 401 generic, в logs debug-причина без PII.
  - Soft-deleted campaign (DELETE /campaigns/{id}) → T1 → 404. Однобуквенный токен в URL → 404 (regex-reject; пометить, что обращения к БД нет — например, по логам).
  - **На расхождение со спекой:** баг реализации — fix code → перезапустить unit-тесты → перезапустить self-check. НЕ пропускать к e2e пока поведение не совпадёт. HALT — только на продуктовой развилке, не описанной в спеке.

*Backend e2e (только после passed self-check):*
- [ ] `backend/e2e/testutil/tma.go` -- `SignInitData`, `SetupCampaignWithInvitedCreator`.
- [ ] `backend/e2e/tma/tma_test.go` -- e2e suite (все строки I/O Matrix, строгие ассерты конкретными значениями).
- [ ] `backend/e2e/campaign/campaign_test.go` -- secret_token validation/conflict/empty (POST + PATCH) + concurrent INSERT race на 23505.

*Frontend implementation:*
- [ ] `frontend/tma/src/api/client.ts` + `middleware.ts` -- openapi-fetch + initData injection.
- [ ] `frontend/tma/src/features/campaign/hooks/useDecision.ts` -- mutation hooks.
- [ ] `frontend/tma/src/features/campaign/CampaignBriefPage.tsx` -- wire backend; убрать mock-функцию.
- [ ] `frontend/tma/src/features/campaign/AcceptedView.tsx`, `DeclinedView.tsx` -- alreadyDecided prop + плашка.
- [ ] `frontend/tma/src/features/campaign/campaigns.ts` -- удалить mock-функцию.
- [ ] `frontend/tma/src/shared/i18n/ru.json` + `errors.ts` -- переводы и маппинг.

*Frontend unit-тесты:*
- [ ] middleware (header injection), hooks (success/error mapping), CampaignBriefPage (loading/error/already_decided состояния), AcceptedView/DeclinedView (banner visibility c `alreadyDecided`).

*Frontend self-check gate через Playwright MCP — ОБЯЗАТЕЛЬНО перед e2e:*
- [ ] Поднять стек: `make start-tma` (тянет backend). Через Playwright MCP открыть TMA с моком initData (получив реальный подписанный init_data из backend `/test/tma/sign-init-data` для seeded creator), пройти ключевые сценарии:
  - happy agree → AcceptedView видим (`[data-testid="tma-accepted-view"]`), плашки `tma-already-decided-banner` нет.
  - reload + повтор agree → AcceptedView + плашка `tma-already-decided-banner` видна.
  - declined creator → click agree → toast `tma-decision-error` с actionable message (422).
  - стек без mockTelegramWebApp (нет initData) → click agree → toast 401.
  - Soft-deleted кампания → переход → toast 404 «Кампания недоступна».
  - **На расхождение со спекой:** баг реализации — fix code → перезапустить unit → перезапустить self-check. НЕ пропускать к e2e пока поведение не совпадёт. HALT — только на продуктовой развилке.

*Frontend e2e (только после passed self-check):*
- [ ] `frontend/e2e/helpers/api.ts` -- `signInitDataForCreator`, `mockTelegramWebApp` через `page.addInitScript`.
- [ ] `frontend/e2e/tma/decision.spec.ts` -- specs (happy/repeat/422/401/404 flows).

**Acceptance Criteria:**
- Given approved creator, привязанный к invited-кампании; when открывает TMA URL и кликает «Согласиться», then 200 + `already_decided=false`; в БД status=`agreed`, `decided_at` обновлён; audit-row `campaign_creator_agree` записан в той же tx с `actor_id=NULL` и payload `{campaign_id, creator_id}`.
- Given creator уже согласился; when кликает «Согласиться» повторно, then 200 + `already_decided=true`; БД не меняется; новый audit-row не пишется (за всю историю — ровно один `campaign_creator_agree`).
- Given admin POST'ит /campaigns с `tma_url`, у которого last-segment 5 chars; when запрос обработан, then 422 `CodeInvalidTmaURL` с actionable message; кампания не создана.
- Given две живые кампании с одинаковым secret_token (concurrent INSERT); then одна проходит, вторая получает 422 `CodeTmaURLConflict`; в БД одна запись.
- Given middleware получает initData с `auth_date` в будущем; then 401 generic; БД не запрашивается; в логах debug-причина без `init_data`/`secret_token`/`telegram_user_id`.
- Given path-token однобуквенный; when T1; then 404 без обращения к БД (regex-reject в handler).
- Given прод-кампания с пустым tma_url до миграции; after migration: secret_token остаётся NULL, кампания работает в админке без изменений; T1/T2 на её URL не применимы (URL пустой → нет открытия из бота).

## Design Notes

Полный концепт и обоснования — `_bmad-output/implementation-artifacts/intent-creator-campaign-decision.md`. Ключевое:

- **Identity vs Authz.** Middleware = identity (HMAC + lookup creator); AuthzService = role + resource. Симметрично с admin-flow (JWT middleware + AuthzService для admin). Service.ApplyDecision получает уже авторизованный auth-context.
- **Defence-in-depth для secret_token.** (1) regex validation в domain на POST/PATCH; (2) partial UNIQUE в БД; (3) точный `=` lookup; (4) regex-reject в handler до БД. Любой одиночный layer compromise не открывает атаку.
- **Идемпотентность симметричная.** Повторный клик из терминального статуса не создаёт audit-row — иначе `agreed→agree×N` плодит N events об одном решении.
- **Backfill safe для прода.** Кампании с пустым/невалидным tma_url остаются с `secret_token=NULL` (недоступны через TMA, в админке работают как раньше).

Domain-validation skeleton:
```go
func ValidateTmaURL(s string) error {
    if s == "" { return nil }                // legacy/draft ok
    u, err := url.Parse(s)
    if err != nil || u.Scheme == "" || u.Host == "" { return ErrInvalidTmaURL }
    if !secretTokenRe.MatchString(lastSegment(u.Path)) { return ErrInvalidTmaURL }
    return nil
}
```

## Verification

**Commands:**
- `make migrate-up && make build-backend` -- миграция и build без ошибок.
- `make generate-api` -- openapi регенерация без неожиданного diff.
- `make lint-backend` -- 0 findings.
- `make test-unit-backend && make test-unit-backend-coverage` -- green; coverage gate ≥80% per-method.
- `make test-e2e-backend` -- green; tma и campaign e2e suites проходят.
- `make lint-tma && make test-unit-tma` -- 0 findings, green.
- `make test-e2e-frontend` -- green; tma decision.spec проходит.

**Manual checks:**
- В Playwright MCP открыть TMA с моком initData (через `signInitDataForCreator`), пройти agree/decline/repeat-decision flows; проверить визуально и через DOM (`tma-accepted-view`, `tma-already-decided-banner`).

## Suggested Review Order

**Доменная модель (валидация и интегритет secret_token)**

- Регулярка с верхней границей и helper'ы для extract/validate — точка истины для валидного формата.
  [`campaign.go:21`](../../backend/internal/domain/campaign.go#L21)

- ValidateCampaignTmaURL: empty=ok (legacy), URL parse, regex на last segment.
  [`campaign.go:161`](../../backend/internal/domain/campaign.go#L161)

- Доменные ошибки + state-machine sentinel'ы для creator decision.
  [`campaign_creator.go:1`](../../backend/internal/domain/campaign_creator.go#L1)

**Слой данных: миграция, repo, partial UNIQUE**

- Up-миграция: NULLABLE column + regex backfill + partial UNIQUE WHERE is_deleted=false.
  [`20260508224533_campaigns_secret_token.sql:1`](../../backend/migrations/20260508224533_campaigns_secret_token.sql#L1)

- Repo Create/Update с EAFP 23505 → ErrTmaURLConflict для обоих constraints.
  [`campaign.go:107`](../../backend/internal/repository/campaign.go#L107)

- GetBySecretToken: точное `=`-сравнение + is_deleted=false фильтр (anti-suffix-attack).
  [`campaign.go:192`](../../backend/internal/repository/campaign.go#L192)

- GetByIDForUpdate + ApplyDecision (FOR UPDATE для serialise concurrent decisions).
  [`campaign_creator.go:1`](../../backend/internal/repository/campaign_creator.go#L1)

**Authn / Authz / Middleware**

- TMAInitDataFromScopes: HMAC + auth_date freshness + creator lookup → ctx; 401 anti-fingerprint.
  [`tma_initdata.go:1`](../../backend/internal/middleware/tma_initdata.go#L1)

- AuthorizeTMACampaignDecision: 4 gates с anti-fingerprint 403 (creator-not-registered ↔ creator-not-in-campaign).
  [`tma.go:1`](../../backend/internal/authz/tma.go#L1)

- TTL config: env name + comment про Go-Duration syntax (post-review fix).
  [`config.go:73`](../../backend/internal/config/config.go#L73)

**Decision flow: service + handler**

- ApplyDecision: WithTx + lock + decideTransition + symmetric idempotent no-op + audit.
  [`tma_campaign_creator.go:57`](../../backend/internal/service/tma_campaign_creator.go#L57)

- decideTransition: state machine для invited/agreed/declined/planned.
  [`tma_campaign_creator.go:114`](../../backend/internal/service/tma_campaign_creator.go#L114)

- Handler: regex-reject 404 ДО любого DB lookup → AuthzService → service.
  [`tma_campaign_creator.go:1`](../../backend/internal/handler/tma_campaign_creator.go#L1)

- DI wiring: middleware register + repo factory + сервисы.
  [`main.go:128`](../../backend/cmd/api/main.go#L128)

**OpenAPI + test endpoint**

- Path-param schema (16-256 regex), security scheme `tmaInitData`, granular error responses.
  [`openapi.yaml:1511`](../../backend/api/openapi.yaml#L1511)

- testapi: SignTMAInitData для e2e + middleware.SignTMAInitDataForTests.
  [`testapi.go:1`](../../backend/internal/handler/testapi.go#L1)

**Frontend TMA**

- API client + middleware (initData → Authorization header) с SDK + hash fallback.
  [`middleware.ts:1`](../../frontend/tma/src/api/middleware.ts#L1)

- React Query mutation хуки, derive `alreadyDecided`.
  [`useDecision.ts:1`](../../frontend/tma/src/features/campaign/useDecision.ts#L1)

- CampaignBriefPage: hooks wiring, AcceptedView/DeclinedView с already-decided баннером.
  [`CampaignBriefPage.tsx:1`](../../frontend/tma/src/features/campaign/CampaignBriefPage.tsx#L1)

**Tests**

- Backend e2e tma: full I/O matrix + audit-row idempotency assertion (post-review).
  [`tma_test.go:1`](../../backend/e2e/tma/tma_test.go#L1)

- Backend e2e campaign: TMA_URL_CONFLICT (POST/PATCH/race на partial UNIQUE secret_token).
  [`campaign_test.go:518`](../../backend/e2e/campaign/campaign_test.go#L518)

- TMA Playwright: signed initData injection + happy/idempotent/state-machine flows.
  [`decision.spec.ts:1`](../../frontend/e2e/tma/decision.spec.ts#L1)

**Deferred work**

- 11 находок ревью с обоснованием — почему не блокируют ship.
  [`deferred-work.md:1`](./deferred-work.md#L1)
