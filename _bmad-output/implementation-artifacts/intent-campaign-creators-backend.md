---
title: "Intent: chunk 10 — campaign_creators backend"
type: intent
status: draft
created: "2026-05-07"
chunk: 10
roadmap: _bmad-output/planning-artifacts/campaign-roadmap.md
design: _bmad-output/planning-artifacts/design-campaign-creator-flow.md
---

# Intent: chunk 10 — backend campaign_creators (миграция + add/remove/list)

## Преамбула — стандарты обязательны

Перед любой строкой production-кода агент обязан полностью загрузить все файлы `docs/standards/` (через `/standards`). Применимы все. Особенно: `backend-architecture.md`, `backend-codegen.md`, `backend-constants.md`, `backend-design.md`, `backend-errors.md`, `backend-libraries.md`, `backend-repository.md`, `backend-testing-e2e.md`, `backend-testing-unit.md`, `backend-transactions.md`, `naming.md`, `security.md`, `review-checklist.md`. Каждое правило — hard rule; отклонение = finding.

## Скоуп

Бэк-чанк 10 из `_bmad-output/planning-artifacts/campaign-roadmap.md`. Полная картина мира — `_bmad-output/planning-artifacts/design-campaign-creator-flow.md` (Группы 4–6).

В этом чанке:
- миграция таблицы `campaign_creators`,
- 3 admin-ручки: **A1** `POST /campaigns/{id}/creators` (батч add → status=planned), **A2** `DELETE /campaigns/{id}/creators/{creatorId}` (single remove), **A3** `GET /campaigns/{id}/creators` (список без пагинации).

**Out of scope** (chunks 12/14): бот-нотификации, ремайндеры, TMA-страницы, agree/decline, partial-success delivery, PATCH `tma_url` lock после рассылок.

## Миграция (`backend/migrations/<timestamp>_campaign_creators.sql`)

```sql
CREATE TABLE campaign_creators (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id     UUID NOT NULL
        CONSTRAINT campaign_creators_campaign_id_fk REFERENCES campaigns(id),
    creator_id      UUID NOT NULL
        CONSTRAINT campaign_creators_creator_id_fk REFERENCES creators(id),
    status          TEXT NOT NULL,
    invited_at      TIMESTAMPTZ,
    invited_count   INT  NOT NULL DEFAULT 0,
    reminded_at     TIMESTAMPTZ,
    reminded_count  INT  NOT NULL DEFAULT 0,
    decided_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT campaign_creators_status_check
        CHECK (status IN ('planned','invited','declined','agreed')),
    CONSTRAINT campaign_creators_campaign_creator_unique
        UNIQUE (campaign_id, creator_id)
);
```

- **Status enum как TEXT + CHECK** (как `creator_applications.status`). Стабильное имя CHECK'а — для возможного `pgErr.ConstraintName` mapping'а.
- **FK без CASCADE** — `campaigns` и `creators` не имеют hard-delete по бизнес-логике. Лучше явная ошибка целостности на race.
- **Explicit FK constraint names** — `campaign_creators_campaign_id_fk` / `_creator_id_fk` — нужны репозиторию, чтобы по `pgErr.ConstraintName` дисамбигуировать какой именно FK сломался при 23503.
- **Без `is_deleted`** — Remove физически удаляет запись (по design'у).
- **DB-defaults только integrity**: `id`, `created_at`, `updated_at`, `invited_count=0`, `reminded_count=0`. Это pure integrity (счётчики начинаются с нуля независимо от бизнеса).
- **`status` без DEFAULT** — business default ставит сервис (`status='planned'` при INSERT в A1) по `backend-repository.md § Целостность данных`.
- **Индексы**: только UNIQUE на `(campaign_id, creator_id)` — composite btree покрывает A3 lookup. Дополнительных индексов сейчас не вводим (нагрузка низкая).
- **Header-комментарий** в стиле `creators.sql` / `campaigns.sql`: scope = chunk 10 campaign-roadmap, объяснение state-полей (4 значения), explicit constraint-имена для pgErr.ConstraintName handling, FK без CASCADE.
- **Down**: `DROP TABLE IF EXISTS campaign_creators;` (всё каскадно через PRIMARY KEY и UNIQUE).

## Domain (`backend/internal/domain/campaign_creator.go`)

```go
type CampaignCreator struct {
    ID            string     `json:"id"`
    CampaignID    string     `json:"campaign_id"`
    CreatorID     string     `json:"creator_id"`
    Status        string     `json:"status"`
    InvitedAt     *time.Time `json:"invited_at,omitempty"`
    InvitedCount  int        `json:"invited_count"`
    RemindedAt    *time.Time `json:"reminded_at,omitempty"`
    RemindedCount int        `json:"reminded_count"`
    DecidedAt     *time.Time `json:"decided_at,omitempty"`
    CreatedAt     time.Time  `json:"created_at"`
    UpdatedAt     time.Time  `json:"updated_at"`
}

const (
    CampaignCreatorStatusPlanned  = "planned"
    CampaignCreatorStatusInvited  = "invited"
    CampaignCreatorStatusDeclined = "declined"
    CampaignCreatorStatusAgreed   = "agreed"
)
```

JSON-теги — snake_case (struct сериализуется в `audit_logs.new_value`/`old_value` как-есть, как `Campaign`).

**Domain-ошибки** (в `backend/internal/domain/campaign_creator.go` — sentinel/business/validation в зависимости от семантики):
- `ErrCreatorAlreadyInCampaign` — `*ValidationError` с granular `CodeCreatorAlreadyInCampaign` → 422 (strict-422 batch). Actionable message: «Креатор уже добавлен в кампанию. Проверьте список и удалите дубликат, либо удалите креатора из кампании, прежде чем добавить заново».
- `ErrCampaignCreatorNotFound` — `errors.New(...)` sentinel → респонз 404 `CodeCampaignCreatorNotFound` (через расширение `respondError`).
- `ErrCampaignCreatorRemoveAfterAgreed` — `*ValidationError` с `CodeCampaignCreatorRemoveAfterAgreed` → 422. Actionable message: «Креатор уже согласился — удалить из кампании нельзя. Дождитесь подписания договора или обратитесь к админу».
- `ErrCreatorNotFound` — переиспользуем уже существующий (если есть в `domain/creator.go`); иначе вводим новый sentinel + `CodeCreatorNotFound` → 422 (в контексте batch).

**Codes** (в `backend/internal/domain/errors.go` — где живут существующие `CodeCampaign*`):
- `CodeCampaignCreatorIdsRequired`
- `CodeCampaignCreatorIdsDuplicates`
- `CodeCampaignCreatorNotFound`
- `CodeCreatorAlreadyInCampaign`
- `CodeCampaignCreatorRemoveAfterAgreed`
- `CodeCreatorNotFound` (если ещё нет)

`respondError` расширяется веткой `errors.Is(err, domain.ErrCampaignCreatorNotFound)` → 404 + `CodeCampaignCreatorNotFound`.

## Repository (`backend/internal/repository/campaign_creator.go`)

**Константы:**
```go
const (
    CampaignCreatorsCampaignCreatorUnique = "campaign_creators_campaign_creator_unique"
    CampaignCreatorsCampaignFK            = "campaign_creators_campaign_id_fk"
    CampaignCreatorsCreatorFK             = "campaign_creators_creator_id_fk"
)

const (
    TableCampaignCreators                = "campaign_creators"
    CampaignCreatorColumnID              = "id"
    CampaignCreatorColumnCampaignID      = "campaign_id"
    CampaignCreatorColumnCreatorID       = "creator_id"
    CampaignCreatorColumnStatus          = "status"
    CampaignCreatorColumnInvitedAt       = "invited_at"
    CampaignCreatorColumnInvitedCount    = "invited_count"
    CampaignCreatorColumnRemindedAt      = "reminded_at"
    CampaignCreatorColumnRemindedCount   = "reminded_count"
    CampaignCreatorColumnDecidedAt       = "decided_at"
    CampaignCreatorColumnCreatedAt       = "created_at"
    CampaignCreatorColumnUpdatedAt       = "updated_at"
)
```

**Row + теги:**
```go
type CampaignCreatorRow struct {
    ID            string     `db:"id"`
    CampaignID    string     `db:"campaign_id"     insert:"campaign_id"`
    CreatorID     string     `db:"creator_id"      insert:"creator_id"`
    Status        string     `db:"status"          insert:"status"`
    InvitedAt     *time.Time `db:"invited_at"`
    InvitedCount  int        `db:"invited_count"`
    RemindedAt    *time.Time `db:"reminded_at"`
    RemindedCount int        `db:"reminded_count"`
    DecidedAt     *time.Time `db:"decided_at"`
    CreatedAt     time.Time  `db:"created_at"`
    UpdatedAt     time.Time  `db:"updated_at"`
}

var (
    campaignCreatorSelectColumns = sortColumns(stom.MustNewStom(CampaignCreatorRow{}).SetTag(string(tagSelect)).TagValues())
    campaignCreatorInsertMapper  = stom.MustNewStom(CampaignCreatorRow{}).SetTag(string(tagInsert))
)
```

**Интерфейс:**
```go
type CampaignCreatorRepo interface {
    Add(ctx context.Context, campaignID, creatorID, status string) (*CampaignCreatorRow, error)
    GetByCampaignAndCreator(ctx context.Context, campaignID, creatorID string) (*CampaignCreatorRow, error)
    DeleteByID(ctx context.Context, id string) error
    ListByCampaign(ctx context.Context, campaignID string) ([]*CampaignCreatorRow, error)
    DeleteForTests(ctx context.Context, id string) error
}
```

**`Add`** — INSERT one row через `squirrel.Insert(TableCampaignCreators).SetMap(toMap(...))` + `RETURNING ...`. На pgErr:
- 23505 + `ConstraintName == CampaignCreatorsCampaignCreatorUnique` → `domain.ErrCreatorAlreadyInCampaign`.
- 23503 + `ConstraintName == CampaignCreatorsCreatorFK` → `domain.ErrCreatorNotFound`.
- 23503 + `ConstraintName == CampaignCreatorsCampaignFK` → `domain.ErrCampaignNotFound`.
- остальные ошибки — обёрнутый `fmt.Errorf("campaign_creators add: %w", err)`.

**`GetByCampaignAndCreator`** — `SELECT ... WHERE campaign_id = ? AND creator_id = ?`. `sql.ErrNoRows` пробрасывается без обёртки (как `campaignRepository.GetByID`); сервис маппит в `ErrCampaignCreatorNotFound`.

**`DeleteByID`** — `DELETE FROM campaign_creators WHERE id = ?`. Вернуть `sql.ErrNoRows` если 0 rows affected (как `campaignRepository.DeleteForTests`).

**`ListByCampaign`** — `SELECT ... WHERE campaign_id = ? ORDER BY created_at ASC, id ASC` (стабильный порядок — старейшие добавления первыми; tie-breaker по `id` для совпадающих timestamp'ов). Возвращает `[]*CampaignCreatorRow` (или `nil, nil` если пусто).

**`DeleteForTests`** — `DELETE FROM campaign_creators WHERE id = ?`. Возвращает `sql.ErrNoRows` если 0 rows.

Repo создаётся через `RepoFactory.NewCampaignCreatorRepo(db dbutil.DB) CampaignCreatorRepo` — добавить в `repository/factory.go`.

## Service (`backend/internal/service/campaign_creator.go`)

```go
type CampaignCreatorRepoFactory interface {
    NewCampaignCreatorRepo(db dbutil.DB) repository.CampaignCreatorRepo
    NewCampaignRepo(db dbutil.DB) repository.CampaignRepo
    NewAuditRepo(db dbutil.DB) repository.AuditRepo
}

type CampaignCreatorService struct {
    pool        dbutil.Pool
    repoFactory CampaignCreatorRepoFactory
    logger      logger.Logger
}

func NewCampaignCreatorService(pool dbutil.Pool, rf CampaignCreatorRepoFactory, log logger.Logger) *CampaignCreatorService { ... }

func (s *CampaignCreatorService) Add(ctx context.Context, campaignID string, creatorIDs []string) ([]*domain.CampaignCreator, error)
func (s *CampaignCreatorService) Remove(ctx context.Context, campaignID, creatorID string) error
func (s *CampaignCreatorService) List(ctx context.Context, campaignID string) ([]*domain.CampaignCreator, error)
```

### Add

1. **Pre-fetch campaign** через `s.repoFactory.NewCampaignRepo(s.pool).GetByID(ctx, campaignID)`:
   - `sql.ErrNoRows` → `domain.ErrCampaignNotFound`.
   - `row.IsDeleted == true` → `domain.ErrCampaignNotFound` (soft-deleted = 404 для этой ручки).
2. `dbutil.WithTx(ctx, s.pool, func(tx) error { ... })`:
   - `creatorRepo := s.repoFactory.NewCampaignCreatorRepo(tx)`; `auditRepo := s.repoFactory.NewAuditRepo(tx)`.
   - `result := make([]*domain.CampaignCreator, 0, len(creatorIDs))`
   - Loop по `creatorIDs`: `row, err := creatorRepo.Add(ctx, campaignID, creatorID, domain.CampaignCreatorStatusPlanned)`:
     - на любую ошибку — return err (rollback всего batch'а — strict-422).
     - `entity := campaignCreatorRowToDomain(row)`; `result = append(result, entity)`.
     - `writeAudit(ctx, auditRepo, AuditActionCampaignCreatorAdd, AuditEntityTypeCampaignCreator, entity.ID, nil, entity)` — return err if fails.
3. После `WithTx` (commit успешен): `s.logger.Info(ctx, "campaign creators added", "campaign_id", campaignID, "count", len(result))`.
4. Return `result, nil`.

**Race** soft-delete между шагом 1 и шагом 2 — acceptable (data осиротеет под soft-deleted, A3 потом вернёт 404). Не фиксим в MVP.

### Remove

1. Pre-fetch campaign (same logic, 404 если not-found / soft-deleted).
2. `dbutil.WithTx`:
   - `creatorRepo := ...`; `auditRepo := ...`.
   - `row, err := creatorRepo.GetByCampaignAndCreator(ctx, campaignID, creatorID)`:
     - `sql.ErrNoRows` → `domain.ErrCampaignCreatorNotFound`.
     - other err → wrapped.
   - if `row.Status == domain.CampaignCreatorStatusAgreed` → `domain.ErrCampaignCreatorRemoveAfterAgreed` (422).
   - `oldEntity := campaignCreatorRowToDomain(row)`.
   - `creatorRepo.DeleteByID(ctx, row.ID)` (sql.ErrNoRows → wrapped, race с concurrent delete — пишем wrapped fmt.Errorf, не sentinel).
   - `writeAudit(ctx, auditRepo, AuditActionCampaignCreatorRemove, AuditEntityTypeCampaignCreator, oldEntity.ID, oldEntity, nil)`.
3. После `WithTx`: `s.logger.Info(ctx, "campaign creator removed", "campaign_id", campaignID, "creator_id", creatorID)`.

### List

1. Pre-fetch campaign (404 если not-found / soft-deleted).
2. `rows, err := s.repoFactory.NewCampaignCreatorRepo(s.pool).ListByCampaign(ctx, campaignID)` (без tx, read-only).
3. Map row→domain. Возвращаем `[]*domain.CampaignCreator` (может быть пустой).

### Helper

`campaignCreatorRowToDomain(*repository.CampaignCreatorRow) *domain.CampaignCreator` — straight field copy.

## Handler (`backend/internal/handler/campaign_creator.go`)

Strict-server-методы по сгенерированным типам (`api.AddCampaignCreatorsRequestObject`, etc.).

### AddCampaignCreators

1. `s.authzService.CanAddCampaignCreators(ctx)` — fail → return err (403 через respondError).
2. Валидация body:
   - `len(request.Body.CreatorIds) == 0` → `domain.NewValidationError(domain.CodeCampaignCreatorIdsRequired, "Список креаторов обязателен. Выберите хотя бы одного креатора для добавления.")`.
   - dedup: построить `seen := map[uuid.UUID]struct{}{}`; если duplicate → `domain.NewValidationError(domain.CodeCampaignCreatorIdsDuplicates, "В списке есть дубликаты creator-id. Удалите повторяющиеся идентификаторы и повторите запрос.")`.
   - cap `maxItems: 200` уже зарезан oapi-codegen schema'ой.
3. `creatorIDs := []string{}` (`u.String()` для каждого UUID).
4. `entities, err := s.campaignCreatorService.Add(ctx, request.Id.String(), creatorIDs)` — error propagation.
5. Map domain→api: `apiItems := make([]api.CampaignCreator, len(entities))`; через `domainCampaignCreatorToAPI`.
6. Return `api.AddCampaignCreators201JSONResponse{Data: api.CampaignCreatorsListData{Items: apiItems}}`.

### RemoveCampaignCreator

1. authz.
2. `err := s.campaignCreatorService.Remove(ctx, request.Id.String(), request.CreatorId.String())` — propagate.
3. Return `api.RemoveCampaignCreator204Response{}`.

### ListCampaignCreators

1. authz.
2. `entities, err := s.campaignCreatorService.List(ctx, request.Id.String())`.
3. Map → `api.CampaignCreatorsListData{Items: apiItems}`.
4. Return `api.ListCampaignCreators200JSONResponse{...}`.

### Mapping helper

```go
func domainCampaignCreatorToAPI(c *domain.CampaignCreator) (api.CampaignCreator, error) {
    id, err := uuid.Parse(c.ID)
    if err != nil { return api.CampaignCreator{}, fmt.Errorf("parse campaign_creator id %q: %w", c.ID, err) }
    campaignID, err := uuid.Parse(c.CampaignID)
    if err != nil { return api.CampaignCreator{}, fmt.Errorf("parse campaign id %q: %w", c.CampaignID, err) }
    creatorID, err := uuid.Parse(c.CreatorID)
    if err != nil { return api.CampaignCreator{}, fmt.Errorf("parse creator id %q: %w", c.CreatorID, err) }

    return api.CampaignCreator{
        Id:            openapi_types.UUID(id),
        CampaignId:    openapi_types.UUID(campaignID),
        CreatorId:     openapi_types.UUID(creatorID),
        Status:        api.CampaignCreatorStatus(c.Status),
        InvitedAt:     c.InvitedAt,
        InvitedCount:  c.InvitedCount,
        RemindedAt:    c.RemindedAt,
        RemindedCount: c.RemindedCount,
        DecidedAt:     c.DecidedAt,
        CreatedAt:     c.CreatedAt,
        UpdatedAt:     c.UpdatedAt,
    }, nil
}
```

## Authz (`backend/internal/authz/campaign_creator.go`)

Mirror campaign-pattern:
```go
func (a *AuthzService) CanAddCampaignCreators(ctx context.Context) error    { return a.requireRole(ctx, api.Admin) }
func (a *AuthzService) CanRemoveCampaignCreator(ctx context.Context) error  { return a.requireRole(ctx, api.Admin) }
func (a *AuthzService) CanListCampaignCreators(ctx context.Context) error   { return a.requireRole(ctx, api.Admin) }
```

(Точное имя `requireRole`/`mustHaveRole` — берётся из существующего authz-сервиса.)

## Audit

**Константы** в `backend/internal/service/audit_constants.go`:
```go
AuditActionCampaignCreatorAdd    = "campaign_creator_add"
AuditActionCampaignCreatorRemove = "campaign_creator_remove"
AuditEntityTypeCampaignCreator   = "campaign_creator"
```

**Per-event схема** (через `writeAudit`):
- `Add` (per creator): `EntityID = campaign_creator.id`, `OldValue = nil`, `NewValue = *domain.CampaignCreator` (полный snapshot после INSERT).
- `Remove`: `EntityID = campaign_creator.id`, `OldValue = *domain.CampaignCreator` (snapshot до DELETE), `NewValue = nil`.

Actor — admin user_id из ctx (`middleware.UserIDFromContext`). PII в payload нет (UUID-ы + status + timestamps + counters).

`List` (A3) — без audit (read-only по стандарту `backend-testing-e2e.md`).

## API surface (правки `backend/api/openapi.yaml`)

Под `tags: [campaigns]` (без отдельного тэга — расширяем существующий ресурс).

**Пути:**

```yaml
/campaigns/{id}/creators:
  parameters:
    - name: id
      in: path
      required: true
      schema: { type: string, format: uuid }
  post:
    operationId: addCampaignCreators
    summary: Add creators to a campaign in batch (admin-only)
    description: |
      Adds the supplied creators to the campaign with status=planned. The
      whole batch is validated atomically: if any creator_id is invalid /
      already in the campaign, the entire request is rejected (strict-422)
      and no row is inserted. Soft-deleted campaigns are treated as not
      found (404), unlike GET /campaigns/{id}.
    security: [{ bearerAuth: [] }]
    requestBody:
      required: true
      content:
        application/json:
          schema: { $ref: "#/components/schemas/AddCampaignCreatorsInput" }
    responses:
      "201": { description: Creators added, content: ... AddCampaignCreatorsResult }
      "401": { ... }
      "403": { $ref: "#/components/responses/Forbidden" }
      "404": { description: Campaign not found or soft-deleted, ... }
      "422": { description: Validation error (empty / duplicate ids, creator already in campaign, creator not found), ... }
      default: { $ref: "#/components/responses/UnexpectedError" }
  get:
    operationId: listCampaignCreators
    summary: List creators in a campaign (admin-only)
    description: |
      Returns all campaign_creators rows for the campaign with status,
      counters and last-action timestamps. No pagination — campaign sizes
      are bounded by the admin-curated batch flow. Soft-deleted campaigns
      are treated as not found (404).
    security: [{ bearerAuth: [] }]
    responses:
      "200": { ... CampaignCreatorsListResult }
      "401": { ... }
      "403": { $ref: "#/components/responses/Forbidden" }
      "404": { description: Campaign not found or soft-deleted, ... }
      default: { $ref: "#/components/responses/UnexpectedError" }

/campaigns/{id}/creators/{creatorId}:
  parameters:
    - { name: id, in: path, required: true, schema: { type: string, format: uuid } }
    - { name: creatorId, in: path, required: true, schema: { type: string, format: uuid } }
  delete:
    operationId: removeCampaignCreator
    summary: Remove a creator from a campaign (admin-only)
    description: |
      Removes the creator from the campaign. Allowed in any state except
      "agreed" — once a creator agrees, the relationship is locked for the
      contract pipeline (chunk 16). Soft-deleted campaigns and missing
      relationships both surface as 404.
    security: [{ bearerAuth: [] }]
    responses:
      "204": { description: Removed }
      "401": { ... }
      "403": { $ref: "#/components/responses/Forbidden" }
      "404": { description: Campaign not found / soft-deleted, or creator not in campaign }
      "422": { description: Validation error (status=agreed) }
      default: { $ref: "#/components/responses/UnexpectedError" }
```

**Schemas:**

```yaml
AddCampaignCreatorsInput:
  type: object
  required: [creatorIds]
  properties:
    creatorIds:
      type: array
      minItems: 1
      maxItems: 200
      items: { type: string, format: uuid }
      description: Creator UUIDs to add. Strict batch — any invalid id rejects all.

CampaignCreatorStatus:
  type: string
  description: Current state of the campaign-creator relationship.
  enum: [planned, invited, declined, agreed]

CampaignCreator:
  type: object
  required: [id, campaignId, creatorId, status, invitedCount, remindedCount, createdAt, updatedAt]
  properties:
    id: { type: string, format: uuid }
    campaignId: { type: string, format: uuid }
    creatorId: { type: string, format: uuid }
    status: { $ref: "#/components/schemas/CampaignCreatorStatus" }
    invitedAt: { type: string, format: date-time, nullable: true }
    invitedCount: { type: integer }
    remindedAt: { type: string, format: date-time, nullable: true }
    remindedCount: { type: integer }
    decidedAt: { type: string, format: date-time, nullable: true }
    createdAt: { type: string, format: date-time }
    updatedAt: { type: string, format: date-time }

CampaignCreatorsListData:
  type: object
  required: [items]
  properties:
    items:
      type: array
      items: { $ref: "#/components/schemas/CampaignCreator" }

AddCampaignCreatorsResult:
  type: object
  required: [data]
  properties:
    data: { $ref: "#/components/schemas/CampaignCreatorsListData" }

CampaignCreatorsListResult:
  type: object
  required: [data]
  properties:
    data: { $ref: "#/components/schemas/CampaignCreatorsListData" }
```

После правки yaml — `make generate-api` (oapi-codegen регенерит server, e2e clients, frontend schema.ts).

## Edge cases (must-cover в тестах)

| # | Сценарий | Ожидаемое поведение | Где тестируется |
|---|---|---|---|
| 1 | Soft-deleted campaign | 404 на A1/A2/A3 | service unit + e2e |
| 2 | Campaign не существует | 404 | service unit + e2e |
| 3 | Race: soft-delete после pre-fetch | данные пишутся; A3 → 404; acceptable | doc comment в service, не тестируем |
| 4 | Re-add того же creator (non-concurrent) | 422 `CreatorAlreadyInCampaign`, rollback всего батча | repo unit + service unit + e2e |
| 5 | Несуществующий creator_id в батче | 422 `CreatorNotFound`, rollback | repo unit + service unit + e2e |
| 6 | Race: campaign hard-delete в момент INSERT | 404 `CampaignNotFound` (через 23503) | repo unit |
| 7 | Empty `creatorIds: []` | 422 `CodeCampaignCreatorIdsRequired` | handler unit + e2e |
| 8 | Duplicate id в батче | 422 `CodeCampaignCreatorIdsDuplicates` | handler unit + e2e |
| 9 | Re-add после Remove | 201, status=planned | e2e |
| 10 | Remove из status=agreed | 422 `RemoveAfterAgreed` | service unit (e2e — в chunk 14, когда появится business-flow для agreed) |
| 11 | Remove несуществующей связи | 404 `CampaignCreatorNotFound` | service unit + e2e |
| 12 | Audit-write fail внутри WithTx | rollback batch / DELETE | service unit |
| 13 | A3 на пустой кампании | 200 `items:[]` | e2e |
| 14 | Double-DELETE A2 | second call → 404 | e2e |
| 15 | brand_manager / no-auth | 403 / 401 | authz unit + handler unit + e2e |

## Тестирование

### Unit (стандарт `backend-testing-unit.md` — gate ≥80% per-method, `t.Parallel()` везде, mockery моки, новый mock на каждый `t.Run`)

- `repository/campaign_creator_test.go`:
  - `TestCampaignCreatorRepository_Add`: SQL-assert (capture INSERT через `mock.Run`), happy → `RETURNING` row, 23505 → `ErrCreatorAlreadyInCampaign`, 23503/creator_fk → `ErrCreatorNotFound`, 23503/campaign_fk → `ErrCampaignNotFound`, generic err → wrapped.
  - `TestCampaignCreatorRepository_GetByCampaignAndCreator`: SQL-assert, happy, sql.ErrNoRows propagated.
  - `TestCampaignCreatorRepository_DeleteByID`: SQL-assert, happy, 0-rows → sql.ErrNoRows.
  - `TestCampaignCreatorRepository_ListByCampaign`: SQL-assert (включая ORDER BY created_at, id), happy с несколькими rows, empty.
  - `TestCampaignCreatorRepository_DeleteForTests`: happy, sql.ErrNoRows.

- `service/campaign_creator_test.go` (`Test{Service}_{Method}` per стандарт):
  - `TestCampaignCreatorService_Add`: campaign-not-found, campaign-soft-deleted, repo Add fails mid-batch → strict-422 rollback (assert: subsequent Add не вызывались, audit на rollback'нутые тоже не вызывались), audit fails → rollback, happy multi-batch (assert returned slice + audit per creator с правильными аргументами через captured `mock.Run`).
  - `TestCampaignCreatorService_Remove`: campaign-not-found, campaign-soft-deleted, GetByCampaignAndCreator sql.ErrNoRows → `ErrCampaignCreatorNotFound`, status=agreed → `ErrCampaignCreatorRemoveAfterAgreed`, DeleteByID error → wrapped, audit fails → rollback, happy (assert audit OldValue snapshot + NewValue=nil).
  - `TestCampaignCreatorService_List`: campaign-not-found, campaign-soft-deleted, repo error → wrapped, happy (assert mapping row→domain).

- `handler/campaign_creator_test.go`:
  - `TestServer_AddCampaignCreators`: authz forbidden → 403, empty CreatorIds → 422 IdsRequired, duplicate → 422 IdsDuplicates, service domain-error propagation (404/422), happy → 201 + items (full struct equal через подмену динамических полей по стандарту).
  - `TestServer_RemoveCampaignCreator`: authz forbidden, service errors propagation, happy → 204.
  - `TestServer_ListCampaignCreators`: authz forbidden, service errors, happy → 200 + items.

- `authz/campaign_creator_test.go`:
  - `TestAuthzService_CanAddCampaignCreators` / `_CanRemoveCampaignCreator` / `_CanListCampaignCreators` — admin OK, brand_manager → ErrForbidden, no-role → ErrForbidden (mirror campaign-pattern).

### E2E (стандарт `backend-testing-e2e.md` — отдельный module, Russian narrative header, `t.Parallel()`, generated client, `testutil.AssertAuditEntry` для mutate-ручек)

`backend/e2e/campaign_creator/campaign_creator_test.go`:
- Header — связный нарратив (русский, godoc): про что каждый `Test*`, какие audit-инварианты закрывает, как работает cleanup через `E2E_CLEANUP`. Без bullet-list.
- `TestAddCampaignCreators`: setup admin + campaign + 2 creator (через approve flow / test-API). Сценарии: happy batch (assert каждое поле `CampaignCreator` строго через `require.Equal` после подмены динамических; assert `testutil.AssertAuditEntry` per creator); strict-422 на re-add (один creator уже в кампании); strict-422 на несуществующий creator_id; 422 на empty/duplicate ids (raw HTTP, бо openapi schema на uuid не пустит пустую строку через generated client); 404 missing campaign; 404 soft-deleted campaign (admin создаёт + soft-deletes сам); 403 brand_manager; 401 anon.
- `TestRemoveCampaignCreator`: setup admin + campaign + creator, Add → Remove → A3 (assert список пустой + audit `removed`); 404 на removed (double-delete); 404 на missing campaign; 404 на missing creator-в-кампании; 403/401. **422-from-agreed не делаем — нет business-flow для status=agreed в чанке 10.**
- `TestListCampaignCreators`: empty campaign → 200 `items:[]`; multi-creator с разными timestamp'ами (assert ORDER BY created_at); 404 missing/soft-deleted; 403/401.
- Cleanup: defer-stack — сначала campaign_creators (через A2 для активных + DeleteForTests для тех, кто в `agreed` если бы тестировали; в чанке 10 не нужно), потом creators, потом campaign. Порядок уважает FK.

### Self-check агента (между unit и e2e — стандарт roadmap'а)

1. `make compose-up && make migrate-up` — миграция применилась без ошибок.
2. `make start-backend` — backend поднят на :8082.
3. Через `curl` (admin token из `/auth/login` test-helper):
   - создать кампанию: `POST /campaigns` { name, tmaUrl } → запомнить `id`.
   - создать 2 креатора через approve-flow или `/test/seed-creator` (если есть).
   - `POST /campaigns/{id}/creators { creatorIds: [c1, c2] }` → 201 + items (2 строки).
   - `GET /campaigns/{id}/creators` → 200 + items.length=2, status="planned" у обоих, counts=0.
   - `DELETE /campaigns/{id}/creators/{c1}` → 204.
   - `GET /campaigns/{id}/creators` → items.length=1.
4. `psql` (через docker exec на postgres-container):
   - `SELECT * FROM campaign_creators WHERE campaign_id = '<id>';` — assert одна строка с creator_id=c2, status='planned', invited_count=0.
   - `SELECT action, entity_id, old_value, new_value FROM audit_logs WHERE entity_type = 'campaign_creator' ORDER BY created_at;` — 2 events `campaign_creator_add` (для c1, c2) + 1 event `campaign_creator_remove` (для c1). Payload — UUID-ы и status.
5. Расхождение со спекой = баг в реализации. Агент сам фиксит, перезапускает self-check, переходит к e2e в той же сессии. HALT — только если всплыла продуктовая развилка, не описанная в спеке.

## Что не делаем в чанке 10

- Бот-нотификации (chunk 12).
- TMA-страницы по `secret_token` (chunk 14).
- A4 / A5 / T1 / T2 ручки (chunks 12, 14).
- PATCH `tma_url` lock после рассылок (chunk 12).
- Status-transitions кроме `→ planned` (только Add ставит status; transitions из/в invited/declined/agreed — chunks 12, 14).
- Race-тест на concurrent insert (по design'у — non-concurrent кейса достаточно).
- Frontend (chunk 11).

## Связанные документы

- Roadmap: `_bmad-output/planning-artifacts/campaign-roadmap.md`
- Design Групп 4–6: `_bmad-output/planning-artifacts/design-campaign-creator-flow.md`
- Стандарты: `docs/standards/`
- Существующий backend campaign-pattern (как эталон): `backend/internal/repository/campaign.go`, `backend/internal/service/campaign.go`, `backend/internal/handler/campaign.go`, `backend/internal/authz/campaign.go`.
