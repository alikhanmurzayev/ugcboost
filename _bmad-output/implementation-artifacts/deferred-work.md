---
title: "Deferred work — pre-existing / out-of-scope findings surfaced by /bmad-quick-dev review"
type: backlog
created: "2026-05-02"
---

# Deferred work

Findings surfaced during reviews that were classified as `defer` (not caused by the current story; pre-existing or out-of-scope). Each entry is a candidate for a future targeted PR.

## 2026-05-02 — chunk 7 review

### TS codegen: `nullable: true` + `$ref` молча дропает `| null` на TS-стороне
- **Источник:** Blind hunter #1, Acceptance auditor #F1
- **Проблема:** `openapi-typescript` для поля с `$ref` + `nullable: true` (как `CreatorApplicationDetailSocial.method`) генерирует `method?: SocialVerificationMethod` без `| null`. Go-codegen `oapi-codegen` правильно даёт `*SocialVerificationMethod`, но TS-клиент (web/tma/landing) видит `undefined` вместо `null`.
- **Почему defer:** Чанк 7 не подключает фронт к этим полям (creator-detail TMA — chunk 11, верификация UI — chunk 12). Нужно решить либо (a) `oneOf: [$ref, type: 'null']` (но oapi-codegen Go это не поддерживает), либо (b) сменить тип на inline с `nullable: true`, либо (c) принять как known-issue и обработать на TS-стороне через `?? null`. Лучше делать в чанке, где первый раз потребляем `method`.
- **Куда смотреть:** `backend/api/openapi.yaml` (`CreatorApplicationDetailSocial.method`) + `frontend/{web,tma,landing,e2e}/src/api/generated/schema.ts`.

### Preflight-валидации повторяются на каждой retry-итерации
- **Источник:** Blind hunter #3, Edge-case hunter #5
- **Проблема:** При коллизии verification_code сервис делает retry всего `submitOnce`, который повторно зовёт `HasActiveByIIN`, `dictRepo.GetActiveByCodes(categories)`, `dictRepo.GetActiveByCodes(cities)`. На worst-case (20 коллизий) — 60 ненужных read-запросов. Сейчас вероятность коллизии низкая (1M-кодовое пространство, ~20 active rows), retry редок — perf gap незаметен.
- **Почему defer:** Не корректность, а оптимизация. Текущая реализация дисциплинированно простая (одна `submitOnce`-функция). Когда verification-active rows вырастут до 10k+, будет смысл вынести retry в более узкое место — обернуть только генерацию кода + `appRepo.Create`, а categories/socials/consents/audit писать после первого успешного INSERT.
- **Куда смотреть:** `backend/internal/service/creator_application.go:127-249` (`submitOnce`).

### Migration backfill блокирует таблицу под Access Exclusive Lock на больших данных
- **Источник:** Edge-case hunter #7
- **Проблема:** Текущая миграция `20260502204252_creator_application_verification_storage.sql` делает ALTER ADD COLUMN + UPDATE bulk + ALTER SET NOT NULL в одной транзакции под Access Exclusive lock. На 20 prod-заявках — мгновенно. На 200к строк (если заявок наберётся) — 30-секундный простой read-запросов.
- **Почему defer:** Сейчас данных мало (20 prod-rows). Когда заявок наберётся 50k+ и появится скейл-планирование — стоит сделать аналогичную миграцию через `-- +goose NO TRANSACTION` + paginated UPDATE с `pg_advisory_lock`.
- **Куда смотреть:** `backend/migrations/20260502204252_creator_application_verification_storage.sql`.

### Graceful degradation для corrupt UUID в admin-list
- **Источник:** Edge-case hunter #3
- **Проблема:** Если в БД окажется row с битым `verified_by_user_id` (manual SQL fix, миграционный bug в чанках 8/9), `domainCreatorApplicationDetailSocialToAPI` вернёт error → admin-list эндпоинт даст 500 на всю страницу. Один битый UUID блокирует админку для всех заявок в той же page.
- **Почему defer:** Сейчас все `verified_by_user_id` = NULL (никто ещё не верифицировал) → corrupt UUID в принципе невозможен. Когда чанки 8/9 начнут писать UUID и появится реальный риск corruption, имеет смысл переключить mapper на skip-row + warn-log fallback (по аналогии с другими graceful-degradation паттернами в проекте).
- **Куда смотреть:** `backend/internal/handler/creator_application.go:240-268` (mapper) + `domainCreatorApplicationListPageToAPI` (call site).
