---
title: "Intent v2: TrustMe-контракт на (campaign, creator) — Группа 7 (chunks 16–17)"
type: intent
status: draft
created: "2026-05-09"
supersedes: _bmad-output/implementation-artifacts/intent-trustme-contract.md
roadmap: _bmad-output/planning-artifacts/campaign-roadmap.md
related_design: _bmad-output/planning-artifacts/design-campaign-creator-flow.md
experiment: _bmad-output/experiments/pdf-overlay/
external_docs:
  blueprint: docs/external/trustme/blueprint.apib
  postman: docs/external/trustme/postman-collection.json
revisions:
  - "2026-05-09: intent создан после серии экспериментов с overlay-рендером в `_bmad-output/experiments/pdf-overlay/`. PDF-pipeline = pure-Go overlay (`signintech/gopdf` + `ledongthuc/pdf`); источник контента — готовый PDF от Аиданы, загружаемый в `campaigns.contract_template_pdf BYTEA`; `ContractData` — 3 поля (`CreatorFIO`, `CreatorIIN`, `IssuedDate`); формат `IssuedDate` — «D» месяц YYYY г."
---

# Intent v2: TrustMe-контракт на каждый `(campaign, creator)`

## Преамбула — стандарты обязательны

Перед любой строкой production-кода агент обязан полностью загрузить все файлы `docs/standards/` (через `/standards`). Применимы все. Особенно: `backend-architecture.md`, `backend-codegen.md`, `backend-constants.md`, `backend-design.md`, `backend-errors.md`, `backend-libraries.md`, `backend-repository.md`, `backend-testing-e2e.md`, `backend-testing-unit.md`, `backend-transactions.md`, `naming.md`, `security.md`, `review-checklist.md`. Каждое правило — hard rule; отклонение = finding.

## Тезис

Группа 7 (chunks 16–17) реализует **отдельный электронный договор на каждый `(campaign, creator)`** через TrustMe (`test.trustme.kz`). Источник контента договора — поле `campaigns.contract_template_pdf BYTEA`: PDF, который Аидана сделала в Google Docs (с тремя плейсхолдерами `{{CreatorFIO}}`, `{{CreatorIIN}}`, `{{IssuedDate}}`) и залила в админке. Бэкенд-ручка `POST /tma/campaigns/{secret_token}/agree` (chunk 14, **pre-req — должен быть смержен в main до старта chunk 16**) ставит `campaign_creators.status='agreed'` и возвращает 200 — TrustMe в этой ручке не дёргается. Фоновый **outbox-worker** (cron-goroutine в `cmd/api`, `@every 10s`) подбирает ряды `agreed` без `contract_id`, **наносит overlay** на шаблонный PDF (pure Go: `ledongthuc/pdf` извлекает bbox плейсхолдеров, `signintech/gopdf` рисует белый rect + значения поверх), шлёт в TrustMe `SendToSignBase64FileExt`, переводит статус в `signing`. Креатор подписывает по СМС-OTP, после него **автоматически срабатывает наш auto-sign** (TrustMe настроил порядок «сначала клиент, потом мы»), webhook приводит `campaign_creators` к терминальному `signed` или `signing_declined`, бот уведомляет креатора. Рамочный договор (один-на-всю-жизнь-креатора) **отменён**. Отзыв/расторжение — out of scope (post-event).

## Скоуп

В Группу 7 входят:

- **chunk 16 — отправка**: миграция `contracts` table + `campaigns.contract_template_pdf` + `campaign_creators.contract_id` FK; outbox-worker; `ContractPDFRenderer` (overlay на основе `ledongthuc/pdf` + `signintech/gopdf`); `TrustMeClient.SendToSign`; **новые endpoints** `PUT /campaigns/{id}/contract-template` (multipart upload PDF) и `GET /campaigns/{id}/contract-template` (download PDF); форма create/edit кампаний расширяется PDF-upload компонентом; defense-in-depth валидация (form/notify-guard/outbox-skip).
- **chunk 17 — приём**: `POST /trustme/webhook` (public, bearer-auth, idempotent); state-transitions `signing → signed / signing_declined`; signed PDF persistent storage через `DownloadContractFile`; audit + бот-уведомления.

**Out of scope:**

- Рамочный договор и его подписание (отменено).
- Группа 8 (pre-event reminder, подтверждение прихода) и Группа 9 (билеты).
- Юридический контент шаблона договора — отдельная задача Аиданы. Бэк описывает только формат подстановки (3 плейсхолдера), не пишет сам текст договора.
- Нормализация ФИО креатора (кириллица/латиница). На текущей итерации шлём в TrustMe `last_name + first_name + middle_name` as-is из `creators` без валидации.
- Frontend-индикатор статуса контракта — встраивается как mini-PR в `chunk 15` (расширение страницы кампании со статусами и счётчиками), не отдельный chunk.
- Pre-flight: SetHook-регистрация webhook URL+token в TrustMe, `/companies` health-check на 401, ручная ротация TrustMe-токена — runbook, не код.
- Sandbox-проверка `POST /search/Contracts` фильтра по `additionalInfo` — runbook на старте chunk 16, не код (см. план B в Decision #10).

## Принятые решения

1. **Договор на каждый `(campaign, creator)`**, не рамочный (отменено).
2. **Способ загрузки в TrustMe — `SendToSignBase64FileExt`** (Base64 в multipart). Не `UploadWithFileURL`: мы не хотим открывать публичный download договоров наружу и связываться с signed-URL TTL под retry на стороне TrustMe.
3. **ФИО креатора — as-is** из `creators.last_name/first_name/middle_name` (конкатенация с пробелами). Без валидации кириллицы/латиницы. Если упадёт реальный кейс — починим точечно.
4. **PDF-pipeline = pure-Go overlay.** Шаблон договора — готовый PDF от Аиданы (Google Docs → File > Download → PDF), хранится в БД в `campaigns.contract_template_pdf BYTEA`. На каждый `(campaign, creator)` outbox-worker:
    - **извлекает bbox плейсхолдеров** через `github.com/ledongthuc/pdf` (читает PDF, итерирует страницы, группирует символы в строки по Y, разбивает на слова по whitespace, ловит токены `{{Name}}` regex'ом);
    - **накладывает overlay** через `github.com/signintech/gopdf`: импортирует исходные страницы как template (`ImportPage(file, n, "/MediaBox")`), рисует белый прямоугольник поверх каждого плейсхолдера (cover ascent + descent + pad), на baseline'е рисует значение шрифтом Liberation Serif 11pt (или fontSize, который ledongthuc отдаёт per-glyph);
    - **отдаёт PDF в TrustMe** как Base64.

    Reference implementation — `_bmad-output/experiments/pdf-overlay/main.go`. Все ключевые параметры (`ascentRatio`, `descentRatio`, `pad`, regex плейсхолдера, размер страницы) зафиксированы в разделе «Параметры overlay-рендера» ниже.

    Оба пакета (`ledongthuc/pdf`, `signintech/gopdf`) — pure Go, без CGo, без внешних утилит, без sidecar'ов; собираются и стартуют в составе обычного `cmd/api`. Обе библиотеки не топовые по популярности на github, но проверены на нашем реальном шаблоне. Альтернатива для overlay — `pdfcpu` (mainstream, ~7k★), но он не извлекает bbox per word; pure-Go альтернатив для bbox extraction почти нет (CGo MuPDF либо poppler-subprocess отвергнуты как тяжёлые/неуниформные с остальным backend'ом).

5. **State-машина после `agreed`** — три новых статуса:
    - `agreed → signing` — PDF загружен в TrustMe, ждём креатора (TrustMe-status=0).
    - `signing → signed` — `terminal happy`. Webhook TrustMe-status=3 (auto-sign отработал после креатора).
    - `signing → signing_declined` — `terminal negative`. Webhook TrustMe-status=9 (клиент отказался).

    **Хранение статуса — `TEXT + CHECK constraint`**, не Postgres `ENUM` (соответствует уже существующему `campaign_creators_status_check` в `backend/migrations/20260507044135_campaign_creators.sql:39`). Миграция = `ALTER TABLE ... DROP CONSTRAINT campaign_creators_status_check; ALTER TABLE ... ADD CONSTRAINT campaign_creators_status_check CHECK (status IN ('planned','invited','declined','agreed','signing','signed','signing_declined'));`. Down-migration — обратная пара ALTER (rollback тривиален, без проблем enum-drop-value).

6. **Auto-sign — порядок «сначала клиент, потом мы»**. TrustMe настроил это для нас. Промежуточный TrustMe-status=2 (подписан клиентом) у нас short-lived; на нём наш статус `signing` не меняем — ждём `status=3`.

7. **Отзыв/расторжение — out of scope.** Мы не инициируем `RevokeContract` из своей системы. Если webhook прилетит со статусом 1/4/5/6/7/8 — `contracts.trustme_status_code` обновляем (это audit-trail TrustMe-события, поле обязано отражать последний known статус), пишем audit + warning-лог. **`campaign_creators.status` НЕ трогаем** — наш бизнес-статус остаётся в `signing`, мы не транслируем эти TrustMe-статусы в свою state-машину. Разбор — отдельным flow позже (post-event).

8. **Универсальная таблица `contracts`** — ubiquitous language. Хранит все документы, отправленные в TrustMe, независимо от исхода (в процессе / подписан / отказ). Поля (черновик): `id`, `subject_kind` (discriminator: `'campaign_creator'` сейчас, расширяемо), `trustme_document_id` UNIQUE, `trustme_short_url`, `trustme_status_code` (0..9), `unsigned_pdf_content BYTEA` (исходный PDF, который мы залили в TrustMe — **результат overlay'а**, не шаблон), `signed_pdf_content BYTEA` NULL (подписанный PDF c QR обеих сторон — **persistent storage, не кеш**: после webhook'а status=3 webhook-handler делает `GET /doc/DownloadContractFile/{document_id}` и UPDATE'ит колонку), `initiated_at`, `signed_at`, `declined_at`, `webhook_received_at`, `created_at`, `updated_at`. Content в БД на MVP-объёмах; миграция в S3 — позже при необходимости. Reverse-FK на стороне subject: `campaign_creators.contract_id UUID NULL REFERENCES contracts(id) ON DELETE SET NULL`. Расширение под `brand_agreement` = `ALTER TABLE brand_agreements ADD COLUMN contract_id UUID REFERENCES contracts(id)` + новая строка в CHECK + новая ветка в webhook-dispatcher. Без миграции данных.

9. **Webhook receiver — generic `/trustme/webhook`** + dispatcher по `contracts.subject_kind`. Auth — статичный bearer из `SetHook.Token`. **Lookup в 2 шага**: (1) `SELECT id, subject_kind, trustme_status_code FROM contracts WHERE trustme_document_id = $1` — 404 если не нашли. (2) Reverse-FK по `subject_kind`: для `'campaign_creator'` → `SELECT id FROM campaign_creators WHERE contract_id = $contract_id`. Затем UPDATE статуса/timestamps в `contracts` + subject-update — в одной транзакции (стандарт audit ВНУТРИ Tx). Ответы handler: 200 / 401 (bad token) / 404 (unknown document_id) / 422 (unknown subject_kind).

10. **Outbox-pattern для отправки договоров — два коротких Tx, network-вызовы вне транзакции.** Бэкенд-ручка `POST /tma/campaigns/{secret_token}/agree` (chunk 14, **pre-req — должен быть в main до старта chunk 16**) **только** ставит `campaign_creators.status = 'agreed'` (никаких походов в TrustMe в самой ручке). Фоновый **worker** в `cmd/api/main.go` через `robfig/cron/v3` (scheduler уже инициализирован в `main.go:69-76`, нужно только `scheduler.AddFunc("@every 10s", contractSenderSvc.RunOnce)`), тикает каждые ~10 сек.

    **Каждый тик — четыре фазы. SQL ниже — псевдокод для читабельности; в коде это отдельные методы repository, каждый делает свой запрос.**

    **Phase 0 — recovery (вне Tx, network).** Найти orphan'ов после прошлых сбоев:
    ```sql
    SELECT id, unsigned_pdf_content FROM contracts
    WHERE trustme_document_id IS NULL AND subject_kind = 'campaign_creator'
    ```
    Для каждого `contract.id`: вызов TrustMe `POST /search/Contracts` (фильтр по `additionalInfo = contracts.id`).
    - Если TrustMe знает наш document → переходим к **Phase 3 finalize** с known `trustme_document_id`.
    - Если не знает + `unsigned_pdf_content IS NOT NULL` → переход к **Phase 2c send** (PDF уже в БД — не рендерим overlay повторно).
    - Если не знает + `unsigned_pdf_content IS NULL` (Phase 2b упал до persist) → переход к **Phase 2a render** + 2b persist + 2c send.

    Идемпотентность гарантируется state'ом на стороне TrustMe (через `additionalInfo`), а не нашим unique-constraint.

    **Phase 1 — claim (Tx_init, миллисекунды).** Один `dbutil.WithTx` callback, в нём три отдельных вызова через repo:
    1. **SELECT-claim** (один запрос, JOIN с `campaigns` и `creators` — все данные для overlay достаём здесь):
       ```sql
       SELECT cc.id AS cc_id, cc.creator_id, cc.campaign_id,
              c.contract_template_pdf,
              cr.last_name, cr.first_name, cr.middle_name, cr.iin, cr.phone
       FROM campaign_creators cc
       JOIN campaigns c ON c.id = cc.campaign_id
       JOIN creators cr ON cr.id = cc.creator_id
       WHERE cc.status = 'agreed'
         AND cc.contract_id IS NULL
         AND c.is_deleted = false
         AND length(c.contract_template_pdf) > 0
       ORDER BY cc.decided_at
       LIMIT 4
       FOR UPDATE SKIP LOCKED
       ```
    2. Для каждого ряда из шага 1 — отдельный INSERT в `contracts`:
       ```sql
       INSERT INTO contracts (id, subject_kind, trustme_status_code, initiated_at)
       VALUES ($1, 'campaign_creator', 0, now())
       RETURNING id
       ```
    3. Для того же ряда — отдельный UPDATE на `campaign_creators`:
       ```sql
       UPDATE campaign_creators SET contract_id = $1, status = 'signing', updated_at = now() WHERE id = $2
       ```

    Tx закрывается миллисекунды, **никаких сетевых вызовов внутри**. После COMMIT ряд `campaign_creators` уже имеет `contract_id` → другие worker-инстансы его не подхватят. Данные `creators` поедут дальше в memory как часть claim'а. **Phone здесь забираем для TrustMe `Requisites.PhoneNumber`**, не для шаблона договора (см. Decision #13).

    **Phase 2a — overlay PDF (вне Tx, CPU).** Для каждого claim'нутого `contract.id`: `ContractPDFRenderer.Render(template []byte, ContractData) ([]byte, error)`. Без сетевых вызовов — pure CPU-работа на шаблоне из БД.

    **Phase 2b — persist unsigned PDF (одиночный UPDATE, без Tx-обёртки).** Сохраняем результат overlay'а до отправки в TrustMe — на случай сбоя Phase 2c recovery подхватит готовый PDF и не будет рендерить повторно:
    ```sql
    UPDATE contracts SET unsigned_pdf_content = $1 WHERE id = $2
    ```
    Один мутирующий запрос — Postgres гарантирует атомарность без явной транзакции.

    **Phase 2c — send в TrustMe (вне Tx, network).** Отправка в TrustMe `SendToSignBase64FileExt` с `additionalInfo = contract.id`. Из ответа достаём `trustme_document_id`, `trustme_short_url`. Сетевые вызовы могут быть медленными — connection pool не страдает, никакой Tx не висит.

    **Phase 3 — finalize (Tx_finalize, миллисекунды).** Один `dbutil.WithTx` callback с двумя отдельными запросами:
    1. **UPDATE contracts** — записать результат TrustMe (unsigned PDF уже в БД с Phase 2b):
       ```sql
       UPDATE contracts
       SET trustme_document_id = $1, trustme_short_url = $2
       WHERE id = $3
       ```
    2. **INSERT audit_logs** — `action='campaign_creator.contract_initiated'` с `actor_id = NULL` (system-actor) — в той же транзакции (стандарт `backend-transactions.md`: audit обязан жить ВНУТРИ Tx с mutate-операцией).

    **После COMMIT (вне Tx callback)** — бот-уведомление креатору «договор отправлен на подпись» (по стандарту: бот / success-логи fire-and-forget живут ПОСЛЕ Tx).

    **Failure scenarios:**
    - Phase 2a упала (overlay-render): contract row остаётся `unsigned_pdf_content IS NULL`, `trustme_document_id IS NULL`. Phase 0 next tick → re-render + persist + send.
    - Phase 2b упала (БД): редкий кейс. На next tick Phase 0 → orphan без PDF → re-render + persist + send.
    - Phase 2c упала (TrustMe): contract row имеет `unsigned_pdf_content`, но `trustme_document_id IS NULL`. Phase 0 next tick → search/Contracts. Если TrustMe знает (мы успели отправить, упало по timeout до получения ответа) → finalize. Если не знает → re-send (PDF уже в памяти из row).
    - Phase 3 упала (БД после успешного send): contract row имеет `unsigned_pdf_content`, `trustme_document_id IS NULL`. Phase 0 next tick → search/Contracts найдёт known document → finalize.

    **⚠ Риск Phase 0 recovery — фильтр `search/Contracts` по `additionalInfo`.** Метод `POST /search/Contracts` существует в blueprint (стр. 912), но структура `searchData[]` для фильтра по `additionalInfo` в blueprint размыта. **Нужна проверка против sandbox** на ранней стадии chunk 16. Если фильтр по `additionalInfo` не работает — план B на MVP: оптимистично перепосылать orphan'ов с тем же `additionalInfo`, принять риск дублей на TrustMe-side (объёмы низкие ~100 на EFW, очистка дубля через ЛК TrustMe — runbook). План B зафиксировать в спеке chunk 16 после теста sandbox.

    Бесконечный retry без backoff (объёмы низкие — на EFW ~100 договоров за всю кампанию). `SKIP LOCKED` защищает от race при HA / blue-green. Никаких промежуточных статусов `signing_pending`/`signing_failed` — `agreed` без `contract_id` = «в outbox»; `signing` с `trustme_document_id IS NULL` = «в recovery». Graceful shutdown — встроенно в существующий `cl.Add("cron", scheduler.Stop)`: при SIGTERM ждёт текущий batch. Risk дубля СМС если упадём между ответом TrustMe и COMMIT Phase 3 — задокументирован, runbook «отозвать дубль через ЛК TrustMe» отдельно. Пример паттерна batch-обработки — `backend/cmd/scripts/notify-pending-creators`.

11. **Источник контента — `campaigns.contract_template_pdf BYTEA NOT NULL DEFAULT '\x'`.** Готовый PDF, который Аидана сделала в Google Docs (с тремя плейсхолдерами в формате `{{Name}}`) и загрузила в админке через PDF-upload компонент. Markdown / HTML / templating-движки в pipeline не участвуют.

    **Загрузка PDF в админке — через два новых endpoint'а** (см. Decision #18):
    - `PUT /campaigns/{id}/contract-template` — multipart upload (form-data field `file`, `Content-Type: application/pdf`).
    - `GET /campaigns/{id}/contract-template` — download PDF (для preview в админке).

    Поля `name` и `tma_url` на эндпоинтах `POST /campaigns` / `PATCH /campaigns/{id}` остаются как есть; PDF не передаётся в JSON.

    **Defense in depth — три уровня валидации:**
    - **(form)** `domain.ValidateContractTemplatePDF(pdf []byte) error` в `internal/domain/contract.go`. Проверки:
       - `len(pdf) > 0` → 422 `CodeContractRequired`.
       - PDF parseable через `ledongthuc/pdf.NewReader` → 422 `CodeContractInvalidPDF`.
       - В шаблоне присутствуют **все три** известных плейсхолдера (`{{CreatorFIO}}`, `{{CreatorIIN}}`, `{{IssuedDate}}`) — extract'им через тот же путь, что и в `ContractPDFRenderer` (whitespace-based word split, regex `\{\{(\w+)\}\}`). Missing → 422 `CodeContractMissingPlaceholder` с перечислением каких именно плейсхолдеров не хватает.
       - Все найденные плейсхолдеры — известные (`CreatorFIO`/`CreatorIIN`/`IssuedDate`). Unknown → 422 `CodeContractUnknownPlaceholder` с перечислением.
    - **(notify)** `POST /campaigns/{id}/notify` (chunk 12, уже в main) — guard первой строкой service-метода: `len(contract_template_pdf) == 0` → 422 `CodeContractRequired`. Защищает legacy-кампании, созданные до миграции (DEFAULT '\x').
    - **(outbox)** `ContractSenderService.RunOnce` — ряд с пустым PDF не попадает в claim (фильтр `length(c.contract_template_pdf) > 0` в SELECT, см. Decision #10 Phase 1). Третий слой защиты на случай race / direct-DB-edit.

    **Блокировка edit'а после первой отправки.** Upload нового PDF (`PUT /campaigns/{id}/contract-template` при наличии существующего шаблона) отклоняется (422 `CodeContractTemplateLocked`) если у этой кампании есть хоть один `campaign_creator` со статусом из `('signing','signed','signing_declined')`. Иначе — коллизия версий: одни креаторы подписали v1 контракта, другие подпишут v2. Проверка в `CampaignService.UploadContractTemplate`. Имена остальных полей кампании (`name`, `tma_url`) остаются редактируемыми всегда.

12. **Движок шаблонизации PDF — pure-Go overlay.** Pipeline: load `contract_template_pdf` из БД → `ledongthuc/pdf.NewReaderEncrypted` парсит шаблон постранично → для каждой страницы извлекаем токены `{{Name}}` и их bbox → `signintech/gopdf` импортирует страницу шаблона как template → рисует белый прямоугольник поверх каждого плейсхолдера → рисует значение шрифтом Liberation Serif на baseline'е → сохраняет результирующий PDF в memory. Без сети, без sidecar'ов, без CGo.

    Сервис — `ContractPDFRenderer` в `internal/contract/`: метод `Render(ctx, template []byte, data ContractData) ([]byte, error)`. Помимо `Render`, отдельный `ExtractPlaceholders(pdf []byte) ([]Placeholder, error)` используется на стадии (form) валидации (Decision #11) — общий код с `Render`, чтобы валидация и рендер использовали один и тот же extraction-алгоритм.

    Embedded TTF шрифт — копия `LiberationSerif-Regular.ttf` (метрики совместимы с Times New Roman, который Google Docs использует) лежит в `internal/contract/fonts/LiberationSerif-Regular.ttf`. В Dockerfile файл уже попадает через стандартный `COPY backend/ /app/backend/`. На системные шрифты `/usr/share/fonts/` не полагаемся — образ может не содержать `liberation-fonts` пакет.

    Reference implementation — `_bmad-output/experiments/pdf-overlay/main.go` (~230 строк). Production-версия отличается тем, что: (а) принимает `[]byte` вместо file path для шаблона; (б) shrифт читается из embedded asset (`embed.FS` или `os.ReadFile` из `internal/contract/fonts/`), не из абсолютного path; (в) `ContractData` собирается из `creators`-row в outbox-worker'е; (г) ошибки оборачиваются через `fmt.Errorf` с контекстом и пробрасываются вызывающему (нет `log.Fatal`); (д) код покрывается unit-тестами с фикстурным PDF в `internal/contract/testdata/template.pdf` (зальём минимальный валидный PDF со всеми тремя плейсхолдерами).

    `ledongthuc/pdf` и `signintech/gopdf` добавляются в `docs/standards/backend-libraries.md` registry в составе chunk 16. Обоснование «нет адекватных pure-Go альтернатив для extract bbox per word без CGo» — в комментарии PR.

13. **`ContractData` — три типизированных поля.**

    ```go
    type ContractData struct {
        // Креатор
        CreatorFIO string  // склейка last_name + " " + first_name + " " + middle_name (без NULL middle_name)
        CreatorIIN string  // 12 цифр

        // Договор
        IssuedDate string  // "«D» месяц YYYY г." — Asia/Almaty, родительный падеж месяца, день без leading zero
    }
    ```

    Поле `IssuedDate` собирается в outbox-worker'е перед рендером:
    ```go
    var months = [...]string{"января","февраля","марта","апреля","мая","июня",
        "июля","августа","сентября","октября","ноября","декабря"}
    loc, _ := time.LoadLocation("Asia/Almaty")
    now := time.Now().In(loc)
    issuedDate := fmt.Sprintf("«%d» %s %d г.", now.Day(), months[now.Month()-1], now.Year())
    ```

    Имя/детали кампании, реквизиты Models Production, текст ТЗ — **статика внутри** `contract_template_pdf` (Аидана зашивает их в шаблон при редактировании). `CampaignName`/`CampaignID` в `ContractData` не передаём.

    **Что не идёт в шаблон.** `CreatorPhone` нужен TrustMe для `Requisites.PhoneNumber` (СМС-OTP), но в текст договора не вставляется; нормализуется через `domain.NormalizePhoneE164(string) string` в `internal/domain/phone.go` непосредственно в `TrustMeClient.SendToSign`. `ContractNumber` для EFW pilot не используется — если в будущем потребуется, добавится 4-м полем `ContractData`.

    Адрес, город, дата рождения креатора — НЕ в шаблоне.

14. **Webhook idempotency — по `(trustme_document_id, target_status_code)`.** Webhook-handler делает `UPDATE contracts SET trustme_status_code = $new WHERE trustme_document_id = $id AND trustme_status_code != $new RETURNING ...`. Если 0 рядов затронуто — повтор того же события, no-op (ни state-transition, ни audit, ни бот-уведомление). 1 ряд затронут — обрабатываем дальше. Это и есть idempotency без отдельной идентификации события (TrustMe-payload не несёт event-ID). Откат состояний (например, webhook прилетит status=2 после status=3) — игнорируем через `WHERE trustme_status_code IN (terminal_or_lower)` clause; finalize'нные `signed/signing_declined` не возвращаются назад.

15. **Soft-deleted кампании исключаются из outbox.** SQL `SELECT FOR UPDATE SKIP LOCKED` в `ContractSenderService.RunOnce` — JOIN с `campaigns` + `WHERE campaigns.is_deleted = false`. Если админ soft-удалил кампанию между `agreed` и тиком worker'а, договор не отправляется; ряд `campaign_creators` остаётся `agreed` навсегда (tombstone). Аналогично — webhook на `campaign_creators`, чья кампания soft-deleted: state-update делаем (TrustMe-договор уже подписан, factual record), бот-уведомление пропускаем + audit-warning.

16. **PII вне stdout-логов (security.md hard rule).** Логируем UUID'ы (`creator_id`, `campaign_id`, `trustme_document_id`), HTTP-метаданные, status codes. **НЕ логируем** ФИО, ИИН, телефон, адрес, содержимое `contract_template_pdf` или `unsigned_pdf_content`. Если нужна диагностика шаблона — sha256-fingerprint первых N байт, не сам контент. В `audit_logs` (БД) — допустимо по проектному правилу `Audit vs stdout-логи`. В `error.Message` (response body) — анти-fingerprinting: одинаковый текст для разных причин отказа, без утечки внутренних данных.

17. **Spy-паттерн для TrustMe.** По образцу `internal/telegram` три класса: (a) `Client` interface — методы реальной интеграции; (b) `RealClient` — HTTP-клиент против настоящего TrustMe; (c) `SpyOnlyClient` (только пишет в `SentSpyStore`, не ходит в сеть; используется при `TRUSTME_MOCK=true`) и `TeeClient` (пишет в spy + дёргает `RealClient`; используется при `EnableTestEndpoints=true && TRUSTME_MOCK=false` — staging mode «реально шлём + e2e может проверить»).

    `internal/trustme/spy_store.go`: `SentRecord{TrustMeDocumentID, ContractData, AdditionalInfo, SentAt, Err}` + `RegisterFailNext(docID, reason)` для имитации 5xx + `RegisterCallback(docID, status)` для имитации разных webhook-исходов на e2e-страховке. Capacity по образцу telegram (5000 ring).

    Test-API endpoints для spy-управления — рядом с существующими в `internal/testapi/` (по образцу `/test/telegram/spy-*`). Доступны только при `EnableTestEndpoints=true`.

18. **API surface — два новых endpoint'а для шаблона PDF.**

    - **`PUT /campaigns/{id}/contract-template`** — multipart upload, form-data field `file`, `Content-Type: application/pdf`. Создание или замена шаблона. Auth — admin-only. Ответы:
      - 200 — `{"hash": "<sha256>"}` (для дальнейшего отображения в админке: «загружен шаблон ###»).
      - 422 — `CodeContractRequired` / `CodeContractInvalidPDF` / `CodeContractMissingPlaceholder` / `CodeContractUnknownPlaceholder` / `CodeContractTemplateLocked`.
      - 404 — `CodeCampaignNotFound`.
    - **`GET /campaigns/{id}/contract-template`** — download PDF, `Content-Type: application/pdf`. Auth — admin-only. Ответ — body = PDF bytes; 404 если шаблон не загружен (`length(contract_template_pdf) == 0`).

    Альтернатива — base64 в существующем `PATCH /campaigns/{id}` JSON. Отвергнута: PDF до ~5 МБ раздуется на 33% в base64 (до 6.7 МБ), плюс binary-в-JSON через openapi-codegen — уродливый шаблон. Multipart чище.

    **OpenAPI extension.** Multipart-upload в `oapi-codegen` поддерживается, но требует ручного описания `requestBody.content."multipart/form-data"`. В spec'е chunk 16 фиксируем точные имена полей и mime types.

## Параметры overlay-рендера (фиксируем после эксперимента)

Ниже — **числа и алгоритмы, которые мы подтвердили на реальном шаблоне** (`legal-documents/Тест, эксперимент BLACK BURN_Соглашение_UGC.docx-1.pdf`, 5 страниц A4, экспорт Google Docs Skia/PDF m149, шрифт Times New Roman + Arial, 11pt). Reference implementation — `_bmad-output/experiments/pdf-overlay/main.go`. Production-код в `internal/contract/pdf_renderer.go` обязан повторить эти параметры один-в-один.

**Размеры страницы:**
- `pageWidth = 596.0`, `pageHeight = 842.0` (A4 в PDF user space, points). Если в будущем шаблон будет другого размера — детектить из `pdf.Page.V.Key("MediaBox")`, не хардкодить. На EFW pilot все шаблоны через Google Docs → A4, можно хардкодить.

**Y-координаты:**
- `ledongthuc/pdf` отдаёт `Text.Y` = baseline в **bottom-left** origin.
- `signintech/gopdf` использует **top-left** origin.
- Конверсия:
  ```go
  yGlyphTop := pageHeight - (yBaseline + fontSize * ascentRatio)
  yGlyphBot := pageHeight - (yBaseline - fontSize * descentRatio)
  ```

**Шрифтовые соотношения** (для Times New Roman / Liberation Serif):
- `ascentRatio = 0.75` — высота над baseline в долях `fontSize`. Подобрано визуально на реальном шаблоне после двух итераций (0.80 → overlay чуть выше реального; 0.75 — точное совпадение).
- `descentRatio = 0.27` — высота под baseline в долях `fontSize`.
- `pad = 1.0` pt — запас по вертикали для white rect (anti-alias halo + микро-дрейф).

**White rect (cover):**
```go
out.SetFillColor(255, 255, 255)
out.RectFromUpperLeftWithStyle(
    ph.XMin,
    yGlyphTop - pad,
    ph.XMax - ph.XMin,
    (ascentRatio + descentRatio) * fontSize + 2 * pad,
    "F",
)
```

**Text overlay:**
```go
out.SetFillColor(0, 0, 0)
out.SetFont("body", "", ph.FontSize)
out.SetX(ph.XMin)
out.SetY(yGlyphTop)
out.Cell(nil, value)
```

**FontSize per placeholder** — берём из `pdf.Text.FontSize` (per-glyph метаданные ledongthuc'а), не хардкодим. На EFW шаблоне все плейсхолдеры 11pt, но при изменении шаблона значение должно подхватываться автоматически.

**Bbox extraction algorithm:**
1. Открыть PDF: `f, r, _ := pdf.Open(path)`. Итерировать `pageNum := 1; pageNum <= r.NumPage()`.
2. Для каждой страницы — `chars := page.Content().Text` (массив `pdf.Text`).
3. **Группировка в строки по Y** — `groupByLine(chars [][]pdf.Text)`:
   - Сортировать копию массива по `Y desc, X asc` (стабильная сортировка).
   - Группировать последовательные элементы по `abs(c.Y - line[0].Y) < 0.5` (tolerance 0.5pt).
4. **Whitespace-based word split** — `splitWords(line []pdf.Text) []wordBox`:
   - **Важно**: `ledongthuc/pdf` всегда отдаёт `W=0` (не считает ширину глифа). Gap-based heuristic не работает.
   - Whitespace глифы (`strings.TrimSpace(c.S) == ""`) — единственный признак границы слова.
   - При встрече whitespace: текущий word завершается, `word.xMax = c.X` (X пробела).
   - Последний word на строке (без trailing whitespace) — fallback estimate `xMax = xMin + fontSize * 0.5 * runeCount(text)` (грубое среднее для proportional font).
5. **Поиск плейсхолдера** — regex `\{\{(\w+)\}\}` на тексте word'а. Match → создаём `Placeholder{Page, Name, XMin, XMax, YBaseline, FontSize}`.

**Whitelist плейсхолдеров:** `CreatorFIO`, `CreatorIIN`, `IssuedDate`. На (form) валидации unknown name → 422 `CodeContractUnknownPlaceholder`.

**Шрифт overlay:** `LiberationSerif-Regular.ttf`, embedded в `internal/contract/fonts/`. Метрики совместимы с Times New Roman (Google Docs default для русского), визуально шов незаметен. На системные шрифты `/usr/share/fonts/` не полагаемся (Docker-image может быть minimal).

**Multi-page templates.** Outbox-worker рендерит **все** страницы шаблона (включая страницы без плейсхолдеров — например, ТЗ с картинками). `signintech/gopdf` импортирует постранично через `ImportPage(file, n, "/MediaBox")` и кладёт на новую страницу через `UseImportedTemplate(tpl, 0, 0, pageWidth, pageHeight)`. Картинки и форматирование исходного PDF сохраняются — overlay только в местах плейсхолдеров.

## Конвенция шаблона PDF (для Аиданы)

В шаблоне Google Docs:

1. **Три плейсхолдера в формате `{{Name}}`**, ровно эти имена (CamelCase, без `_`):
   - `{{CreatorFIO}}` — ФИО креатора целиком.
   - `{{CreatorIIN}}` — ИИН (12 цифр).
   - `{{IssuedDate}}` — дата подписания целиком (`«9» мая 2026 г.`).

2. **Каждый плейсхолдер — на отдельной строке**, либо **в самом конце строки**. Никакого значимого текста справа от плейсхолдера на той же строке. Бэк рисует overlay начиная с xMin плейсхолдера; если справа окажется значимый текст, длинное значение перекроет его.

3. **Один стиль текста на маркер.** Без bold/italic в середине плейсхолдера, без переноса строки внутри. Иначе Google Docs разбивает токен на несколько runs, и overlay-extractor его не сматчит regex'ом.

4. **Допускается дублирование** одного плейсхолдера в разных местах документа (например, ФИО в шапке + в реквизитах + в подписной части) — бэк подставит везде одинаково.

5. **Underscore в именах плейсхолдеров запрещён** (например, `{{CREATOR_FIO}}`). Markdown-экспорт Google Docs эскейпит `_` как `\_`, что ломает любой text/template-based pipeline. CamelCase обходит проблему.

6. **Запас места справа** не обязателен — overlay сам рисует на любую ширину до правого края страницы. Но если не хочешь, чтобы длинное ФИО подъехало под подпись справа — оставь после плейсхолдера 30+ пробелов до правой границы (для подстраховки).

Эта конвенция фиксируется в admin-help: страница Help в админке (или onboarding-tooltip над PDF-upload) ссылается на этот раздел.

## Данные на креатора в БД (для запроса в TrustMe)

Источник — таблица `creators` (миграция `20260505052212_creators.sql`):

| Поле в TrustMe (обязательное) | Источник у нас                                                            | Заметка                                                                                                                                                                     |
|-------------------------------|---------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Requisites[i].FIO`           | `last_name + ' ' + first_name + ' ' + middle_name` (если NULL — без него) | as-is                                                                                                                                                                       |
| `Requisites[i].IIN_BIN`       | `creators.iin`                                                            | уже валиден `^[0-9]{12}$` (CHECK на `creator_applications.iin` + `creators_iin_unique`)                                                                                     |
| `Requisites[i].PhoneNumber`   | `creators.phone`                                                          | формат E.164. **Нормализуется** через `domain.NormalizePhoneE164` в `TrustMeClient.SendToSign` (НЕ в `ContractData` — Phone в шаблон договора не идёт). БД хранит phone как ввёл пользователь (`maxLength:32` без regex) |
| `Requisites[i].CompanyName`   | литерал «Креатор» (или ФИО)                                               | необязательно, для ЛК TrustMe                                                                                                                                               |

Email **не нужен** — TrustMe для физлица-подписанта работает по СМС / ЭЦП.

Поля шаблона (`ContractData`) и поля `Requisites` собираются параллельно в outbox-worker'е после Phase 1 claim'а: `creators`-row уже в memory (см. SELECT в Decision #10 Phase 1).

## TrustMe API — что используем

Из `docs/external/trustme/blueprint.apib`. Базовый URL — `https://test.trustme.kz/trust_contract_public_apis/` (prod-host получим, когда TrustMe выдадут production-токен; на pilot работаем против test-окружения). Auth — статичный токен в header `Authorization: <token>` (без `Bearer`-префикса). Лимит — 4 req/sec.

| Метод                                                                       | Назначение                                            | Когда вызываем                                                                                                |
|-----------------------------------------------------------------------------|-------------------------------------------------------|---------------------------------------------------------------------------------------------------------------|
| `POST /SendToSignBase64FileExt/{file_extension}` (где `file_extension=pdf`) | Загрузка PDF договора                                 | outbox-worker Phase 2c (после claim ряда `campaign_creators` в `signing` и overlay'а PDF в Phase 2a)          |
| `POST /search/Contracts`                                                    | Найти TrustMe-document по нашему `additionalInfo`     | outbox-worker Phase 0 (recovery — установить, получил ли TrustMe наш предыдущий send, до того как ретраить)   |
| `GET /ContractStatus/{document_id}`                                         | Получить статус документа                             | резерв на recover-flow если webhook просрос (chunk 17)                                                        |
| `GET /doc/DownloadContractFile/{document_id}`                               | Скачать подписанный PDF                               | по webhook'у status=3 (chunk 17) + admin-кнопка скачивания                                                    |
| `POST /SetHook`                                                             | Зарегистрировать webhook URL + статичный bearer-токен | один раз при настройке окружения (admin-ручка или CLI)                                                        |
| `GET /HookInfo`                                                             | Прочитать установленный webhook                       | диагностика                                                                                                   |

**Webhook payload (входящий к нам):**

```json
{
  "contract_id": "2uml98kxc",
  "status": 3,
  "client": "77071507875",
  "contract_url": "www.tct.kz/uploader/2uml98kxc"
}
```

Карта статусов TrustMe (`status` в response/webhook — integer):

| Code | Смысл                                |
|------|--------------------------------------|
| 0    | Не подписан                          |
| 1    | Подписан компанией (нашей стороной)  |
| 2    | Подписан клиентом                    |
| 3    | Полностью подписан                   |
| 4    | Отозван компанией                    |
| 5    | Компания инициировала расторжение    |
| 6    | Клиент инициировал расторжение       |
| 7    | Клиент отказался от расторжения      |
| 8    | Расторгнут                           |
| 9    | Клиент отказался подписывать договор |

## Webhook flow (контекст для chunk 17)

Из blueprint: webhook стреляет при **каждом изменении статуса** документа. Наша конфигурация — auto-sign на стороне компании настроен «после клиента».

**Happy path (3 шага, 2 входящих webhook'а):**

1. **Phase 2c outbox** → `SendToSignBase64FileExt`. В ответе TrustMe возвращает `document_id` со статусом 0. Webhook здесь не стреляет (первичная загрузка, не изменение); если и стрельнёт — `WHERE trustme_status_code != $new` в handler'е (см. Decision #14) сделает no-op.
2. **Креатор подписал по СМС-OTP** → статус 0 → **2**. Прилетает webhook `{status: 2, client: <phone креатора>}`. Реакция: `UPDATE contracts SET trustme_status_code = 2 WHERE trustme_document_id = $1 AND trustme_status_code != 2`. `campaign_creators.status` остаётся `signing` (intermediate). **Бот не стреляет** — креатор и так знает, что подписал; не шумим.
3. **Auto-sign** на нашей стороне триггерится TrustMe'ом → статус 2 → **3**. Прилетает webhook `{status: 3, client: <phone компании>}`. Реакция (всё в одной Tx, см. Decision #14): `UPDATE contracts SET trustme_status_code = 3, signed_at = now()` + `UPDATE campaign_creators SET status = 'signed'` + audit `campaign_creator.contract_signed`. **После Tx (вне callback'а)**: `GET /doc/DownloadContractFile/{document_id}` → `UPDATE contracts SET signed_pdf_content = $1` + бот креатору «договор подписан обеими сторонами».

   Между webhook'ами 2 и 3 — задержка на TrustMe-side обработку auto-sign'а (секунды-минуты). Хэндлер идемпотентен, повторные webhook'и со status=3 = no-op.

**Decline path (1 webhook):**

- Креатор нажимает «Отказаться» в TrustMe → статус 0 → **9**. Auto-sign не триггерится. Прилетает webhook `{status: 9}`. Реакция: `UPDATE contracts SET trustme_status_code = 9, declined_at = now()` + `UPDATE campaign_creators SET status = 'signing_declined'` + audit `campaign_creator.contract_signing_declined`. Бот креатору в decline-flow — открытая развилка спеки chunk 17.

**Прочие статусы (1, 4, 5, 6, 7, 8)** — поведение зафиксировано в Decision #7: `contracts.trustme_status_code` обновляем (audit-trail), `campaign_creators.status` не трогаем, audit + warning-log.

**Скачивание signed PDF — после COMMIT, не внутри webhook Tx.** TrustMe `DownloadContractFile` — медленный сетевой вызов; если он упадёт, webhook-handler уже за пределами Tx → 200 в TrustMe вернули, статус в БД зафиксирован, PDF загружается отдельным retry (recovery-job или admin-кнопка). Тот же принцип «network вне Tx», что и в outbox-worker'е.

## Открытые развилки

Все крупные структурные развилки закрыты решениями 1–18. Что остаётся для финализации в спеке (chunk 16/17 spec через `bmad-quick-dev`):

- Точные имена констант ошибок (`CodeContractRequired`, `CodeContractInvalidPDF`, `CodeContractMissingPlaceholder`, `CodeContractUnknownPlaceholder`, `CodeContractTemplateLocked`) и их HTTP-маппинг.
- Точные тексты бот-уведомлений (один-два варианта на каждое событие).
- Структура папок: `internal/contract/`, `internal/trustme/` — детали в спеке.
- Test-API эндпоинт для force-trigger `ContractSenderService.RunOnce` (e2e не должны ждать 10 сек).
- Конфиг-переменные в `internal/config/config.go`: `TrustMeBaseURL`, `TrustMeToken`, `TrustMeWebhookToken` + соответствующие `*Mock` для test-окружения.
- OpenAPI-схема для `POST /trustme/webhook` payload (есть в blueprint, переписать в openapi.yaml).
- OpenAPI-схема для `PUT /campaigns/{id}/contract-template` (multipart) и `GET /campaigns/{id}/contract-template` (binary response). Пример multipart upload в существующих ручках — отсутствует, описание делаем с нуля по oapi-codegen documentation (Decision #18).
- Sandbox-тест `POST /search/Contracts` фильтра по `additionalInfo` — runbook на старте chunk 16 (Decision #10 Phase 0 risk).

## Нарезка на 2 PR-чанка

|                 | **Chunk 16 — отправка**                                                                                                                                                                                                                                                                                                                                                                                                              | **Chunk 17 — приём**                                                                                                                                      |
|-----------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------|
| Миграции        | `contracts` table; `campaigns ADD COLUMN contract_template_pdf BYTEA NOT NULL DEFAULT '\x'`; `campaign_creators ADD COLUMN contract_id` + ALTER state-machine `agreed→signing`                                                                                                                                                                                                                                                       | none                                                                                                                                                      |
| Compose         | none                                                                                                                                                                                                                                                                                                                                                                                                                                 | none                                                                                                                                                      |
| State-machine   | `agreed → signing`                                                                                                                                                                                                                                                                                                                                                                                                                   | `signing → signed / signing_declined`                                                                                                                     |
| Сервисы         | `ContractSenderService.RunOnce` (outbox-worker), `ContractPDFRenderer` (`Render` + `ExtractPlaceholders`, на `ledongthuc/pdf` + `signintech/gopdf`), `TrustMeClient.SendToSign(...)`; notify-guard в существующем `NotifyCampaignCreators` (chunk 12); `CampaignService.UploadContractTemplate` + `GetContractTemplate`                                                                                                               | `TrustMeWebhookService.HandleEvent(payload)` + dispatcher по `subject_kind`, `TrustMeClient.DownloadContract(docID)`                                      |
| OpenAPI         | новые `PUT /campaigns/{id}/contract-template` (multipart) + `GET /campaigns/{id}/contract-template` (binary); коды ошибок `CodeContractRequired`, `CodeContractInvalidPDF`, `CodeContractMissingPlaceholder`, `CodeContractUnknownPlaceholder`, `CodeContractTemplateLocked`                                                                                                                                                         | новый `POST /trustme/webhook`                                                                                                                             |
| Frontend        | PDF-upload компонент в форме `/campaigns/new` + `/campaigns/{id}/edit`; preview найденных плейсхолдеров; download-кнопка существующего шаблона; валидация notEmpty + parseable + всех известных плейсхолдеров                                                                                                                                                                                                                        | none                                                                                                                                                      |
| Endpoints (бэк) | `PUT /campaigns/{id}/contract-template`, `GET /campaigns/{id}/contract-template` (cron triggers внутри outbox)                                                                                                                                                                                                                                                                                                                       | `POST /trustme/webhook` (public, bearer-auth, idempotent по `(trustme_document_id, status_code)`)                                                         |
| Audit           | `campaign.contract_template_uploaded`, `campaign_creator.contract_initiated`                                                                                                                                                                                                                                                                                                                                                         | `campaign_creator.contract_signed`, `…contract_signing_declined`, `…contract_unexpected_status`                                                           |
| Бот креатору    | «договор отправлен на подпись»                                                                                                                                                                                                                                                                                                                                                                                                       | «договор подписан обеими сторонами»                                                                                                                       |
| Тесты           | unit ≥80% per-method на каждый сервис + domain-валидация (form/notify/outbox); unit на `ContractPDFRenderer.Render` (фикстурный PDF в `internal/contract/testdata/template.pdf` → подставить → re-extract → значения совпадают) и на `ExtractPlaceholders` (валидный/без плейсхолдеров/с unknown); integration outbox-flow со spy-TrustMe; race-test `go test -race` на parallel `RunOnce`; TrustMe-down resiliency; пустой `contract_template_pdf` → 422 form, 422 notify, skip+claim-исключение в outbox; **lock-edit test** (`PUT /campaigns/{id}/contract-template` когда есть `campaign_creator.status IN ('signing','signed','signing_declined')` → 422 `CodeContractTemplateLocked`); **persist-then-send test** (Phase 2c упала после Phase 2b → ряд имеет `unsigned_pdf_content`, recovery шлёт без re-render) | unit на `HandleEvent` для каждого статуса 0..9; integration на endpoint + idempotency-test; edge cases: 401, 404, 422, неожиданный статус → audit-warning |

**Frontend-индикатор** статуса контракта в карточке `campaign_creator` — встраиваем как mini-PR в `chunk 15` (расширение страницы кампании со статусами и счётчиками), не отдельный chunk.

**Pre-flight** (вне Группы 7, отдельные мини-PR'ы или runbook): SetHook-регистрация, `/companies` health-check, ротация TrustMe-токена, sandbox-тест `search/Contracts` фильтра.

## Ссылки

- Roadmap: `_bmad-output/planning-artifacts/campaign-roadmap.md`, Группа 7 (chunks 16–17).
- Design Групп 4–6 (chunks 10–15): `_bmad-output/planning-artifacts/design-campaign-creator-flow.md`.
- Reference implementation overlay-рендера: `_bmad-output/experiments/pdf-overlay/main.go` + сгенерированный `contract-filled.pdf`.
- Тестовый шаблон Аиданы: `legal-documents/Тест, эксперимент BLACK BURN_Соглашение_UGC.docx-1.pdf`.
- Юр-handoff: `legal-documents/КОНТЕКСТ И СТАТУС РАБОТЫ НАД ДОГОВОРАМИ.md`, `legal-documents/ПРИНЯТЫЕ РЕШЕНИЯ.md`.
- TrustMe blueprint: `docs/external/trustme/blueprint.apib`.
- TrustMe Postman: `docs/external/trustme/postman-collection.json`.
- Стандарты: `docs/standards/`.
