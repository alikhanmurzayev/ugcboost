---
title: 'Бот-фундамент notify-сервис + welcome и verification-approved'
type: feature
created: '2026-05-03'
status: done
baseline_commit: e2b275db882d586ac9c420db6ff25c39a8f39222
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/creator-onboarding-roadmap.md
  - _bmad-output/planning-artifacts/creator-verification-concept.md
---

> Перед реализацией обязательно полностью загрузить все файлы `docs/standards/` — это hard rules, спека не дублирует их.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Chunk 9 roadmap'а. Roadmap зафиксировал: бот = единственный канал коммуникации с креатором, TMA выпилен из онбординга. Сейчас единственное место, где бот пишет креатору с инструкцией, — chunk 8 placeholder (`telegram.SendVerificationNotification`) с WebApp-кнопкой на TMA. Welcome-сообщения после `/start`-link нет — креатор привязал TG и слышит короткий `MessageLinkSuccess` без инструкции по верификации. ~100 живых заявок в `verification` ждут pipeline'а. Дополнительный риск: текущий `TestTelegramLink` ассертит welcome через `/test/telegram/message` (per-call spy внутри handler'а) — после миграции welcome в async-notify такой ассерт ничего не покрывает; нужно явно перевести welcome-ассерт на `/test/telegram/sent`, иначе e2e будет green при сломанном реальном поведении.

**Approach:** В `internal/telegram` появляется типизированный `Notifier` с методами по событиям (заменяет free-function `SendVerificationNotification`). Notifier инкапсулирует fire-and-forget goroutine, timeout и WaitGroup; сервисы зовут `notifier.NotifyApplicationLinked(...)` / `NotifyVerificationApproved(...)` после commit'а WithTx. Welcome шлётся из `CreatorApplicationTelegramService.LinkTelegram` (заменяет immediate `MessageLinkSuccess` от handler'а): подробный текст с verification-кодом и Direct-ссылкой если в заявке указан IG, иначе — нейтральное «Спасибо за заявку! Обрабатываем» без раскрытия внутренней механики верификации. Verification-approved упрощается до текста без WebApp-кнопки и универсален (применим и к auto-IG, и к будущему manual-verify). `TMAPublicURL` удаляется из конфига и из конструктора `CreatorApplicationService`. E2E мигрируется: welcome (success-ветки) ассертится через `/test/telegram/sent` со spy-assertion на конкретные подстроки (`UGC-` + `ig.me/m/ugc_boost` ↔ IG-вариант / `Спасибо за заявку` без `UGC-` ↔ no-IG); error-ветки (already-linked-другим, fallback, not-found) остаются на `/test/telegram/message` reply.

## Boundaries & Constraints

**Always:**
- `Notifier` — concrete struct в `internal/telegram` (interfaces определяет потребитель в service per `accept interfaces, return structs`). Конструктор `NewNotifier(sender Sender, wg *sync.WaitGroup, log logger.Logger) *Notifier`. Каждый публичный метод сразу спавнит fire-and-forget goroutine с `context.WithoutCancel(ctx) + WithTimeout(telegramNotifyTimeout)` (10s, существующая константа), регистрирует в WG; ошибки SendMessage логирует Error без проброса. Sync-возврата нет.
- Методы Notifier: `NotifyApplicationLinked(ctx, chatID int64, p ApplicationLinkedPayload)` и `NotifyVerificationApproved(ctx, chatID int64)`. `ApplicationLinkedPayload`: `VerificationCode string`, `HasInstagram bool`.
- `CreatorApplicationService` и `CreatorApplicationTelegramService` объявляют у себя интерфейсы потребителя (`creatorAppNotifier` / `creatorAppTelegramNotifier`) с только нужными методами. `Sender`, `*sync.WaitGroup`, `tmaPublicURL` уезжают из их конструкторов целиком.
- `CreatorApplicationTelegramService.LinkTelegram` после commit'а WithTx (новой ветки или idempotent-success): вычитывает соцсети заявки через repo (`socialRepo.ListByApplicationID`, расширение интерфейса `CreatorApplicationTelegramRepoFactory` если нужно), собирает `ApplicationLinkedPayload{verification_code, hasIG}`, зовёт `notifier.NotifyApplicationLinked(ctx, telegramUserID, payload)`. Idempotent re-link (тот же telegram_user_id) тоже триггерит welcome — пользователь жмёт /start ещё раз, шлём ещё раз.
- `telegram.Handler.Handle` на success-ветке `LinkTelegram` больше **не** делает `h.reply(...)` — welcome уходит async через notifier. Error-ветки (`AlreadyLinked` другим, fallback, `ApplicationNotFound`) — без изменений: handler reply сразу с константной строкой.
- `CreatorApplicationService.notifyVerificationApproved` становится тонкой обёрткой: nil-чек на telegram_user_id (warn-лог + return), иначе `s.notifier.NotifyVerificationApproved(ctx, *telegramUserID)`. Внутреннее go/WG/timeout/WithoutCancel переезжает внутрь Notifier.
- Конфиг: поле `TMAPublicURL` удаляется из `Config`; env `TMA_PUBLIC_URL` уходит из `.env.example` / `.env.ci`. Никаких ребрендов «на будущее».
- E2E test-endpoint `/test/telegram/sent` (openapi-test.yaml + handler) контракт не меняем; после refactor'а поле `WebAppUrl` становится `nil` для всех новых сообщений.
- **Тексты сообщений** (parse_mode=HTML, в коде хранятся как Go raw-string templates; код заявки подставляется через `fmt.Sprintf` в `<pre>%s</pre>`; экранирование `<`/`>`/`&` в payload-полях не требуется — креатор-input в текст не подмешивается). Точные строки фиксированы intent'ом:

  Welcome — IG в заявке (`<pre>%s</pre>` подставляется реальный `UGC-NNNNNN`):
  ```
  Здравствуйте! 👋

  Получили вашу заявку.
  Чтобы подтвердить Instagram, нужна одна минута:

  1. Скопируйте код (тап по блоку):

     <pre>UGC-NNNNNN</pre>

  2. Откройте Direct и отправьте его нам:

     https://ig.me/m/ugc_boost

  Напишу сюда, как только проверим.
  ```

  Welcome — IG не указан:
  ```
  Здравствуйте! 👋

  Спасибо за заявку! Обрабатываем.

  Напишу сюда, как только будет готово.
  ```

  Verification approved (заменяет chunk 8 placeholder, без inline-keyboard):
  ```
  Заявка ушла на модерацию ✅

  Напишу сюда, как только модератор примет решение.
  ```
- Покрытие: unit на `Notifier` (captured params, WG drain после `defer Done`, timeout, error-branch); расширение unit'ов `LinkTelegram` (mock notifier — assert payload в new-link, idempotent-link, NOT-call в already-linked-другим / not-found / fallback); существующий unit `VerifyInstagramByCode` переходит с mocked Sender на mocked notifier.
- **E2E test-mode contract** (фиксируется godoc'ом в обоих spec-файлах + Design Notes): `/test/telegram/sent` ловит то, что бэк попытался отправить (params, ChatID, ReplyMarkup) — не факт доставки. В TeeSender-режиме (`mock=false` + EnableTestEndpoints) реальный `bot.SendMessage` зовётся, на синтетических chat_id всегда падает, `Err` пишется в spy и **намеренно не ассертится** (tests verify outbound params, not delivery). Доставка проверяется только manual-smoke'ом с реальным TG-аккаунтом против staging-бота.
- E2E расширения: `backend/e2e/telegram/telegram_test.go` — на success-ветках `TestTelegramLink` welcome ассертится через `/test/telegram/sent` (spy-assertion на text); error-ветки (already-linked-другим, fallback, not-found) ассертят reply через `/test/telegram/message`. Сценарии в spy: с-IG → text содержит подстроку `UGC-` (с реальным кодом из заявки) **и** URL `https://ig.me/m/ugc_boost`; без-IG → text содержит подстроку `Спасибо за заявку`, не содержит `UGC-` и `ig.me`; idempotent re-link → 2 идентичных welcome-записи; already-linked-другим / fallback / not-found → 0 записей в spy. `backend/e2e/webhooks/sendpulse_instagram_test.go` — happy-path меняет ассерт `WebAppUrl != nil` на `WebAppUrl == nil` + content-assert: text содержит `модерацию`, не содержит `tma`/`mini`/`webapp`/`ig.me`/`UGC-`.

**Ask First:**
- (нет — все intent-вопросы по текстам, IG-handle, формату закрыты в interview-фазе перед approve и зафиксированы в Always)

**Never:**
- In-memory queue / persistent retry / outbox для notify — fire-and-forget по дизайну, ровно как chunk 8.
- Per-event interfaces в `telegram`-пакете — потребительские интерфейсы только в service-пакетах.
- Сохранение `TMAPublicURL` под другим именем «на возврат TMA» — удаляем целиком.
- Inline-кнопки на любом сообщении этого чанка (включая verification-approved) — простой текст.
- Изменения в state-machine, audit-action vocabulary, существующих миграциях.
- Добавление новых конфиг-vars на этом чанке.
- **Ассерт `Err == ""` в spy-записях** — TeeSender-режим против синтетических chat_id всегда даёт реальный Telegram-error, это by design; e2e ловит outbound params, не факт доставки.
- Welcome-ассерт через `/test/telegram/message` reply на success-ветках — после refactor'а welcome уходит async через notifier, в reply-массиве per-call spy его нет; пытаться ловить там welcome = false-green test.

## I/O & Edge-Case Matrix

| Сценарий | Input / State | Expected Behavior | Error Handling |
|---|---|---|---|
| /start link с IG | заявка `verification`, social platform=`instagram` | INSERT link + audit (как chunk 1); `/test/telegram/message` reply пустой; `/test/telegram/sent` за `since` — ровно 1 запись (chatID == TG user_id), text содержит подстроку `UGC-` (с реальным кодом из заявки) **и** URL `https://ig.me/m/ugc_boost`, `WebAppUrl == nil` | LinkTelegram error до commit → notify не зовётся, handler sync-reply error-string |
| /start link без IG | заявка `verification`, only TikTok / Threads | INSERT link + audit; spy: 1 запись welcome, text содержит подстроку `Спасибо за заявку`, **не** содержит подстроку `UGC-` и **не** содержит URL `ig.me`, `WebAppUrl == nil` | то же |
| Idempotent re-link | тот же telegram_user_id повторно жмёт /start | no INSERT, no audit; notify зовётся → 2 идентичных welcome-записи в spy за `since` | то же |
| Already-linked другим | другой telegram_user_id | LinkTelegram → `CodeTelegramApplicationAlreadyLinked`; handler sync-reply `MessageApplicationAlreadyLinked` | notify не зовётся, **0 новых записей в `/test/telegram/sent`** |
| /start без payload или с unknown UUID | bot.Message без или с не-UUID/несуществующим | handler sync-reply `MessageFallback` / `MessageApplicationNotFound` | notify не зовётся, 0 новых записей в spy |
| IG-webhook успех | chunk 8 path | verification-approved: 1 spy-запись, text содержит подстроки `модерацию` и `модератор`, **не** содержит `tma`/`mini`/`webapp`/`ig.me`/`UGC-`, `WebAppUrl == nil` | sender error → log Error, не пробрасывается |
| IG-webhook успех на нелинкованной заявке | telegram_user_id == nil | nil-чек в `service.notifyVerificationApproved` → warn-лог, notify не зовётся, 0 новых записей в spy | N/A |
| SIGTERM с pending notify | closer-фаза | существующий `registerNotifyWaiter` ждёт WG до `telegramNotifyTimeout` | hard-cap timeout, как сейчас |

</frozen-after-approval>

## Code Map

- `backend/internal/telegram/notifier.go` (NEW) — `Notifier`, `ApplicationLinkedPayload`, методы `NotifyApplicationLinked` / `NotifyVerificationApproved`. Внутри — `fire(ctx, op, chatID, fn)` helper c WG/timeout/WithoutCancel/Error-лог.
- `backend/internal/telegram/messages.go` — удалить `MessageLinkSuccess`, `MessageVerificationApproved`, `MessageVerificationApprovedButton`. Новые тексты живут в `notifier.go` рядом с потребителем.
- `backend/internal/telegram/notify.go` — удалить файл.
- `backend/internal/telegram/handler.go` — на ветке `nil`-error от `LinkTelegram` убрать `h.reply(...MessageLinkSuccess)` (welcome уходит через notifier).
- `backend/internal/telegram/notifier_test.go` (NEW) — captured-params на `bot.SendMessageParams.Text` / `ChatID`; WG.Done после fire; timeout; sender-error → log без panic.
- `backend/internal/service/creator_application_telegram.go` — конструктор: убрать `Sender`/`WaitGroup`, добавить `notifier creatorAppTelegramNotifier`. После commit'а WithTx — `socialRepo.ListByApplicationID` (если нужно расширить `CreatorApplicationTelegramRepoFactory`), сборка payload, зов notifier. Идемпотентная ветка — тот же зов.
- `backend/internal/service/creator_application_telegram_test.go` — расширение: mock notifier; ассерт payload (with-IG / no-IG / idempotent-recall / not-called в already-linked-другим / not-found).
- `backend/internal/service/creator_application.go` — конструктор: убрать `Sender`/`WaitGroup`/`tmaPublicURL`, добавить `notifier creatorAppNotifier`. `notifyVerificationApproved` сводится к nil-чеку + delegate. Удалить импорт пакета `telegram` если становится лишним.
- `backend/internal/service/creator_application_test.go` — заменить mocked Sender на mocked notifier. Удалить ассерты на WebAppURL и tmaPublicURL.
- `backend/internal/config/config.go` — удалить поле `TMAPublicURL`.
- `backend/.env.example`, `backend/.env.ci` — удалить `TMA_PUBLIC_URL`.
- `backend/cmd/api/telegram.go` — `setupTelegram` теперь возвращает `*Notifier` (вместо Sender + NotifyWG); `Spy` всё ещё возвращается отдельно для test-endpoint. Notifier собирает Sender внутрь себя.
- `backend/cmd/api/main.go` — `creatorApplicationSvc := service.NewCreatorApplicationService(pool, repoFactory, tgRig.Notifier, appLogger)`; то же для `creatorApplicationTelegramSvc`. Регистрация `registerNotifyWaiter` в closer'е остаётся, но теперь WG живёт внутри Notifier — экспонируем через `tgRig.Notifier.Wait` или поле.
- `backend/e2e/telegram/telegram_test.go` — happy-path link: spy-assert на текст с/без `UGC-`. Idempotent re-link: 2 spy-записи (oba welcome). Already-linked-другим: 0 новых записей.
- `backend/e2e/webhooks/sendpulse_instagram_test.go` — happy-path: `require.Nil(t, sent[0].WebAppUrl)`, ассерт текст без `tma` / `mini` / `webapp`.
- `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` — chunk 9: `[ ]` → `[~]` сейчас, при merge → `[x]`.

## Tasks & Acceptance

**Execution:**
- [x] `internal/telegram/notifier.go` — `Notifier`, payload, методы, RU-тексты-константы, fire-helper с WG/timeout/WithoutCancel.
- [x] `internal/telegram/notify.go` — удалить.
- [x] `internal/telegram/messages.go` — удалить устаревшие константы.
- [x] `internal/telegram/handler.go` — убрать `reply` на success-ветке link.
- [x] `internal/service/creator_application_telegram.go` — конструктор + post-commit notify с payload (расширить RepoFactory если нужно).
- [x] `internal/service/creator_application.go` — конструктор + delegate-notifyVerificationApproved.
- [x] `internal/config/config.go` + `.env.example` + `.env.ci` — удалить `TMA_PUBLIC_URL`.
- [x] `cmd/api/telegram.go` + `cmd/api/main.go` — wire Notifier; экспозиция `Wait` для closer'а.
- [x] `make generate-mocks` — мок Notifier'а, мок consumer-интерфейсов.
- [x] Unit-тесты `notifier_test.go`, расширение тестов `creator_application_telegram_test.go` и `creator_application_test.go`.
- [x] E2E расширения `telegram/telegram_test.go` и `webhooks/sendpulse_instagram_test.go`.
- [x] Roadmap chunk 9: `[~]` сейчас, при merge → `[x]`.

**Acceptance Criteria:**
- Given full make-gate, when `make build-backend lint-backend test-unit-backend-coverage test-e2e-backend generate-api generate-mocks` запущен, then всё зелёное, per-method coverage ≥80% на новых файлах.
- Given grep на пакет `telegram` из service-файлов, when проверка после рефакторинга, then только импорт типов notifier через consumer-interface — никаких прямых `telegram.Sender` / `tmaPublicURL` / `*sync.WaitGroup` в service-конструкторах.
- Given env `TMA_PUBLIC_URL=anything`, when `Config.Load()`, then значение игнорируется (поля нет в struct'е).
- Given SIGTERM в момент активной notify-горутины, when closer фазит, then Notifier.Wait отрабатывает в существующем `registerNotifyWaiter` без изменения наблюдаемого порядка graceful shutdown (regression).
- Given handler `/test/telegram/sent` контракт, when E2E старого вызова через generated client, then формат response не изменился (поле `WebAppUrl` есть, теперь `nil` для всех новых сообщений).

## Design Notes

**Тексты сообщений** — финал в Boundaries → Always (frozen). Tone, IG-handle, format (HTML + plain URL + `<pre>` для code-block) утверждены через interview-фазу перед approve.

**Notifier-форма (golden):**
```go
type Notifier struct {
    sender  Sender
    wg      *sync.WaitGroup
    timeout time.Duration
    log     logger.Logger
}

type ApplicationLinkedPayload struct {
    VerificationCode string
    HasInstagram     bool
}

func (n *Notifier) NotifyApplicationLinked(ctx context.Context, chatID int64, p ApplicationLinkedPayload) {
    n.fire(ctx, "application_linked", chatID, func(c context.Context) error {
        return n.send(c, chatID, buildWelcomeText(p))
    })
}
```

**Решение про TMAPublicURL:** удаляем целиком. Возврат TMA в будущем — отдельная инициатива, она перевведёт env-var в правильной форме.

**Решение про idempotent re-link:** notify зовём всегда на nil-error от LinkTelegram. Реальные кейсы: пользователь потерял чат, перезалил Telegram, переустановил клиент — повторный /start должен вернуть welcome с актуальным кодом. Telegram сам rate-limit'ит спам.

**E2E test-mode contract** (фиксируется godoc'ом обоих spec-файлов):
- `/test/telegram/message` (POST, synthetic Update) — ловит **синхронные handler-reply'и** через per-call SpyOnlySender, **глобальный Sender обходится**. Используется только для error-веток после refactor'а.
- `/test/telegram/sent` (GET) — читает глобальный SentSpyStore, populated SpyOnlySender'ом (mock=true) или TeeSender'ом (mock=false + EnableTestEndpoints). Ловит **fire-and-forget notify** (welcome, verification-approved). Под TeeSender'ом реальный bot.SendMessage **зовётся** и на синтетических chat_id всегда падает; spy записывает params + Err. **`Err` намеренно не ассертим** — тесты verify outbound params, не факт доставки. Доставка проверяется только manual-smoke'ом против staging-бота с реального TG-аккаунта Алихана.

## Verification

**Commands:**
- `make generate-api generate-mocks` — codegen + моки актуальные.
- `make build-backend lint-backend` — 0 issues.
- `make test-unit-backend-coverage` — per-method ≥80%.
- `make test-e2e-backend` — telegram + webhooks specs зелёные.

**Manual smoke (после реализации, перед PR):**
- `make compose-up && make migrate-up && make start-backend`.
- Подать тестовую заявку через лендос с указанным Instagram → /start в боте → пришло welcome с `UGC-NNNNNN` и упоминанием IG.
- Подать тестовую заявку без IG → /start → welcome «Спасибо за заявку! Обрабатываем», без `UGC-` и без Direct-ссылки.
- Прогнать `curl POST /webhooks/sendpulse/instagram` с правильным payload → пришло verification-approved сообщение, без inline-кнопки.

## Spec Change Log

### 2026-05-03 — step-04 review patches (post-implementation)


- **`NewNotifier` сигнатура — внутреннее противоречие frozen-блока.** Boundaries → Always line 26 фиксирует `NewNotifier(sender Sender, wg *sync.WaitGroup, log logger.Logger) *Notifier`, но Always line 31 говорит «Внутреннее go/WG/timeout/WithoutCancel переезжает внутрь Notifier», а Code Map line 114 — «WG живёт внутри Notifier — экспонируем через `tgRig.Notifier.Wait`». Реализация выбрала вариант "Notifier owns WG" (line 31, line 114) → `NewNotifier(sender Sender, log logger.Logger) *Notifier` + `Notifier.Wait()`. Acceptance auditor зафиксировал это как spec-gap. Замораживать внешнюю инжекцию WG смысла нет — Wait через Notifier чище. Если человек хочет другое прочтение — открыть отдельный PR с переключением.
- **`Info` → `Warn` для skip-notify-on-no-link.** Реализация деплоилась с `s.logger.Info`, спека дважды требует «warn-лог» (Always line 30, I/O Matrix line 96). Патч приведён к Warn, тест переехал в выделенный `t.Run` "verified path skips notify and warns when application not linked".
- **`fire()` recover.** Комментарий `NewNotifier` обещал `defer recover()`, фактического кода не было. Патч добавил recover + Error-log с panic+stack. Покрыто новым unit-тестом `TestNotifier_FireAndForget/panic in sender is recovered and logged, Wait still drains`.
- **HTML escape `VerificationCode`.** Defensive: текущий генератор UGC-NNNNNN безопасен, но `html.EscapeString` поставлен в `buildWelcomeText` для защиты от будущих изменений формата.
- **`ParseMode=HTML` только для IG-варианта.** No-IG текст plain — стрэй `&` или `<` в будущей правке копии не уронит парсер на Telegram-стороне.
- **e2e `time.Sleep(300ms)` → `EnsureNoNewTelegramSent`.** Polling-helper в `testutil/telegram_sent.go` фиксит фаззи sleep в негативной ассерции «intruder welcome не отправлен».
- **t.Run order в `creator_application_telegram_test.go`** приведён к `backend-testing-unit.md` § Нейминг (ранние выходы → idempotent → happy → edge happy).
- **`captureSend` hang protection.** WaitGroup → buffered chan + `waitFor` с 2s deadline; double-call SendMessage теперь падает явной ошибкой вместо panic.

## Suggested Review Order

**Notifier (новая поверхность)**

- Notifier-структура, методы по событиям, fire-helper с recover + WG/timeout/WithoutCancel.
  `backend/internal/telegram/notifier.go:65`
- Шаблоны сообщений (HTML с `<pre>` для кода, plain для verification-approved) — точные строки, согласованные с пользователем.
  `backend/internal/telegram/notifier.go:26`
- Unit-тесты Notifier'а: captured params, panic recovery, WithoutCancel, WG.Wait drain.
  `backend/internal/telegram/notifier_test.go:56`

**Service-слой: сервисы переезжают на consumer-side notifier**

- `CreatorApplicationTelegramService.LinkTelegram` — post-commit notify с payload, idempotent re-link тоже триггерит welcome, IG-флаг читается из той же tx.
  `backend/internal/service/creator_application_telegram.go:76`
- `creatorAppNotifier` consumer-interface + `notifyVerificationApproved` стал тонким delegate'ом с warn-логом на nil-link.
  `backend/internal/service/creator_application.go:55`

**Telegram-handler: убран sync-reply на success-link**

- Welcome теперь async через notifier — handler shellит только error-ветки.
  `backend/internal/telegram/handler.go:95`
- Handler-tests подтверждают: на success-link spy не получает SendMessage.
  `backend/internal/telegram/handler_test.go:155`

**Wiring + cleanup**

- `setupTelegram` собирает Notifier с внутренним Sender; `Spy` всё ещё эксортится для test-endpoint.
  `backend/cmd/api/telegram.go:31`
- `main.go`: оба сервиса получают `tgRig.Notifier`; `registerNotifyWaiter` — на `Notifier.Wait()`.
  `backend/cmd/api/main.go:93`
- `Config.TMAPublicURL` удалён, `.env.example`/`.env.ci` без `TMA_PUBLIC_URL`.
  `backend/internal/config/config.go:62`

**E2E: welcome через `/test/telegram/sent`, не reply**

- Success-link e2e: проверяет welcome через spy с конкретными подстроками (`UGC-`/`ig.me` для IG, «Спасибо за заявку» для no-IG); `LinkTelegramToApplication` дренирует welcome чтобы не ловить его в чужие assertion'ы.
  `backend/e2e/telegram/telegram_test.go:138`
- Webhook e2e: `WebAppUrl == nil` + проверка отсутствия `tma`/`mini`/`webapp`/`ig.me`/`UGC-` через `assertVerificationApprovedShape`.
  `backend/e2e/webhooks/sendpulse_instagram_test.go:359`
- `EnsureNoNewTelegramSent` — polling-helper для негативных ассертов «notify не должен фигачить» вместо `time.Sleep`.
  `backend/e2e/testutil/telegram_sent.go:64`

**Удалённое + обновлённое**

- `internal/telegram/notify.go` + `notify_test.go` — удалены целиком (заменены Notifier'ом).
- `messages.go` без `MessageLinkSuccess` / `MessageVerificationApproved*` — sync-replies сужены до error-веток.
  `backend/internal/telegram/messages.go:6`
- Roadmap chunk 9 в `[~]`.
  `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md:59`
