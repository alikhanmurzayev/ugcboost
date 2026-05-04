# Deferred work

Кросс-кутящие пробелы, найденные ревью, не относящиеся напрямую к закрытию текущего чанка. Каждая запись — кандидат на отдельный чанк/standard-update.

## 2026-05-04 — chunk 10 (manual verify) review

### 1. Валидация `actorUserID != ""` на входе сервисов

- **Trigger:** `internal/service/creator_application.go:872` (и аналоги в Submit/VerifyInstagramByCode).
- **Issue:** `middleware.UserIDFromContext` тихо возвращает `""` при отсутствии ключа. Для admin-mutating сервисов это ведёт к `pointer.ToString("")` в `verified_by_user_id` (UUID column → 22P02, 500) и к `audit_logs.actor_id = NULL` (NOT NULL violation). Production middleware гарантирует non-empty userID, но defense-in-depth не повредит.
- **Suggested fix:** правило в `docs/standards/backend-architecture.md` — admin-mutating сервисы валидируют `actorUserID != ""` перед `WithTx`, возвращая sentinel `domain.ErrUnauthorized`. Применить сразу к Submit/VerifyInstagramByCode/VerifyApplicationSocialManually единым PR.

### 2. `SELECT … FOR UPDATE` для admin transitions

- **Trigger:** `internal/service/creator_application.go:879` (новый chunk 10) + `internal/service/creator_application.go:771` (chunk 8 SendPulse). Нет ни в одном.
- **Issue:** Default `READ COMMITTED` + `appRepo.GetByID` без блокировки → race между двумя одновременными `verify`-вызовами на одну заявку: оба видят `verification`, оба делают `applyTransition(verification → moderation)`, в `creator_application_status_transitions` появляются две лишние строки + два audit-row. UpdateStatus идемпотентен по полю, но история становится противоречивой.
- **Suggested fix:** добавить `repository.CreatorApplicationRepo.GetByIDForUpdate` (squirrel `Suffix("FOR UPDATE")`) и звать его в начале tx у chunk 10 + retroactively chunk 8. Альтернатива — CAS-апдейт `UPDATE … WHERE status='verification'` + `RowsAffected==1`. Добавить правило в `docs/standards/backend-transactions.md`.

### 3. `GetByIDAndApplicationID` репо-метод

- **Trigger:** `internal/service/creator_application.go:891` (manual verify) + `:780` (SendPulse).
- **Issue:** `socialRepo.ListByApplicationID` тянет все соцсети заявки и линейно фильтрует в Go. Для текущих 1–3 соцсетей дёшево, но семантика «соцсеть должна принадлежать заявке» зашита в Go-коде, не в БД. Soft-delete или фильтрация в репо в будущем замаскирует ошибки под `ErrCreatorApplicationSocialNotFound`.
- **Suggested fix:** добавить `socialRepo.GetByIDAndApplicationID(ctx, socialID, applicationID)` — `WHERE id=$1 AND application_id=$2`, `sql.ErrNoRows` → sentinel. Заменить два call-site'а.

### 4. Clock injection для тестируемого времени

- **Trigger:** `internal/service/creator_application.go:920` (`time.Now().UTC()`) — pre-existing pattern по всему коду.
- **Issue:** Нельзя заморозить в unit-тестах — приходится ассертить через `WithinDuration`. Для нашего chunk не блокер (e2e ходят через реальное время), но рост числа state-machine действий (reject, withdraw, contract — chunks 12–19) сделает паттерн дороже.
- **Suggested fix:** интерфейс `clock.Clock` с `Now() time.Time`, инжектируется в сервисы. Заменять постепенно, начиная с новых state-mutating методов.

### 5. Zero-downtime hardening для FK-fix миграций

- **Trigger:** `backend/migrations/20260504010647_fix_socials_verified_by_user_fk_set_null.sql`.
- **Issue:** `DROP CONSTRAINT … ADD CONSTRAINT … REFERENCES users` в одной tx берёт `SHARE ROW EXCLUSIVE` lock на `creator_application_socials` И `users`. Под нагрузкой долгий SELECT по `users` блокирует миграцию, queue заполняет пул соединений.
- **Suggested fix:** для будущих FK-фиксов — `ADD CONSTRAINT … NOT VALID` сразу + отдельная миграция `VALIDATE CONSTRAINT`. Минимум — `SET lock_timeout = '5s'` в начале миграции. Документировать в `docs/standards/backend-database.md`. Конкретно для этой миграции: на проде применять во время low-traffic окна.

### 6. OpenAPI `additionalProperties: false` для пустых body

- **Trigger:** `backend/api/openapi.yaml:687` (chunk 10) + любые будущие endpoints с `requestBody.required: false` + пустым `type: object`.
- **Issue:** Без `additionalProperties: false` клиент может прислать `{"note":"..."}` или `{"force":true}` — сервер тихо проглотит. Админ думает, что добавил `note` — в audit его нет.
- **Suggested fix:** правило в `docs/standards/backend-codegen.md` — для всех empty-body endpoints явно `additionalProperties: false`. Cross-check'нуть существующие схемы (`SendPulseInstagramWebhookResult` в т.ч.).

### 7. Убрать `Handle` из `UpdateSocialVerificationParams` для manual-paths

- **Trigger:** `internal/service/creator_application.go:921` — manual verify передаёт `Handle: target.Handle`.
- **Issue:** `Handle` загружен в начале tx, между read и write параллельная транзакция (auto-verify webhook) могла его нормализовать → lost update. На manual-path Handle принципиально не меняется (спека: «никаких UPDATE'ов на других social-полях, только verified-блок»), но контракт репо требует поле.
- **Suggested fix:** разделить контракт — `UpdateVerificationOnly(ctx, id, verified, method, by, at)` без Handle для manual-path; `UpdateVerificationWithSelfFix(ctx, …, handle)` для SendPulse self-fix. Либо документировать инвариант комментарием + assert в тесте.
