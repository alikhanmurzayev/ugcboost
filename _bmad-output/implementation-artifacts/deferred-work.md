---
title: Deferred work — bmad-quick-dev review findings backlog
type: backlog
status: living
created: '2026-05-07'
---

# Deferred work

Findings из агентских review-сессий, которые не блокирующие для текущего PR'а, но требуют отдельного внимания. По мере того как чанки доходят до main — собирается список технических долгов и рисков, которые стоит обсудить и адресовать в будущих итерациях.

## chunk 10 (campaign_creators backend) — review 2026-05-07

### TOCTOU `assertCampaignActive` вне `WithTx`

**Источник:** edge-case-hunter (blocker), blind-hunter (minor) — `backend/internal/service/campaign_creator.go:48-50, 88-90, 132-134`.

**Суть:** Pre-fetch кампании выполняется через `s.pool` ДО открытия транзакции. Параллельный admin может soft-delete'нуть кампанию между gate'ом и mutate-операцией → строки попадут в campaign_creators под soft-deleted кампанией. Спека (Boundaries → Always) **явно требует** «Pre-fetch ДО `WithTx`», так что текущая реализация ей соответствует. Но смежный сервис `CampaignService.UpdateCampaign` делает gate ВНУТРИ tx — это разъезжающаяся практика. Правильный подход — `SELECT ... FOR UPDATE` внутри tx или single-statement conditional mutate.

**Когда фиксить:** когда появится handler soft-delete (chunk 7+) и race-window станет реальным. Параллельно — обсудить со spec'ом, нужно ли менять boundary «pre-fetch ДО WithTx» на «внутри WithTx с FOR UPDATE».

### `Remove` без `FOR UPDATE` на pre-fetch

**Источник:** edge-case-hunter (major) — `backend/internal/service/campaign_creator.go:103-105`.

**Суть:** `GetByCampaignAndCreator` + LBYL-проверка `status == agreed` + `DeleteByID` — три шага без row-lock. Под READ COMMITTED concurrent транзакция (chunk 14, TMA-flow) может перевести row в `agreed` после нашего SELECT и до нашего DELETE, и мы удалим уже-согласованного. В chunk 10 writer'ов в `agreed` нет, race'а физически нет.

**Когда фиксить:** в chunk 14 (TMA agree/decline), когда появится альтернативный writer. Варианты: добавить `Suffix("FOR UPDATE")` в `GetByCampaignAndCreator` ИЛИ переписать на conditional DELETE: `DELETE ... WHERE id = $1 AND status != 'agreed' RETURNING *`. Прецедент `FOR UPDATE` уже есть в `creator_application.go:240`.

### `ctx.Err()` check в batch-loop `Add`

**Источник:** edge-case-hunter (major) — `backend/internal/service/campaign_creator.go:47-79`.

**Суть:** Если клиент разорвёт соединение в середине батча, pgx упадёт на ближайшем `Add`/`writeAudit` с `context canceled`, что транслируется в default-branch `respondError` → 500 `INTERNAL_ERROR` с записью в Error-log как «unexpected». Rollback и так произойдёт через `WithTx`, но логи генерируют ложные 500.

**Когда фиксить:** при ближайшем рефакторинге `respondError`/middleware — добавить branch на `errors.Is(err, context.Canceled)` → suppressed log / 499 Client Closed Request. Применимо к ВСЕМ mutate-сервисам, не только chunk 10.

### E2e cleanup ломается на `agreed`-row

**Источник:** blind-hunter (minor) — `backend/e2e/testutil/campaign_creator.go:21-43`.

**Суть:** `RegisterCampaignCreatorCleanup` использует production-A2 (DELETE), который имеет 422 guard на `status == agreed`. В chunk 10 status у всех строк = `planned`, всё ок. После chunk 14 тесты, продвигающие row в `agreed`, не смогут убрать через cleanup — `unexpected status 422`.

**Когда фиксить:** в chunk 14. Варианты: принимать 422 как success в cleanup-helper'е, либо мапить через `/test/cleanup-entity` с типом `campaign_creator` (расширить testapi).

### E2e не покрывает soft-deleted campaign на 3 ручках

**Источник:** acceptance-auditor (minor) — спека I/O Matrix lines 54, 59, 62.

**Суть:** Спека требует «Add/Remove/List к soft-deleted campaign → 404 CAMPAIGN_NOT_FOUND». Unit-тесты сервиса покрывают, e2e — нет. Сейчас физически нет способа создать soft-deleted кампанию через публичные API: handler `DELETE /campaigns/{id}` ещё не реализован (chunk 7+ из roadmap).

**Когда фиксить:** в чанке, который добавит DELETE /campaigns/{id} (soft-delete handler). Параллельно добавить 3 subtest'а в `campaign_creator_test.go`.

## chunk 10 — round-2 review (2026-05-07)

### FK без `ON DELETE CASCADE` — cleanup footgun

**Источник:** blind-hunter (major) — `backend/migrations/20260507044135_campaign_creators.sql:30-31`.

**Суть:** `creator_id_fk` и `campaign_id_fk` используют default `NO ACTION`. TestApi `/test/cleanup-entity` делает HARD DELETE на creators и campaigns. Если e2e-тест забывает `RegisterCampaignCreatorCleanup` (или порядок LIFO нарушен) — parent cleanup упадёт с PG 23503, грязное состояние накапливается. Сравни `creator_socials` / `creator_categories` — они С `ON DELETE CASCADE` именно для этой причины.

**Дизайн trade-off:** Хотя CASCADE упростил бы cleanup, может оказаться нежелательным для prod-сценариев (если когда-нибудь появится hard-delete creator/campaign). Стоит обсудить — оставлять как есть или добавлять CASCADE.

**Когда фиксить:** при появлении первой реальной поломки cleanup в CI или при принятии решения о CASCADE policy для prod.

### 23502/23514 не транслируются в repo `Add`

**Источник:** edge-case-hunter (major) — `backend/internal/repository/campaign_creator.go:90-114`.

**Суть:** Repo ловит 23505/23503, но НЕ ловит 23502 (`NOT NULL violation`) и 23514 (`campaign_creators_status_check` violation). Сейчас сервис всегда передаёт `domain.CampaignCreatorStatusPlanned` — branch недостижим. Но в chunks 12/14 появятся новые writers (notify, agreed, declined). Если кто-то ошибётся с константой — получит сырой 500 без диагностики.

**Когда фиксить:** в chunk 12 при добавлении новых writers — сразу добавить defensive translation.

### Content-Type / max-body → 413 vs 422

**Источник:** edge-case-hunter (major) — middleware-level, не chunk 10.

**Суть:** Strict-server `json.Decode` принимает любой body как long as он валидный JSON, не сверяет Content-Type. BodyLimit middleware при превышении возвращает 422 «Invalid request body» через RequestErrorHandlerFunc вместо 413 — admin не понимает, что лимит превышен.

**Когда фиксить:** общий рефактор middleware — отдельный кросскат-task. Применимо ко всем endpoint'ам.

### Индекс на `creator_id` для chunk 12+

**Источник:** edge-case-hunter (minor) — `backend/migrations/20260507044135_campaign_creators.sql:42`.

**Суть:** UNIQUE `(campaign_id, creator_id)` создаёт композитный btree с leading `campaign_id`. Запрос `WHERE campaign_id = ?` оптимально покрывается. Но `WHERE creator_id = ?` (например, "найти все кампании, в которых участвует креатор X") fallback'ится в seq scan. Сейчас такого запроса нет — но он точно появится в chunk 12+ (notify) и/или в TMA flow.

**Когда фиксить:** в чанке, где появится первый запрос по `creator_id` — добавить `CREATE INDEX campaign_creators_creator_id_idx ON campaign_creators(creator_id);` отдельной forward-миграцией.

## Кандидаты в стандарты (review)

Перед добавлением в `docs/standards/` стоит обсудить с командой и проверить, что паттерн уже плодится по нескольким сервисам, а не рождается в этом PR.

### OpenAPI minItems/maxItems требуют дублирующего runtime-check'а в handler'е

**Источник:** edge-case-hunter, blind-hunter.

**Обоснование:** oapi-codegen без явных валидаторов не применяет schema-level limits. В chunk 10 `maxItems=200` декларирован, но не enforced — открытый DoS. В этом PR пофиксили, но паттерн всплывёт ещё (POST /creators/list, batch-endpoints в creator_application и т.п.). Добавить hard-rule в `security.md` или `backend-codegen.md`.

### Soft-delete gate должен быть внутри той же транзакции, что и mutate

**Источник:** blind-hunter, edge-case-hunter.

**Обоснование:** `CampaignService.UpdateCampaign` делает gate внутри tx (правильно). `CampaignCreatorService` делает gate через pool ДО tx (по требованию текущей спеки, но создаёт TOCTOU). Нужно решить какой паттерн каноничный, и зафиксировать в `backend-transactions.md`.

### Strict-422 batch rollback assertion: e2e обязан включать valid+invalid сущности

**Источник:** acceptance-auditor.

**Обоснование:** Тест с `[bogus]`-only не доказывает rollback — empty list был бы и без rollback'а. Нужен mixed batch `[valid, invalid]`. Применимо к ВСЕМ strict-422 batch-операциям. Добавить в `backend-testing-e2e.md` § Сценарии.

### Детерминистический lock-order при batch INSERT/UPDATE (round-2)

**Источник:** edge-case-hunter (round 2, blocker).

**Обоснование:** Concurrent admin'ы могут отправить `Add(camp1, [A, B])` и `Add(camp1, [B, A])` — Postgres ловит deadlock 40P01, один tx убивается, admin видит 500 на легитимный input. В chunk 10 пофиксили сортировкой `creatorIDs` ASC перед циклом. Любой будущий batch endpoint (POST /creators/list с upsert'ами, batch reset для notify в chunk 12) повторит ошибку, пока правила нет. Добавить в `backend-transactions.md` § Concurrency.

### Регистрация e2e cleanup ДО `require`-ассертов

**Источник:** edge-case-hunter (round 2, major).

**Обоснование:** Если cleanup регистрируется после require'ов и любой из них fail'нет — строки в campaign_creators остаются → cleanup creator (LIFO) попытается hard-delete creator → FK 23503 → row leak. В chunk 10 пофиксили перенесением. Применимо ко всем e2e тестам с child-rows. Добавить в `backend-testing-e2e.md` § Cleanup.
