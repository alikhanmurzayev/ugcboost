---
title: "Intent: TrustMe — CompanyName=ФИО + KzBmg env-flag (prod-trial)"
type: intent
status: draft
created: "2026-05-09"
parent_intent: _bmad-output/implementation-artifacts/archive/2026-05-09-intent-trustme-contract-v2.md
---

# Intent: TrustMe — CompanyName=ФИО + KzBmg env-flag (prod-trial)

## Преамбула — стандарты обязательны

Перед любой строкой production-кода агент обязан полностью загрузить все файлы `docs/standards/` (через `/standards`). Применимы все. Особенно: `backend-architecture.md`, `backend-design.md`, `backend-libraries.md`, `backend-testing-unit.md`, `naming.md`, `security.md`, `review-checklist.md`. Каждое правило — hard rule; отклонение = finding.

## Тезис

Два независимых слайса в один мини-PR на отдельной ветке (НЕ в `alikhan/chunk-17-trustme-webhook` — там идёт работа другого агента):

- **A — UX-фикс**: `Requisites[].CompanyName` в TrustMe-payload поменять с литерала `"Креатор"` на полное ФИО (тот же `composeFIO`, что уже собирает `Requisite.FIO` и `ContractData.CreatorFIO`). В ЛК TrustMe сторона договора будет отображаться как «Иванов Иван Иванович», а не как «Креатор».
- **B — KzBmg env-flag для prod-trial**: добавить env-переменную `TRUSTME_KZ_BMG` (default `false`) и пробрасывать её в `sendToSignDetails.KzBmg`. После merge & deploy выставить на prod в `true` и отправить договор тестовому креатору с заведомо левой связкой ИИН↔Phone — посмотреть, что вернёт TrustMe (фича «База мобильных граждан» в их API требует отдельного подключения).

## Скоуп

**В скоупе:**

- (A) Удалить константу `trustMeCreatorCompanyName` в `internal/contract/sender_service.go:36-37`. В обоих местах формирования `[]trustme.Requisite` (Phase 2c — `processClaim`:308-313; Phase 0 orphan resend — `resendOrphan`:205-210) `CompanyName ← composeFIO(...)` — тот же вызов, что уже стоит на следующей строке для `FIO`. Перенести вычисление в локальную переменную (`fio := composeFIO(...)`), чтобы не звать дважды.
- (B) `TrustMeKzBmg bool ` в `internal/config/config.go` (env `TRUSTME_KZ_BMG`, `envDefault:"false"`, без `required`).
- (B) Поле `kzBmg bool` в `RealClient`; расширенный конструктор `NewRealClient(baseURL, token string, kzBmg bool, httpClient *http.Client)`; в `SendToSign` (`real_client.go:105-109`) `sendToSignDetails.KzBmg ← c.kzBmg`.
- (B) Wiring в `cmd/api/trustme.go:36` — `trustme.NewRealClient(cfg.TrustMeBaseURL, cfg.TrustMeToken, cfg.TrustMeKzBmg, nil)`.
- Unit-тесты на оба слайса (см. § Тесты).

**Out of scope:**

- `FaceId` (биометрическая верификация подписанта на стороне TrustMe) — отдельный трек. Поле в `sendToSignDetails` уже есть, но мы его не трогаем в этом PR.
- Своя валидация связки ИИН↔ФИО↔Phone через госреестр (если prod-trial покажет, что `KzBmg` платный/недоступен) — backlog после результата trial'а.
- Backward-compat для legacy-договоров с `CompanyName="Креатор"` — это display-only поле в ЛК TrustMe, на наши БД не влияет, перезатирать ничего не нужно.
- Расширение `SendToSignInput` под per-request `KzBmg` — глобальный config-флаг достаточен (применяется ко всем договорам). Если позже понадобится включать выборочно (например, только для непроверенных креаторов) — отдельный intent.
- Изменения в `SpyOnlyClient` / `spy.SentRecord` — spy наблюдает Requisites через свои поля (`FIO`/`IIN`/`Phone`), `CompanyName` и `KzBmg` не логирует. Это ок: проверка идёт через unit-mock на уровне `sender_service` / `real_client`.

## Принятые решения

1. **(A) `CompanyName ← composeFIO(...)` без отдельного поля в `SendToSignInput`.**

   Источник тот же, что у `FIO` (`creator.last_name + first_name + middle_name`). Заводить отдельный field в `SendToSignInput` нет смысла — это удвоит маппинг и создаст риск рассинхрона. Вычисление `composeFIO` поднимается в локальную переменную перед литералом `Requisite{...}`, чтобы не звать функцию дважды.

   Parent intent v2 (Decision #13, archived) допускал «литерал Креатор (или ФИО)» — этот intent явно ходит по второй ветке (UX-причина: в ЛК TrustMe видна сторона как абстрактный «Креатор», без понятного human-readable имени).

2. **(B) `KzBmg` хранится в `RealClient` (Variant 1), не в `SendToSignInput`.**

   `KzBmg` — глобальный config-флаг для всех TrustMe-вызовов одного instance'а, симметрично с уже хранящимися `baseURL`/`token`/`limiter`. Хранить per-request в `SendToSignInput` — лишняя гибкость без use-case'а; если завтра понадобится per-creator selective enabling, легко рефакторим (single-call site).

   Кладём в trustme-package (а не в sender_service) потому что это специфика wire-format'а TrustMe API, не бизнес-логика worker'а.

3. **(B) Constructor signature расширяется (`NewRealClient(baseURL, token, kzBmg, httpClient)`), без `Set*`-метода.**

   Per `backend-design.md`: «Структура иммутабельна после создания — никаких проверок на nil при использовании зависимости. Методы `Set*()` для зависимостей запрещены». Этот же принцип применяется к конфиг-параметрам.

4. **(B) Default `TRUSTME_KZ_BMG=false`, без `required`.**

   Добавление новой env-переменной не должно ломать запуск local/staging/CI окружений. На staging — без эффекта (`TrustMeMock=true` → SpyOnly не использует `KzBmg`). На prod — изначально `false` (текущее поведение), переключаем вручную через Dokploy env override после deploy.

5. **(B) Prod-trial — runbook, не часть кода.**

   После merge & deploy:
   1. В Dokploy production выставить `TRUSTME_KZ_BMG=true`.
   2. Перезапустить контейнер backend (env vars читаются через `caarlos0/env` при старте).
   3. Отправить договор тестовому креатору с **заведомо невалидной связкой**: например, реальный ИИН + чужой телефон, или ФИО не совпадающее с паспортом ИИН. Триггер — обычный flow: campaign_creator с `agreement_status='agreed'` → outbox-worker подхватит на следующем тике (10s).
   4. Наблюдать `contracts.last_error_message` / spy-логи / TrustMe ЛК. Возможные исходы:
      - HTTP 200 + `status="Ok"` → фича не активна или TrustMe её игнорирует молча. Откатить флаг, открыть тикет с TrustMe.
      - HTTP 200 + `status="Error"` + `errorText` про «требует подключения / нет тарифа» → ожидаемый исход для unsubscribed аккаунта. Откатить, открыть тикет с TrustMe на подключение.
      - HTTP 200 + `status="Error"` + `errorText` про несовпадение связки → желаемый исход, фича работает. Держим флаг `true`, закрываем gap в безопасности.
   5. После trial'а — обновить этот intent до `status: complete` с фактическим исходом, либо открыть followup (например, своя валидация через GBDFL/EDS, если KzBmg недоступен).

6. **PR-стратегия — отдельная ветка от main.**

   Текущая ветка `alikhan/chunk-17-trustme-webhook` занята webhook-receiver'ом другого агента — не вмешиваемся. Берём базу от `main` после merge chunk 17 (либо параллельно, если конфликтов в `config.go` / `cmd/api/main.go` нет — проверим в spec'е). Slug intent'а — `trustme-companyname-kzbmg`.

## Тесты

**Unit:**

- `internal/contract/sender_service_test.go` — расширить captured-input ассерты в уже существующих happy-path тестах `TestContractSenderService_processClaim` / `TestContractSenderService_resendOrphan` (или как они называются — финализируется в spec). Через `mock.Run(...)` на `trustMeClient.EXPECT().SendToSign(...)` достать `SendToSignInput.Requisites[0]` и ассертить `CompanyName == composeFIO(creator.LastName, creator.FirstName, creator.MiddleName)`. Никаких новых `t.Run` не создаём — расширяем существующие.
- `internal/trustme/real_client_test.go` — два сценария:
  - `t.Run("kzbmg true", ...)` — `NewRealClient(.., true, ..)` → отправка → перехват HTTP-запроса через `httptest.Server` (уже используется в файле) → парсинг multipart body → парсинг `details` JSON → `require.Equal(true, details.KzBmg)`.
  - `t.Run("kzbmg false", ...)` — то же с `false`.
- `internal/config/config_test.go` (если есть, иначе пропускаем — тривиальный env-маппинг).

**E2E:** не требуется. Слайс A — display-only в ЛК TrustMe (наша БД не меняется), слайс B — runbook prod-trial. Существующие e2e от chunk 16 продолжают проходить (`spy.SentRecord` не наблюдает `CompanyName` и `KzBmg`).

**Coverage gate:** `make test-unit-backend-coverage` — публичные/приватные методы в затронутых пакетах остаются ≥80% (мы не добавляем новых функций, только расширяем существующие тесты).

## Code Map

**Создаём:** (нет новых файлов).

**Изменяем:**

- `backend/internal/contract/sender_service.go` — удалить константу `trustMeCreatorCompanyName` (строки 36-37); в `resendOrphan` (200-211) и `processClaim` (303-314) поднять `fio := composeFIO(...)` в локальную переменную, использовать в обоих полях `Requisite.CompanyName` и `Requisite.FIO`.
- `backend/internal/contract/sender_service_test.go` — captured-input ассерт на `CompanyName` (см. § Тесты).
- `backend/internal/config/config.go` — `TrustMeKzBmg bool `env:"TRUSTME_KZ_BMG" envDefault:"false"`` рядом с `TrustMeToken` (строка 67).
- `backend/internal/trustme/real_client.go` — поле `kzBmg bool` в `RealClient` struct (после `limiter`); конструктор `NewRealClient(baseURL, token string, kzBmg bool, httpClient *http.Client)` — параметр перед `httpClient`, так логичнее (config → io); в `SendToSign` (строка 105) `sendToSignDetails.KzBmg: c.kzBmg`.
- `backend/internal/trustme/real_client_test.go` — два теста на сериализацию `KzBmg` (см. § Тесты).
- `backend/cmd/api/trustme.go:36` — `trustme.NewRealClient(cfg.TrustMeBaseURL, cfg.TrustMeToken, cfg.TrustMeKzBmg, nil)`.

**Reference (читать, не править):**

- `_bmad-output/implementation-artifacts/archive/2026-05-09-intent-trustme-contract-v2.md` Decision #13 — рассуждение про CompanyName литерал vs ФИО.
- `docs/external/trustme/blueprint.apib` строки 158-162, 287-291, 414-417, 644-648 — спецификация `KzBmg` в разных endpoint'ах TrustMe API.
- `backend/internal/trustme/spy/` — для понимания, какие поля spy наблюдает (CompanyName и KzBmg в spy не отражаются — это ОК, см. Decision #4 / Out of scope).

## Миграции — НЕТ

Никакие колонки/CHECK/индексы не добавляются. Это чисто code-side изменение + новая env-переменная.

## Открытые развилки

Финализируются в spec через bmad-quick-dev:

- (B) Точное имя env: `TRUSTME_KZ_BMG` (default) или `TRUSTME_KZBMG`? Первый предпочтительнее по env-conventions (snake_case с разделителями); blueprint TrustMe пишет `KzBmg`, но это camelCase JSON-key, не env name.
- (A) Положение `fio := composeFIO(...)` локальной переменной — в начале блока `processClaim`/`resendOrphan` или прямо перед литералом `Requisite{}`? Default — прямо перед литералом, минимальная области видимости.
- Параллельные агенты могут конфликтовать с этим PR на:
  - `config.go` (chunk 17 добавляет `TrustMeWebhookToken` ~рядом) — резолв тривиальный.
  - `sender_service.go` — chunk 17 файл не трогает (он работает в `webhook_service.go`), конфликта быть не должно. Проверить на старте spec'а.
  - `cmd/api/main.go` / `cmd/api/trustme.go` — chunk 17 трогает `main.go` (webhook handler wiring), `trustme.go` не трогает. Конфликт маловероятен.

## Ссылки

- Parent intent (архитектурный контекст TrustMe-интеграции): `_bmad-output/implementation-artifacts/archive/2026-05-09-intent-trustme-contract-v2.md`.
- TrustMe blueprint: `docs/external/trustme/blueprint.apib`.
- Стандарты: `docs/standards/`.
- Memory: `project_trustme_no_sandbox` — у TrustMe нет sandbox, реальный API только из prod, на staging — SpyOnly.
