---
title: 'GH-39: Корректный client IP за Cloudflare'
type: 'bugfix'
created: '2026-05-01'
status: 'done'
baseline_commit: '5a588a9c012953193caff7f388b007e33bd59cbf'
context:
  - docs/standards/security.md
  - docs/standards/backend-testing-unit.md
  - docs/standards/backend-testing-e2e.md
---

## Перед началом работы (для агента-исполнителя)

Перед любыми правками **полностью** загрузить в контекст:

- Все файлы из `docs/standards/` (на момент создания: `backend-architecture.md`, `backend-codegen.md`, `backend-constants.md`, `backend-design.md`, `backend-errors.md`, `backend-libraries.md`, `backend-repository.md`, `backend-testing-e2e.md`, `backend-testing-unit.md`, `backend-transactions.md`, `frontend-*.md`, `naming.md`, `review-checklist.md`, `security.md`). Если появились новые — тоже взять.
- `CLAUDE.md` в корне репозитория.

Без выборочного чтения / grep'а — стандарты применяются как hard rules.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** В цепочке `Cloudflare → VPS → Dokploy → Docker → backend` клиентский IP резолвится неправильно: `chi/middleware.RealIP` (используется в `middleware/client_ip.go`) знает только `True-Client-IP`, `X-Real-IP`, `X-Forwarded-For` и игнорирует `CF-Connecting-IP` — авторитетный заголовок Cloudflare. В результате `audit_logs.ip_address` и structured-логи содержат IP edge-узла или Dokploy-прокси вместо реального клиента.

**Approach:** Заменить re-export `chi.RealIP` собственным middleware с приоритетной цепочкой `CF-Connecting-IP → True-Client-IP → X-Real-IP → X-Forwarded-For (leftmost) → r.RemoteAddr`. Каждый кандидат валидируется `net.ParseIP`; первый валидный выигрывает и переписывается в `r.RemoteAddr`. Параллельно `middleware/logging.go` переключаем на `ClientIPFromContext`, чтобы structured-лог совпадал с audit.

## Boundaries & Constraints

**Always:**
- Заголовки читаются строго в порядке: `CF-Connecting-IP` → `True-Client-IP` → `X-Real-IP` → `X-Forwarded-For` (первый элемент списка) → `r.RemoteAddr` (host без порта).
- Невалидный (`net.ParseIP` returns nil) кандидат — пропускается, не блокирует следующий.
- Имена заголовков — экспортированные константы в `middleware/`, не литералы.
- Резолвленный IP записывается обратно в `r.RemoteAddr` (совместимо с текущим поведением `chi.RealIP`).
- Test-seam `WithClientIP` (Telegram bot) сохраняется без изменений.

**Ask First:**
- Если потребуется application-уровневый whitelist trusted-proxy CIDR — отдельная задача, **HALT** и согласовать с Alikhan.

**Never:**
- Не вводить runtime-конфиг trusted-proxy / fetch Cloudflare-ranges — security обеспечивается сетевым firewall (см. memory `project_prod_firewall.md`).
- Не менять схему `audit_logs.ip_address` — это правка middleware, не миграция.
- Не делать BREAKING change сигнатуры `testutil.PostRaw` — расширяем через variadic-опции, существующие call-sites не ломаем.

## I/O & Edge-Case Matrix

| Scenario | Headers + RemoteAddr | Expected `ClientIPFromContext` |
|---|---|---|
| CF-Connecting-IP wins | `CF-Connecting-IP: 203.0.113.7`, `X-Forwarded-For: 1.2.3.4`, `RemoteAddr: 172.18.0.5:33000` | `203.0.113.7` |
| True-Client-IP if no CF | `True-Client-IP: 5.6.7.8`, `X-Forwarded-For: 1.2.3.4` | `5.6.7.8` |
| X-Real-IP if no CF/TC | `X-Real-IP: 9.9.9.9`, `RemoteAddr: 127.0.0.1:1234` | `9.9.9.9` |
| X-Forwarded-For leftmost | `X-Forwarded-For: 1.2.3.4, 5.6.7.8` | `1.2.3.4` |
| Все headers невалидные | `CF-Connecting-IP: not-an-ip`, `X-Real-IP: bogus`, `RemoteAddr: 10.0.0.1:5555` | `10.0.0.1` |
| Headers пусты | `RemoteAddr: 10.0.0.1:5555` | `10.0.0.1` |
| RemoteAddr без порта | `RemoteAddr: 10.0.0.1` | `10.0.0.1` |
| IPv6 | `CF-Connecting-IP: 2001:db8::1`, `RemoteAddr: [::1]:443` | `2001:db8::1` |

</frozen-after-approval>

## Code Map

- `backend/internal/middleware/client_ip.go` — заменить `var RealIP = chimw.RealIP` на функцию-middleware с приоритетной цепочкой; экспортировать header-константы (`HeaderCFConnectingIP`, `HeaderTrueClientIP`, `HeaderXRealIP`, `HeaderXForwardedFor`); удалить импорт `chi/middleware` если осиротеет; обновить doc-comments.
- `backend/internal/middleware/client_ip_test.go` — добавить `TestRealIP` table-driven по матрице; существующие `TestClientIP`/`TestWithClientIP` оставить.
- `backend/internal/middleware/logging.go` — `remote_addr: r.RemoteAddr` → `client_ip: middleware.ClientIPFromContext(r.Context())`. Имя поля переименовать в `client_ip`.
- `backend/internal/middleware/logging_test.go` — провязать через `WithClientIP` или RealIP+ClientIP цепочку; assert на новый log-key.
- `backend/internal/handler/constants.go` — удалить `HeaderXForwardedFor`/`HeaderXRealIP` (переехали в middleware; в handler не используются).
- `backend/e2e/testutil/raw.go` — добавить variadic-опции `RawOption` + `WithHeader(key, value)`; backward-compat сохраняется (опции не обязательны).
- `backend/e2e/testutil/audit.go` — добавить `FindAuditEntry(...) *apiclient.AuditLogEntry` (возвращает запись по action, fail-fast если нет); `AssertAuditEntry` оставить как тонкую обёртку поверх Find.
- `backend/e2e/creator_application/creator_application_test.go` — добавить `TestSubmitCreatorApplicationCFIP`: POST с заголовком `CF-Connecting-IP: 203.0.113.7` через raw → audit row через `FindAuditEntry` → assert `ipAddress == "203.0.113.7"`.

## Tasks & Acceptance

**Execution:**
- [x] `backend/internal/middleware/client_ip.go` -- реализовать `RealIP(next http.Handler) http.Handler` с приоритетной цепочкой, экспортировать header-константы, обновить doc-comments. -- Закрывает корневую причину неправильного резолва.
- [x] `backend/internal/middleware/client_ip_test.go` -- добавить `TestRealIP` table-driven (8 кейсов матрицы); существующие тесты оставить. -- Покрывает все ветки приоритета.
- [x] `backend/internal/middleware/logging.go` -- переключить с `r.RemoteAddr` на `ClientIPFromContext`; переименовать log-key в `client_ip`. -- Унификация с audit_logs.
- [x] `backend/internal/middleware/logging_test.go` -- обновить тест под новый log-key и источник IP из ctx. -- Сопровождение правки в logging.go.
- [x] `backend/internal/handler/constants.go` -- удалить осиротевшие `HeaderXForwardedFor`/`HeaderXRealIP`. -- Источник правды по header-именам — `middleware/`.
- [x] `backend/e2e/testutil/raw.go` -- добавить variadic `RawOption` + `WithHeader(key, value)` для PostRaw. -- Чтобы e2e-тест мог послать `CF-Connecting-IP`.
- [x] `backend/e2e/testutil/audit.go` -- добавить `FindAuditEntry(...) *apiclient.AuditLogEntry`; `AssertAuditEntry` ужать до обёртки. -- Доступ к полям audit-row для inspection IP.
- [x] `backend/e2e/creator_application/creator_application_test.go` -- добавить `TestSubmitCreatorApplicationCFIP`. -- E2E-проверка полной цепочки HTTP → middleware → service → DB → API для `CF-Connecting-IP`.
- [x] `backend/e2e/testutil/const.go` -- добавить константу `HeaderCFConnectingIP` (e2e module не импортирует internal/, нужен локальный источник правды). -- Out-of-spec расширение, обнаружено при имплементации e2e теста.

**Acceptance Criteria:**
- Given HTTP-запрос с `CF-Connecting-IP: 203.0.113.7` и `X-Forwarded-For: 1.2.3.4`, when цепочка `RealIP → ClientIP` отработала, then `ClientIPFromContext(ctx) == "203.0.113.7"` и structured-лог `client_ip == "203.0.113.7"`.
- Given mutation-ручка в этом же запросе, when audit-row создан через `service/audit.go writeAudit`, then `audit_logs.ip_address == "203.0.113.7"`.
- Given запрос с невалидным `CF-Connecting-IP: garbage` и валидным `X-Real-IP: 9.9.9.9`, when middleware отработал, then `ClientIPFromContext(ctx) == "9.9.9.9"`.
- Given Telegram-bot путь (`WithClientIP(ctx, "telegram-bot")`), when audit пишется, then `ip_address == "telegram-bot"` — backward-compat сохранён.
- Given e2e-запрос на `POST /creators/applications` с `CF-Connecting-IP: 203.0.113.7`, when тест читает audit-row через `ListAuditLogs`, then `entry.IpAddress == "203.0.113.7"`.
- `make lint-backend`, `make test-unit-backend`, `make test-unit-backend-coverage`, `make test-e2e-backend` — все зелёные.

## Spec Change Log

### 2026-05-01 — review iteration 1 (patches, no spec revert)

- **Trigger:** acceptance-auditor flagged literal Code Map deviation — `client_ip_test.go` Code Map said «существующие `TestClientIP`/`TestWithClientIP` оставить», but the executor consolidated `TestClientIP`'s three subtests into `TestRealIP` table-driven cases. **Recorded:** consolidation is intentional; functional coverage of all three original scenarios (XFF leftmost, X-Real-IP overrides, fallback to RemoteAddr) preserved as named rows in `TestRealIP`. KEEP: do not re-introduce `TestClientIP` separately — duplication, not coverage.
- **Trigger:** acceptance-auditor flagged that AC #3 («невалидный CF + валидный X-Real-IP → 9.9.9.9») has no first-class row in the I/O & Edge-Case Matrix even though the AC is explicit. **Recorded:** I/O Matrix row «Все headers невалидные» does not cover the skip-on-invalid-then-pick-next branch. Patch added explicit `TestRealIP` case `skip invalid CF, fall to valid X-Real-IP`. KEEP: AC #3 must always have a dedicated row; future matrix edits must mirror every Acceptance Criteria scenario one-to-one.
- **Trigger:** out-of-spec extension — `backend/e2e/testutil/const.go` added `HeaderCFConnectingIP` constant. **Recorded:** required because the e2e module is physically isolated from `backend/internal/` (separate Go module), so the middleware-side `HeaderCFConnectingIP` is unreachable from e2e tests. KEEP: e2e mirror constants live under `backend/e2e/testutil/`, never imported from internal.
- **Trigger:** blind-hunter + edge-case-hunter both flagged that `TestSubmitCreatorApplicationCFIP` would silently fail in the CI staging gate (`ci.yml:411` — `E2E_BASE_URL=https://staging-api.ugcboost.kz`) because Cloudflare rewrites `CF-Connecting-IP` at the edge. **Recorded:** added `t.Skip` guard when `testutil.BaseURL` is not localhost / 127.0.0.1. KEEP: any future test that synthesises CF-only headers must include the same guard or document why it is exempt.
- **Trigger:** edge-case-hunter flagged unbounded header-value length feeding `net.ParseIP` (micro-DoS surface). **Recorded:** added `maxIPTextLen=45` skip in `parseHeaderIP`. KEEP: header-IP candidates must be size-checked before any expensive validation step.
- **Trigger:** blind-hunter flagged that `clientIPSingleHeaders` was a mutable package `var`. **Recorded:** changed to fixed-size `[3]string` array. KEEP: in-package immutable lookup tables should default to fixed-size arrays.
- **Trigger:** edge-case-hunter flagged misleading trust-model doc-comment claiming Cloudflare ranges protect all four headers. **Recorded:** doc-comment now distinguishes authoritative CF-set headers (CF-Connecting-IP, True-Client-IP) from best-effort Dokploy fallbacks (X-Real-IP, X-Forwarded-For). KEEP: trust-grade documentation must match each header's actual provenance.

## Design Notes

**Trust-модель.** Backend принимает HTTP только через `Cloudflare → Dokploy → Docker`. Прод-iptables ограничивает входящие Cloudflare-ranges (memory `project_prod_firewall.md`); внутри Docker network до backend достаёт только Dokploy reverse-proxy. Header-spoofing на уровне приложения невозможен — security обеспечена сетью. Application-whitelist Cloudflare-CIDR не вводим: дублирование сетевого firewall + дорогой maintenance.

**Почему такой порядок.** `CF-Connecting-IP` — Cloudflare всегда выставляет один валидный IP, промежуточные прокси его не трогают. `True-Client-IP` — Cloudflare Enterprise / Akamai контракт, backup. `X-Real-IP`/`X-Forwarded-For` — fallback для запросов мимо Cloudflare (staging-deploy без CF, локальные тесты).

## Verification

**Commands:**
- `cd backend && go test ./internal/middleware/... -count=1 -race -timeout 5m` -- expected: зелёные, `TestRealIP` покрывает все 8 сценариев матрицы.
- `make test-unit-backend-coverage` -- expected: per-method 80% порог не нарушен.
- `make test-e2e-backend` -- expected: зелёные, `TestSubmitCreatorApplicationCFIP` проходит.
- `make lint-backend` -- expected: zero warnings.
- `make build-backend` -- expected: компилируется.

**Manual checks (после деплоя на staging):**
- Подать creator application с известного IP, проверить `audit_logs.ip_address` — должно совпадать с whatismyip.com.
- В structured-логах backend `http request` поле `client_ip` совпадает с тем же IP.

## Suggested Review Order

**Корневая правка middleware**

- Источник правды по приоритету заголовков; `[3]string` вместо мутабельного var.
  `backend/internal/middleware/client_ip.go:27`

- Сам `RealIP` middleware: единственная точка перезаписи `r.RemoteAddr`, doc-comment описывает trust-model + grades заголовков.
  `backend/internal/middleware/client_ip.go:57`

- `extractClientIP` крутит цепочку, делегирует валидацию `parseHeaderIP` (trim → length-cap → ParseIP).
  `backend/internal/middleware/client_ip.go:66`

- `parseHeaderIP` — единая точка длинного-cap'а (`maxIPTextLen=45`) и `net.ParseIP`-валидации.
  `backend/internal/middleware/client_ip.go:86`

**Логирование (унификация с audit)**

- `client_ip` теперь приходит из `ClientIPFromContext`, а не сырой `r.RemoteAddr` — тот же IP что и в `audit_logs.ip_address`.
  `backend/internal/middleware/logging.go:34`

**Очистка осиротевших констант**

- `HeaderXForwardedFor`/`HeaderXRealIP` уехали в `middleware/`; в `handler/constants.go` они больше не нужны.
  `backend/internal/handler/constants.go:8`

**Unit-coverage**

- `TestRealIP` table-driven: 10 кейсов матрицы + AC #3 (skip invalid CF, fall to X-Real-IP) + length-cap edge.
  `backend/internal/middleware/client_ip_test.go:29`

- Logging-тест: `WithClientIP` source-of-truth, assert на новый log-key `client_ip`.
  `backend/internal/middleware/logging_test.go:29`

**E2E full-chain проверка**

- `TestSubmitCreatorApplicationCFIP`: POST с `CF-Connecting-IP: 203.0.113.7` → audit-row через `ListAuditLogs` API → assert `entry.IpAddress`. `t.Skip` guard на CF-fronted `BaseURL`, чтобы CI staging не валил тест по wrong reason.
  `backend/e2e/creator_application/creator_application_test.go:341`

- `WithHeader` variadic option для `PostRaw` — backward-compat сохранён (опции опциональны).
  `backend/e2e/testutil/raw.go:21`

- `FindAuditEntry` вернёт `*AuditLogEntry` для inspection полей; `AssertAuditEntry` оставлен как тонкая обёртка.
  `backend/e2e/testutil/audit.go:17`

- `HeaderCFConnectingIP` константа в e2e: модуль изолирован от `internal/`, нужен локальный источник правды.
  `backend/e2e/testutil/const.go:24`
