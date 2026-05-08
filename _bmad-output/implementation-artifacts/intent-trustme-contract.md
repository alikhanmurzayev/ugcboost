---
title: "Intent: TrustMe-контракт на (campaign, creator) — Группа 7 (chunks 16–17)"
type: intent
status: draft
created: "2026-05-08"
roadmap: _bmad-output/planning-artifacts/campaign-roadmap.md
related_design: _bmad-output/planning-artifacts/design-campaign-creator-flow.md
external_docs:
  blueprint: docs/external/trustme/blueprint.apib
  postman: docs/external/trustme/postman-collection.json
---

# Intent: TrustMe-контракт на каждый `(campaign, creator)`

## Преамбула — стандарты обязательны

Перед любой строкой production-кода агент обязан полностью загрузить все файлы `docs/standards/` (через `/standards`).
Применимы все. Особенно: `backend-architecture.md`, `backend-codegen.md`, `backend-constants.md`, `backend-design.md`,
`backend-errors.md`, `backend-libraries.md`, `backend-repository.md`, `backend-testing-e2e.md`,
`backend-testing-unit.md`, `backend-transactions.md`, `naming.md`, `security.md`, `review-checklist.md`. Каждое
правило — hard rule; отклонение = finding.

## Тезис

Группа 7 (chunks 16–17) реализует **отдельный электронный договор на каждый `(campaign, creator)`** через TrustMe (`test.trustme.kz`). Источник контента договора+ТЗ — поле `campaigns.contract_template_md` (Markdown с Go-template-плейсхолдерами под переменные креатора, **свой шаблон у каждой кампании**). Бэкенд-ручка `POST /tma/campaigns/{secret_token}/agree` (chunk 14, **pre-req — должен быть смержен в main до старта chunk 16**) ставит `campaign_creators.status='agreed'` и возвращает 200 — TrustMe в этой ручке не дёргается. Фоновый **outbox-worker** (cron-goroutine в `cmd/api`) подбирает ряды `agreed` без `contract_id`, рендерит PDF через **Gotenberg sidecar**, шлёт в TrustMe `SendToSignBase64FileExt`, переводит статус в `signing`. Креатор подписывает по СМС-OTP, после него **автоматически срабатывает наш auto-sign** (TrustMe настроил порядок «сначала клиент, потом мы»), webhook приводит `campaign_creators` к терминальному `signed` или `signing_declined`, бот уведомляет креатора. Рамочный договор (один-на-всю-жизнь-креатора) **отменён**. Отзыв/расторжение — out of scope (post-event).

## Скоуп

В Группу 7 входят:

- **chunk 16 — отправка**: миграция `contracts` table + `campaigns.contract_template_md` +
  `campaign_creators.contract_id` FK; outbox-worker; ContractPDFRenderer (Gotenberg-клиент); TrustMeClient.SendToSign;
  форма create/edit кампаний расширяется textarea для контракта; defense-in-depth валидация (form + notify-guard +
  outbox-skip).
- **chunk 17 — приём**: `POST /trustme/webhook` (public, bearer-auth, idempotent); state-transitions
  `signing → signed / signing_declined`; signed PDF persistent storage через `DownloadContractFile`; audit +
  бот-уведомления.

**Out of scope:**

- Рамочный договор и его подписание (отменено).
- Группа 8 (pre-event reminder, подтверждение прихода) и Группа 9 (билеты).
- Юридический контент шаблона договора+ТЗ — отдельная задача Аиданы. Бэк описывает только форму подстановки (поля,
  формат), не пишет сам текст договора.
- Нормализация ФИО креатора (кириллица/латиница). На текущей итерации шлём в TrustMe
  `last_name + first_name + middle_name` as-is из `creators` без валидации.
- Frontend-индикатор статуса контракта — встраивается как mini-PR в `chunk 15` (расширение страницы кампании со
  статусами и счётчиками), не отдельный chunk.
- Pre-flight: SetHook-регистрация webhook URL+token в TrustMe, `/companies` health-check на 401, ручная ротация
  TrustMe-токена — runbook, не код.

## Принятые решения

1. **Договор на каждый `(campaign, creator)`**, не рамочный (отменено).
2. **Способ загрузки в TrustMe — `SendToSignBase64FileExt`** (Base64 в multipart). Не `UploadWithFileURL`: мы не хотим
   открывать публичный download договоров наружу и связываться с signed-URL TTL под retry на стороне TrustMe.
3. **ФИО креатора — as-is** из `creators.last_name/first_name/middle_name` (конкатенация с пробелами). Без валидации
   кириллицы/латиницы. Если упадёт реальный кейс — починим точечно.
4. **PDF-pipeline = Gotenberg sidecar** (отдельный сервис в `docker-compose.yml`). Backend пушит multipart `index.html` → Gotenberg chromium-route возвращает PDF bytes → base64 в TrustMe. Шаблон договора+ТЗ живёт **в БД per-campaign** в `campaigns.contract_template_md` (см. решение #11), не в репозитории — каждая кампания имеет свой шаблон, который Аидана редактирует через UI кампании. **Без отдельного CSS-файла**: HTML-обёртка вокруг сгенерированного goldmark'ом контента — минимальная inline-конструкция в Go-коде (`<html><head><meta charset="utf-8"></head><body>{{html}}</body></html>`); полагаемся на дефолтные стили Chromium для A4-документа. Без pandoc и без Chrome в backend-image, без `os.exec`.
5. **State-машина после `agreed`** — три новых статуса:
    - `agreed → signing` — PDF загружен в TrustMe, ждём креатора (TrustMe-status=0).
    - `signing → signed` — `terminal happy`. Webhook TrustMe-status=3 (auto-sign отработал после креатора).
    - `signing → signing_declined` — `terminal negative`. Webhook TrustMe-status=9 (клиент отказался).

    **Хранение статуса — `TEXT + CHECK constraint`**, не Postgres `ENUM` (соответствует уже существующему `campaign_creators_status_check` в `backend/migrations/20260507044135_campaign_creators.sql:39`). Миграция = `ALTER TABLE ... DROP CONSTRAINT campaign_creators_status_check; ALTER TABLE ... ADD CONSTRAINT campaign_creators_status_check CHECK (status IN ('planned','invited','declined','agreed','signing','signed','signing_declined'));`. Down-migration — обратная пара ALTER (rollback тривиален, без проблем enum-drop-value).
6. **Auto-sign — порядок «сначала клиент, потом мы»**. TrustMe настроил это для нас. Промежуточный TrustMe-status=2 (
   подписан клиентом) у нас short-lived; на нём наш статус `signing` не меняем — ждём `status=3`.
7. **Отзыв/расторжение — out of scope.** Мы не инициируем `RevokeContract` из своей системы. Если webhook прилетит со
   статусом 1/4/5/6/7/8 — `contracts.trustme_status_code` обновляем (это audit-trail TrustMe-события, поле обязано
   отражать последний known статус), пишем audit + warning-лог. **`campaign_creators.status` НЕ трогаем** — наш
   бизнес-статус остаётся в `signing`, мы не транслируем эти TrustMe-статусы в свою state-машину. Разбор — отдельным
   flow позже (post-event).
8. **Универсальная таблица `contracts`** — ubiquitous language. Хранит все документы, отправленные в TrustMe, независимо
   от исхода (в процессе / подписан / отказ). Поля (черновик): `id`, `subject_kind` (discriminator: `'campaign_creator'`
   сейчас, расширяемо), `trustme_document_id` UNIQUE, `trustme_short_url`, `trustme_status_code` (0..9),
   `unsigned_pdf_content BYTEA` (исходный PDF, который мы залили в TrustMe), `signed_pdf_content BYTEA` NULL (
   подписанный PDF c QR обеих сторон — **persistent storage, не кеш**: после webhook'а status=3 webhook-handler делает
   `GET /doc/DownloadContractFile/{document_id}` и UPDATE'ит колонку; источник правды на подписанный артефакт держим у
   себя, чтобы не зависеть от TrustMe-uptime/смены провайдера), `initiated_at`, `signed_at`,
   `declined_at`, `webhook_received_at`, `created_at`, `updated_at`. Content в БД на MVP-объёмах; миграция в S3 — позже
   при необходимости. Reverse-FK на стороне subject:
   `campaign_creators.contract_id UUID NULL REFERENCES contracts(id) ON DELETE SET NULL`. Расширение под
   `brand_agreement` = `ALTER TABLE brand_agreements ADD COLUMN contract_id UUID REFERENCES contracts(id)` + новая
   строка в CHECK + новая ветка в webhook-dispatcher. Без миграции данных.
9. **Webhook receiver — generic `/trustme/webhook`** + dispatcher по `contracts.subject_kind`. Auth — статичный bearer
   из `SetHook.Token`. **Lookup в 2 шага**: (1) `SELECT id, subject_kind, trustme_status_code FROM contracts WHERE
   trustme_document_id = $1` — 404 если не нашли. (2) Reverse-FK по `subject_kind`: для `'campaign_creator'` →
   `SELECT id FROM campaign_creators WHERE contract_id = $contract_id`. Затем UPDATE статуса/timestamps в `contracts`
   + subject-update — в одной транзакции (стандарт audit ВНУТРИ Tx). Ответы handler: 200 / 401 (bad token) / 404 (unknown
   document_id) / 422 (unknown subject_kind).
10. **Outbox-pattern для отправки договоров — два коротких Tx, network-вызовы вне транзакции.** Бэкенд-ручка
    `POST /tma/campaigns/{secret_token}/agree` (chunk 14, **pre-req — должен быть в main до старта chunk 16**) **только**
    ставит `campaign_creators.status = 'agreed'` (никаких походов в TrustMe в самой ручке). Фоновый **worker** в
    `cmd/api/main.go` через `robfig/cron/v3` (scheduler уже инициализирован в `main.go:69-76`, нужно только
    `scheduler.AddFunc("@every 10s", contractSenderSvc.RunOnce)`), тикает каждые ~10 сек.

    **Каждый тик — четыре фазы. SQL ниже — псевдокод для читабельности; в коде это отдельные методы repository, каждый
    делает свой запрос (никакого одного "BEGIN; ...; ...; COMMIT;"-литерала).**

    **Phase 0 — recovery (вне Tx, network).** Найти orphan'ов после прошлых сбоев:
    ```sql
    SELECT id, unsigned_pdf_content FROM contracts
    WHERE trustme_document_id IS NULL AND subject_kind = 'campaign_creator'
    ```
    Для каждого `contract.id`: вызов TrustMe `POST /search/Contracts` (фильтр по `additionalInfo = contracts.id`).
    - Если TrustMe знает наш document → переходим к **Phase 3 finalize** с known `trustme_document_id`.
    - Если не знает + `unsigned_pdf_content IS NOT NULL` → переход к **Phase 2c send** (PDF уже в БД — не рендерим
      повторно).
    - Если не знает + `unsigned_pdf_content IS NULL` (Phase 2b упал до persist) → переход к **Phase 2a render** + 2b
      persist + 2c send.

    Идемпотентность гарантируется state'ом на стороне TrustMe (через `additionalInfo`), а не нашим unique-constraint.

    **Phase 1 — claim (Tx_init, миллисекунды).** Один `dbutil.WithTx` callback, в нём три отдельных вызова через repo:
    1. **SELECT-claim** (один запрос, JOIN с `campaigns` и `creators` — все данные для рендера достаём здесь):
       ```sql
       SELECT cc.id AS cc_id, cc.creator_id, cc.campaign_id,
              c.contract_template_md,
              cr.last_name, cr.first_name, cr.middle_name, cr.iin, cr.phone
       FROM campaign_creators cc
       JOIN campaigns c ON c.id = cc.campaign_id
       JOIN creators cr ON cr.id = cc.creator_id
       WHERE cc.status = 'agreed'
         AND cc.contract_id IS NULL
         AND c.is_deleted = false
         AND c.contract_template_md <> ''
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

    Tx закрывается миллисекунды, **никаких сетевых вызовов внутри**. После COMMIT ряд `campaign_creators` уже имеет
    `contract_id` → другие worker-инстансы его не подхватят. Данные `creators` поедут дальше в memory как часть claim'а
    (никаких повторных SELECT'ов в Phase 2).

    **Phase 2a — render PDF (вне Tx, network в Gotenberg).** Для каждого claim'нутого `contract.id`:
    `ContractPDFRenderer.Render(template, ContractData)` → goldmark MD→HTML → multipart на Gotenberg → PDF bytes.

    **Phase 2b — persist unsigned PDF (одиночный UPDATE, без Tx-обёртки).** Сохраняем PDF до отправки в TrustMe — на
    случай сбоя Phase 2c recovery подхватит готовый PDF и не будет рендерить повторно:
    ```sql
    UPDATE contracts SET unsigned_pdf_content = $1 WHERE id = $2
    ```
    Один мутирующий запрос — Postgres гарантирует атомарность без явной транзакции.

    **Phase 2c — send в TrustMe (вне Tx, network).** Отправка в TrustMe `SendToSignBase64FileExt` с
    `additionalInfo = contract.id`. Из ответа достаём `trustme_document_id`, `trustme_short_url`.

    Сетевые вызовы могут быть медленными — connection pool не страдает, никакой Tx не висит.

    **Phase 3 — finalize (Tx_finalize, миллисекунды).** Один `dbutil.WithTx` callback с двумя отдельными запросами:
    1. **UPDATE contracts** — записать результат TrustMe (unsigned PDF уже в БД с Phase 2b):
       ```sql
       UPDATE contracts
       SET trustme_document_id = $1, trustme_short_url = $2
       WHERE id = $3
       ```
    2. **INSERT audit_logs** — `action='campaign_creator.contract_initiated'` с `actor_id = NULL` (system-actor) — в той
       же транзакции (стандарт `backend-transactions.md`: audit обязан жить ВНУТРИ Tx с mutate-операцией).

    **После COMMIT (вне Tx callback)** — бот-уведомление креатору «договор отправлен на подпись» (по стандарту: бот /
    success-логи fire-and-forget живут ПОСЛЕ Tx).

    **Failure scenarios:**
    - Phase 2a упала (Gotenberg): contract row остаётся `unsigned_pdf_content IS NULL`, `trustme_document_id IS NULL`.
      Phase 0 next tick → re-render + persist + send.
    - Phase 2b упала (БД): редкий кейс. На next tick Phase 0 → orphan без PDF → re-render + persist + send.
    - Phase 2c упала (TrustMe): contract row имеет `unsigned_pdf_content`, но `trustme_document_id IS NULL`. Phase 0 next
      tick → search/Contracts. Если TrustMe знает (мы успели отправить, упало по timeout до получения ответа) →
      finalize. Если не знает → re-send (PDF уже в памяти из row).
    - Phase 3 упала (БД после успешного send): contract row имеет `unsigned_pdf_content`, `trustme_document_id IS NULL`.
      Phase 0 next tick → search/Contracts найдёт known document → finalize.

    **⚠ Риск Phase 0 recovery — фильтр `search/Contracts` по `additionalInfo`.** Метод `POST /search/Contracts`
    существует в blueprint (стр. 912), но структура `searchData[]` для фильтра по `additionalInfo` в blueprint
    размыта: описано как «поиск по ключевым словам», точные `fieldName`/`fieldValue` не задокументированы. **Нужна
    проверка против sandbox** на ранней стадии chunk 16. Если фильтр по `additionalInfo` не работает — план B на MVP:
    оптимистично перепосылать orphan'ов с тем же `additionalInfo`, принять риск дублей на TrustMe-side (объёмы низкие
    ~100 на EFW, очистка дубля через ЛК TrustMe — runbook). План B зафиксировать в спеке chunk 16 после теста sandbox.

    Бесконечный retry без backoff (объёмы низкие — на EFW ~100 договоров за всю кампанию). `SKIP LOCKED` защищает от
    race при HA / blue-green. Никаких промежуточных статусов `signing_pending`/`signing_failed` — `agreed` без
    `contract_id` = «в outbox»; `signing` с `trustme_document_id IS NULL` = «в recovery».
    Graceful shutdown — встроенно в существующий `cl.Add("cron", scheduler.Stop)`: при SIGTERM ждёт текущий batch.
    Risk дубля СМС если упадём между ответом TrustMe и COMMIT Phase 3 — задокументирован, runbook «отозвать дубль через
    ЛК TrustMe» отдельно. Пример паттерна batch-обработки — `backend/cmd/scripts/notify-pending-creators`.

11. **Источник контента договора — `campaigns.contract_template_md TEXT NOT NULL DEFAULT ''`.** Полный текст договора
    **с встроенным ТЗ под конкретную кампанию**, в одном поле. Это **шаблон с Go-template-плейсхолдерами**:
    `{{.CreatorFIO}}`, `{{.CreatorIIN}}`, `{{.CreatorPhone}}`, `{{.ContractNumber}}`, `{{.IssuedDate}}` (точный список —
    `ContractData` в Decision #13). Бэк подставляет значения `(campaign, creator)` и шлёт в Gotenberg. Имя/детали
    кампании и реквизиты бренда — **статика внутри** `contract_template_md`, не плейсхолдеры. Два источника правды
    (ТЗ-хардкод в TMA-приложении + контракт в БД) — **accepted divergence на pilot EFW**, отрефакторить пост-EFW
    (tech-debt). Поле редактируется
    админом через расширенную форму CRUD кампаний (`/campaigns/new` + edit) — большое `<textarea>`. **Defense in depth —
    три уровня валидации:**
    - (form) `domain.Validate*Campaign*` — trim + non-empty check, возвращает `CodeContractRequired` 422 при
      пустом/whitespace-only.
    - (notify) `POST /campaigns/{id}/notify` (chunk 12, уже в main) — guard первой строкой service-метода:
      `contract_template_md = ''` → 422 `CodeContractRequired`. Защищает legacy-кампании, созданные до миграции (
      DEFAULT '').
    - (outbox) `ContractSenderService.RunOnce` — ряд с пустым контрактом пропускается + warning-log. Третий слой защиты
      на случай race / direct-DB-edit / создания кампании в обход формы.

    Миграция `campaigns ADD COLUMN contract_template_md TEXT NOT NULL DEFAULT ''` + расширение OpenAPI (поле
    `contractTemplateMd` в Create/Update/Response) + form-update в `frontend/web/src/features/campaigns/` + notify-guard
    в `internal/service/campaign_creator.go` (или где живёт `NotifyCampaignCreators` — chunk 12 уже в main, дополняем
    существующий handler) — **всё в составе chunk 16**, отдельным pre-flight-PR не делаем.

    **Блокировка edit'а после первой отправки.** `PATCH /campaigns/{id}` с изменением `contract_template_md`
    отклоняется (422 `CodeContractTemplateLocked`) если у этой кампании есть хоть один `campaign_creator` со статусом
    из `('signing','signed','signing_declined')`. Иначе — коллизия версий: одни креаторы подписали v1 контракта, другие
    подпишут v2. Проверка в `CampaignService.Update` — выполняется только если в payload поле `contract_template_md`
    реально пришло (опциональное обновление). Имена остальных полей кампании остаются редактируемыми всегда.

12. **Движок шаблонизации PDF — двухстадийный на бэке.**
    Pipeline: `contract_template_md` (MD с Go-template) → `text/template` подставляет переменные `(campaign, creator)` →
    `github.com/yuin/goldmark` (MD → HTML) → оборачиваем в минимальный `<html><head><meta charset="utf-8"></head><body>...</body></html>` (inline в Go-коде, без отдельного CSS-файла) → multipart на Gotenberg chromium-route → PDF bytes.
    Аидана редактирует MD (привычный формат). Templating-логика у нас → unit-тестируется без Gotenberg-контейнера.
    `text/template` (а не `html/template`) — потому что после рендера ещё goldmark делает MD→HTML с авто-escape; двойной
    escape от `html/template` поломает Markdown-синтаксис. XSS-вектор закрывается на стадии goldmark.
    Сервис — `ContractPDFRenderer` в `internal/contract/`: метод
    `Render(ctx, template string, data ContractData) ([]byte, error)`. Spy-Gotenberg-клиент —
    `internal/gotenberg/spy_client.go` по образцу `internal/telegram/spy_store.go`. `goldmark` добавляется в
    `docs/standards/backend-libraries.md` registry как канонная библиотека для MD→HTML.

13. **`ContractData` — типизированная структура переменных шаблона.**
    ```go
    type ContractData struct {
        // Креатор
        CreatorFIO   string  // склейка last_name + " " + first_name + " " + middle_name (без NULL middle_name)
        CreatorIIN   string
        CreatorPhone string  // нормализованный E.164 (без пробелов/тире/скобок)

        // Договор
        ContractNumber string  // "EFW-{shortCreatorID}-{shortCampaignID}" — первые 8 hex-символов UUID каждой стороны
        IssuedDate     string  // "ДД.ММ.ГГГГ" — дата выпуска (отправки в TrustMe), таймзона Asia/Almaty
    }
    ```
    Поле называется `IssuedDate`, а не `SignedDate` — на момент рендера PDF договор ещё не подписан (креатор подпишет
    через минуты/часы/дни). Таймзона — фиксированная `Asia/Almaty` (пользователи и юр-сторона в KZ; UTC в шапке
    договора будет читаться как ошибка).

    Реквизиты Models Production, имя/детали кампании, текст ТЗ — **статика внутри `contract_template_md`** (Аидана зашивает
    их в шаблон при редактировании каждой кампании). Имя/идентификатор кампании в БД нужны UI и audit, не для PDF — поэтому
    `CampaignName`/`CampaignID` в `ContractData` не передаём. Адрес, город, дата рождения креатора — **НЕ в шаблоне**:
    адресы у нас не собираются (поля в `creators` просто нет), дата рождения избыточна для коммерческого договора, город
    кампании EFW зашивается статикой в текст ТЗ.

    **Phone normalization — единый source of truth = domain helper `domain.NormalizePhoneE164(string) string`** в
    `internal/domain/phone.go`. Вызывается **один раз** при сборке `ContractData` (в outbox-worker'е перед рендером).
    `TrustMeClient.SendToSign` потребляет `Requisites.PhoneNumber` уже нормализованным — берёт из `data.CreatorPhone`,
    повторно не нормализует. Так PDF и TrustMe-метаданные используют одно и то же значение, без двух разных
    «нормализованных» строк.

14. **Webhook idempotency — по `(trustme_document_id, target_status_code)`.** Webhook-handler делает
    `UPDATE contracts SET trustme_status_code = $new WHERE trustme_document_id = $id AND trustme_status_code != $new RETURNING ...`.
    Если 0 рядов затронуто — повтор того же события, no-op (ни state-transition, ни audit, ни бот-уведомление). 1 ряд
    затронут — обрабатываем дальше. Это и есть idempotency без отдельной идентификации события (TrustMe-payload не несёт
    event-ID). Откат состояний (например, webhook прилетит status=2 после status=3) — игнорируем через
    `WHERE trustme_status_code IN (terminal_or_lower)` clause; finalize'нные `signed/signing_declined` не возвращаются
    назад.

15. **Soft-deleted кампании исключаются из outbox.** SQL `SELECT FOR UPDATE SKIP LOCKED` в
    `ContractSenderService.RunOnce` — JOIN с `campaigns` + `WHERE campaigns.is_deleted = false`. Если админ soft-удалил
    кампанию между `agreed` и тиком worker'а, договор не отправляется; ряд `campaign_creators` остаётся `agreed`
    навсегда (tombstone). Аналогично — webhook на `campaign_creators`, чья кампания soft-deleted: state-update делаем (
    TrustMe-договор уже подписан, factual record), бот-уведомление пропускаем + audit-warning.

16. **PII вне stdout-логов (security.md hard rule).** Логируем UUID'ы (`creator_id`, `campaign_id`,
    `trustme_document_id`), HTTP-метаданные, status codes. **НЕ логируем** ФИО, ИИН, телефон, адрес,
    `contract_template_md`. В `audit_logs` (БД) — допустимо по проектному правилу `Audit vs stdout-логи`. В
    `error.Message` (response body) — анти-fingerprinting: одинаковый текст для разных причин отказа, без утечки
    внутренних данных.

17. **Spy-паттерн для TrustMe и Gotenberg — по образцу `internal/telegram`.** Три класса в каждом интеграционном
    пакете: (a) `Client` interface — методы реальной интеграции; (b) `RealClient` — HTTP-клиент против настоящего
    TrustMe/Gotenberg; (c) `SpyOnlyClient` (только пишет в `SentSpyStore`, не ходит в сеть; используется при `*_MOCK=true`)
    и `TeeClient` (пишет в spy + дёргает `RealClient`; используется при `EnableTestEndpoints=true && *_MOCK=false` —
    staging mode «реально шлём + e2e может проверить»).

    `internal/trustme/spy_store.go`: `SentRecord{TrustMeDocumentID, ContractData, AdditionalInfo, SentAt, Err}` +
    `RegisterFailNext(docID, reason)` для имитации 5xx + `RegisterCallback(docID, status)` для имитации
    разных webhook-исходов на e2e-страховке. Capacity по образцу telegram (5000 ring).

    `internal/gotenberg/spy_store.go`: `RenderRecord{TemplateMD, ContractData, RenderedHTML, RenderedAt, Err}` +
    `RegisterFailNext(reason)` для имитации сбоя рендера.

    Test-API endpoints для spy-управления — рядом с существующими в `internal/testapi/` (по образцу
    `/test/telegram/spy-*`). Доступны только при `EnableTestEndpoints=true`.

## Данные на креатора в БД (для запроса в TrustMe)

Источник — таблица `creators` (миграция `20260505052212_creators.sql`):

| Поле в TrustMe (обязательное) | Источник у нас                                                            | Заметка                                                                                                                                                           |
|-------------------------------|---------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Requisites[i].FIO`           | `last_name + ' ' + first_name + ' ' + middle_name` (если NULL — без него) | as-is                                                                                                                                                             |
| `Requisites[i].IIN_BIN`       | `creators.iin`                                                            | уже валиден `^[0-9]{12}$` (CHECK на `creator_applications.iin` + `creators_iin_unique`)                                                                           |
| `Requisites[i].PhoneNumber`   | `creators.phone`                                                          | формат E.164. **Нормализуется при сборке `ContractData`** через `domain.NormalizePhoneE164` (стрипаем пробелы/тире/скобки — TrustMe их запрещает). `TrustMeClient` берёт уже нормализованное `data.CreatorPhone`. БД хранит phone как ввёл пользователь (`maxLength:32` без regex) |
| `Requisites[i].CompanyName`   | литерал «Креатор» (или ФИО)                                               | необязательно, для ЛК TrustMe                                                                                                                                     |

Email **не нужен** — TrustMe для физлица-подписанта работает по СМС / ЭЦП.

## TrustMe API — что используем

Из `docs/external/trustme/blueprint.apib`. Базовый URL — `https://test.trustme.kz/trust_contract_public_apis/` (prod-host получим, когда TrustMe выдадут production-токен; на pilot работаем против test-окружения). Auth — статичный токен в header `Authorization: <token>` (без `Bearer`-префикса). Лимит — 4 req/sec.

| Метод                                                                       | Назначение                                            | Когда вызываем                                                                                                |
|-----------------------------------------------------------------------------|-------------------------------------------------------|---------------------------------------------------------------------------------------------------------------|
| `POST /SendToSignBase64FileExt/{file_extension}` (где `file_extension=pdf`) | Загрузка PDF договора                                 | outbox-worker Phase 2 (после claim ряда `campaign_creators` в `signing`)                                      |
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

Из blueprint: webhook стреляет при **каждом изменении статуса** документа. Наша конфигурация — auto-sign на стороне
компании настроен «после клиента».

**Happy path (3 шага, 2 входящих webhook'а):**

1. **Phase 2 outbox** → `SendToSignBase64FileExt`. В ответе TrustMe возвращает `document_id` со статусом 0. Webhook
   здесь не стреляет (первичная загрузка, не изменение); если и стрельнёт — `WHERE trustme_status_code != $new` в
   handler'е (см. Decision #14) сделает no-op.
2. **Креатор подписал по СМС-OTP** → статус 0 → **2**. Прилетает webhook `{status: 2, client: <phone креатора>}`.
   Реакция: `UPDATE contracts SET trustme_status_code = 2 WHERE trustme_document_id = $1 AND trustme_status_code != 2`.
   `campaign_creators.status` остаётся `signing` (intermediate). **Бот не стреляет** — креатор и так знает, что
   подписал; не шумим.
3. **Auto-sign** на нашей стороне триггерится TrustMe'ом → статус 2 → **3**. Прилетает webhook
   `{status: 3, client: <phone компании>}`. Реакция (всё в одной Tx, см. Decision #14): `UPDATE contracts SET
   trustme_status_code = 3, signed_at = now()` + `UPDATE campaign_creators SET status = 'signed'` + audit
   `campaign_creator.contract_signed`. **После Tx (вне callback'а)**: `GET /doc/DownloadContractFile/{document_id}` →
   `UPDATE contracts SET signed_pdf_content = $1` + бот креатору «договор подписан обеими сторонами».

   Между webhook'ами 2 и 3 — задержка на TrustMe-side обработку auto-sign'а (секунды-минуты). Хэндлер идемпотентен,
   повторные webhook'и со status=3 = no-op.

**Decline path (1 webhook):**

- Креатор нажимает «Отказаться» в TrustMe → статус 0 → **9**. Auto-sign не триггерится. Прилетает webhook
  `{status: 9}`. Реакция: `UPDATE contracts SET trustme_status_code = 9, declined_at = now()` + `UPDATE
  campaign_creators SET status = 'signing_declined'` + audit `campaign_creator.contract_signing_declined`. Бот
  креатору в декларации decline-flow — открытая развилка спеки chunk 17.

**Прочие статусы (1, 4, 5, 6, 7, 8)** — поведение зафиксировано в Decision #7: `contracts.trustme_status_code`
обновляем (audit-trail), `campaign_creators.status` не трогаем, audit + warning-log.

**Скачивание signed PDF — после COMMIT, не внутри webhook Tx.** TrustMe `DownloadContractFile` — медленный сетевой
вызов; если он упадёт, webhook-handler уже за пределами Tx → 200 в TrustMe вернули, статус в БД зафиксирован, PDF
загружается отдельным retry (recovery-job или admin-кнопка). Тот же принцип «network вне Tx», что и в outbox-worker'е.

## Открытые развилки

Все крупные структурные развилки закрыты решениями 1–14. Что остаётся для финализации в спеке (chunk 16/17 spec через
`bmad-quick-dev`):

- Точные имена констант ошибок (`CodeContractRequired` и т.п.) и их HTTP-маппинг.
- Точные тексты бот-уведомлений (один-два варианта на каждое событие).
- Структура папок: `internal/contract/`, `internal/trustme/`, `internal/gotenberg/` — детали в спеке.
- Test-API эндпоинт для force-trigger `ContractSenderService.RunOnce` (e2e не должны ждать 10 сек).
- Конфиг-переменные в `internal/config/config.go`: `TrustMeBaseURL`, `TrustMeToken`, `TrustMeWebhookToken`,
  `GotenbergURL` + соответствующие `*Mock` для test-окружения.
- OpenAPI-схема для `POST /trustme/webhook` payload (есть в blueprint, переписать в openapi.yaml).

## Нарезка на 2 PR-чанка

|                 | **Chunk 16 — отправка**                                                                                                                                                                                                                                                                      | **Chunk 17 — приём**                                                                                                                                      |
|-----------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------|
| Миграции        | `contracts` table; `campaigns ADD COLUMN contract_template_md`; `campaign_creators ADD COLUMN contract_id` + ALTER state-machine `agreed→signing`                                                                                                                                            | none                                                                                                                                                      |
| Compose         | Gotenberg sidecar                                                                                                                                                                                                                                                                            | none                                                                                                                                                      |
| State-machine   | `agreed → signing`                                                                                                                                                                                                                                                                           | `signing → signed / signing_declined`                                                                                                                     |
| Сервисы         | `ContractSenderService.RunOnce` (outbox-worker), `ContractPDFRenderer` (Gotenberg-клиент), `TrustMeClient.SendToSign(...)`; notify-guard в существующем `NotifyCampaignCreators` (chunk 12)                                                                                                  | `TrustMeWebhookService.HandleEvent(payload)` + dispatcher по `subject_kind`, `TrustMeClient.DownloadContract(docID)`                                      |
| OpenAPI         | поле `contractTemplateMd` в Create/Update/Response Campaign; `CodeContractRequired` в errors-enum                                                                                                                                                                                            | новый `POST /trustme/webhook`                                                                                                                             |
| Frontend        | `<textarea>` поля контракта в форме `/campaigns/new` + `/campaigns/{id}/edit`, валидация notEmpty                                                                                                                                                                                            | none                                                                                                                                                      |
| Endpoints (бэк) | none (cron triggers внутри)                                                                                                                                                                                                                                                                  | `POST /trustme/webhook` (public, bearer-auth, idempotent по `(trustme_document_id, status_code)`)                                                         |
| Audit           | `campaign_creator.contract_initiated`                                                                                                                                                                                                                                                        | `campaign_creator.contract_signed`, `…contract_signing_declined`, `…contract_unexpected_status`                                                           |
| Бот креатору    | «договор отправлен на подпись»                                                                                                                                                                                                                                                               | «договор подписан обеими сторонами»                                                                                                                       |
| Тесты           | unit ≥80% per-method на каждый сервис + domain-валидация (form/notify/outbox); integration outbox-flow со spy-TrustMe + spy-Gotenberg; race-test `go test -race` на parallel `RunOnce`; TrustMe-down resiliency; пустой `contract_template_md` → 422 form, 422 notify, skip+warning в outbox; **lock-edit test** (`PATCH /campaigns/{id}` с новым `contract_template_md` когда есть `campaign_creator.status IN ('signing','signed','signing_declined')` → 422 `CodeContractTemplateLocked`); **persist-then-send test** (Phase 2c упала после Phase 2b → ряд имеет `unsigned_pdf_content`, recovery шлёт без re-render) | unit на `HandleEvent` для каждого статуса 0..9; integration на endpoint + idempotency-test; edge cases: 401, 404, 422, неожиданный статус → audit-warning |

**Frontend-индикатор** статуса контракта в карточке `campaign_creator` — встраиваем как mini-PR в `chunk 15` (расширение
страницы кампании со статусами и счётчиками), не отдельный chunk.

**Pre-flight** (вне Группы 7, отдельные мини-PR'ы или runbook): SetHook-регистрация, `/companies` health-check, ротация
TrustMe-токена.

## Ссылки

- Roadmap: `_bmad-output/planning-artifacts/campaign-roadmap.md`, Группа 7 (chunks 16–17).
- Design Групп 4–6 (chunks 10–15): `_bmad-output/planning-artifacts/design-campaign-creator-flow.md`.
- Юр-handoff: `legal-documents/КОНТЕКСТ И СТАТУС РАБОТЫ НАД ДОГОВОРАМИ.md`, `legal-documents/ПРИНЯТЫЕ РЕШЕНИЯ.md`.
- TrustMe blueprint: `docs/external/trustme/blueprint.apib`.
- TrustMe Postman: `docs/external/trustme/postman-collection.json`.
- Стандарты: `docs/standards/`.
