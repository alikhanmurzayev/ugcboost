---
title: 'Deferred work — пре-существующие находки и cross-cutting вопросы'
type: backlog
status: living
created: '2026-05-06'
updated: '2026-05-06'
---

# Deferred work

Living-документ. Сюда попадают findings из ревью PR'ов, которые **не относятся к текущей story**: пре-существующие проблемы, cross-cutting вопросы, инфраструктура для будущих чанков. Каждая запись — кандидат в отдельный chunk / технический долг.

Формат: `- [ ] **категория** — описание (источник: PR #N) — статус / план`.

## Cross-cutting (architecture / errors)

- [ ] **respondError catch-all `*BusinessError` → 409** — `backend/internal/handler/response.go:37-41` отдаёт 409 для всех будущих BusinessError. Если появится `BusinessError` с семантикой 422 (например, `CampaignFull`/`CampaignNotEditable`) — клиент получит 409, не описанный в OpenAPI-схеме. (источник: PR #72) — план: либо `BusinessError` несёт `HTTPStatus int`, либо явный switch на конкретные sentinels вместо catch-all.
- [ ] **Unmapped 23505 / 23503 в repo** — switch в каждом repo (creator/brand/campaign/...) ловит только known constraints; новый partial UNIQUE / FK = silent regression до 500. (источник: PR #72) — план: общая политика — default-ветка возвращает `domain.ErrConflict` с логом `unknown constraint %s`.
- [ ] **Unicode normalisation для unique-полей** — `strings.TrimSpace` + `utf8.RuneCountInString` пропускают zero-width chars, NFC-варианты, homoglyphs → возможны "duplicate-by-eye" имена кампаний/креаторов/брендов. (источник: PR #72) — план: NFC через `golang.org/x/text/unicode/norm` + запрет zero-width/control runes как cross-cutting validation rule.
- [ ] **Audit-payload allow-list** — `audit_logs.new_value = json.Marshal(domainEntity)` сериализует ВСЕ поля. Добавление любого sensitive поля в `domain.X` молча уйдёт в audit. (источник: PR #72) — план: либо явный sub-DTO для audit, либо инвариант "domain содержит только safe-to-audit поля" + чек-правило.

## Database / migrations

- [ ] **Partial UNIQUE без COLLATE** — `campaigns_name_active_unique` использует default collation; "Promo" и "promo" — разные. Скорее всего бизнес-баг. (источник: PR #72) — план: бизнес-решение про case-sensitivity, затем `lower(name)` индекс или `COLLATE "C"` / `"und-x-icu"`.
- [ ] **`updated_at` без BEFORE UPDATE триггера** — `DEFAULT now()` срабатывает только на INSERT. Когда чанк #5 (PATCH) добавит UPDATE — каждый репо должен явно `SET updated_at = now()`, иначе `updated_at == created_at` навсегда. (источник: PR #72) — план: общий `moddatetime` trigger или helper `WithUpdatedAt()` в squirrel.
- [ ] **Утф-8 руны vs байты в TEXT** — валидация `utf8.RuneCountInString` ≤ N, БД без CHECK. Если будущий CHECK добавят в байтах — кириллица / эмодзи упадут. (источник: PR #72) — план: документировать "лимит в рунах" в комментарии к колонке + использовать `char_length()` в любых будущих CHECK'ах.

## Test infrastructure

- [ ] **Audit-row test без `contextWithActor`** — service-unit тесты создают audit-row через `context.Background()`; ассерт по структуре проходит, даже если `writeAudit` сломает actor-attribution. (источник: PR #72; pre-existing pattern в `brand_test.go`) — план: переделать `expectAudit` так, чтобы он принимал ожидаемый actor и проверял `ActorRole/ActorID/IPAddress`.
- [ ] **Race-test sync barrier как стандарт** — pattern из `TestCreateCampaign_RaceUniqueName` (chan barrier) должен стать стандартом для всех race-тестов. (источник: PR #72) — план: добавить в `backend-testing-e2e.md` § Race-сценарии явное требование `chan struct{}`-барьера.

## Future chunk hooks

- [ ] **`DeleteForTests` FK awareness** — когда чанки #10+ добавят `campaign_creators` с FK ON DELETE RESTRICT, текущий `DeleteForTests` упадёт 23503 → cleanup-stack оставит мусор. (источник: PR #72) — план: либо CASCADE в FK-миграции, либо switch'ить 23503 → silent success в `DeleteForTests`.
- [ ] **Rate-limit на /campaigns** — admin-only endpoint, низкий приоритет, но free-text body 16KB+ возможен. (источник: PR #72) — план: общий `BodyLimit` в middleware уже есть; конкретный rate-limit не нужен пока admin-only.
- [ ] **Structured-log при rejection (`name-taken`)** — текущий service логирует только success; 409-rejection не оставляет следа в stdout (audit-row тоже не пишется при rollback). (источник: PR #72) — план: `s.logger.Info(ctx, "campaign create rejected", "reason", "name_taken")` перед return.
