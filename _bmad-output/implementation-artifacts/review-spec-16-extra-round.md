---
title: 'Чанк 16 — extra-round review findings'
type: review
created: '2026-05-09'
spec: spec-16-trustme-outbox-worker.md
baseline_commit: c570a5a2310fb202271c581a04eada3af7709027
reviewers:
  - blind-hunter
  - edge-case-hunter
  - acceptance-auditor
  - test-auditor
  - frontend-codegen-auditor (clean)
  - security-auditor
  - manual-qa (clean — backend-only)
---

> Findings от 6 ревьюверов (frontend-codegen и manual-qa отчитались clean).
> Кластеризация по корневой причине, дедуп пересечений между ревьюверами.
> Классификация: `intent_gap` / `bad_spec` / `patch` / `defer` / `reject`.

## Cluster A — Backoff incomplete для Phase 2/Phase 0 failure paths (BLOCKER)

**Корень:** только Phase 2c send-fail и Phase 0 resend send-fail вызывают `recordFailedAttempt`. Остальные failure-пути (Phase 2a render fail, Phase 2b persist fail, Phase 0 search non-NotFound error, Phase 0 GetOrphanRequisites NotFound, Phase 0 orphan без `unsigned_pdf`) логируют + return без обновления `next_retry_at`. Один poison row → log spam каждые 10 секунд + N×TrustMe search RPS впустую.

**Findings:**
- Edge#1 [blocker] Phase 0 search transient error → infinite tight loop
- Blind#1 [major] Phase 2a/2b fail → poisoned row, no backoff
- Edge#4 [major] Panic in processClaim → batch остаётся orphan без PDF
- Edge#5 [major] Render-fail без backoff
- Edge#7 [major] GetOrphanRequisites NotFound (FK ON DELETE SET NULL) → вечный retry
- Edge#8 [major] Empty creators.iin/phone → TrustMe 1219 forever без специфичного backoff
- Acc#1 [minor] Phase 0 orphan без unsigned_pdf — log+skip вместо render+persist+send (отклонение от Decision #10)

**Класс:** `bad_spec`. Spec и intent-v2 Decision #10 описывают only happy/known/resend paths; не задают единое правило «любой failure → recordFailedAttempt». Spec должна чётко требовать: каждый return из processClaim/recoverOne по error-пути идёт через `recordFailedAttempt`, специфический backoff для validation-кейсов (24h), специфический marker для abandoned (FK NULLed).

## Cluster B — Audit row entity_id/entity_type mismatch (BLOCKER)

**Finding:** Blind#2 [major] `EntityType="campaign_creator"`, но `EntityID=contractID` (UUID контракта, не campaign_creator). Запросы по `(entity_type, entity_id)` не найдут строки.

**Файл:** `backend/internal/contract/sender_service.go:359-368` (recordAudit).

**Класс:** `patch`. Решение: либо `EntityID=ccID + EntityType="campaign_creator"`, либо `EntityType="contract" + EntityID=contractID`. Декида — что считается owning entity.

## Cluster C — Phase 0 race → duplicate notify + duplicate audit (MAJOR)

**Finding:** Edge#2 [major] / Blind#11 [minor] — Variant A документирует accept'нутый риск дублей в TrustMe, но не в нашей БД. `UpdateAfterSend` идемпотентный (n=0 без error), но код после неё пишет audit и шлёт `notifyCreator` независимо от n. → 2 worker'а Phase 0 на один orphan = 2 audit_logs row + 2 Telegram-сообщения креатору.

**Файлы:** `sender_service.go:178-204` (`finalizeKnownOrphan`), `:206-236` (`resendOrphan`), `repository/contracts.go:312-331` (`UpdateAfterSend`).

**Класс:** `intent_gap`. Frozen «Boundaries & Constraints» intent'а говорит «Бот-уведомление ПОСЛЕ Tx (fire-and-forget)», но не описывает идемпотентность для бот-нотификации и audit на race. Нужно решение: либо `UpdateAfterSend` возвращает `(int64, error)` и финализация скипается при n=0, либо `creator_notified_at` колонка + skip.

## Cluster D — Hand-rolled mocks вместо mockery (BLOCKER)

**Finding:** Test#1, Test#2 [blocker]
- `internal/contract` отсутствует в `.mockery.yaml` — `sender_service_test.go` использует hand-rolled `stubRenderer`, `stubTrustMe`, `stubNotifier`, `stubResolver`, `repoStubFactory`.
- `internal/handler/trustmeport` отсутствует в `.mockery.yaml` — `testapi_trustme_test.go` использует `fakeRunner`, `fakeSpyStore`.

Стандарт `backend-testing-unit.md:36`: «mockery + testify. Ручные моки запрещены». → blocker по стандарту.

**Файлы:** `backend/.mockery.yaml`, `internal/contract/sender_service_test.go:23-87`, `internal/handler/testapi_trustme_test.go:23-65`.

**Класс:** `patch`. Подключить пакеты в mockery, regen, заменить stubs.

## Cluster E — Test assertions (BLOCKER + MAJOR)

**Findings:**
- Test#3 [blocker] Captured-input не сделан для audit row (нет `JSONEq` на `NewValue` со всеми ключами payload, `EntityID/EntityType/ActorRole/ActorID` не проверены).
- Test#4 [blocker] `mock.AnythingOfType("time.Time")` для `RecordFailedAttempt`'s `nextRetryAt` — должен быть точный `fixedNow.Add(retryBackoff)` через инжекцию `s.now`.
- Test#5 [major] `mock.MatchedBy` на `audit.Action` теряет проверку остальных полей.
- Test#6 [major] PDF render тестируется через substring-search, не через геометрический assert (overlay реально перекрыл placeholder?).
- Test#11, #18 [major] `RunOnce_HappyPath` не проверяет `IssuedDate` / `notifier.calls[0].chatID`.
- Test#12, #13, #15 [major] `TestRealClient_SendToSign` substring search вместо multipart parsing; `TestSpyClient` — поля по одному, не структурой целиком; `TestTrustMeSpyList` — raw `map[string]any`, не сгенерированные типы.
- Test#14 [major] `_NoRunner_ReturnsValidation` — `NotEqual(204)` слабый assert, имя врёт (404, не 422).
- Test#16, #17 [major] `RunsAndReturns204` не проверяет ctx propagation; `SkipMissingValues` не проверяет behavior.

**Класс:** `patch`. Все жёсткие — переписать assert'ы под стандарт.

## Cluster F — Test coverage gaps (MAJOR)

**Findings:**
- Test#7-#10, #34 [major] не покрыты branches: `Query` error, `Scan` error, `rows.Err()` non-nil в `SelectAgreedForClaim` / `SelectOrphansForRecovery` / `GetOrphanRequisites`.
- Test#8 [major] `RecordFailedAttempt` не покрывает «propagates other errors» (только success + sql.ErrNoRows).
- Test#10 [major] `TestRealRenderer` не покрывает ~7 error-ветвей (`extract`, `tempfile`, `add font`, `set font`, `Cell`, `WriteTo`, `readNumPages`).
- Test#26 [major] Phase 1 claim Tx не покрывает `Insert` / `UpdateContractIDAndStatus` / `Select` error inside Tx.

**Класс:** `patch`. Coverage gate (≥80% per-method) скорее всего поймает в CI, но findings конкретно перечисляют.

## Cluster G — E2E test issues (MAJOR)

**Findings:**
- Test#19 [major] Header-комментарий — bullet-list `Happy:` / `Empty template:` / `Soft-deleted:` etc., не нарратив (стандарт `backend-testing-e2e.md:91`).
- Test#20 [major] Raw HTTP в `tmaPostAgree` вместо сгенерированного клиента.
- Test#21 [major] Happy path не проверяет `cc.status='signing'` и `cc.contract_id != nil` (комментарий говорит «verify cc status flipped to signing», реально только `NotEmpty(CampaignCreatorID)` setup-data).
- Test#22 [major] Send-fail-recovery не проверяет `last_error_code`/`next_retry_at`/`last_attempted_at` колонки в БД.
- Test#23, #24 [major] Race: фильтрация spy-list по `len > 0` / `>= 2 attempts` под параллельными e2e — может выбрать чужую запись из другого теста. Должно фильтроваться по конкретному `cc.ContractID`.
- Blind#3 [major] `spyFailNext("", 1)` wildcard под параллельным cron'ом не гарантирует консистентность.

**Класс:** `patch`. Заменить filter на based-on contract_id, переписать header в narrative, использовать сгенерированный клиент в `tmaPostAgree`, добавить assertions на статус и last_error_*.

## Cluster H — PDF overlay длина value vs bbox (MAJOR)

**Finding:** Edge#6 [major] `Render` не проверяет `len(value) * charWidth <= ph.XMax-ph.XMin`. Длинное ФИО («Иванов-Петров Александр Константинович» ~40 chars) или ИИН с пробелами наезжает на соседнее поле — креатор подписывает визуально сломанный договор → legal risk.

**Класс:** `bad_spec`. Intent-v2 Decision #12-13 не описывает overflow-policy. Spec должна сказать: warn-and-continue / truncate-with-ellipsis / error-and-recordFailedAttempt.

## Cluster I — auto_sign deviation от Decision #6 (MINOR)

**Finding:** Acc#3 [minor] `auto_sign=0` хардкоднут, расхождение с Decision #6 («auto-sign после клиента»). Документировано в коде комментарием.

**Класс:** `defer`. Открытый вопрос с TrustMe-side активацией; на Decision #6 нужен `Open Forks` либо TODO до фактической активации.

## Cluster J — Security minor (MINOR)

**Findings:**
- Sec#1 / Blind#7 / Acc#5 [minor] Raw `sendErr.Error()` в `last_error_message` — TrustMe blueprint не эхо-инжектирует payload, но HTTP-уровень от nginx/cloudflare может вернуть echoed paths. Длина не ограничена.
- Sec#2 [minor] `PdfBase64` в `/test/trustme/spy-list` содержит rendered PDF с PII. Endpoint защищён `EnableTestEndpoints` (production=false), но Cloudflare Access на staging открывает канал.
- Sec#3 [nitpick] `documentID` в URL path без `url.PathEscape`.

**Класс:** `patch` (по одному per finding):
- (a) В `truncate()` — парсить JSON, доставать только `errorText` поле, или сохранять только `Error.Code` в БД.
- (b) Заменить `PdfBase64` на `PdfSha256` в spy-list response.
- (c) Обернуть `documentID` в `url.PathEscape`.

## Cluster K — Operational/scaling concerns (MINOR)

**Findings:**
- Edge#3 [major] Graceful shutdown игнорирует `shutdownCtx.Deadline()` — `cl.Add("cron")` callback ждёт `scheduler.Stop()` до конца текущего job, может зависнуть до 60s × claimBatchSize. Под k8s default `terminationGracePeriodSeconds=30` → SIGKILL до завершения.
- Edge#13 [minor] Cron `@every 10s` × N pod'ов = N параллельных Phase 0 tick'ов. SkipIfStillRunning локальный, не глобальный. При rate-limit 4 RPS и 100 контрактов риск приближения к границе.
- Edge#11 [minor] Нет `UNIQUE` partial-index на `cc.contract_id` — теоретически возможен 2:1 mapping cc → contract.
- Blind#10 [minor] Partial index `idx_contracts_orphan` не покрывает `next_retry_at` ORDER BY.
- Blind#13 [minor] `retryBackoff=0` (CI default) → defeats backoff entirely; на проде с минусным timezone возможно clock skew.

**Класс:** `defer` (Edge#3 — `bad_spec` если хочется довести в этом PR). Зависят от объёма prod-traffic; на MVP scale (~100 контрактов/EFW) не критично.

## Cluster L — Other minor findings (MINOR)

- Acc#2 [minor] Phone normalization в caller, не в `TrustMeClient.SendToSign` — расхождение с буквой Decision #13.
- Acc#4 [minor] `ContractRow` без поля `UnsignedPDFContent` — by design (BYTEA не тащим в default SELECT), но без явного комментария-конвенции.
- Edge#9 [minor] Phase 0 search Page:1 без пагинации.
- Edge#10 [minor] Empty `data.URL` в TrustMe response не валидируется.
- Edge#12 [minor] tempfile cleanup игнорирует Remove error.
- Blind#5 [minor] SpyStore re-slice leak (старый PDFBase64 в backing array до GC).
- Blind#6 [minor] `documentIDFromAdditionalInfo` использует `sum[:5]` — birthday-paradox риск на dev/spy.
- Blind#8 [minor] Fingerprint comment 16-hex-chars vs `sum[:8]` — текстовая неточность.
- Blind#9 [minor] `FOR UPDATE SKIP LOCKED` в `SelectAgreedForClaim` — нет runtime-check что вызвано из Tx.
- Blind#12 [minor] `PDFBase64` в spy-ring без cap — потенциально ~1GB RAM на staging при 5000 records.

**Класс:** mix `patch` / `defer` / `reject`. Большинство `defer`.

## Cluster M — Nitpicks

- Test#29-#33 [minor] `t.Fatalf` вместо `require.Equal` (phone_test, contract_date_test); `Test{Struct}_{Method}` стиль для handler/render тестов; FIFO порядок не проверен в spy ring; race-mutex на `received*` closure-vars в httptest.
- Blind#14-#17 / Acc#6-#7 / Edge#14 — мелкие naming/comment/docs.

**Класс:** `reject` (в большинстве) / `patch` (где исправляется одной правкой).

---

# Рекомендация по приоритетам

| Cluster | Severity | Класс | Решение в этом PR? |
|---------|----------|-------|---------------------|
| A — backoff incomplete | blocker | bad_spec | **Да.** Spec amend → перезапустить implement. |
| B — audit entity mismatch | blocker | patch | **Да.** Однострочная правка. |
| C — Phase 0 race duplicate | major | intent_gap | **Решить с человеком.** Idempotent guard или accept duplicate. |
| D — handrolled mocks | blocker | patch | **Да.** mockery regen + замена stubs. |
| E — test assertions | blocker+major | patch | **Да.** |
| F — coverage gaps | major | patch | **Да.** |
| G — e2e issues | major | patch | **Да.** |
| H — bbox overflow | major | bad_spec | **Решить с человеком.** Spec policy на overflow. |
| I — auto_sign deviation | minor | defer | Backlog (TrustMe-side активация). |
| J — security minor | minor | patch | **Да.** |
| K — operational | minor (Edge#3 major) | defer | Backlog (зависит от scale). |
| L — other minor | minor | mix | По выбору. |
| M — nitpicks | nitpick | reject | Drop. |

**Выводы по step-04:**
- 2 `bad_spec` (Cluster A + Cluster H) и 1 `intent_gap` (Cluster C) → формально trigger loopback на step-02 plan.
- Альтернатива (для startup-MVP в активной итерации): зафиксировать findings, патчить `bad_spec`/`patch` в текущем PR без полного цикла re-derive, переходить дальше.
