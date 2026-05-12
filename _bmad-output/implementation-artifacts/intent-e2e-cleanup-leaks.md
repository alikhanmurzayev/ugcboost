# Intent: E2E cleanup leaks — campaign_creators и contracts

## Контекст

На staging после прогона backend + frontend E2E тестов остаются тестовые данные:
кампании, campaign_creators, креаторы, заявки, contracts. Механизм cleanup
существует (LIFO через `t.Cleanup` / frontend `afterEach`), но молча ломается
в двух местах, оставляя строки в БД.

## Root causes

### Cause 1 — campaign_creators с терминальными статусами

`RegisterCampaignCreatorCleanup` зовёт production DELETE
`/campaigns/{id}/creators/{creatorId}`. Сервисный слой блокирует удаление:

```go
if row.Status == domain.CampaignCreatorStatusAgreed {
    return domain.ErrCampaignCreatorRemoveAfterAgreed
}
```

Тесты chunk-16/17/18 (tma_test, contract_test, webhook_test) переводят
campaign_creators в `agreed` / `signing` / `signed` / `declined` — cleanup
возвращает 422, молча логируется через `t.Logf`, строка остаётся.

Следствие: после того как campaign_creator завис, `CampaignRepo.DeleteForTests`
пытается сделать `DELETE FROM campaigns WHERE id = ?` — FK
`campaign_creators.campaign_id → campaigns(id)` без CASCADE блокирует удаление,
ошибка снова молча логируется. Кампания тоже остаётся.

### Cause 2 — contracts вообще не очищаются

`contracts` строки создаются внутри `SetupCampaignWithSigningCreator` через
outbox-worker. При этом:
- `ContractRepo` не имеет `DeleteForTests`
- в `CleanupEntity` нет `case testapi.Contract`
- нет `RegisterContractCleanup` ни в `contract_setup.go`, ни в `testutil/`

Все contract-тесты оставляют строки в `contracts` навсегда.

## Решение

### 1. `openapi-test.yaml` — два новых типа в `CleanupEntityType`

```yaml
CleanupEntityType:
  enum: [user, brand, creator_application, creator, campaign,
         campaign_creator, contract]   # ← добавить эти два
```

После изменения: `make generate-api` (регенерирует `testapi/server.gen.go`,
`backend/e2e/testclient/types.gen.go`, `frontend/e2e/types/test-schema.ts`).

### 2. `CampaignCreatorRepo` — новый метод `DeleteForTests`

Файл: `backend/internal/repository/campaign_creator.go`

Добавить в интерфейс `CampaignCreatorRepo`:
```go
DeleteForTests(ctx context.Context, campaignID, creatorID string) error
```

Реализация: `DELETE FROM campaign_creators WHERE campaign_id = $1 AND creator_id = $2`.
Возвращает `sql.ErrNoRows` если строки нет (cleanup treat-as-success).

Идентификатор — пара `(campaignID, creatorID)`, потому что callers всегда
имеют оба значения, а значение `campaign_creator.id` не всегда доступно
на стороне тестов (особенно в `approve_with_campaigns_test.go`).

### 3. `ContractRepo` — новый метод `DeleteForTests`

Файл: `backend/internal/repository/contracts.go`

Добавить в интерфейс `ContractRepo`:
```go
DeleteForTests(ctx context.Context, id string) error
```

Реализация: `DELETE FROM contracts WHERE id = $1`. FK
`campaign_creators.contract_id → contracts ON DELETE SET NULL` означает,
что при удалении contract Postgres сам обнулит ссылку в campaign_creators —
никаких дополнительных cascade-шагов не нужно.

### 4. `TestAPICleanupRepoFactory` — расширить интерфейс

Файл: `backend/internal/handler/testapi.go`

```go
type TestAPICleanupRepoFactory interface {
    NewUserRepo(db dbutil.DB) repository.UserRepo
    NewBrandRepo(db dbutil.DB) repository.BrandRepo
    NewCreatorApplicationRepo(db dbutil.DB) repository.CreatorApplicationRepo
    NewCreatorRepo(db dbutil.DB) repository.CreatorRepo
    NewCampaignRepo(db dbutil.DB) repository.CampaignRepo
    NewCampaignCreatorRepo(db dbutil.DB) repository.CampaignCreatorRepo  // ← NEW
    NewContractRepo(db dbutil.DB) repository.ContractRepo                // ← NEW
}
```

### 5. `CleanupEntity` handler — два новых case

Файл: `backend/internal/handler/testapi.go`

```go
case testapi.CampaignCreator:
    // id field = "campaignID:creatorID" (двоеточие-разделитель)
    parts := strings.SplitN(req.Id, ":", 2)
    if len(parts) != 2 {
        return nil, domain.NewValidationError(domain.CodeValidation,
            "campaign_creator id must be 'campaignID:creatorID'")
    }
    deleteErr = h.repos.NewCampaignCreatorRepo(h.pool).
        DeleteForTests(ctx, parts[0], parts[1])

case testapi.Contract:
    deleteErr = h.repos.NewContractRepo(h.pool).DeleteForTests(ctx, req.Id)
```

### 6. `CampaignRepo.DeleteForTests` — каскад в транзакции

Файл: `backend/internal/repository/campaign.go`

Заменить текущий простой DELETE на транзакцию:

```go
func (r *campaignRepository) DeleteForTests(ctx context.Context, id string) error {
    // Сначала удаляем campaign_creators (FK без CASCADE)
    delCC := sq.Delete(TableCampaignCreators).
        Where(sq.Eq{CampaignCreatorColumnCampaignID: id})
    if _, err := dbutil.Exec(ctx, r.db, delCC); err != nil {
        return fmt.Errorf("delete campaign_creators for campaign %s: %w", id, err)
    }
    // Затем кампанию
    delCamp := sq.Delete(TableCampaigns).Where(sq.Eq{CampaignColumnID: id})
    n, err := dbutil.Exec(ctx, r.db, delCamp)
    if err != nil {
        return err
    }
    if n == 0 {
        return sql.ErrNoRows
    }
    return nil
}
```

Примечание: этот метод вызывается уже внутри transport-level (не в tx сервиса),
поэтому wraps в `dbutil.WithTx` нужен на уровне handler'а если нужна атомарность
cascade. Если `r.db` уже поддерживает `dbutil.DB` — можно сделать два
последовательных Exec без явного WithTx (оба в одном соединении из пула,
PostgreSQL не даст grounding). Проще всего: обернуть оба Exec в `dbutil.WithTx`
в самом методе (repo исключительно для тестов, не нарушает архитектурный принцип).

### 7. `testutil/campaign_creator.go` — новая реализация

Полностью заменить `RegisterCampaignCreatorCleanup`:

```go
// RegisterCampaignCreatorCleanup schedules a hard-delete of the
// campaign_creators row via POST /test/cleanup-entity (bypasses the
// business-level guard that blocks removal after status=agreed). Use
// this instead of the production DELETE endpoint so tests that drive
// campaign_creators to terminal statuses (agreed/signing/signed/declined)
// still clean up correctly.
func RegisterCampaignCreatorCleanup(t *testing.T, campaignID, creatorID string) {
    t.Helper()
    RegisterCleanup(t, func(ctx context.Context) error {
        tc := NewTestClient(t)
        resp, err := tc.CleanupEntityWithResponse(ctx, testclient.CleanupEntityJSONRequestBody{
            Type: testclient.CampaignCreator,
            Id:   campaignID + ":" + creatorID,
        })
        if err != nil {
            return fmt.Errorf("cleanup campaign_creator (%s, %s): %w", campaignID, creatorID, err)
        }
        if resp.StatusCode() != http.StatusNoContent && resp.StatusCode() != http.StatusNotFound {
            return fmt.Errorf("cleanup campaign_creator (%s, %s): unexpected status %d",
                campaignID, creatorID, resp.StatusCode())
        }
        return nil
    })
}
```

**Callers** — убрать параметры `c *apiclient.ClientWithResponses` и `adminToken string`
(они больше не нужны), везде где передавались:

| Файл | Строка | Было | Станет |
|------|--------|------|--------|
| `testutil/tma.go` | 124 | `(t, c, adminToken, campaignID, creator.CreatorID)` | `(t, campaignID, creator.CreatorID)` |
| `tma/tma_test.go` | 393 | `(t, c, adminToken, campaignID, creator.CreatorID)` | `(t, campaignID, creator.CreatorID)` |
| `campaign_creator/campaign_creator_test.go` | 219, 252, 388, 505 | `(t, adminClient, adminToken, ...)` | `(t, ...)` |
| `campaign_creator/campaign_notify_test.go` | 116 | `(t, c, adminToken, ...)` | `(t, ...)` |
| `creator_applications/approve_with_campaigns_test.go` | 119, 120 | `(t, c, fx.AdminToken, ...)` | `(t, ...)` |

### 8. `testutil/contract_setup.go` — `RegisterContractCleanup`

Добавить в `testutil/` новый хелпер (можно в `contract_setup.go` или отдельный
`contract.go`):

```go
// RegisterContractCleanup schedules a hard-delete of the contracts row
// via POST /test/cleanup-entity. Must be registered AFTER
// RegisterCampaignCreatorCleanup so LIFO fires contract cleanup last
// (campaign_creators.contract_id SET NULL happens automatically on delete,
// no FK violation).
func RegisterContractCleanup(t *testing.T, contractID string) {
    t.Helper()
    RegisterCleanup(t, func(ctx context.Context) error {
        tc := NewTestClient(t)
        resp, err := tc.CleanupEntityWithResponse(ctx, testclient.CleanupEntityJSONRequestBody{
            Type: testclient.Contract,
            Id:   contractID,
        })
        if err != nil {
            return fmt.Errorf("cleanup contract %s: %w", contractID, err)
        }
        if resp.StatusCode() != http.StatusNoContent && resp.StatusCode() != http.StatusNotFound {
            return fmt.Errorf("cleanup contract %s: unexpected status %d", contractID, resp.StatusCode())
        }
        return nil
    })
}
```

Вызов в `SetupCampaignWithSigningCreator` (после того как `rec.AdditionalInfo`
стал известен — это и есть `contracts.id`):

```go
// rec.AdditionalInfo = contracts.id (наш UUID, прописывается в additional_info
// при SendToSign).
RegisterContractCleanup(t, rec.AdditionalInfo)
```

LIFO-порядок в `SetupCampaignWithSigningCreator` после изменений:
1. `RegisterContractCleanup` (регистрируется последним → fires first) — удаляет contracts строку, SET NULL на campaign_creators.contract_id
2. `RegisterCampaignCreatorCleanup` (из `SetupCampaignWithInvitedCreator`) — hard-delete campaign_creator
3. `RegisterCreatorCleanup` — удаляет creator
4. `RegisterCreatorApplicationCleanup` — удаляет creator_application
5. `RegisterCampaignCleanup` (регистрируется первым → fires last) — теперь без FK-блокировки

### 9. `TestAPICleanupRepoFactory` implementation в `cmd/api`

Файл: `backend/cmd/api/main.go` (или где wire-up dependencies).
`repository.RepoFactory` уже реализует все нужные методы — просто добавить
два новых в интерфейс и убедиться, что `RepoFactory` их тоже реализует
(методы `NewCampaignCreatorRepo` и `NewContractRepo` скорее всего уже есть).

## Файлы к изменению

| Файл | Изменение |
|------|-----------|
| `backend/api/openapi-test.yaml` | +2 значения в `CleanupEntityType` enum |
| `backend/internal/repository/campaign_creator.go` | +`DeleteForTests(ctx, campaignID, creatorID)` в интерфейс и реализацию |
| `backend/internal/repository/contracts.go` | +`DeleteForTests(ctx, id)` в интерфейс и реализацию |
| `backend/internal/repository/campaign.go` | Изменить `DeleteForTests` — каскад через campaign_creators |
| `backend/internal/handler/testapi.go` | +2 поля в `TestAPICleanupRepoFactory`; +2 case в `CleanupEntity` |
| `backend/e2e/testutil/campaign_creator.go` | Новая реализация `RegisterCampaignCreatorCleanup` (drop c/adminToken params) |
| `backend/e2e/testutil/contract_setup.go` | +`RegisterContractCleanup`; вызов из `SetupCampaignWithSigningCreator` |
| `backend/e2e/testutil/tma.go` | Обновить вызов `RegisterCampaignCreatorCleanup` |
| `backend/e2e/tma/tma_test.go` | Обновить вызов `RegisterCampaignCreatorCleanup` |
| `backend/e2e/campaign_creator/campaign_creator_test.go` | Обновить 4 вызова `RegisterCampaignCreatorCleanup` |
| `backend/e2e/campaign_creator/campaign_notify_test.go` | Обновить 1 вызов |
| `backend/e2e/creator_applications/approve_with_campaigns_test.go` | Обновить 2 вызова |
| `backend/internal/testapi/server.gen.go` | Регенерировать (`make generate-api`) |
| `backend/e2e/testclient/types.gen.go` | Регенерировать |
| `frontend/e2e/types/test-schema.ts` | Регенерировать |

## Что НЕ меняется

- Фронтенд E2E хелперы (`removeCampaignCreator` в `helpers/api.ts`) — frontend-тесты
  не доводят campaign_creators до terminal статусов, production DELETE достаточен.
- Миграции — никаких DDL-изменений, только application-level code.
- Unit-тесты репозиториев — новые методы нужно покрыть через мокирование pgxmock
  (стандарт `backend-testing-unit.md`). Добавить в `repository/campaign_creator_test.go`
  и `repository/contracts_test.go`.
- Моки — `make generate-mocks` после изменения интерфейсов.

## Порядок реализации

1. `openapi-test.yaml` → `make generate-api`
2. Repository-слой (три файла: campaign_creator, contracts, campaign)
3. `make generate-mocks`
4. Unit-тесты для новых repo методов
5. `TestAPICleanupRepoFactory` + `CleanupEntity` handler
6. `testutil/campaign_creator.go` + `testutil/contract_setup.go`
7. Обновить callers (9 мест)
8. `make test-e2e-backend` — убедиться что тесты проходят и staging чист
