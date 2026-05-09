---
title: "Intent: chunk 14 — TMA приём решения креатора (agree/decline) + secret_token integrity"
type: intent
status: living
created: "2026-05-08"
chunk: 14
roadmap: "_bmad-output/planning-artifacts/campaign-roadmap.md"
design: "_bmad-output/planning-artifacts/design-campaign-creator-flow.md"
---

# Intent: TMA приём решения креатора (agree/decline) + secret_token integrity

Chunk 14 из `campaign-roadmap.md` (Группа 6). Источник правды по продуктовому замыслу — `design-campaign-creator-flow.md`. Этот intent фиксирует развилки и решения, дополняющие design'у, и служит входом для `bmad-quick-dev`. **Расширение относительно изначальной формулировки chunk'а:** включает миграцию + validation + backfill для `campaigns.secret_token`, а также EAFP-обработку 23505 в POST/PATCH `/campaigns` (chunks 3/5 дополняются). Без этих частей T1/T2 уязвимы к suffix-attack на `secret_token`.

## Преамбула для агента-исполнителя

Перед любой строкой кода: полностью загрузить `docs/standards/`. Каждое правило стандартов — hard rule. Этот intent — над design'ом, не вместо него.

## Тезис

Два публичных TMA-эндпоинта `POST /tma/campaigns/{secret_token}/{agree|decline}` с middleware HMAC-валидации Telegram initData (по `TELEGRAM_BOT_TOKEN`) + резолюцией креатора. Middleware занимается identity (initData → telegram_user_id → resolve в creators → role), AuthzService — авторизацией ресурса (роль creator + принадлежность кампании), business-service — только transition в `agreed`/`declined` через `WithTx` с `SELECT FOR UPDATE` и audit. Идемпотентность симметричная: повторный `agree` из `agreed` и повторный `decline` из `declined` отдают 200 + `already_decided=true` без UPDATE и без audit. Прочие несовместимые переходы — 422 с granular кодами; soft-deleted кампания — 404; креатор не в кампании / не найден по telegram_user_id — 403. На TMA-фронте — wire backend через openapi-fetch middleware с initData header; UI всегда показывает полный ТЗ + 2 кнопки, реакция диктуется ответом бэка; для `already_decided=true` — те же `AcceptedView`/`DeclinedView` + вводная плашка.

Параллельно решается проблема suffix-attack на `secret_token`: добавляется реальная (не computed) колонка `campaigns.secret_token`, заполняемая бэком при INSERT/UPDATE через `domain.ExtractSecretToken` после `domain.ValidateTmaURL`; partial UNIQUE INDEX гарантирует уникальность среди живых кампаний с непустым URL; lookup в TMA-ручках идёт точным сравнением `WHERE secret_token = $1`, без LIKE; невалидные/коротко-токеновые `tma_url` отвергаются на POST/PATCH; легаси-записи backfill'ятся в миграции, невалидные оставляются с `secret_token=NULL` (недоступны через TMA до обновления).

## Scope

**Включено:**

- **Schema (новая миграция):**
  - ADD `campaigns.secret_token TEXT` (NULLABLE).
  - Backfill: `UPDATE campaigns SET secret_token = regexp_replace(tma_url, '^.*/', '') WHERE tma_url IS NOT NULL AND tma_url <> '' AND regexp_replace(tma_url, '^.*/', '') ~ '^[A-Za-z0-9_-]{16,}$'`. Невалидные/пустые остаются NULL.
  - `CREATE UNIQUE INDEX campaigns_secret_token_uniq ON campaigns(secret_token) WHERE secret_token IS NOT NULL AND is_deleted = false`.
  - Down: drop index + drop column.

- **Domain (расширение):**
  - `ValidateTmaURL(s string) error` — допускает пустую строку (legacy/draft); если непустая — парсится через `net/url`, last path-segment должен быть `^[A-Za-z0-9_-]{16,}$`. Granular `CodeInvalidTmaURL` + actionable message.
  - `ExtractSecretToken(s string) string` — last path-segment непустого URL; для пустой строки возвращает `""`.
  - Новые ошибки `ErrTmaURLConflict` (UNIQUE 23505 на secret_token), `ErrCampaignCreatorNotInvited`, `ErrCampaignCreatorAlreadyAgreed`, `ErrCampaignCreatorDeclinedNeedReinvite`, `ErrTMAForbidden`, `ErrCampaignNotFound`.
  - Типы `CampaignCreatorDecision` (agree|decline), `CampaignCreatorDecisionResult` (`{Status, AlreadyDecided}`).

- **Service (расширение существующего `CampaignService` + новый `TmaCampaignCreatorService`):**
  - `CampaignService.Create/Update` дополняются: вызов `ValidateTmaURL` → `ExtractSecretToken` → передача в repo (пустая строка → repo пишет NULL).
  - `AuthzService.AuthorizeTMACampaignDecision(ctx, secretToken)`.
  - `TmaCampaignCreatorService.ApplyDecision(ctx, auth, decision)`.

- **Repository (расширение):**
  - `CampaignRepo.Insert/Update`: колонка `secret_token` входит в insert/update mappers; EAFP `pgErr 23505` на partial UNIQUE → `domain.ErrTmaURLConflict`.
  - `CampaignRepo.GetBySecretToken(ctx, token)`: `WHERE secret_token = $1 AND is_deleted = false`. Точное совпадение, без LIKE.
  - `CampaignCreatorRepo.GetByIDForUpdate(ctx, id)` (новый, `SELECT … FOR UPDATE`), `ApplyDecision(ctx, id, status, decidedAt)` (новый).
  - `CreatorRepo.GetByTelegramUserID(ctx, tgID)` — добавляется, если ещё нет (проверить наследие chunks 10/12).

- **Backend handler/middleware:**
  - `middleware/tma_initdata.go`: HMAC + creator lookup → ctx с `{telegram_user_id, creator_id (nullable), role}`.
  - `handler/tma_campaign_creator.go`: T1/T2 ручки (тонкие, через AuthzService → Service).
  - Ранний reject в handler: если `secret_token` из path не соответствует `^[A-Za-z0-9_-]{16,}$` → 404 anti-fingerprint, без обращения к БД.

- **OpenAPI / config / testapi:**
  - `openapi.yaml` — T1/T2 paths + securityScheme `tmaInitData` + schemas + `CodeInvalidTmaURL` / `CodeTmaURLConflict` для POST/PATCH `/campaigns`.
  - `openapi-test.yaml` — `/test/tma/sign-init-data`.
  - `config.TMAInitDataTTL` (env `TMA_INITDATA_TTL_SECONDS`, default 86400).

- **TMA frontend:** openapi-fetch middleware с initData header, хуки `useAgree`/`useDecline`, wire `CampaignBriefPage` на реальный backend, расширение `AcceptedView`/`DeclinedView` пропом `alreadyDecided`, обработка ошибок (401/403/404/422).

- **Тесты:** backend unit (handler/service/repo/middleware/authz), backend e2e на новые TMA-ручки + valid/invalid tma_url POST/PATCH + race на UNIQUE, frontend unit + Playwright e2e на TMA-flow.

**Не включено:**
- TrustMe-триггер при переходе в `agreed` (chunks 16/17).
- Любые изменения в admin-flow (chunks 11/13/15).
- Миграция `campaign_creators` (уже есть из chunks 10/12); `audit_logs.actor_id` уже nullable.
- PATCH `tma_url` ограничения для уже разосланных приглашений — реализовано в chunk 12 (после первой рассылки `tma_url` не меняется).
- Auto-генерация `tma_url`/`secret_token` на бэке — admin сам вводит URL в форме создания кампании (UI chunk 8a/8b).

## Schema migration

`backend/migrations/<timestamp>_campaigns_secret_token.sql`:

```sql
-- +goose Up
-- +goose StatementBegin
ALTER TABLE campaigns ADD COLUMN secret_token TEXT;

-- Backfill: только валидные tma_url (last-segment ≥16 URL-safe chars).
-- Прод-записи с пустым/невалидным tma_url остаются с secret_token = NULL,
-- они недоступны через TMA до явного обновления URL администратором.
UPDATE campaigns
SET secret_token = regexp_replace(tma_url, '^.*/', '')
WHERE tma_url IS NOT NULL
  AND tma_url <> ''
  AND regexp_replace(tma_url, '^.*/', '') ~ '^[A-Za-z0-9_-]{16,}$';

CREATE UNIQUE INDEX campaigns_secret_token_uniq
  ON campaigns (secret_token)
  WHERE secret_token IS NOT NULL AND is_deleted = false;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS campaigns_secret_token_uniq;
ALTER TABLE campaigns DROP COLUMN IF EXISTS secret_token;
-- +goose StatementEnd
```

**Prod-замечание:** в проде есть кампании с пустым `tma_url`. Backfill их игнорирует; колонка остаётся NULLABLE; UNIQUE — partial. После миграции эти кампании продолжают работать в админке как сейчас (просто недоступны через TMA, что и было раньше).

## Domain — validation и extraction

`backend/internal/domain/campaign.go` (или существующий файл с `Campaign.Validate`):

```go
const (
    secretTokenPattern = `^[A-Za-z0-9_-]{16,}$`
)

var (
    secretTokenRe = regexp.MustCompile(secretTokenPattern)
)

// ValidateTmaURL — пустая строка допустима (legacy/draft);
// непустая — должна быть валидным URL с last path-segment, соответствующим
// secretTokenPattern.
func ValidateTmaURL(s string) error {
    if s == "" {
        return nil
    }
    u, err := url.Parse(s)
    if err != nil || u.Scheme == "" || u.Host == "" {
        return NewValidationError(CodeInvalidTmaURL, "Ссылка на TMA должна быть валидным URL.")
    }
    token := lastSegment(u.Path)
    if !secretTokenRe.MatchString(token) {
        return NewValidationError(CodeInvalidTmaURL, "Последний сегмент ссылки должен быть длиной от 16 символов и содержать только латинские буквы, цифры, '_' и '-'.")
    }
    return nil
}

func ExtractSecretToken(s string) string {
    if s == "" {
        return ""
    }
    u, err := url.Parse(s)
    if err != nil {
        return ""
    }
    return lastSegment(u.Path)
}

func lastSegment(p string) string {
    p = strings.TrimRight(p, "/")
    if i := strings.LastIndex(p, "/"); i >= 0 {
        return p[i+1:]
    }
    return p
}
```

`Campaign.Validate()` — добавить вызов `ValidateTmaURL(c.TmaURL)`. Если уже валидируется на длину — оставить, дополнить.

## Service — расширение CampaignService

`Create(ctx, input)` и `Update(ctx, id, patch)`:
1. `domain.ValidateTmaURL(input.TmaURL)` — early return с 422 `CodeInvalidTmaURL`.
2. `secret := domain.ExtractSecretToken(input.TmaURL)` — пустая строка для пустого URL.
3. Передаётся в `CampaignRepo.Insert/Update` отдельным полем (или через расширение Row-структуры).
4. Repo возвращает `domain.ErrTmaURLConflict` на 23505 → handler мапит в 422 `CodeTmaURLConflict` с message «Эта ссылка на TMA уже используется в другой кампании».

## Repository — Insert/Update + GetBySecretToken

### `CampaignRow` (расширение)
- Поле `SecretToken sql.NullString` с тегами `db:"secret_token" insert:"secret_token"`. NULL ↔ пустая строка из service.

### `CampaignRepo.Insert(ctx, row)` / `Update(ctx, id, patch)`
- `secret_token` входит в insertColumns (через stom).
- EAFP-ловля `pgErr 23505` на индексе `campaigns_secret_token_uniq` → `domain.ErrTmaURLConflict`. Другие 23505 (например, partial unique на name) пропускаются дальше как соответствующие domain-ошибки.

### `CampaignRepo.GetBySecretToken(ctx, token string) (*CampaignRow, error)`
- `WHERE secret_token = $1 AND is_deleted = false`. Точное совпадение, без LIKE.
- Использует `entitySelectColumns` через stom.
- `sql.ErrNoRows` → service → AuthzService → 404.
- Repo НЕ ассертит N>1 — partial UNIQUE гарантирует ≤1 row на уровне БД. Если БД отдаст >1 — это инвариантная ошибка БД, propagate как internal.

## TMA initData middleware

**Путь:** `backend/internal/middleware/tma_initdata.go`.

**Зависимости:** `config.TelegramBotToken`, `config.TMAInitDataTTL`, `CreatorRepo.GetByTelegramUserID`.

**Алгоритм:**
1. Прочитать `Authorization`. Префикс `tma ` (с пробелом). Иначе → 401.
2. Извлечь raw initData; распарсить как `url.Values` (`hash`, `auth_date`, `user`). Любое отсутствие → 401.
3. `auth_date > now` → 401 (защита от подписи «из будущего»).
4. `now - auth_date > TMAInitDataTTL` → 401 (истёкший).
5. Собрать `data_check_string`: ключи кроме `hash`, sorted alphabetically, формат `key=value\nkey=value`.
6. `secret_key = HMAC_SHA256(bot_token, "WebAppData")`. `expected = hex(HMAC_SHA256(data_check_string, secret_key))`. `subtle.ConstantTimeCompare` mismatch → 401.
7. JSON-парсинг `user`: `id` int64, `> 0`. Иначе → 401.
8. `creator, err := creatorRepo.GetByTelegramUserID(ctx, id)`:
   - найден → ctx содержит `{telegramUserIDKey, creatorIDKey, creatorRoleKey=ROLE_CREATOR}`.
   - `sql.ErrNoRows` → ctx содержит только `telegramUserIDKey`. Решение оставляем AuthzService.
   - другая ошибка → 500 (не глотаем).
9. → next handler.

**Anti-fingerprinting:** все 401 — один generic JSON `{code: "CodeUnauthorized", message: "Не удалось подтвердить доступ"}`.

**Логирование:** debug-уровень. Никогда не пишем в stdout `init_data`, `secret_token`, `telegram_user_id`. Допустимо: method/path/status/duration, абстрактная причина 401 (`hmac_mismatch`/`auth_date_invalid`/`header_missing`/`creator_lookup_db_error`).

## AuthzService — расширение

```go
type TMACampaignDecisionAuth struct {
    CreatorID         uuid.UUID
    CampaignID        uuid.UUID
    CampaignCreatorID uuid.UUID
    CurrentStatus     string
}

func (a *AuthzService) AuthorizeTMACampaignDecision(
    ctx context.Context, secretToken string,
) (TMACampaignDecisionAuth, error)
```

1. ctx без role/creator_id → `domain.ErrTMAForbidden` (403).
2. `campaign := campaignRepo.GetBySecretToken(ctx, secretToken)`:
   - `sql.ErrNoRows` → `domain.ErrCampaignNotFound` (404).
   - Repo уже фильтрует `is_deleted = false` (soft-deleted = not found).
3. `cc := campaignCreatorRepo.GetByCampaignAndCreator(ctx, campaign.ID, creatorID)`:
   - `sql.ErrNoRows` → `domain.ErrTMAForbidden` (403, тот же текст что и creator-not-found anti-fingerprint).
4. Возврат `TMACampaignDecisionAuth{...}`.

`AuthzService` зависит от `CampaignRepo` и `CampaignCreatorRepo` (через RepoFactory + pool).

## Handler

`backend/internal/handler/tma_campaign_creator.go`:

```go
func (h *TmaCampaignCreatorHandler) TmaAgree(ctx, req) (resp, error) {
    if !secretTokenRe.MatchString(req.SecretToken) {
        return nil, domain.ErrCampaignNotFound // 404 anti-fingerprint, без БД-вызовов
    }
    auth, err := h.authz.AuthorizeTMACampaignDecision(ctx, req.SecretToken)
    if err != nil { return nil, err }
    result, err := h.svc.ApplyDecision(ctx, auth, domain.CampaignCreatorDecisionAgree)
    if err != nil { return nil, err }
    return TmaDecisionResult{Status: result.Status, AlreadyDecided: result.AlreadyDecided}, nil
}
```

Аналогично `TmaDecline`. Никаких прямых repo-вызовов и сравнений ролей в handler. Маппинг domain-ошибок → HTTP — через стандартный mapper (соглашение проекта).

## Service `TmaCampaignCreatorService`

`backend/internal/service/tma_campaign_creator.go`:

```go
func (s *TmaCampaignCreatorService) ApplyDecision(
    ctx context.Context, auth TMACampaignDecisionAuth, decision domain.CampaignCreatorDecision,
) (domain.CampaignCreatorDecisionResult, error)
```

Структура (`dbutil.WithTx`):
1. `cc := repo.GetByIDForUpdate(ctx, auth.CampaignCreatorID)` — `SELECT … FOR UPDATE`. Защита от concurrent agree/decline.
2. Switch по `(cc.Status, decision)`:
   - **Реальный переход** (`invited` + agree → `agreed`, `invited` + decline → `declined`):
     - `repo.ApplyDecision(ctx, cc.ID, newStatus, now)` — UPDATE status, decided_at, updated_at.
     - `auditRepo.Create(ctx, ...)` (`actor_id=NULL`, payload `{campaign_id, creator_id}`).
     - return `{Status: newStatus, AlreadyDecided: false}`.
   - **No-op** (`agreed` + agree, `declined` + decline):
     - return `{Status: cc.Status, AlreadyDecided: true}`. Без UPDATE и audit.
   - **Несовместимое** (`agreed` + decline, `declined` + agree, `planned` + любое):
     - return granular domain-error.
3. После `WithTx` — log info на success (не внутри callback).

## Domain — granular errors и codes

В `backend/internal/domain/`:

```go
type CampaignCreatorDecision string

const (
    CampaignCreatorDecisionAgree   CampaignCreatorDecision = "agree"
    CampaignCreatorDecisionDecline CampaignCreatorDecision = "decline"
)

type CampaignCreatorDecisionResult struct {
    Status         string
    AlreadyDecided bool
}

var (
    // 422 — TMA decision branches
    ErrCampaignCreatorNotInvited           = NewBusinessError(CodeCampaignCreatorNotInvited,           "Приглашение ещё не отправлено. Дождитесь приглашения от менеджера.")
    ErrCampaignCreatorAlreadyAgreed        = NewBusinessError(CodeCampaignCreatorAlreadyAgreed,        "Вы уже согласились участвовать. Чтобы изменить решение, обратитесь к менеджеру.")
    ErrCampaignCreatorDeclinedNeedReinvite = NewBusinessError(CodeCampaignCreatorDeclinedNeedReinvite, "Вы уже отказались. Чтобы согласиться, попросите менеджера прислать приглашение заново.")

    // 403 — anti-fingerprint
    ErrTMAForbidden = NewAuthError(CodeTMAForbidden, "У вас нет приглашения на эту кампанию.")

    // 404
    ErrCampaignNotFound = NewBusinessError(CodeCampaignNotFound, "Кампания недоступна.")

    // 422 — campaign tma_url validation/conflict (POST/PATCH)
    ErrInvalidTmaURL  = NewValidationError(CodeInvalidTmaURL, "Последний сегмент ссылки должен быть от 16 символов и содержать только латинские буквы, цифры, '_' и '-'.")
    ErrTmaURLConflict = NewBusinessError(CodeTmaURLConflict, "Эта ссылка на TMA уже используется в другой кампании.")
)
```

Точные имена кодов добавляются в существующий пакет констант рядом с существующими.

## Audit

Новые константы в `backend/internal/service/audit_constants.go`:
- `AuditActionCampaignCreatorAgree   = "campaign_creator_agree"`
- `AuditActionCampaignCreatorDecline = "campaign_creator_decline"`

Соглашение совпадает с существующими (`_add`, `_invite`, `_remind`, `_remove`).

**Payload:** `{campaign_id, creator_id}`. Без `telegram_user_id`.

**Actor:** `actor_id = NULL`.

**Когда пишется:** только на реальный переход (`already_decided=false`).

**Где пишется:** в одной транзакции с UPDATE `campaign_creators` (`dbutil.WithTx`).

## Config

```go
TMAInitDataTTL time.Duration `env:"TMA_INITDATA_TTL_SECONDS" envDefault:"86400s"`
```

(Точная типизация — по существующему стилю env-парсинга в проекте.) Default 24 часа.

## OpenAPI обновления

**`backend/api/openapi.yaml`:**
- Новые paths: `/tma/campaigns/{secret_token}/agree`, `/tma/campaigns/{secret_token}/decline`.
- securityScheme `tmaInitData` (apiKey in header `Authorization`, pattern документируется).
- Schemas: `TmaDecisionResult` (`{status: CampaignCreatorStatus, already_decided: boolean}`).
- Новые error codes в существующих error-response schemas: `CodeUnauthorized`, `CodeTMAForbidden`, `CodeCampaignNotFound`, `CodeCampaignCreatorNotInvited`, `CodeCampaignCreatorAlreadyAgreed`, `CodeCampaignCreatorDeclinedNeedReinvite`, `CodeInvalidTmaURL`, `CodeTmaURLConflict`.

**`backend/api/openapi-test.yaml`:**
- Новый path `POST /test/tma/sign-init-data`. Request: `{telegram_user_id: integer, auth_date?: integer}`. Response: `{init_data: string}`.

После изменений — `make generate-api`.

## TMA frontend

### openapi-fetch middleware

`frontend/tma/src/api/middleware.ts`. На каждый request — берёт initData через `@telegram-apps/sdk-react`, кладёт в `Authorization: tma <init-data>`. Пустой initData — пропускает, бэк ответит 401.

Подключение — в `frontend/tma/src/api/client.ts`.

### Хуки `useAgreeDecision` / `useDeclineDecision`

`frontend/tma/src/features/campaign/hooks/useDecision.ts`. `useMutation`, обязательный `onError`, double-submit guard через external `isSubmitting` flag, кнопки `disabled` при `isPending`.

### `CampaignBriefPage`

- Удалить mock `getCampaignByToken`. Хардкод ТЗ остаётся (по design).
- Состояния: initial → ТЗ + 2 кнопки; success → `AcceptedView`/`DeclinedView` с `alreadyDecided`; error → toast.

### `AcceptedView` / `DeclinedView`

- Prop `alreadyDecided: boolean`. `true` → плашка `tma-already-decided-banner` («Вы уже согласились/отказались ранее»).
- i18n через `react-i18next`. Строки в `frontend/tma/src/i18n/locales/ru/campaign.json`.

### Error handling

- 401 → toast «Не удалось подтвердить ваш аккаунт. Откройте приложение через бот UGCBoost».
- 403 → toast «У вас нет приглашения на эту кампанию».
- 404 → toast «Кампания недоступна».
- 422 + `code` → message из ошибки backend.
- 5xx / network → toast «Не удалось обработать запрос. Попробуйте позже.»

### data-testid

`tma-agree-button`, `tma-decline-button`, `tma-already-decided-banner`, `tma-decision-error`, `tma-accepted-view`, `tma-declined-view`.

## Тестирование

### Backend unit (gate ≥80% per-method)

- **domain/campaign_test.go:** `ValidateTmaURL` — пустая строка ok; невалидный URL → ошибка; last-segment <16 → ошибка; формат с не-URL-safe символами → ошибка; happy path. `ExtractSecretToken` — пустая строка ok; happy path; URL без path → пустая строка.
- **middleware/tma_initdata_test.go:** valid initData + creator найден → ctx содержит `telegram_user_id, creator_id, role`; valid initData + creator НЕ найден → ctx только с `telegram_user_id`; header missing/wrong-prefix → 401; HMAC mismatch → 401; `auth_date` истёк / `auth_date > now` → 401; битый `user` JSON / `user.id <= 0` → 401; CreatorRepo вернул не-NotFound ошибку → 500.
- **service/authz_test.go (расширение):** `AuthorizeTMACampaignDecision` — ctx без role → 403; campaign не найдена / soft-deleted → 404; campaign_creator row нет → 403; happy path.
- **service/campaign_test.go (расширение):** Create/Update — невалидный `tma_url` → 422 `CodeInvalidTmaURL`; пустой `tma_url` ok (secret_token NULL); валидный → secret_token извлечён; `pgErr 23505` от repo → 422 `CodeTmaURLConflict`.
- **service/tma_campaign_creator_test.go:** все строки таблицы transitions, idempotent no-op (без UPDATE и без audit), audit-row в той же tx, `SELECT FOR UPDATE` lock-семантика (mock: проверка `repo.GetByIDForUpdate` вызван внутри tx).
- **repository unit-тесты:** SQL-asserts для `GetBySecretToken` (точное `=`, не LIKE), `GetByTelegramUserID`, `ApplyDecision`, `GetByIDForUpdate`, `Insert/Update` campaigns с EAFP на 23505.
- **handler unit-тесты:** captured-input на `creator_id`/`telegram_user_id` из ctx, response shape (200 с `{status, already_decided}`), ранний reject невалидного `secret_token` без обращения к service.

### Backend e2e

`backend/e2e/tma/tma_test.go` — TMA-decision flow. `backend/e2e/campaign/campaign_test.go` (расширение) — секрет-токен validation/conflict в POST/PATCH.

Helpers (`backend/e2e/testutil/tma.go`):
- `SignInitData(t, telegramUserID, opts) → string` — POST `/test/tma/sign-init-data`.
- `SetupCampaignWithInvitedCreator(t, ...)` — composable: create campaign + create approved creator с `telegram_user_id` + add (A1) + notify (A4) → возвращает `{campaignID, creatorID, secretToken, telegramUserID}`. Использует валидный tma_url с генерируемым secret_token.

Кейсы:

**TMA decisions:**
- happy agree / decline / re-invite cycle.
- idempotent agree (T1 → T1 → оба 200; первый `already_decided=false`, второй `true`; в `audit_logs` ровно один `campaign_creator_agree`). Идемпотентный decline — симметрично.
- 422 granular на `agree из planned`, `agree из declined`, `decline из agreed`, `decline из planned`.
- 404 soft-deleted (A1 → A4 → DELETE → T1). 404 unknown secret_token. 404 на secret_token не соответствующий формату (ранний reject в handler).
- 403 creator not in campaign. 403 telegram_user_id not in creators.
- 401 missing/bad/expired/`auth_date > now` initData.

**Campaigns (расширение):**
- POST с пустым `tma_url` → 200, `secret_token=NULL` в БД, кампания создана.
- POST с валидным `tma_url` → 200, `secret_token = last segment` в БД.
- POST с невалидным `tma_url` (last-segment <16 / не URL-safe / битый URL) → 422 `CodeInvalidTmaURL`.
- POST с уже занятым `tma_url` (двойная вставка) → 422 `CodeTmaURLConflict`. Race-кейс: два concurrent POST с одинаковым tma_url, один проходит, другой получает Conflict.
- PATCH `tma_url`: невалидный → 422; уже занятый → 422 `CodeTmaURLConflict`.
- PATCH с пустым `tma_url` (clear) → secret_token становится NULL.

Все assert'ы — строгие, конкретными значениями. Каждая mutate-ручка проверяет audit-row через `testutil.AssertAuditEntry`.

### Frontend unit (Vitest)

- `api/middleware.test.ts`: header injection.
- `hooks/useDecision.test.ts`: успех/ошибка маппятся правильно; `onError` всегда есть; double-submit guard.
- `CampaignBriefPage.test.tsx`: разные ответы бэка → правильное состояние UI.
- `AcceptedView.test.tsx` / `DeclinedView.test.tsx`: `alreadyDecided=true` → плашка видна; иначе скрыта.

i18n не мокается — `I18nextProvider` с реальными переводами.

### Frontend e2e — `frontend/e2e/tma/decision.spec.ts`

Helpers (`frontend/e2e/helpers/tma.ts`):
- `signInitDataForCreator(telegramUserID)` — fetch к testapi.
- `mockTelegramWebApp(page, initData)` — `page.addInitScript`.

Кейсы:
- happy agree flow.
- repeat-decision flow (плашка `tma-already-decided-banner`).
- 422 flow (declined → agree → error toast).
- 401 flow (без mockTelegramWebApp).
- 404 flow (soft-deleted кампания).

### Self-check агента (между unit и e2e, обязателен)

- **Backend:** после реализации middleware + AuthzService + service + handler + миграции — `make migrate-up`, `curl` POST /campaigns с валидным tma_url (получить secret_token из ответа или БД), `/test/tma/sign-init-data` для creator'а, `curl` к T1, чтение БД (status, decided_at, audit-row), проверка response shape. Дополнительно — POST /campaigns с занятым tma_url → 422.
- **Frontend:** `make start-tma`, в Playwright MCP открыть TMA с моком initData, потыкать agree/decline.

## Деливерейблы

### Backend

- `backend/migrations/<ts>_campaigns_secret_token.sql`
- `backend/internal/middleware/tma_initdata.go` (+ tests)
- `backend/internal/service/authz.go` (новые методы + tests)
- `backend/internal/handler/tma_campaign_creator.go` (+ tests)
- `backend/internal/service/tma_campaign_creator.go` (+ tests)
- `backend/internal/service/campaign.go` (расширение Create/Update; + tests)
- `backend/internal/service/audit_constants.go` (+ 2 константы)
- `backend/internal/domain/campaign.go` (+ ValidateTmaURL, ExtractSecretToken)
- `backend/internal/domain/campaign_creator.go` (+ Decision/Result типы + 3 granular ошибки)
- `backend/internal/domain/errors.go` (+ коды TMAForbidden/CampaignNotFound/InvalidTmaURL/TmaURLConflict/CampaignCreator*)
- `backend/internal/repository/campaign.go` (+ `GetBySecretToken`; расширение Row + Insert/Update + EAFP 23505)
- `backend/internal/repository/creator.go` (+ `GetByTelegramUserID`, если ещё нет)
- `backend/internal/repository/campaign_creator.go` (+ `ApplyDecision`, `GetByIDForUpdate`)
- `backend/internal/config/config.go` (+ `TMAInitDataTTL`)
- `backend/internal/testapi/handler/tma_sign_init_data.go` (+ tests)
- `backend/api/openapi.yaml` (T1/T2 paths + schemas + securityScheme + новые error codes)
- `backend/api/openapi-test.yaml` (sign-init-data)
- `backend/cmd/api/main.go` (регистрация middleware на `/tma/*` group; wiring в DI)
- `backend/e2e/tma/tma_test.go`
- `backend/e2e/campaign/campaign_test.go` (расширение — secret_token cases)
- `backend/e2e/testutil/tma.go`

### Frontend (TMA)

- `frontend/tma/src/api/client.ts`, `frontend/tma/src/api/middleware.ts`
- `frontend/tma/src/api/generated/schema.ts` (regenerated)
- `frontend/tma/src/features/campaign/hooks/useDecision.ts`
- `frontend/tma/src/features/campaign/CampaignBriefPage.tsx`
- `frontend/tma/src/features/campaign/AcceptedView.tsx`, `DeclinedView.tsx`
- `frontend/tma/src/i18n/locales/ru/campaign.json`
- `frontend/tma/src/features/campaign/campaigns.ts` (убрать mock-функцию)
- `frontend/e2e/helpers/tma.ts`
- `frontend/e2e/tma/decision.spec.ts`
- `frontend/e2e/types/schema.ts`, `frontend/e2e/types/test-schema.ts` (regenerated)

## Связанные документы

- Roadmap: `_bmad-output/planning-artifacts/campaign-roadmap.md`
- Design: `_bmad-output/planning-artifacts/design-campaign-creator-flow.md`
- Стандарты: `docs/standards/`
- Предыдущие spec'и группы: `archive/spec-campaign-creators-backend.md`, `archive/spec-campaign-notifications-backend.md`
