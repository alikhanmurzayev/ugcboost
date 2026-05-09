---
title: "Chunk 9a — CRUD шаблона договора в кампании"
type: feature
created: "2026-05-09"
status: done
chunk: 9a
group: "Группа 3 — Фронт кампаний"
branch_proposed: alikhan/campaign-contract-template
baseline_commit: fc965942ba581d969b5d9f1e9b1c6b7009449d31
roadmap: _bmad-output/planning-artifacts/campaign-roadmap.md
related_intent: _bmad-output/implementation-artifacts/intent-trustme-contract-v2.md
experiment: _bmad-output/experiments/pdf-overlay/
context:
  - docs/standards/backend-architecture.md
  - docs/standards/backend-codegen.md
  - docs/standards/backend-constants.md
  - docs/standards/backend-design.md
  - docs/standards/backend-errors.md
  - docs/standards/backend-libraries.md
  - docs/standards/backend-repository.md
  - docs/standards/backend-testing-e2e.md
  - docs/standards/backend-testing-unit.md
  - docs/standards/backend-transactions.md
  - docs/standards/frontend-api.md
  - docs/standards/frontend-components.md
  - docs/standards/frontend-quality.md
  - docs/standards/frontend-state.md
  - docs/standards/frontend-testing-e2e.md
  - docs/standards/frontend-testing-unit.md
  - docs/standards/frontend-types.md
  - docs/standards/naming.md
  - docs/standards/security.md
  - docs/standards/review-checklist.md
---

# Chunk 9a — CRUD шаблона договора в кампании

## Преамбула — стандарты обязательны

Перед любой строкой production-кода агент обязан полностью загрузить все файлы `docs/standards/` (через `/standards`). Применимы все. Каждое правило — hard rule; отклонение = finding.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Chunks 16/17/18 (Группа 7, intent v2) автоматически отправляют договоры в TrustMe и принимают webhook'и. Источник контента — PDF-шаблон с тремя плейсхолдерами `{{CreatorFIO}}`, `{{CreatorIIN}}`, `{{IssuedDate}}`, который Аидана делает в Google Docs. Сейчас в БД нет места под этот шаблон, и в админке нет UI для загрузки. Без этого Аидана не может ни заливать, ни проверять шаблоны, и chunk 16 не имеет источника для overlay-рендера.

**Approach:** Расширяем `campaigns` таблицу полем `contract_template_pdf BYTEA`, добавляем два endpoint'а (`PUT/GET /campaigns/{id}/contract-template` с raw `application/pdf` body, без multipart), на фронте — компонент `ContractTemplateField` в форме create/edit + download-кнопка. Валидация загруженного PDF через `ledongthuc/pdf` (parseable + три known placeholders + нет unknown). TrustMe-интеграция, outbox-worker, lock-edit, notify-guard — **НЕ** в этом chunk'е.

## Boundaries & Constraints

**Always:**
- Миграция: `ALTER TABLE campaigns ADD COLUMN contract_template_pdf BYTEA NOT NULL DEFAULT '\x';`. DEFAULT нужен для существующих рядов в проде.
- Endpoints в `backend/api/openapi.yaml`:
  - `PUT /campaigns/{id}/contract-template` — request body `application/pdf` (raw bytes), response 200 `{"hash":"<sha256>","placeholders":[<name>,...]}`. Auth — admin-only.
  - `GET /campaigns/{id}/contract-template` — response `application/pdf` body. Auth — admin-only. 404 если шаблон не загружен.
- Валидация при upload (последовательно, первая failed → return):
  1. `len(body) == 0` → 422 `CodeContractRequired`.
  2. `pdf.NewReader(body)` падает → 422 `CodeContractInvalidPDF`.
  3. После `Extractor.ExtractPlaceholders`: каждое имя из `domain.KnownContractPlaceholders` присутствует хотя бы один раз → иначе 422 `CodeContractMissingPlaceholder` с `details.missing: [...]`.
  4. Все найденные имена ∈ `domain.KnownContractPlaceholders` → иначе 422 `CodeContractUnknownPlaceholder` с `details.unknown: [...]`.
- `domain.KnownContractPlaceholders = []string{"CreatorFIO", "CreatorIIN", "IssuedDate"}` — единственный источник правды по списку плейсхолдеров; используется и в form-validation, и в `Render` будущего chunk 16.
- `internal/contract/` — новый пакет:
  - `Placeholder` struct — `Page int`, `Name string`, `XMin/XMax/YBaseline/FontSize float64`.
  - `Extractor` interface с методом `ExtractPlaceholders(pdf []byte) ([]Placeholder, error)`.
  - `RealExtractor` (default) — реализация через `github.com/ledongthuc/pdf`. Алгоритм — копия из reference implementation `_bmad-output/experiments/pdf-overlay/main.go` (line grouping по Y tolerance 0.5pt, whitespace-based word split — `ledongthuc` отдаёт `W=0`, regex `\{\{(\w+)\}\}`).
- `domain.ValidateContractTemplatePDF(pdfLen int, placeholderNames []string) error` — pure-domain функция (без PDF parsing); возвращает один из четырёх sentinel-error'ов, обёрнутых через `domain.NewValidationError` с актуальным кодом.
- `internal/repository/campaign.go` — новые методы:
  - `UpdateContractTemplate(ctx, id, pdf []byte) error`
  - `GetContractTemplate(ctx, id) ([]byte, error)` (возвращает `sql.ErrNoRows` если кампании нет; пустой `[]byte` если кампания есть но шаблон не загружен).
- `internal/service/campaign.go` — новые методы:
  - `UploadContractTemplate(ctx, id uuid.UUID, pdf []byte, actorID uuid.UUID) (UploadContractTemplateResult, error)` — вся пайплайн валидации + repo.UpdateContractTemplate + audit; всё внутри одной `dbutil.WithTx`.
  - `GetContractTemplate(ctx, id uuid.UUID) ([]byte, error)` — read-only через pool. Возвращает `domain.ErrCampaignNotFound` если кампании нет, `domain.ErrContractTemplateNotFound` если шаблон пуст.
- `service.UploadContractTemplateResult` — типизированная структура `{ Hash string; Placeholders []string }`. Hash — `sha256` от raw PDF.
- Audit: action `campaign.contract_template_uploaded`, entity `campaign`, metadata = `{"hash": "<sha256>", "placeholders": ["CreatorFIO", "CreatorIIN", "IssuedDate"], "size_bytes": <int>}`. Внутри той же Tx, что и `UpdateContractTemplate`.
- `internal/handler/campaign.go` — два новых хендлера через сгенерированный `ServerInterfaceWrapper`. PUT-хендлер читает body как `[]byte` (oapi-codegen для `application/pdf` отдаёт `multipart.File` или `io.Reader` — конкретику смотрим в spec'е после `make generate-api`; если strict-server не подходит — обходим через chi-handler рядом с routes, как `/test/*`).
- OpenAPI errors-enum расширяется кодами `ContractRequired`, `ContractInvalidPDF`, `ContractMissingPlaceholder`, `ContractUnknownPlaceholder`, `ContractTemplateNotFound`. Все 422 кроме `ContractTemplateNotFound` — он 404.
- `domain/errors.go` — соответствующие sentinel-error'ы и константы `CodeContractRequired` и т.д. Текст user-facing actionable (стандарт `backend-errors.md`):
  - `CodeContractRequired`: «Загрузите PDF-шаблон договора. Файл не должен быть пустым».
  - `CodeContractInvalidPDF`: «Файл не распознаётся как PDF. Проверьте, что вы экспортировали документ из Google Docs через File → Download → PDF».
  - `CodeContractMissingPlaceholder`: «В шаблоне не найдены обязательные плейсхолдеры: {{names}}. Проверьте, что они написаны в формате {{Name}} (CamelCase, без подчёркиваний) и каждый — на отдельной строке».
  - `CodeContractUnknownPlaceholder`: «В шаблоне найдены незнакомые плейсхолдеры: {{names}}. Известные плейсхолдеры: CreatorFIO, CreatorIIN, IssuedDate».
  - `CodeContractTemplateNotFound`: «Шаблон договора для этой кампании ещё не загружен».
- `internal/authz/campaign.go` — авторизация upload/get как для существующих campaign-методов (admin-only). Новых ролей не вводим.
- `docs/standards/backend-libraries.md` — добавить запись `github.com/ledongthuc/pdf` в registry с обоснованием: «PDF text + bbox extraction; mainstream pure-Go альтернатив (без CGo) для извлечения координат per word нет — `pdfcpu` не отдаёт bbox, MuPDF только через CGo. Используется только в `internal/contract/Extractor` для validation шаблонов и outbox-render'а». Зависимость добавляется в `backend/go.mod`.
- Frontend: новый каталог `frontend/web/src/features/campaigns/contract-template/`:
  - `ContractTemplateField.tsx` — компонент: file input (accept=`.pdf`/`application/pdf`) + кнопка «Загрузить шаблон» / «Заменить шаблон» / «Скачать шаблон» в зависимости от состояния + блок preview найденных плейсхолдеров после успешного upload + блок ошибки.
  - `ContractTemplateField.test.tsx` — unit (RTL).
  - `useContractTemplate.ts` — хук вокруг `useMutation` (PUT) и helper для триггера download (GET через openapi-fetch с responseType `blob`).
  - `useContractTemplate.test.ts` — unit на mutation.
- Подключение компонента: на странице `/campaigns/new` (`features/campaigns/CampaignNewPage.tsx` или эквивалент) + на `/campaigns/{id}/edit` (`features/campaigns/CampaignEditPage.tsx`). Если страница new — upload **disabled** до создания campaign'а (нужен ID); пользователь сначала сохраняет campaign базовыми полями, потом загружает шаблон. Альтернатива (defer upload до save) — out of scope, оставляем простой UX «сначала save, потом upload».
- Download-кнопка на детальной (`features/campaigns/CampaignDetailPage.tsx`) — если шаблон загружен (определяется по флагу из `GET /campaigns/{id}` ответа; см. ниже про backend-расширение campaign-DTO).
- Backend-расширение: `Campaign` schema в OpenAPI получает поле `hasContractTemplate: boolean` (computed `length(contract_template_pdf) > 0`). Само содержимое PDF не отдаётся в general-ручке `GET /campaigns/{id}` — только через специализированный `GET /campaigns/{id}/contract-template`.
- i18n keys в `frontend/web/src/shared/i18n/locales/ru/campaigns.json` под секцией `campaigns.contractTemplate.*`:
  - `label`: «Шаблон договора (PDF)».
  - `descriptionEmpty`: «Загрузите PDF-шаблон, экспортированный из Google Docs. В шаблоне должны быть три плейсхолдера: {{CreatorFIO}}, {{CreatorIIN}}, {{IssuedDate}} — каждый в формате CamelCase на отдельной строке».
  - `uploadButton`: «Загрузить шаблон».
  - `replaceButton`: «Заменить шаблон».
  - `downloadButton`: «Скачать существующий шаблон».
  - `placeholdersFound`: «Найдены плейсхолдеры:».
  - `loading`: «Загружаем…».
  - `errorRequired`/`errorInvalidPDF`/`errorMissingPlaceholder`/`errorUnknownPlaceholder` — повторы user-facing текстов из бэка (НЕ заменять текст бэка — рендерить `error.message` из response, эти ключи только для fallback при сетевой ошибке).
- `data-testid` на интерактивных элементах:
  - `contract-template-section` — корневой div блока.
  - `contract-template-input` — `<input type="file">`.
  - `contract-template-upload-button`, `contract-template-replace-button`, `contract-template-download-button`.
  - `contract-template-loading`, `contract-template-error`.
  - `contract-template-placeholder-{name}` — для каждого найденного плейсхолдера в preview-блоке.
- `useMutation` для upload обязан иметь `onError` handler с toast (стандарт `frontend-api.md`).
- React Query keys — фабрика в `shared/constants/queryKeys.ts` или `features/campaigns/queryKeys.ts` (где живут существующие campaign-keys). Ключ `["campaigns", id, "contractTemplate"]` для invalidation после upload.
- Стандарты `docs/standards/*` — все hard rules.

**Ask First:**
- Если в реальном тестовом PDF (`legal-documents/Тест, эксперимент BLACK BURN_Соглашение_UGC.docx-1.pdf`) `Extractor.ExtractPlaceholders` не находит все три плейсхолдера — **HALT**, разбираемся почему. Reference implementation в `_bmad-output/experiments/pdf-overlay/main.go` уже работает на этом файле и извлекает 6 instances всех трёх плейсхолдеров (стр.1: 3 шт, стр.2: 2 шт, стр.3: 1 шт). Если production-extractor отличается — найти расхождение.
- Если `oapi-codegen` strict-server **не поддерживает** `application/pdf` raw body для PUT — HALT, обсуждаем альтернативу: chi-handler рядом со strict-server'ом (как существующий `/test/*` flow) или multipart.
- Если на фронте `openapi-fetch` не умеет responseType=`blob` для GET binary — HALT, ищем фолбэк (raw `fetch` с комментарием почему — стандарт `frontend-api.md`).

**Never:**
- **Lock-edit** `contract_template_pdf` (отказ PUT при наличии `campaign_creator.status IN ('signing','signed','signing_declined')`) — НЕ здесь, едет в **chunk 16** одновременно с миграцией этих статусов. До chunk 16 эти статусы в БД невозможны → guard был бы noop.
- **Notify-guard** в `NotifyCampaignCreators` (chunk 12) при пустом `contract_template_pdf` — НЕ здесь, едет в **chunk 16** (включается одновременно с outbox-flow). Иначе сломаем уже работающий /notify для существующих кампаний без шаблонов.
- **TrustMe-интеграция, outbox-worker, `Render` метод в `internal/contract/`** — chunks 16/17.
- **`contracts` table, `campaign_creators.contract_id`** — chunk 16.
- **multipart/form-data** для upload — используем raw `application/pdf`. См. Intent → Approach.
- **Embedded шрифт** `LiberationSerif-Regular.ttf` в `internal/contract/fonts/` — НЕ здесь, нужен только для `Render` в chunk 16.
- **Хардкод текста договора / содержимого PDF в коде** — это responsibility Аиданы, она готовит шаблон.
- **Логирование PDF-bytes / содержимого** — security.md hard rule. В логах допустимо: `campaign_id`, `actor_id`, `size_bytes`, `sha256_hash` (короткий fingerprint), список placeholder names. **НЕ** логируем сам PDF.
- **`SELECT contract_template_pdf` в листинге кампаний** (`GET /campaigns`) — это сразу гигабайты body на 100 кампаниях. Listing — без шаблона; полный шаблон тянем только через `GET /campaigns/{id}/contract-template`.

## I/O & Edge-Case Matrix

| Scenario | Input | Expected |
|---|---|---|
| Happy upload | valid PDF с `{{CreatorFIO}}`, `{{CreatorIIN}}`, `{{IssuedDate}}` (например, реальный шаблон Аиданы) | 200, response `{"hash":"<sha256>","placeholders":["CreatorFIO","CreatorIIN","IssuedDate"]}`; audit row `campaign.contract_template_uploaded` |
| Empty body | `Content-Length: 0` | 422 `ContractRequired` |
| Не-PDF | header правильный, body — JPEG/text | 422 `ContractInvalidPDF` |
| PDF без плейсхолдеров | valid PDF без `{{...}}` (например, `legal-documents/РАМОЧНЫЙ ДОГОВОР...pdf`) | 422 `ContractMissingPlaceholder`, `details.missing=["CreatorFIO","CreatorIIN","IssuedDate"]` |
| PDF с одним missing | есть FIO + Date, нет IIN | 422 `ContractMissingPlaceholder`, `details.missing=["CreatorIIN"]` |
| PDF с unknown | есть все три + `{{CreatorEmail}}` | 422 `ContractUnknownPlaceholder`, `details.unknown=["CreatorEmail"]` |
| Replace existing | PUT поверх уже загруженного | 200, новая audit-row; шаблон перезаписан, hash обновлён |
| GET, шаблон загружен | id с непустым PDF | 200 `application/pdf`, body = bytes |
| GET, шаблон не загружен | id с пустым `contract_template_pdf` | 404 `ContractTemplateNotFound` |
| GET / PUT, кампания не найдена | id не в БД | 404 `CampaignNotFound` |
| GET / PUT, soft-deleted кампания | id в БД, `is_deleted=true` | 404 `CampaignNotFound` (soft-deleted = «не существует» для админских ручек campaign-домена) |
| GET / PUT, не admin | brand_manager | 403 |
| GET / PUT, anonymous | без токена | 401 |
| Listing `GET /campaigns` | любой юзер | response item НЕ содержит `contractTemplatePdf`, но содержит `hasContractTemplate: bool` |
| Encrypted PDF | password-protected PDF | 422 `ContractInvalidPDF` (`ledongthuc/pdf.NewReader` вернёт ошибку) |
| Огромный PDF (>10 MB) | 50 MB PDF | 413 (стандартный chi/middleware body-limit) или 200, если в config'е лимит выше. Конкретный лимит — фиксировать в spec'е после проверки текущего `MaxRequestBodySize` |

## Code Map

**Backend:**
- `backend/migrations/<timestamp>_campaigns_contract_template.sql` — миграция Up/Down.
- `backend/api/openapi.yaml` — два новых path, расширение `Campaign` schema (`hasContractTemplate`), новые error codes.
- `backend/internal/domain/contract.go` (новый файл) — `KnownContractPlaceholders`, `ValidateContractTemplatePDF`.
- `backend/internal/domain/errors.go` — пять новых code-констант + sentinel errors.
- `backend/internal/contract/extractor.go` (новый пакет) — `Placeholder`, `Extractor`, `RealExtractor`, `ExtractPlaceholders` (порт алгоритма из experiments/pdf-overlay/main.go).
- `backend/internal/contract/extractor_test.go` — unit на extractor с фикстурами в `internal/contract/testdata/`.
- `backend/internal/contract/testdata/template-valid.pdf` — копия `legal-documents/Тест, эксперимент BLACK BURN_Соглашение_UGC.docx-1.pdf` (или любой PDF Аиданы, проходящий валидацию).
- `backend/internal/contract/testdata/template-no-placeholders.pdf` — PDF без `{{...}}` (взять `legal-documents/РАМОЧНЫЙ ДОГОВОР С КРЕАТОРОМ UGC BOOST.pdf` или подобный).
- `backend/internal/contract/testdata/not-a-pdf.txt` — текстовый файл для negative test.
- `backend/internal/repository/campaign.go` — `UpdateContractTemplate`, `GetContractTemplate`, расширение `selectColumns` (НЕ добавляем `contract_template_pdf` в дефолтный SELECT — только опциональный путь).
- `backend/internal/repository/campaign_test.go` — unit на новые методы.
- `backend/internal/service/campaign.go` — `UploadContractTemplate`, `GetContractTemplate`, `UploadContractTemplateResult` тип.
- `backend/internal/service/campaign_test.go` — unit (mock'аем `Extractor` через интерфейс).
- `backend/internal/service/audit_constants.go` — `AuditActionCampaignContractTemplateUploaded = "campaign.contract_template_uploaded"`.
- `backend/internal/service/mocks/mock_extractor.go` (через mockery `all: true`) — auto-generated.
- `backend/internal/handler/campaign.go` — два новых handler-метода.
- `backend/internal/handler/campaign_test.go` — unit на handlers.
- `backend/internal/api/server.gen.go`, `backend/e2e/apiclient/*.gen.go`, frontend `schema.ts` — auto-regen через `make generate-api`.
- `backend/e2e/campaign/campaign_test.go` — расширение существующего e2e-сценария: `TestCampaignContractTemplate` со всеми кейсами из I/O Matrix (happy, empty, invalid, missing, unknown, replace, get-not-uploaded, get-not-found).
- `backend/e2e/testutil/seed.go` — helper `SetupCampaignWithContractTemplate(t, adminToken, ...)` (читает фикстуру PDF и делает PUT).
- `docs/standards/backend-libraries.md` — registry-update.
- `backend/go.mod` / `go.sum` — добавление `github.com/ledongthuc/pdf`.

**Frontend:**
- `frontend/web/src/features/campaigns/contract-template/ContractTemplateField.tsx` (новый).
- `frontend/web/src/features/campaigns/contract-template/ContractTemplateField.test.tsx` (новый).
- `frontend/web/src/features/campaigns/contract-template/useContractTemplate.ts` (новый — мутация upload + helper download).
- `frontend/web/src/features/campaigns/contract-template/useContractTemplate.test.ts` (новый).
- `frontend/web/src/features/campaigns/CampaignEditPage.tsx` (или эквивалент) — встроить `<ContractTemplateField campaignId={id} />`.
- `frontend/web/src/features/campaigns/CampaignDetailPage.tsx` — кнопка «Скачать шаблон» (видна только если `hasContractTemplate=true`).
- `frontend/web/src/features/campaigns/queryKeys.ts` (если существует — расширить; иначе создать) — `contractTemplateKey(campaignId)`.
- `frontend/web/src/shared/i18n/locales/ru/campaigns.json` — секция `campaigns.contractTemplate.*`.
- `frontend/e2e/web/campaign-contract-template.spec.ts` (новый) — Playwright e2e. Header — JSDoc на русском в виде нарратива (стандарт `frontend-testing-e2e.md`).
- `frontend/e2e/helpers/api.ts` — helper `seedCampaignWithoutContract(t, adminToken)` если ещё нет.
- TMA / landing — НЕ трогаем.

</frozen-after-approval>

## Tasks & Acceptance

**Execution (бэк):**
- [x] Миграция `<timestamp>_campaigns_contract_template.sql` — Up/Down. Проверить: `make migrate-up` на чистой БД, существующие row получают DEFAULT '\x'.
- [x] OpenAPI расширение: два path + `hasContractTemplate` в `Campaign` schema + 5 новых error code. `make generate-api`. Сверить с `frontend/web/src/api/generated/schema.ts`.
- [x] `domain/contract.go` — `KnownContractPlaceholders`, `ValidateContractTemplatePDF` (4 sentinel error'а).
- [x] `domain/errors.go` — `CodeContractRequired`, `CodeContractInvalidPDF`, `CodeContractMissingPlaceholder`, `CodeContractUnknownPlaceholder`, `CodeContractTemplateNotFound` + соответствующие `ErrXxx` sentinel'ы. Mapping на HTTP status в `handler/response.go`.
- [x] `internal/contract/extractor.go` — порт алгоритма из experiments/pdf-overlay/main.go (groupByLine, splitWords, regex). Интерфейс `Extractor`, реализация `RealExtractor`. `ledongthuc/pdf` через `pdf.NewReader(io.ReadSeeker(bytes.NewReader(pdf)), int64(len(pdf)))`.
- [x] `internal/contract/extractor_test.go` — фикстуры:
  - `template-valid.pdf` — должно вернуть 6 placeholders по pages 1, 2, 3 (как в reference experiment).
  - `template-no-placeholders.pdf` — `len(result) == 0`.
  - `not-a-pdf.txt` (читаем как bytes) — error.
- [x] `internal/contract/testdata/*.pdf` — залить фикстуры. `template-valid.pdf` — копия от Аиданы (`legal-documents/Тест...`); `template-no-placeholders.pdf` — `legal-documents/РАМОЧНЫЙ ДОГОВОР...pdf` или подобный.
- [x] `repository/campaign.go` — `UpdateContractTemplate(ctx, id, pdf)`; `GetContractTemplate(ctx, id) ([]byte, error)`. Не трогать дефолтный `selectColumns` (PDF тянем отдельным запросом).
- [x] `repository/campaign_test.go` — captureExec/captureQuery на новые методы; SQL-литералы.
- [x] `service/campaign.go` — `UploadContractTemplate` (внутри `WithTx`: extract + validate + UPDATE + audit), `GetContractTemplate` (read pool + 404-mapping). Тип `UploadContractTemplateResult`.
- [x] `service/audit_constants.go` — `AuditActionCampaignContractTemplateUploaded`.
- [x] `service/campaign_test.go` — unit на оба метода с mock Extractor + mock RepoFactory + mock AuditRepo. Все edge-cases из I/O Matrix.
- [x] `make generate-mocks`.
- [x] `handler/campaign.go` — два метода. Если strict-server несовместим с `application/pdf` body — обходим через chi-handler в `handler/server.go` рядом с роутами (`r.Put("/campaigns/{id}/contract-template", h.uploadContractTemplate)`, `r.Get`); тогда auth + admin-check вручную через middleware-chain как в `/test/*`.
- [x] `handler/campaign_test.go` — unit на handlers, captured-input для middleware-derived (actor_id из context).
- [x] `authz/campaign.go` — admin-only check (использовать существующий механизм).
- [x] `e2e/campaign/campaign_test.go` — `TestCampaignContractTemplate` со всеми сценариями. Audit-row через `testutil.AssertAuditEntry`.
- [x] `e2e/testutil/seed.go` — `SetupCampaignWithContractTemplate(t, adminToken, ...) campaignID`.
- [x] `docs/standards/backend-libraries.md` — добавить `ledongthuc/pdf` строку в registry с обоснованием.
- [x] `make lint-backend && make test-unit-backend && make test-unit-backend-coverage && make test-e2e-backend`.

**Execution (фронт):**
- [x] `features/campaigns/contract-template/ContractTemplateField.tsx`:
  - props: `campaignId: string`, `hasTemplate: boolean` (из `hasContractTemplate` в Campaign-DTO).
  - state: hidden file input (программный trigger через ref), local `isSubmitting` flag (стандарт `frontend-state.md` про double-submit guard).
  - render: label + description + кнопка upload/replace + после успеха preview-блок «Найдены плейсхолдеры: [CreatorFIO, CreatorIIN, IssuedDate]» + при `hasTemplate=true` — кнопка download.
  - Loading state, Error state (рендерить `error.message` из response).
- [x] `useContractTemplate.ts`:
  - `useUploadContractTemplate(campaignId)` — `useMutation` с `mutationFn` через openapi-fetch PUT (raw body); `onSuccess` инвалидирует `["campaigns", id]` и `["campaigns", id, "contractTemplate"]`; `onError` toast.
  - `triggerDownloadContractTemplate(campaignId)` — async function: openapi-fetch GET → blob → анкор-клик скачивание.
- [x] `ContractTemplateField.test.tsx` — unit (RTL, userEvent, не fireEvent):
  - `hasTemplate=false` рендерит «Загрузить шаблон»; клик открывает file input.
  - Successful upload показывает preview-блок с тремя плейсхолдерами.
  - Error response (422 с message) показывает error-block.
  - `hasTemplate=true` рендерит «Заменить шаблон» + «Скачать шаблон».
- [x] `useContractTemplate.test.ts` — unit на mutation (моки openapi-fetch).
- [x] Подключение: `CampaignEditPage.tsx` — `<ContractTemplateField campaignId={id} hasTemplate={campaign.hasContractTemplate} />` под существующими полями.
- [x] `CampaignDetailPage.tsx` — Download button если `hasContractTemplate=true`.
- [x] i18n keys в `locales/ru/campaigns.json`. Никаких хардкод-строк в JSX.
- [x] `data-testid` на интерактивных элементах (см. Always).
- [x] `frontend/e2e/web/campaign-contract-template.spec.ts` — Playwright e2e:
  - Login as admin → create campaign → open edit → upload valid PDF → assert preview-block с 3 placeholders → reload → assert «Заменить шаблон» visible → upload invalid PDF → assert error-block → click download → assert download triggered (response status 200, content-type application/pdf).
  - Cleanup-stack через `E2E_CLEANUP=true`.
  - Header — JSDoc на русском, нарратив (см. стандарт).
- [x] `make lint-web && make test-unit-web && make test-e2e-frontend`.

**Acceptance Criteria:**
- Given существующая кампания без шаблона, when admin делает `PUT /campaigns/{id}/contract-template` с валидным PDF от Аиданы, then 200 с `{hash, placeholders: ["CreatorFIO","CreatorIIN","IssuedDate"]}` и audit-row `campaign.contract_template_uploaded`.
- Given кампания с уже загруженным шаблоном, when admin делает `PUT` с другим валидным PDF, then 200 (replace), новая audit-row, `GET` возвращает новый PDF.
- Given кампания, when admin шлёт PUT с пустым body, then 422 `ContractRequired`. Аналогично для не-PDF (422 `ContractInvalidPDF`), missing placeholder (422 `ContractMissingPlaceholder` с `details.missing=[...]`), unknown placeholder (422 `ContractUnknownPlaceholder` с `details.unknown=[...]`).
- Given кампания без шаблона, when GET → 404 `ContractTemplateNotFound`.
- Given несуществующая или soft-deleted кампания, when PUT/GET → 404 `CampaignNotFound`.
- Given не-admin user, when PUT/GET → 403.
- Given `GET /campaigns/{id}`, when шаблон загружен/не загружен, then `hasContractTemplate` = `true`/`false` соответственно. Поле `contractTemplatePdf` в JSON ответа отсутствует.
- На фронте: страница edit показывает upload-кнопку, после успешного upload рендерится preview-блок «Найдены плейсхолдеры: CreatorFIO, CreatorIIN, IssuedDate», при reload отображается «Заменить шаблон» + «Скачать шаблон».
- При ошибке upload фронт показывает error-блок с сообщением из бэкенда (не своим хардкодом).
- `make lint-backend && make test-unit-backend && make test-unit-backend-coverage && make test-e2e-backend && make lint-web && make test-unit-web && make test-e2e-frontend` — все зелёные.
- Coverage gate per-method ≥ 80% на `service.UploadContractTemplate`, `service.GetContractTemplate`, `repository.UpdateContractTemplate`, `repository.GetContractTemplate`, `handler.uploadContractTemplate`, `handler.getContractTemplate`, `domain.ValidateContractTemplatePDF`, `contract.RealExtractor.ExtractPlaceholders`.
- Reference implementation `_bmad-output/experiments/pdf-overlay/main.go` остаётся неизменным (мы только копируем алгоритм в production-пакет; sandbox-версия живёт как референс).

## Связанные документы

- Roadmap, Группа 3 chunk 9a: `_bmad-output/planning-artifacts/campaign-roadmap.md`.
- Intent Группы 7 (chunks 16/17/18), для понимания, под что готовится этот pre-step: `_bmad-output/implementation-artifacts/intent-trustme-contract-v2.md`.
- Reference implementation overlay-extractor'а: `_bmad-output/experiments/pdf-overlay/main.go` + `contract-filled.pdf`.
- Тестовый шаблон Аиданы: `legal-documents/Тест, эксперимент BLACK BURN_Соглашение_UGC.docx-1.pdf`.
- Стандарты: `docs/standards/`.

## Spec Change Log

**2026-05-09 — review patches**

Адверсариальный ревью (Blind hunter + Edge case hunter + Acceptance auditor) дал 75 finding'ов. После дедупа и классификации intent_gap/bad_spec не обнаружено — loopback не нужен. Применены 11 патчей:

- Audit `placeholders` теперь содержит фактически извлечённые имена (deduped), а не canonical `KnownContractPlaceholders` — audit становится evidentiary `backend/internal/service/campaign.go:285`.
- `BODY_LIMIT_BYTES` дефолт поднят `1 MB → 5 MB`. Раздел "I/O Matrix" §"Огромный PDF" фиксирует лимит — Google-Docs PDF типично 100 KB – 2 MB, до 3 MB на сканах. Глобальный middleware, env-override `BODY_LIMIT_BYTES` `backend/internal/config/config.go:43`.
- `Cache-Control: private, no-store` + `Content-Disposition: attachment; filename="..."` на download — PDF никогда не сидит в shared caches/proxies, браузер сразу открывает file dialog `backend/internal/handler/campaign.go:229`.
- Client-side файл-тип guard в `ContractTemplateField` — отбрасывает не-PDF до mutate (бэкенд CONTRACT_INVALID_PDF остаётся second line of defense) `frontend/web/src/features/campaigns/contract-template/ContractTemplateField.tsx:38`.
- Download API client пропагирует `serverMessage`/`details` (CONTRACT_TEMPLATE_NOT_FOUND рендерится корректно) `frontend/web/src/api/campaigns.ts:111`.
- Cache-инвалидация `campaignKeys.lists()` после upload — list-view сразу подхватывает обновлённый `hasContractTemplate` `frontend/web/src/features/campaigns/contract-template/useContractTemplate.ts:14`.
- i18n key `uploading` → `loading` (соответствие spec line 101).
- `as unknown as never` casts получили объяснительный комментарий (frontend-quality.md exception) `frontend/web/src/api/campaigns.ts:80`.
- `abs()` helper заменён на `math.Abs` (backend-libraries.md) `backend/internal/contract/extractor.go:108`.
- Удалён dead helper `BuildPlainPDF` (testutil/contract_template.go).
- Spec-level: добавлен `lists()` в `campaignKeys`-фабрике `frontend/web/src/shared/constants/queryKeys.ts:25`.

**Defer (отдельным заданием — не chunk 9a):**

- E2E для soft-deleted кампании PUT/GET → 404 CAMPAIGN_NOT_FOUND. Нет публичного DELETE-endpoint и нет test-API для soft-delete; repo-unit покрывает `is_deleted=false` filter. Включить в chunk, который добавит DELETE `/campaigns/{id}`.
- Reference PDF от Аиданы (`internal/contract/testdata/template-valid.pdf`). Production-extractor сейчас тестируется на gofpdf-фикстурах. Включить когда получим финальный шаблон.
- `RealExtractor` thread-safety verification — claim "stateless, safe to share" не покрыт concurrent-тестом. Минор для chunk 9a (single-instance bind в main.go), значимо для chunk 16 outbox-worker.
- `ledongthuc/pdf` corner case: `page.V.IsNull()` skip silent (extractor.go:74). Edge case, маловероятно на реальных PDF.
- Slowloris / context-aware reader для `io.ReadAll(request.Body)`. Покрывается глобальными `WriteTimeout`/`ReadTimeout` сейчас.
- Race UpdateContractTemplate vs concurrent DeleteCampaign. Стандартная гонка, нет defensive `FOR UPDATE`. Минор пока DELETE отсутствует.
- `GetContractTemplate` использует primary pool (нет read-replica routing). Шаблонная проблема всего проекта.

## Suggested Review Order

**Domain — известные плейсхолдеры и pure-domain валидатор**

- Single source of truth для placeholder-набора, разделяемого с chunk 16 overlay-render'ом.
  [`contract.go`](../../backend/internal/domain/contract.go)

- Валидатор без PDF-парсинга — отдельные коды для missing vs unknown с структурированными деталями.
  [`contract_test.go`](../../backend/internal/domain/contract_test.go)

- Пять новых error-кодов в общем enum (CONTRACT_REQUIRED / INVALID_PDF / MISSING / UNKNOWN / TEMPLATE_NOT_FOUND).
  [`errors.go`](../../backend/internal/domain/errors.go)

**Extractor — порт алгоритма из experiments в production-пакет**

- `RealExtractor.ExtractPlaceholders` — line clustering (Y-tolerance 0.5pt) + word splitting + regex `{{Name}}`.
  [`extractor.go:51`](../../backend/internal/contract/extractor.go#L51)

- Замена самописного `abs()` на `math.Abs` (backend-libraries.md).
  [`extractor.go:108`](../../backend/internal/contract/extractor.go#L108)

- Фикстуры через `gofpdf` (in-memory) — production-deps не тащим, тестовый PDF собираем сами.
  [`extractor_test.go`](../../backend/internal/contract/extractor_test.go)

**Repository — миграция + два метода + computed projection**

- `ALTER TABLE campaigns ADD COLUMN contract_template_pdf BYTEA NOT NULL DEFAULT '\x'` — пустой bytea для существующих рядов.
  [`20260509041036_campaigns_contract_template.sql`](../../backend/migrations/20260509041036_campaigns_contract_template.sql)

- `campaignSelectColumns` рерайтит `has_contract_template_pdf` → `(octet_length(...) > 0) AS has_contract_template_pdf` — флаг возвращается через дефолтный SELECT, сам PDF — нет.
  [`campaign.go`](../../backend/internal/repository/campaign.go)

- Captured-SQL тесты на `UpdateContractTemplate` / `GetContractTemplate` со строковыми литералами SQL (двойная проверка констант).
  [`campaign_test.go`](../../backend/internal/repository/campaign_test.go)

**Service — оркестрация валидации, audit в той же tx**

- `UploadContractTemplate` — empty/parse/validate → WithTx{UPDATE + audit}; deduped placeholder-список в audit (а не canonical hardcode).
  [`campaign.go:249`](../../backend/internal/service/campaign.go#L249)

- `dedupNames` — package-level helper для audit-точности при репитах в PDF.
  [`campaign.go:328`](../../backend/internal/service/campaign.go#L328)

- `GetContractTemplate` — sql.ErrNoRows → ErrCampaignNotFound; пустой bytea → ErrContractTemplateNotFound (распознавание двух 404).
  [`campaign.go:314`](../../backend/internal/service/campaign.go#L314)

- Audit action constant отдельной константой.
  [`audit_constants.go`](../../backend/internal/service/audit_constants.go)

**Handler — strict-server + custom headers через wrapper**

- `UploadCampaignContractTemplate` — authz → `io.ReadAll(request.Body)` → service. Body-limit middleware (5 MB) защищает заранее.
  [`campaign.go:186`](../../backend/internal/handler/campaign.go#L186)

- `contractTemplateDownloadResponse` оборачивает strict-server response, выставляет `Cache-Control: private, no-store` + `Content-Disposition: attachment` без правки OpenAPI.
  [`campaign.go:236`](../../backend/internal/handler/campaign.go#L236)

- Mapping `*ContractValidationError` → 422 с `details.missing`/`details.unknown`; `ErrContractTemplateNotFound` → 404 с собственным кодом.
  [`response.go`](../../backend/internal/handler/response.go)

**Authz — admin-only для обоих endpoint'ов**

- `CanUpload/GetCampaignContractTemplate` через существующий `AuthzService`-механизм.
  [`campaign.go`](../../backend/internal/authz/campaign.go)

**API contract — OpenAPI**

- Два новых path с raw `application/pdf` body (PUT) и `application/pdf` response (GET); расширение `Campaign` schema полем `hasContractTemplate`.
  [`openapi.yaml:1511`](../../backend/api/openapi.yaml#L1511)

**Body limit — глобальный default поднят с 1 MB до 5 MB**

- Раз PDF-шаблоны Google Docs стабильно 100 KB – 2 MB, 1 MB был too tight; 5 MB — безопасный потолок для всех endpoint'ов.
  [`config.go:43`](../../backend/internal/config/config.go#L43)

**Frontend API client — raw PDF body + blob download**

- `uploadCampaignContractTemplate` — openapi-fetch с `bodySerializer` для raw PDF; cast'ы прокомментированы (frontend-quality.md exception).
  [`campaigns.ts:80`](../../frontend/web/src/api/campaigns.ts#L80)

- `downloadCampaignContractTemplate` — `parseAs: "blob"`; ApiError несёт `serverMessage`/`details` для CONTRACT_TEMPLATE_NOT_FOUND рендера.
  [`campaigns.ts:107`](../../frontend/web/src/api/campaigns.ts#L107)

**Frontend hook — invalidation strategy**

- Upload-mutation инвалидирует detail + contractTemplate + lists() — список кампаний сразу видит флаг.
  [`useContractTemplate.ts:13`](../../frontend/web/src/features/campaigns/contract-template/useContractTemplate.ts#L13)

- `triggerDownloadContractTemplate` — Blob → object URL → anchor click → revoke в finally (без гонки на throw).
  [`useContractTemplate.ts:30`](../../frontend/web/src/features/campaigns/contract-template/useContractTemplate.ts#L30)

**Frontend компонент — UI binding на CampaignDetailPage**

- Hidden file input + upload/replace/download кнопки + preview-блок чипов плейсхолдеров после успеха.
  [`ContractTemplateField.tsx`](../../frontend/web/src/features/campaigns/contract-template/ContractTemplateField.tsx)

- Client-side файл-тип guard перед mutate — экономит roundtrip + быстрее даёт фидбек.
  [`ContractTemplateField.tsx:38`](../../frontend/web/src/features/campaigns/contract-template/ContractTemplateField.tsx#L38)

- Подключение в `CampaignDetailPage` между секцией "о кампании" и креатор-секцией.
  [`CampaignDetailPage.tsx`](../../frontend/web/src/features/campaigns/CampaignDetailPage.tsx)

**i18n + query keys**

- Все user-facing строки в campaigns.json под `contractTemplate.*`; ключ `loading` (был `uploading`, переименован под spec).
  [`campaigns.json:59`](../../frontend/web/src/shared/i18n/locales/ru/campaigns.json#L59)

- `campaignKeys.lists()` + `campaignKeys.contractTemplate(id)` — фабричные ключи для invalidation после upload.
  [`queryKeys.ts:24`](../../frontend/web/src/shared/constants/queryKeys.ts#L24)

**Backend tests**

- Service unit: 7 sub-test'ов на UploadContractTemplate (empty/parse/missing/unknown/replace/db-fail/happy + audit-капчуринг) + 4 на GetContractTemplate.
  [`campaign_test.go`](../../backend/internal/service/campaign_test.go)

- Handler unit: 7 sub-test'ов на upload + 4 на get с `doPDF` helper'ом.
  [`campaign_test.go`](../../backend/internal/handler/campaign_test.go)

- Backend e2e: 15 sub-test'ов покрывают всю I/O-матрицу.
  [`campaign_test.go:1101`](../../backend/e2e/campaign/campaign_test.go#L1101)

**Frontend tests**

- Component unit (RTL + real i18n): hasTemplate=false/true, happy upload рендерит preview, 422 рендерит backend message.
  [`ContractTemplateField.test.tsx`](../../frontend/web/src/features/campaigns/contract-template/ContractTemplateField.test.tsx)

- Hook unit: upload success/error, download blob, revokeObjectURL даже если click throws.
  [`useContractTemplate.test.ts`](../../frontend/web/src/features/campaigns/contract-template/useContractTemplate.test.ts)

- Frontend e2e (Playwright): happy upload рендерит preview / reload удерживает replace+download / 422 рендерит backend message inline / download triggers PDF event.
  [`admin-campaign-contract-template.spec.ts`](../../frontend/e2e/web/admin-campaign-contract-template.spec.ts)

**Standards / fixtures**

- `ledongthuc/pdf` зарегистрирован в библиотечном реестре с обоснованием.
  [`backend-libraries.md:32`](../../docs/standards/backend-libraries.md#L32)

- Roadmap revision 2026-05-09 фиксирует chunk 9a + redesign Группы 7.
  [`campaign-roadmap.md`](../planning-artifacts/campaign-roadmap.md)
