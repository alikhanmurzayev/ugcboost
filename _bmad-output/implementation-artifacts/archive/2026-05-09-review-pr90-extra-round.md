# Extra-round step-04 review — PR #90

**Ветка:** `alikhan/creator-campaign-decision`
**PR:** [#90 — feat(tma): creator decision flow + secret_token integrity](https://github.com/alikhanmurzayev/ugcboost/pull/90)
**Baseline:** `556fb09382f71eb04ea2e0d27db976719aa1b529`
**HEAD:** `eb89b3bc9c55cdb3669add8de13c5c35e1e73900`
**Объём diff'а:** 106 changed files, +8604 / −666

## Сводка по ревьюерам

| Ревьюер | Raw findings | Статус |
|---|---|---|
| Blind hunter | 12 | OK |
| Edge case hunter | 6 | OK |
| Acceptance auditor | 0 | clean |
| Test auditor | 28 | OK |
| Frontend codegen auditor | 4 | OK |
| Security auditor | 5 | OK |
| Manual QA | 0 | clean (11 user flows прошли) |
| **Итого raw** | **55** | |
| **После дедупа** | **~50 уникальных** | |

Спека прошла acceptance audit, manual QA не нашёл регрессий.

---

## Сводка по категориям

| Категория | Count | Действие |
|---|---|---|
| **bad_spec** (blocker) | 3 | Не отпускать в merge без фикса |
| **bad_spec** (major) | 7 | Обсудить (ship vs spec-amend + патч) |
| **bad_spec** (minor) | 1 | Решение по ситуации |
| **patch** (major) | 8 | Тривиально фиксится — пакетно |
| **patch** (minor) | 18 | Пакетно |
| **patch** (nitpick) | 2 | Опционально |
| **defer** | 3 | В `deferred-work.md` |
| **reject** | 6 | Drop silently |

---

## Bad_spec — снаружи `<frozen-after-approval>`

### Blocker

| ID | Тема | Файл / точка | Источник |
|---|---|---|---|
| BS-1 | **Migration backfill ломается на дубликатах `tma_url`** — два live-кампания с одинаковым last-segment → `CREATE UNIQUE INDEX` упадёт, deploy залипнет. Также backfill regex `{16,}$` без верхней границы (рассогласовано с runtime `{16,256}`). | `backend/migrations/20260508224533_campaigns_secret_token.sql:8-19` | edge-case + blind |
| BS-8 | **Manual `DecisionResult` type** дублирует `components["schemas"]["TmaDecisionResult"]` из codegen. Прямое нарушение `frontend-types.md` § «API-типы — только из кодогенерации». | `frontend/tma/src/features/campaign/useDecision.ts:13-16` | codegen |
| BS-9 | **Manual `DecisionError` type** вместо `components["schemas"]["APIError"]` из schema. | `frontend/tma/src/features/campaign/useDecision.ts:7-11` | codegen |

### Major

| ID | Тема | Файл / точка | Источник |
|---|---|---|---|
| BS-2 | **`secret_token` попадает в stdout-логи через `r.URL.Path`** — прямое нарушение `security.md` § PII. 4 call-site'а. | `backend/internal/middleware/logging.go:31`, `recovery.go:22`, `handler/response.go:25,100` | security |
| BS-3 | **HMAC bypass при пустом `TELEGRAM_BOT_TOKEN`** — нет fail-fast guard в startup. Если оператор поднимет API без BotToken (или скопирует staging-конфиг с TelegramMock=true в прод), middleware считает HMAC от пустой строки → любой подделает initData. | `backend/cmd/api/main.go:141-147`, `backend/internal/config/config.go:65-78` | blind |
| BS-4 | **Race AuthzService → ApplyDecision**: `GetByCampaignAndCreator` читает cc вне tx; admin может удалить cc в окне; `GetByIDForUpdate` ловит `sql.ErrNoRows` → `fmt.Errorf` оборачивает → попадает в default → **500 вместо 404/422**. | `backend/internal/authz/tma.go:51-58`, `backend/internal/service/tma_campaign_creator.go:74-78` | blind |
| BS-5 | **Soft-delete кампании между authz и tx** — `is_deleted=false` фильтр только в lookup; внутри `WithTx` нет re-check. Decision коммитится на soft-deleted кампанию, audit ссылается на inactive ресурс. | `backend/internal/service/tma_campaign_creator.go:62-97` | edge-case |
| BS-7 | **`/test/tma/sign-init-data` одиночный гейт** — endpoint минтит валидно подписанный init_data для **любого** `telegram_user_id` ключом prod-bot-token. Защита только через `cfg.EnableTestEndpoints` без двойного guard'а по `Environment != production`. | `backend/internal/handler/testapi.go:308-325`, `backend/cmd/api/main.go:158-164` | blind |
| BS-10 | **Хардкод `"agreed"/"declined"`** литералов в TMA — нет const-объекта (на web `shared/constants/campaignCreatorStatus.ts` есть). При смене enum в OpenAPI рассинхронизация молча. | `frontend/tma/src/features/campaign/useDecision.ts:18-25`, `CampaignBriefPage.tsx:44,47` | codegen |
| BS-11 | **`useMutation` без `onError`** — `useAgreeDecision` / `useDeclineDecision`. Ошибка показывается через `agree.error ?? decline.error` в JSX (работает), но не соответствует букве `frontend-api.md` § «Мутации — обработка ошибок обязательна». | `frontend/tma/src/features/campaign/useDecision.ts:61-87` | codegen |

### Minor

| ID | Тема | Файл / точка | Источник |
|---|---|---|---|
| BS-6 | **`getCampaignByToken` всегда возвращает `genericBrief(token)`** — `NotFoundPage` недостижим. Любой токен показывает легитимный flow с CTA + NDA gate; 404 от бэка приходит только на mutate. Фишинг-риск. | `frontend/tma/src/features/campaign/campaigns.ts:79-100`, `CampaignBriefPage.tsx:38-41` | blind |

---

## Patch — тривиально без human input

### Код / поведение

| ID | Sev | Тема | Файл / точка | Источник |
|---|---|---|---|---|
| P-1 | minor | Backfill regex `{16,}` → `{16,256}` для соответствия runtime | `migrations/20260508224533_*.sql:9-13` | blind |
| P-2 | minor | `middleware.ts` initData fallback — двойной `readInitDataFromHash()` (catch + post-if), оставить один путь | `frontend/tma/src/api/middleware.ts:21-32` | blind |
| P-3 | minor | `useDecision.callDecision` — guard `response?.status ?? 0` для network failures | `frontend/tma/src/features/campaign/useDecision.ts:35-58` | blind + edge |
| P-4 | minor | `middleware.ts` — `if (!initData) throw …` вместо тихой отправки unauth-запроса | `frontend/tma/src/api/middleware.ts:20-36` | edge |
| P-5 | minor | `decideTransition` default-branch для unknown decision value | `backend/internal/service/tma_campaign_creator.go:114-137` | edge |
| P-6 | minor | `respondError` — подтвердить (или дописать) ветки маппинга для новых ValidationError'ов | `backend/internal/handler/response.go:88-90` | blind |
| P-7 | nitpick | `CampaignCreatorGroupSection.tsx` — `onRemove` стал optional, проверить call-sites и условный рендер trash | `frontend/web/.../CampaignCreatorGroupSection.tsx:34` | blind |

### Тесты — major

| ID | Тема | Файл / точка |
|---|---|---|
| T-1 | handler `mock.Anything` → exact `service.TmaDecisionAuth` в негативных кейсах | `backend/internal/handler/tma_campaign_creator_test.go:103,118,192,212` |
| T-2 | service audit-payload через captured-input (`mock.Run`) | `backend/internal/service/tma_campaign_creator_test.go:61,78` |
| T-3 | full struct equal в 4 repo-тестах (`GetByIDForUpdate.success`, `ApplyDecision.success`, `GetBySecretToken.success`, soft-deleted) | `backend/internal/repository/{campaign,campaign_creator}_test.go` |
| T-4 | `freshValidTmaURL` → `testutil/` (3 дубликата) | `backend/e2e/{campaign,campaign_creator,creator_applications}/*_test.go` |
| T-5 | tma e2e: HMAC-mismatch case (есть unit, нет e2e) | `backend/e2e/tma/tma_test.go:171-205` |
| T-6 | tma e2e: anti-fingerprint comparison (not-registered vs not-in-campaign vs invalid-initData) | `backend/e2e/tma/tma_test.go` (отсутствует) |
| T-7 | tma e2e: deleted-campaign 404 ветка | `backend/e2e/tma/tma_test.go` (отсутствует) |
| T-8 | handler: idempotent-agree case (симметричен idempotent-decline) | `backend/internal/handler/tma_campaign_creator_test.go:26-128` |

### Тесты — minor / nitpick

| ID | Sev | Тема | Файл |
|---|---|---|---|
| T-9 | minor | `.Maybe()` removal в service rig | `service/tma_campaign_creator_test.go:33-49` |
| T-10 | minor | middleware: body-assert для anti-fingerprint в негативных | `middleware/tma_initdata_test.go:151-198` |
| T-11 | minor | 13 `TestTMA*` функций → `t.Run` внутри одной | `middleware/tma_initdata_test.go` |
| T-12 | minor | tma e2e: typed unmarshal вместо `Contains` | `backend/e2e/tma/tma_test.go:21-50` |
| T-13 | minor | tma e2e: header-comment лжёт про HMAC mismatch / wrong scheme | `backend/e2e/tma/tma_test.go:15-19` |
| T-14 | minor | `useDecision.test.tsx`: shared `mockedPost` между describe | `frontend/tma/.../useDecision.test.tsx:14-15` |
| T-15 | minor | `middleware.test.ts`: проверка return value `onRequest` | `frontend/tma/src/api/middleware.test.ts:18,35,48` |
| T-16 | minor | `decision.spec.ts`: header нумерованный список (запрещено стандартом) | `frontend/e2e/tma/decision.spec.ts:1-25` |
| T-17 | minor | `decision.spec.ts`: `acceptNda` race | `frontend/e2e/tma/decision.spec.ts:170-180,208-218` |
| T-18 | minor | `decision.spec.ts`: text-based assert ошибки → testid | `frontend/e2e/tma/decision.spec.ts:213` |
| T-19 | minor | admin-campaigns: `secret_token` формат с дефисом + label-suffix лишний | `frontend/e2e/web/admin-campaign-*.spec.ts` |
| T-20 | minor | service: `decideTransition` unexpected-status fallback тест | `service/tma_campaign_creator.go:138` |
| T-21 | nitpick | `TestExtractSecretToken`: edge `https://` | `domain/campaign_test.go:299` |

---

## Defer — pre-existing / не caused by story

| ID | Sev | Тема |
|---|---|---|
| D-1 | minor | Admin `Remove` vs creator `ApplyDecision` race — `Remove` не использует `FOR UPDATE`, race окно существовало до PR'а, новый flow его обнажил |
| D-2 | minor | Rate-limiting на публичных `/tma/campaigns/*/agree\|decline` отсутствует |
| D-3 | nitpick | Audit row enrichment (previous_status, IP, UA) для T1/T2 |

---

## Reject — drop silently

| ID | Тема | Почему |
|---|---|---|
| R-1 | Timing leak в `AuthzService` (early-return без DB при `creatorID==""` vs два SQL после) | Спека требует anti-fingerprint только для 403, поверхность мала (атакующий узнаёт только про себя) |
| R-2 | Двойной env-guard для `/test/tma/sign-init-data` (security nitpick) | `cfg.EnableTestEndpoints` уже зависит от `Environment` в `config.Load()` |
| R-3 | Race-test на partial unique secret_token unit-уровень | Закрыт e2e (`TestCreateCampaign_RaceUniqueSecretToken`), стандарт требует именно e2e |
| R-4 | `useDecision.test.tsx` codes vs текст | Не gap, чисто информационная пометка test-auditor'а |
| R-5 | Race-test name suffix | Не gap |
| R-6 | Captured-input для empty tmaUrl в handler/campaign_test | Не gap (нет middleware-derived поля) |

---

## Артефакты Manual QA

Скриншоты в `.playwright-mcp/`:
- `qa-tma-01-brief.png`, `qa-tma-02-accepted.png` — agree happy path
- `qa-tma-03-declined.png` — decline happy path
- `qa-tma-04-already-agreed.png`, `qa-tma-05-already-declined.png` — idempotent banner
- `qa-tma-06-real-brief-top.png`, `qa-tma-07-real-brief-bottom.png`, `qa-tma-08-real-brief-middle.png` — sticky CTA
- `qa-tma-09-422-error-already-agreed.png` — 422 inline error
- `qa-web-01-campaign-mixed.png`, `qa-web-02-campaigns-list.png`, `qa-web-03-agreed-only-no-trash.png` — web trash gating

---

## Что предлагаю дальше

1. **Сначала bad_spec**: blocker'ы BS-1, BS-8, BS-9 — не отпускать в merge без фикса. Major'ы BS-2/3/4/5/7/10/11 — обсудить (ship vs spec-amend + патч).
2. **Patch'и P-1…P-7 + T-1…T-21** — пакетно после решения по bad_spec'у.
3. **Defer D-1/D-2/D-3** — допиши в `deferred-work.md`.
4. **Reject** — drop silently.
