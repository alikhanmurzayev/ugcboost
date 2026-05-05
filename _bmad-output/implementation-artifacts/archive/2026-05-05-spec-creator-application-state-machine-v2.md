---
title: 'State-machine v2: добавить approved, выпилить awaiting_contract / contract_sent / signed'
type: feature
created: '2026-05-05'
status: in-review
baseline_commit: f8dc573
context:
  - docs/standards/backend-architecture.md
  - docs/standards/backend-codegen.md
  - docs/standards/backend-constants.md
  - docs/standards/backend-errors.md
  - docs/standards/backend-repository.md
  - docs/standards/backend-testing-unit.md
  - docs/standards/backend-testing-e2e.md
  - docs/standards/frontend-api.md
  - docs/standards/frontend-components.md
  - docs/standards/frontend-state.md
  - docs/standards/frontend-testing-unit.md
  - docs/standards/frontend-types.md
  - docs/standards/naming.md
  - docs/standards/security.md
---

<frozen-after-approval>

## Intent

**Problem:** Текущая v1-стейт-машина (миграция `20260501222829`, `creator-application-state-machine.md` от 2026-05-01) описывает 7-статусный pipeline вокруг договора: `verification → moderation → awaiting_contract → contract_sent → signed`. По решению от 2026-05-03 (revision в `creator-onboarding-roadmap.md`) договор / TrustMe / подпись уехали из creator-onboarding'а в будущий campaign-roadmap. Онбординг кончается на `approved`. Бэкенд-сервисы и handler'ы дропаемые статусы не читают, но они присутствуют в нескольких местах: enum (`backend/api/openapi.yaml`, generated schemas), domain-константах, partial unique index по ИИН, exhaustive-switch на фронте (`ApplicationActions.tsx` из chunk 14), i18n-ключах раздела «Договоры», stub-странице `ContractsPage` и роуте на неё. Каждое из этих мест — балласт, который путает читателя кода, расширяет attack surface (любой переход в дропаемые статусы пройдёт CHECK) и ломает экспирьенс модератора (мёртвый пункт «Договоры» в sidebar'е, ведущий на coming-soon-заглушку).

**Approach:** Один forward-only PR в три слоя. **БД**: миграция с guard на отсутствие рядов в дропаемых статусах → транзитный CHECK с `approved` → финальный CHECK `(verification, moderation, approved, rejected, withdrawn)` → перестройка `creator_applications_iin_active_idx` на active-set `{verification, moderation}`. **Backend Go**: дропнуть три константы, добавить `*StatusApproved`, сжать `CreatorApplicationActiveStatuses` и `CreatorApplicationAllStatuses`. OpenAPI: enum → 5 значений. `creatorApplicationAllowedTransitions` **не трогаем** — переход `moderation → approved` подключит chunk 18. `withdrawn` остаётся в enum как зарезервированный терминал без переходов. **Frontend**: `make generate-api` сжимает `CreatorApplicationStatus` union, после чего: убрать три `case` из exhaustive-switch в `ApplicationActions.tsx` (добавить `case "approved": return null;` — approve UI прилетает в chunk 19), сжать `it.each` в `ApplicationActions.test.tsx`, удалить мёртвый «Договоры»-flow целиком (stub-страница + роут в `App.tsx` + menu item в `DashboardLayout.tsx` + ключ `stages.contracts` в i18n). Backend и frontend тесты приводятся к новому набору. Design-docs (`creator-application-state-machine.md`, `creator-verification-concept.md`) переписываются под v2 в этом же PR. Roadmap получает revision-entry от 2026-05-XX и `[x]`-метку на chunk 17.

## Boundaries & Constraints

**Always:**
- Forward-only миграция, в одной транзакции (default goose). Промежуточные состояния (DROP CHECK без ADD, DROP INDEX без CREATE) не должны быть наблюдаемы извне — паттерн как в v1-миграции `20260501222829_creator_applications_state_machine.sql`.
- Guard в Up: `RAISE EXCEPTION`, если есть ряды со статусами `awaiting_contract` / `contract_sent` / `signed`. Прод-данных в этих статусах нет (pipeline до договора так и не активировался — все ~100 prod-заявок в `verification`, `moderation` или `rejected`), но guard защищает на случай, если кто-то проиграл локально.
- Down: симметричный guard на наличие `approved`-рядов (если chunk 18 успел запуститься — Down невалиден). Документировать ограничение в комментарии миграции.
- `make generate-api` прогоняется в этом же PR; сгенерированные файлы (`*.gen.go`, `frontend/*/generated/schema.ts`, `frontend/e2e/types/schema.ts`) коммитятся той же ревизией, что и yaml-источник (`backend-codegen.md` § Что ревьюить).
- Production-код пишется без объяснительных комментариев — только WHY, и только когда неочевидно (`naming.md` § Комментарии). Комментарий миграции — исключение по тому же паттерну, что в v1.
- Design-docs (state-machine + verification-concept) обновляются ровно в этом PR — иначе разъезд кода и living-документации.

**Ask First:**
- Если grep по фронту/бэку **после** перечисленных в Code Map правок находит хоть одно дополнительное место, где runtime-логика читает `awaiting_contract` / `contract_sent` / `signed` (вне generated-schema'ов, архивных спек и test-литералов, явно перечисленных в Tasks), — HALT, не выпиливать молча. На момент написания спеки runtime-зависимости найдены ровно в `ApplicationActions.tsx`, `ApplicationActions.test.tsx`, `ContractsPage.tsx`, `App.tsx` (роут), `DashboardLayout.tsx` (menu item), i18n-ключе `stages.contracts`. Бэкенд (service / handler / repository) — чисто.
- Если параллельный chunk 16.5 (e2e на moderation flow) к моменту реализации chunk 17 затронул `ApplicationActions.tsx` или `ApplicationActions.test.tsx` — HALT, согласовать переписывание (он не должен этого делать, но проверить грепом перед мерджем).
- Если на момент мержа в roadmap'е появится новый revision-entry, который меняет состав v2 (например, добавляет `signed` обратно как resurrection терминал) — HALT и согласовать.

**Never:**
- Не добавлять переход `moderation → approved` в `creatorApplicationAllowedTransitions`. Это чанк 18 — там придёт `*ApproveService`, audit, transition row, создание `users`-row.
- Не добавлять переходы для `withdrawn` (на текущий момент нет ни UI, ни endpoint'а; `withdrawn` — зарезервированный терминал на будущее).
- Не редактировать миграцию `20260501222829_creator_applications_state_machine.sql` in-place — она прогонялась на staging и в локальных prod-снапшотах (`backend-repository.md` § Миграции). Любая правка — новой forward-миграцией.
- Не вводить кодовое название «approved-but-pending-contract» или похожее — `approved` окончательный, обратного перехода в onboarding-roadmap нет.
- Не добавлять business default `DEFAULT 'verification'` или подобный в миграцию — статус задаётся сервисом при подаче (`backend-repository.md` § Целостность данных).
- В `ApplicationActions.tsx` для `case "approved":` возвращать `return null;` (approve-кнопка прилетит в chunk 19). Никаких inline-кнопок «Одобрить» или approve-handler'ов в этом чанке.
- Не удалять `RejectedPage` stub и роут `/rejected` (это будущий отдельный экран, не часть договорного flow).
- Не удалять прототип в `frontend/web/src/_prototype/` (он ссылается на собственный локальный contract-status-union `"not_sent" | "sent" | "signed"`, не на `CreatorApplicationStatus`; остаётся как референс будущих UI).

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|---|---|---|---|
| Up на чистой staging-БД (нет рядов в дропаемых статусах) | enum после v1 + ряды только в `verification`/`moderation`/`rejected` | Guard проходит → CHECK расширяется на `approved` → CHECK сужается до 5 значений → index перестраивается на 2 active-статуса | N/A |
| Up на «грязной» БД (есть ряд в `signed`) | хотя бы один ряд `status IN ('awaiting_contract','contract_sent','signed')` | `RAISE EXCEPTION 'creator_applications has rows with status awaiting_contract/contract_sent/signed; no automatic mapping in v2 state-machine, manual review required before applying this migration'` | Транзакция откатывается, миграция не применяется |
| Down до старта chunk 18 (нет approved-рядов) | enum v2 + ряды только в legal v1-подмножестве | Index перестраивается обратно на 4 active-статуса → CHECK расширяется → CHECK сужается до v1-набора | N/A |
| Down после старта chunk 18 (есть approved-ряд) | хотя бы один ряд `status='approved'` | `RAISE EXCEPTION 'creator_applications has rows with status approved; no 1:1 mapping in v1 state-machine, manual review required before reverting this migration'` | Транзакция откатывается, Down не применяется |
| Concurrent submit с одинаковым ИИН | два креатора подают одновременно с одним ИИН, оба попадают в `verification` | Partial unique index `WHERE status IN ('verification','moderation')` → один INSERT проходит, второй ловит 23505 → repo транслирует в `ErrCreatorApplicationDuplicate` | Без изменений в коде repo (сообщение об ошибке остаётся то же) |
| Повторная подача после `rejected` | предыдущая заявка с тем же ИИН в `rejected` | Index не задевает терминальные статусы → новая заявка в `verification` проходит | N/A |
| `make generate-api` после правки openapi | enum v2 в yaml | `server.gen.go` / `apiclient/types.gen.go` / `frontend/*/generated/schema.ts` теряют `AwaitingContract`/`ContractSent`/`Signed`, получают `Approved`; TS union `CreatorApplicationStatus` сжимается до 5 значений | Любые ручные `case "signed":` / `if status == api.Signed` в коде упадут компиляцией (TS — exhaustiveness, Go — undefined identifier). Это и есть инструмент гарантии от тихого drift'а |
| Удаление `case "awaiting_contract"` / `"contract_sent"` / `"signed"` из exhaustive-switch `ApplicationActions.tsx` | сжатый `ApplicationStatus` union | После правки switch покрывает 5 веток: `verification` / `moderation` (action-кнопки), `approved` / `rejected` / `withdrawn` (return null). `default: never` остаётся как safety-net | Если забыть `case "approved":` — TS компиляция упадёт на exhaustiveness check, ошибка укажет на `default` ветку |
| Удаление stub'а `ContractsPage` + роута + menu item'а | в `App.tsx` нет роута `/admin/creator-applications/contracts`, в sidebar нет «Договоры» | Sidebar показывает только три рабочих пункта (Верификация / Модерация / Отклонённые); попытка зайти по старому URL — стандартный 404 от React Router | Если оставить роут без stub'а — `tsc` упадёт на impossibleimport. Если оставить menu item без роута — клик ведёт в 404 |
| Финальный grep по runtime-коду | после всех правок, поиск `awaiting_contract`/`contract_sent`/`StatusSigned`/`StatusAwaiting`/`StatusContract` вне `_bmad-output/implementation-artifacts/archive/` и комментариев миграций | Пусто во всём backend (включая generated `*.gen.go`) и frontend (включая `frontend/*/generated/schema.ts`); единственное допустимое — литералы внутри новой v2-миграции и комментарий внутри v1-миграции (документ-история) | Если grep находит ещё одно место — HALT (см. Ask First) |

</frozen-after-approval>

## Code Map

**Миграция (новая):**
- `backend/migrations/{ts}_creator_applications_state_machine_v2.sql` — создаётся через `make migrate-create NAME=creator_applications_state_machine_v2`. Паттерн — клон структуры v1-миграции `20260501222829_creator_applications_state_machine.sql` (guard / транзитный CHECK / финальный CHECK / DROP+CREATE INDEX).

**Backend (Go):**
- `backend/internal/domain/creator_application.go:13–28` — константы статусов и `CreatorApplicationActiveStatuses`. Дропнуть три, добавить `*StatusApproved`, сжать active-список.
- `backend/internal/domain/creator_application.go:118–121` — комментарий на `CodeCreatorApplicationNotInVerification` упоминает `moderation/awaiting_contract/…`. Заменить на `moderation/approved/...` или просто «после verification».
- `backend/internal/domain/creator_application.go:438–446` — `CreatorApplicationAllStatuses`. Сжать до 5 значений.
- `backend/internal/domain/creator_application.go:515–528` — `creatorApplicationAllowedTransitions`. **Не трогать** в этом чанке.

**OpenAPI:**
- `backend/api/openapi.yaml:565` — описание counts-endpoint'а («default: verification + moderation + awaiting_contract»). Поправить (фронт сейчас использует только verification + moderation).
- `backend/api/openapi.yaml:906–911` — `CreatorApplicationStatus.enum`. Заменить на `[verification, moderation, approved, rejected, withdrawn]`.

**Сгенерированный код (обновляется через `make generate-api`):**
- `backend/internal/api/server.gen.go` — константы статусов.
- `backend/e2e/apiclient/types.gen.go` — то же.
- `frontend/web/src/api/generated/schema.ts`.
- `frontend/tma/src/api/generated/schema.ts`.
- `frontend/landing/src/api/generated/schema.ts`.
- `frontend/e2e/types/schema.ts`.

**Backend unit-тесты:**
- `backend/internal/repository/creator_application_test.go:30, 44, 58` — `WithArgs("950515312348", "verification", "moderation", "awaiting_contract", "contract_sent")` × 3. Сократить до `WithArgs("950515312348", "verification", "moderation")`.
- `backend/internal/domain/creator_application_test.go:144` — комментарий + кейс `Moderation → awaiting_contract`. Заменить на «`Moderation → approved` belongs to chunk 18; until that handler lands the only legal exit is reject» + кейс `CreatorApplicationStatusApproved` (не legal без расширения transitions).
- `backend/internal/service/creator_application_test.go:1978–1980` — table-сценарий с `Awaiting/ContractSent/Signed` как «нелегальные стартовые статусы для X». Адаптировать: оставить только `Approved`, `Rejected`, `Withdrawn` как нелегальные стартовые (они терминальные).
- `backend/internal/service/creator_application_test.go:2187, 2193` — фикстура `Status: domain.CreatorApplicationStatusSigned` + ассерт `ErrorContains(...Signed)`. Заменить на `*StatusApproved`.
- `backend/internal/handler/creator_application_test.go:694–696` — три ряда таблицы (`AwaitingContract` / `ContractSent` / `Signed` → wire-enum). Удалить три ряда, добавить один `(*StatusApproved, api.Approved)`.

**Backend e2e:**
- `backend/e2e/creator_applications/counts_test.go:9` — комментарий «(moderation, awaiting_contract, ...)». Заменить на «(moderation, approved, ...)» или нейтральное «остальные статусы добавляются по мере появления переходов».
- `backend/e2e/webhooks/sendpulse_instagram_test.go` — на момент обзора захардкоженных дропаемых статусов нет, проверить grep'ом ещё раз при реализации.

**Frontend (TypeScript):**
- `frontend/web/src/features/creatorApplications/components/ApplicationActions.tsx:52–57` — exhaustive-switch на `ApplicationStatus`. Удалить три `case "awaiting_contract"` / `"contract_sent"` / `"signed"`. Добавить `case "approved":` с `return null;` (approve UI прилетит в chunk 19; сейчас approved-заявка показывает пустой actions-блок, как rejected/withdrawn). `default: never` оставить.
- `frontend/web/src/features/creatorApplications/components/ApplicationActions.test.tsx:96–107` — `it.each([...])` тестирует «renders nothing for $status». Удалить три строки (`"awaiting_contract"`, `"contract_sent"`, `"signed"`), добавить `"approved"`. Финальный массив: `["approved", "rejected", "withdrawn"]`.
- `frontend/web/src/features/creatorApplications/stubs/ContractsPage.tsx` — удалить файл целиком (мёртвая страница после выпиливания договорного flow).
- `frontend/web/src/App.tsx:14, 77` — убрать импорт `ContractsPage` и `<Route element={<ContractsPage />} />` на путь `/admin/creator-applications/contracts`. Убедиться, что соответствующий путь нигде не остался в `routes.ts` / навигации.
- `frontend/web/src/shared/layouts/DashboardLayout.tsx:79` — удалить menu item «Договоры» (`label: t("creatorApplications:stages.contracts.title")`) целиком (включая его `to`/`testid`/возможный `badge` ключ).
- `frontend/web/src/shared/i18n/locales/ru/creatorApplications.json` — удалить ключ `stages.contracts` целиком (объект `{"title": "Договоры"}`). Не добавлять `stages.approved.*` в этом чанке — нет screen'а для approved до chunk 19.

**Living docs:**
- `_bmad-output/planning-artifacts/creator-application-state-machine.md` — переписать целиком под v2: 2 активных + 3 терминальных (`approved`, `rejected`, `withdrawn`). Убрать переходы вокруг договора. **Удалить секцию «Видимые состояния для креатора (TMA)» целиком** — креатор не имеет UI заявки в MVP, узнаёт о переходах только через Telegram-бот. `withdrawn` помечается как зарезервированный — переходов нет до отдельной инициативы.
- `_bmad-output/planning-artifacts/creator-verification-concept.md:43, 47` — формулировки «доступно на любом активном статусе» подразумевают 4 активных. Сжать до «`verification` или `moderation`» (явно). Поле «Очерёдность реализации» (l.50) уже соответствует факту, не трогать.

## Tasks & Acceptance

**Execution:**

- [x] `backend/migrations/{ts}_creator_applications_state_machine_v2.sql` -- создать через `make migrate-create NAME=creator_applications_state_machine_v2`. Структура: guard на `awaiting_contract`/`contract_sent`/`signed` → DROP CHECK → ADD транзитный CHECK `(verification, moderation, awaiting_contract, contract_sent, signed, rejected, withdrawn, approved)` → DROP CHECK → ADD финальный CHECK `(verification, moderation, approved, rejected, withdrawn)` → DROP INDEX `creator_applications_iin_active_idx` → CREATE INDEX `WHERE status IN ('verification', 'moderation')`. Down: симметрично, с guard'ом на `approved`. -- Целевой schema-shift.
- [x] `backend/internal/domain/creator_application.go` -- дропнуть `CreatorApplicationStatusAwaitingContract` / `*ContractSent` / `*Signed` (l.16-18); добавить `CreatorApplicationStatusApproved = "approved"` рядом с остальными активными/терминальными. Сжать `CreatorApplicationActiveStatuses` (l.23-28) до `[verification, moderation]`. Сжать `CreatorApplicationAllStatuses` (l.438-446) до `[verification, moderation, approved, rejected, withdrawn]`. Поправить комментарий на l.118-121 (упоминание `awaiting_contract/...`). `creatorApplicationAllowedTransitions` (l.520-528) -- НЕ ТРОГАТЬ. -- Source of truth для статусов в Go.
- [x] `backend/api/openapi.yaml:911` -- enum → `[verification, moderation, approved, rejected, withdrawn]`. -- Wire-контракт.
- [x] `backend/api/openapi.yaml:565` -- описание counts-endpoint'а: убрать упоминание `awaiting_contract` из примера дефолтного бейджа (фронт сейчас суммирует `verification + moderation`). -- Документация под фактическое использование.
- [x] `make generate-api` -- прогнать; коммит-дельта по `backend/internal/api/server.gen.go`, `backend/e2e/apiclient/types.gen.go`, `frontend/{web,tma,landing}/src/api/generated/schema.ts`, `frontend/e2e/types/schema.ts`. -- Вторичный источник правды должен соответствовать yaml.
- [x] `backend/internal/repository/creator_application_test.go` -- l.30, 44, 58: `WithArgs("950515312348", "verification", "moderation", "awaiting_contract", "contract_sent")` → `WithArgs("950515312348", "verification", "moderation")`. SQL-литерал в тесте (двойная проверка констант) тоже сжать до 2 placeholder'ов на active set. -- Зеркало нового partial unique index.
- [x] `backend/internal/domain/creator_application_test.go:144` -- кейс `Moderation → awaiting_contract` заменить на `Moderation → approved` (нелегальный, потому что transition не подключён в этом чанке) + комментарий «approved transition belongs to chunk 18». -- Гарантия, что вне явного transition `approved` не пройдёт.
- [x] `backend/internal/service/creator_application_test.go` -- l.1978-1980: список «нелегальных стартовых статусов» для соответствующего сценария обновить (`Awaiting`/`ContractSent`/`Signed` → удалить, добавить `Approved`, оставить `Rejected`/`Withdrawn`). l.2187, 2193: фикстура `*StatusSigned` → `*StatusApproved`, ассерт по подстроке тоже. -- Сценарии «уже терминальная заявка» и «нелегальный старт» под новый набор.
- [x] `backend/internal/handler/creator_application_test.go:694-696` -- три ряда таблицы (`AwaitingContract`/`ContractSent`/`Signed` ↔ wire-enum) удалить, добавить один `{domain.CreatorApplicationStatusApproved, api.Approved}`. -- Покрытие domain↔wire-mapping для нового статуса; уверенность, что дропнутые wire-константы больше не существуют (компилятор сразу подскажет).
- [x] `backend/e2e/creator_applications/counts_test.go:9` -- header-комментарий «(moderation, awaiting_contract, ...)» переформулировать без упоминания дропнутых статусов. -- Header-нарратив (см. `backend-testing-e2e.md` § Комментарий в начале файла).
- [x] `_bmad-output/planning-artifacts/creator-application-state-machine.md` -- переписать целиком: статусы → 2 активных + 3 терминальных (`approved`/`rejected`/`withdrawn`); переходы → только `verification → moderation`, `verification → rejected`, `moderation → rejected`; `moderation → approved` помечен как «реализуется в chunk 18»; `withdrawn` помечен как «зарезервированный, переходов нет»; убрать упоминания `awaiting_contract`/`contract_sent`/`signed` и весь договорной flow. **Удалить секцию «Видимые состояния для креатора (TMA)» целиком** — UI для креатора в скоупе MVP нет (TMA выпилен 2026-05-03, см. ремарку в roadmap'е). Заменить её на короткий абзац: «Креатор не имеет UI с состоянием заявки. О каждом значимом переходе он узнаёт из уведомления Telegram-бота — конкретные сообщения подключаются по мере реализации соответствующих чанков (см. roadmap группы 1, 3, 5)». Обновить `updated:` в frontmatter. -- Living source of truth.
- [x] `_bmad-output/planning-artifacts/creator-verification-concept.md:43, 47` -- формулировки «на любом активном статусе» сжать до «`verification` или `moderation`». Обновить `updated:` в frontmatter. -- Согласованность с v2.
- [x] `frontend/web/src/features/creatorApplications/components/ApplicationActions.tsx` -- l.52-54: убрать три `case "awaiting_contract"` / `"contract_sent"` / `"signed"`. Добавить `case "approved":` с `return null;` рядом с rejected/withdrawn. `default: never` оставить как safety-net. -- Сжатие exhaustive-switch под v2 enum.
- [x] `frontend/web/src/features/creatorApplications/components/ApplicationActions.test.tsx:96-101` -- удалить три строки (`"awaiting_contract"`, `"contract_sent"`, `"signed"`), добавить `"approved"`; обновить `it.each` description при необходимости. -- Покрытие нового набора статусов «нечего показывать».
- [x] `frontend/web/src/features/creatorApplications/stubs/ContractsPage.tsx` -- удалить файл целиком. -- Stub мёртвый flow.
- [x] `frontend/web/src/App.tsx` -- l.14: убрать `import ContractsPage from "@/features/creatorApplications/stubs/ContractsPage"`. l.77: удалить `<Route ... element={<ContractsPage />} />` на путь contracts. Грепом убедиться, что соседних `routes.ts`-констант / тестов на этот путь больше нет. -- Удаление роута.
- [x] `frontend/web/src/shared/layouts/DashboardLayout.tsx:79` -- удалить menu item «Договоры» целиком (объект с `label: t("creatorApplications:stages.contracts.title")` и его `to`/`testid`/`badge`). -- Удаление мёртвой ссылки в sidebar.
- [x] `frontend/web/src/shared/i18n/locales/ru/creatorApplications.json` -- удалить ключ `stages.contracts` целиком. -- Удаление мёртвых переводов.
- [x] `frontend/web/src/shared/layouts/DashboardLayout.test.tsx` -- если в нём есть кейс на бейдж «Договоры» / на проверку «navigation has 4 items» — обновить под новый набор (3 menu items вместо 4 в creator-applications-группе). -- Тест-снимок навигации не должен соврать.
- [x] `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` -- frontmatter `revisions:` добавить новую строку «2026-05-XX: chunk 17 готов — state-machine v2: дропнуты awaiting_contract/contract_sent/signed, добавлен approved, partial unique index по ИИН на active-set {verification, moderation}, выпилен договорный flow на фронте (ContractsPage stub + роут + menu item + i18n)». В разделе «Группа 5. Approve» chunk 17 пометить `[x]`, добавить ссылку на смерженный PR. -- Living roadmap должен синхронно отражать факт.

**Acceptance Criteria:**

- Given чистая staging-БД с заявками только в `verification`/`moderation`/`rejected`, when `make migrate-up` применяет новую миграцию, then `creator_applications_status_check` ограничивает status пятёркой `(verification, moderation, approved, rejected, withdrawn)`, а `creator_applications_iin_active_idx` имеет `WHERE status IN ('verification', 'moderation')`.
- Given в БД есть хотя бы один ряд `creator_applications.status IN ('awaiting_contract','contract_sent','signed')`, when `goose up` запускает Up-миграцию, then миграция падает с `RAISE EXCEPTION` и не оставляет частичных изменений (single-tx гарантия).
- Given v2-миграция применена и есть хотя бы один ряд со `status='approved'`, when `goose down` пробует откатить, then миграция падает с `RAISE EXCEPTION` про невозможность 1:1-маппинга `approved` обратно в v1.
- Given после `make generate-api`, when `go build ./...` (бэк) и `tsc --noEmit` (фронт) запускаются, then компиляция проходит на всех трёх frontend-приложениях и на бэке без ссылок на удалённые `Awaiting/ContractSent/Signed` константы.
- Given `make test-unit-backend` и `make test-unit-backend-coverage` запускаются после правки тестов, then оба зелёные, per-method coverage gate соблюдён, race-detector не падает.
- Given `make test-e2e-backend` и `make test-e2e-frontend` запускаются после миграции, then оба зелёные. Race-покрытие partial unique index по ИИН остаётся на repo-уровне (pgxmock в `repository/creator_application_test.go` — три сценария, см. l.30/44/58); отдельного e2e-теста на concurrent submit в проекте сейчас нет, и в этом чанке его не вводим.
- Given grep по проекту вне `_bmad-output/implementation-artifacts/archive/`, `frontend/web/src/_prototype/`, generated-схем (`*.gen.go`, `frontend/*/generated/*.ts`, `frontend/e2e/types/*.ts`) и комментариев миграций, when ищется `awaiting_contract|contract_sent|StatusAwaiting|StatusContract|StatusSigned|ContractsPage|stages\.contracts`, then нет ни одного runtime-вхождения. Generated-схемы после `make generate-api` тоже не содержат этих токенов — это вторичная гарантия.
- Given `creator-application-state-machine.md` переписан, when ревьюер читает его рядом с диффом миграции и domain-кода, then обе истории согласованы (статусы, переходы, partial unique index), секция «Видимые состояния для креатора» удалена и заменена на короткое упоминание Telegram-нотификаций, и living-doc явно ссылается на chunk 18 как источник `moderation → approved`.
- Given Алихан проверяет PR-дельту по `_bmad-output/planning-artifacts/`, when смотрит revision-список в `creator-onboarding-roadmap.md`, then **этот PR добавляет revision-entry от 2026-05-XX про закрытие chunk 17** (короткая строка про дроп старых статусов и добавление `approved`). Сам chunk помечается `[x]` в роадмапе.

## Spec Change Log

- **2026-05-05** — Реализация чанка завершена; все Tasks помечены `[x]`. PR ещё не создан (Алихан создаст вручную после ревью). Отклонения от спеки: (1) v2-миграция добавляет `approved` в транзитный CHECK *в конце* списка, а не в середине (косметика, читаемость SQL); (2) комментарий на `CodeCreatorApplicationNotInVerification` теперь указывает «moderation/approved/...» вместо «moderation/awaiting_contract/...»; (3) `creator-verification-concept.md` дополнительно отражает новую реальность с `withdraw` (в MVP не реализован, `withdrawn` — зарезервированный терминал) — это шире, чем требовала Code Map (только сжатие формулировок), но синхронизирует с переписанным state-machine.md. Найдена смежная проблема в `Makefile`: `make lint-{web,tma,landing}` использует `npx tsc --noEmit` без `-b`, и для проектов с `tsconfig.json` в режиме project references это **не проверяет** `src/` (компиляция мгновенная, exit=0). Истинная проверка — `npx tsc -b` или `tsc -p tsconfig.app.json --noEmit`. Текущий Makefile-лeyer пропустил бы этот PR с битым `ApplicationActions.tsx` (был обнаружен на финальном grep). В скоуп этого PR'а починка Makefile **не входит** — отдельный следующий чанк / [чеклист-кандидат] для Алихана.

## Design Notes

**Почему forward-only без бэкфила.** На момент написания спеки в prod (~100 заявок) ни одна не находится в `awaiting_contract`/`contract_sent`/`signed` — этот pipeline-кусок никогда не активировался (см. revision от 2026-05-03 в `creator-onboarding-roadmap.md`). Бэкфил-стратегия не нужна, и вместо неё — fail-fast guard. Это то же решение, что v1-миграция приняла для дропаемых `approved`/`blocked` (`backend/migrations/20260501222829_creator_applications_state_machine.sql:24-29`).

**Почему `creatorApplicationAllowedTransitions` не трогаем.** Roadmap chunk 18 целиком про `moderation → approved` — там придёт `*ApproveService` с созданием `users`-row, audit-row, transition-row в одной WithTx. Положить только enum-значение, не подключив переход — нормально: `IsCreatorApplicationTransitionAllowed` отвергнет любой move в `approved`, пока chunk 18 явно не разрешит. Так чанки остаются маленькими и независимо мерджимыми.

**Почему `withdrawn` не дропаем, хотя реально не используется.** Roadmap явно резервирует его на будущее («креатор-инициированный отзыв» из чата с Алиханом, без явного срока). Дроп сейчас потребует ещё одной миграции, когда отзыв вернётся. Стоимость удержания одного неактивного terminal-значения в enum'е — нулевая. Стоимость дополнительной миграции туда-обратно — ненулевая.

**Down-миграция.** Безопасна только до запуска chunk 18 (нет approved-рядов). После — guard упадёт, и Down станет однонаправленным «manual rollback only». Это идентично v1-миграции (Down безопасен только до первого реального перехода). Документировать ограничение в комментарии.

**Codegen-дрейф ловится компилятором.** Дроп wire-констант `api.AwaitingContract`/`api.ContractSent`/`api.Signed` из `server.gen.go` сразу провалит сборку любого ручного места, которое на них ссылается. Если такое место найдётся при сборке — это баг, который мы ловим именно сейчас, бесплатно (а не на staging через тихий мисс-кейс).

**Конфликт с параллельным агентом chunk 16.5.** На момент записи спеки на той же машине идёт работа над `frontend/e2e/web/admin-creator-applications-moderation*.spec.ts` (ветка `alikhan/admin-creator-applications-moderation-e2e`). При реализации chunk 17 мы трогаем больше фронт-файлов, чем казалось вначале:
- `frontend/e2e/types/schema.ts` (через `make generate-api`) — из его working-set'а косвенно.
- `ApplicationActions.tsx` / `ApplicationActions.test.tsx` — он не должен их трогать в e2e-чанке, но проверить грепом перед мерджем.
- `App.tsx`, `DashboardLayout.tsx` и `DashboardLayout.test.tsx` — chunk 16 их уже изменил (PR #61), агент 16.5 теоретически может их редактировать снова.
- `creatorApplications.json` (i18n) — chunk 16 туда добавил `stages.moderation.description`; конфликт по близким ключам возможен.

Стратегия: дождаться мержа PR агента 16.5, поднять `alikhan/creator-application-state-machine-v2` от свежего main, прогнать `make generate-api`, прогнать перечисленные правки фронта, пересмотреть Code Map / Tasks по факту, если агент 16.5 ещё что-то сдвинул в этих файлах.

**Почему добавление `case "approved":` сейчас, а не в chunk 19.** Сжатие union'а `CreatorApplicationStatus` после `make generate-api` сделает текущий exhaustive-switch компилируемо-неполным: TS пожалуется через `default: never`, что `"approved"` не покрыт. Молчаливый branch для `approved` ломает гарантию exhaustive-switch'а. В chunk 19 этот case заменится на actual approve-кнопку — `return null;` живёт ровно столько, сколько идёт интервал между мерджем chunk 17 и chunk 19. Альтернатива (оставить `default: never` ловить `approved`) превратила бы открытие drawer'а на approved-заявке в runtime-ошибку.

## Verification

**Commands:**
- `make migrate-up` -- expected: новая миграция применяется без ошибок на чистой локальной БД (после reset).
- `make build-backend` -- expected: компиляция без ссылок на удалённые `*StatusAwaitingContract`/`*StatusContractSent`/`*StatusSigned`.
- `make lint-backend` -- expected: golangci-lint без находок.
- `make test-unit-backend` + `make test-unit-backend-coverage` -- expected: оба зелёные, per-method gate соблюдён.
- `make test-e2e-backend` -- expected: зелёный (race-покрытие partial unique index на repo-уровне в pgxmock, отдельного e2e на concurrent submit нет — этот чанк его не вводит).
- `make lint-web` + `make test-unit-web` -- expected: tsc + eslint + vitest зелёные. То же для tma и landing (`make lint-tma`, `make lint-landing`, `make test-unit-tma`, `make test-unit-landing`).
- `make test-e2e-frontend` -- expected: зелёный (косвенно проверяет, что фронт-часть `frontend/e2e/types/schema.ts` срегенерирована корректно).
- `git grep -nE "awaiting_contract|contract_sent|StatusAwaiting|StatusContract|StatusSigned|ContractsPage|stages\\.contracts" -- ':!_bmad-output/implementation-artifacts/archive/' ':!frontend/web/src/_prototype/' ':!**/generated/**' ':!frontend/e2e/types/**' ':!**/*.gen.go' ':!backend/migrations/20260501*'` -- expected: пусто. Любое попадание — нерешённый dangling reference, HALT.

**Manual checks:**
- `make migrate-reset` → `make migrate-up` на локалке: пройти весь стек миграций, убедиться, что финальный enum в `\d+ creator_applications` показывает новые 5 значений и что `\di+ creator_applications_iin_active_idx` показывает `WHERE status = ANY (ARRAY['verification', 'moderation'])`.
- Применить миграцию на снимке staging-БД (Docker pull дампа), проверить guard'ом на отсутствие дропаемых рядов: `SELECT count(*) FROM creator_applications WHERE status IN ('awaiting_contract','contract_sent','signed')` → 0 ожидается.
- `make start-web` → залогиниться админом: в sidebar в группе «Заявки креаторов» осталось три пункта (Верификация / Модерация / Отклонённые), нет «Договоры». Попытка зайти по URL `/admin/creator-applications/contracts` приводит к 404 / fallback'у роутера.
- На moderation-экране открыть drawer на тестовой заявке со `status = approved` (если таковой нет в локальных данных — поменять руками через psql на одну заявку): footer drawer'а пуст, нет ни кнопки reject, ни approve, контент-блок detail'а отображается без ошибок в консоли.
- Прочитать обновлённый `creator-application-state-machine.md` сверху вниз: один раз, без перекрёстных ссылок — должен быть полностью самодостаточным как living source of truth.
