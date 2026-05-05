---
title: "Roadmap: онбординг креатора до approved"
type: roadmap
status: living
created: "2026-04-29"
updated: "2026-05-04"
revisions:
  - "2026-05-02: добавлен chunk 5 — POST /creators/applications/counts для бейджа в админке; нумерация 5–11 сдвинута на 6–12"
  - "2026-05-02: chunk 5 в работе — GET /creators/applications/counts (массив пар, sparse) + API hygiene (dedup enums/responses, BrandInput, DictionaryItem, pagination через PaginationInput, camelCase query везде)"
  - "2026-05-02: добавлен chunk 6.5 — Playwright e2e на admin verification flow (отделён от chunk 6 чтобы не блокировать UI и писать тесты на стабилизированном интерфейсе)"
  - "2026-05-02: chunk 6 готов — PR #49 смержен. Включает ревью-фиксы: единый sidebar, фильтр telegramLinked, колонка Telegram, копирование bot-link сообщения для not-linked"
  - "2026-05-02: chunk 6.5 готов — PR #50 смержен. Включает PR-fix-итерацию: codegen для frontend/e2e/types/, fail-fast cleanup с per-call timeout, crypto.randomBytes для IIN/TG-id, middleName=null/empty покрытие, drawer testid'ы на ФИО/birth-date/iin/city/categories вместо getByText"
  - "2026-05-02: бывшие chunks 7–8 переработаны на основе концепции верификации (`creator-verification-concept.md`) — расщеплены на 9 более мелких чанков 7–15 (бэк first, потом фронт-админка, потом TMA); reject и withdraw вынесены из бывшего chunk 10 в отдельные ранние чанки; прежние 9–12 сдвинуты на 16–19"
  - "2026-05-02: reject/withdraw перенесены ЗА верификационный flow в TMA — приоритет: сначала довести верификацию целиком (7 → 12), потом отказы/отзывы. Бывший 12 (verify+reject в drawer) расщеплён: verify-часть стала chunk 10, reject-часть — chunk 14. Нумерация 10–19 → 10–20."
  - "2026-05-03: chunk 8 готов, выкачен в прод (PR #52). TMA целиком выпилен из онбординг-флоу — креатор-канал = только Telegram-бот (текстовые сообщения + inline-кнопки). frontend/tma/ и прототип в web остаются как future-work, не удаляются. tma-auth-foundation.md удалён. Договор / TrustMe / подпись и withdraw как UI-фича уходят из этого roadmap'а: онбординг кончается на approved, договор — за ним в campaign-roadmap. State-machine v2: терминалы только approved + rejected; withdrawn остаётся как зарезервированный терминал без объявленных переходов; уходят awaiting_contract / contract_sent / signed. approved окончательный, обратного перехода нет. Ad-hoc broadcast существующим ~100 prod-заявкам (приветствие + код) — локальный скрипт вне репо, не chunk. Нумерация после реструктуризации: 9–21."
  - "2026-05-04: chunk 12 готов (PR #58, бэк reject). В группе 3 swap'нуты chunks 13 ↔ 14: новый chunk 13 — Telegram-уведомление о rejected (приоритет: чтобы admin-action из chunk 12 сразу давал креатору сигнал), новый chunk 14 — фронт-кнопка reject в drawer (теперь работает на полностью готовом backend + notify flow). Chunk 16 (экран модерации) обновлён — переиспользует обработчик reject из chunk 14."
  - "2026-05-04: chunk 14 готов (PR #60, фронт reject в drawer). Outlined-red trigger в новом sticky-footer'е drawer'а, conditional confirm-modal по telegramLink, единый apiError-banner для verify/reject через DrawerContext, exhaustive-switch container ApplicationActions готов под chunk 19/16. Спека архивирована — `_bmad-output/implementation-artifacts/archive/2026-05-04-spec-creator-application-reject-frontend.md`. Группа 3 (Reject) закрыта целиком."
---

# Roadmap: онбординг креатора до approved

Living document. Покрывает путь от подачи заявки на лендинге до перехода заявки в `approved`. Договор и сотрудничество по конкретной кампании — за границей этого roadmap'а, в отдельном campaign-roadmap. Обновляется по ходу реализации.

## Как использовать

- Документ — карта маршрута, не story-spec. Уровень — chunk, не экран и не сущность
- Перед стартом каждого chunk — отдельный `/bmad-quick-dev` в `_bmad-output/implementation-artifacts/`, ссылку добавляем сюда
- Отмечаем прогресс чек-боксами: `[ ]` → `[~]` (в работе) → `[x]` (готово, в main)
- Перед любой реализацией агент обязан полностью загрузить `docs/standards/`

## Архитектурные принципы

- **Telegram-бот — единственный канал коммуникации с креатором.** Никаких клиентских интерфейсов: ни TMA, ни мобильного приложения. Все сообщения, инструкции и уведомления — через бот (текстом и inline-кнопками)
- **Креатор-side public REST не появляется.** Все мутации, инициированные креатором, идут через telegram-handler'ы → внутренние service-методы. Identity креатора = `telegram_user_id` из бот-update'а
- **Бот-модуль изолирован.** Серверная часть (приём updates, привязка `/start`-payload, отправка уведомлений) живёт в `internal/telegram/` отдельным модулем с чёткой границей
- **Тон коммуникации с креатором** — определяется в момент реализации соответствующих чанков. Сообщения чёткие, не раскрывают внутренние процессы, но дают понять что происходит и что от креатора ждут

## Темп процесса

До недели, реалистично — день-два до `approved`. Пушим креатора на быструю верификацию через бот. Не закладываем длинные таймеры там, где можно дёрнуть напоминанием.

## Нелинейность

Возможны отклонения с фидбэком (`rejected`) и повторные заявки. Креатор узнаёт об актуальном статусе заявки через сообщения бота на каждом значимом переходе.

## Путь заявки

- [x] **0. Форма заявки на лендинге.** Уже реализовано — заявка сохраняется на бэке
- [x] **1. Привязка Telegram-аккаунта к заявке.** Кнопка на лендинге → автозапуск бота → автовыполнение `/start` со скрытым ID заявки в payload → бот ловит, привязывает Telegram-пользователя к заявке → открытие TMA с экраном статуса. Реализация — PR #35 (минимальный бот на go-telegram/bot), PR #38 (привязка через `/start`-payload + audit), review-fixes — `_bmad-output/implementation-artifacts/archive/spec-review-fixes-telegram-link.md`
- [x] **2. Спроектировать стейт-машину заявки.** Жизненный цикл заявки от подачи до подписания договора, легальные переходы, представление для креатора. Источник истины для БД, бэка, админки и TMA. Документ — `_bmad-output/planning-artifacts/creator-application-state-machine.md` (будет переработан в chunk 17 — state-machine v2)
- [x] **3. Реализация целевой стейт-машины.** Один PR. Миграция: обновить значения существующего enum статусов под целевую модель (см. state-machine doc) + бэкфил существующих заявок `pending` → `verification` + перестроить partial unique index по ИИН на новых активных значениях. Код: domain, repository, openapi, ручка приёма заявок с лендинга (`POST /creators/applications`) теперь пишет `verification`. Unit + e2e тесты под новые статусы. Без таблицы истории переходов и сервиса переходов — появятся с первым реальным переходом. PR #46, спека — `_bmad-output/implementation-artifacts/spec-creator-application-state-machine.md`
- [x] **4. Админка-бэк: ручка списка заявок.** `POST /creators/applications/list` (admin-only, POST из-за PII в search) с фильтрами (status/cities/categories arrays, dateFrom/To, ageFrom/To, telegramLinked, search), сортировкой (created_at|updated_at|full_name|birth_date|city_name × asc|desc) и пагинацией. Item-shape лёгкий (без phone/address/consents) — расширяется новыми optional полями для будущих чанков. PR #47, спека — `_bmad-output/implementation-artifacts/archive/2026-05-02-spec-creator-applications-list.md`
- [x] **5. Админка-бэк: счётчики заявок по статусам.** `GET /creators/applications/counts` (admin-only) — один SQL `GROUP BY status` возвращает массив пар `{status, count}`, sparse (статусы без рядов отсутствуют — фронт лукапит `?? 0`). Без фильтров и пагинации; фронт сам решает какие статусы суммировать в бейдж нотификаций (по умолчанию — `verification + moderation + awaiting_contract`). Read-only, без audit-log. Кеш не закладываем сразу — добавим, если поллинг станет частым. В одном PR с зачисткой openapi-дублей (shared enums/responses/parameters, BrandInput, DictionaryItem, pagination через PaginationInput, camelCase query везде). Спека — `_bmad-output/implementation-artifacts/spec-creator-applications-counts.md`
- [x] **6. Админка-фронт: список + карточка заявки на верификации.** Перенос из прототипа Айданы (`frontend/web/src/_prototype/`) в реальный `features/creatorApplications/`. Один экран — список заявок со статусом `verification` + карточка-drawer. Подключение к реальному API + RoleGuard(ADMIN). Без действий модерации. Бейдж нотификаций в sidebar/header читает counts из chunk 5. Покрытие: unit-тесты + ручной прогон полного flow через Playwright MCP (лендос → подача → админка → таблица → drawer). E2E Playwright — отдельный chunk 6.5. PR #49, спека архивирована — `_bmad-output/implementation-artifacts/archive/2026-05-02-spec-admin-creator-applications-verification.md`. Включает ревью-фиксы: единый sidebar (DashboardLayout поглотил AdminLayout), фильтр `telegramLinked` (URL + UI segmented), колонка Telegram в таблице, кнопка «Скопировать сообщение» в drawer для not-linked заявок (бэк отдаёт `telegramBotUrl` в detail).
- [x] **6.5. Browser e2e на admin verification flow.** Playwright spec в `frontend/e2e/web/admin-creator-applications-verification.spec.ts` — 6 тестов (happy path / creator without middleName / drawer prev-next / filter telegramLinked / empty state / RoleGuard). Расширен `frontend/e2e/helpers/api.ts` с derive'ами из generated `frontend/e2e/types/{schema,test-schema}.ts`, `seedAdmin/seedBrandManager/seedCreatorApplication/linkTelegramToApplication`, `uniqueIIN`/`uniqueTelegramUserId` через `crypto.randomBytes` (зеркалит `backend/e2e/testutil`). PR #50 смержен. Спека — `_bmad-output/implementation-artifacts/archive/2026-05-02-spec-admin-creator-applications-verification-e2e.md`. Включает PR-review-фиксы: codegen для e2e, fail-fast cleanup, drawer testid'ы (`drawer-full-name`, `drawer-iin`, `drawer-birth-date`, `drawer-city`, `drawer-category-{code}`, `drawer-category-other-text`), restored content-asserts на login-error/dashboard/sidebar.
- [x] **7. Бэк-фундамент: storage верификации + verification-код у заявки.** Миграция: поля верификации на соцсетях заявки (`verified` / `method` / `verified_by_user_id` / `verified_at`) + verification-код у заявки + бэкфил кодов для существующих 20 prod-заявок. Domain-типы. Расширение admin-detail ручки заявки — выводит поля верификации соцсетей (без кода). Без action-endpoint'ов. Фундамент для 8 (auto-verify) и 10 (manual verify). Концепция — `_bmad-output/planning-artifacts/creator-verification-concept.md` (будет обновлена в chunk 17). Спека — `_bmad-output/implementation-artifacts/spec-creator-verification-foundation.md`
- [x] **8. Бэк: webhook от SendPulse для auto-верификации Instagram.** `POST /webhooks/sendpulse/instagram` с bearer-secret middleware (constant-time). Парсинг `UGC-NNNNNN` → lookup заявки в `verification` → проверка IG-социалки → self-fix handle на mismatch → UPDATE social + applyTransition (`verification → moderation`) + audit + transition row в одной WithTx → fire-and-forget Telegram notify (placeholder, переделывается в chunk 9) после commit. Создаёт `creator_application_status_transitions` (без бэкфила) + миграцию нормализации существующих IG handle'ов. Зависит от 7. PR #52 смержен, выкачено в прод 2026-05-03. Спека — `_bmad-output/implementation-artifacts/archive/2026-05-03-spec-creator-verification-instagram-webhook.md`. Раунд PR-ревью-фиксов (verificationCode в admin detail, `TELEGRAM_MOCK` switch с config-валидацией, outbound TG observability через `SpyOnlySender`/`TeeSender` + ring-buffer spy-store, WaitGroup drain notify-горутин в closer'е, `cmd/api/telegram.go` для чистоты `main.go`, e2e через generated client) — спека `_bmad-output/implementation-artifacts/archive/2026-05-03-spec-creator-verification-pr52-fixes.md`

### Группа 1. Pipeline под live prod (приоритет — ~100 заявок в `verification`)

- [~] **9. Бот-фундамент notify-сервис + два первых сообщения.** Notify-сервис в `internal/telegram` с типизированными методами по событиям. Первое сообщение — приветственное после `/start`-link заявки в `verification`: суть — креатор сразу понимает, что он на этапе верификации, и получает инструкцию (с кодом, если IG указан; без кода и с другой формулировкой — что заявка пойдёт на ручную верификацию админом, если IG не указан). Второе сообщение — успешное прохождение IG-верификации (заявка ушла на модерацию). Дёргается после commit, fire-and-forget (паттерн как в chunk 8). Заменяет placeholder из chunk 8. Тексты сообщений — определяются при реализации
- (out of scope) Существующим ~100 prod-заявкам в `verification`, у которых не было приветствия и кода: Алихан прогонит локальный скрипт с прод-токеном бота и ID-шниками, выгруженными из прод-БД. Скрипт не коммитится в репо. Креаторам без link к боту — пишем ручками в WhatsApp по номеру телефона, чтобы перешли в бот; дальше pipeline подхватит автоматически

### Группа 2. Ручная верификация

- [x] **10. Бэк: manual verify соцсети админом.** Admin-endpoint, действие из карточки заявки. Помечает конкретную соцсеть как `manual`-верифицированную → авто-переход `verification → moderation`, если это первая верификация. Audit + state-history. Зависит от 7
- [x] **11. Фронт-админка: action verify в drawer карточки.** Кнопка «Подтвердить вручную» с выбором соцсети. На текущем экране `verification` (chunk 6). Зависит от 10. E2E — расширение spec'а из 6.5 или новый рядом

### Группа 3. Reject

- [x] **12. Бэк: reject заявки админом.** Admin-endpoint, доступен на `verification` и `moderation`. Body пустой (тело уведомления для креатора захардкожено в chunk 13). Перевод в `rejected`. Audit + state-history. Переходы добавляются в текущую state-machine, без миграций. PR #58, спека архивирована — `_bmad-output/implementation-artifacts/archive/2026-05-04-spec-creator-application-reject.md`
- [x] **13. Бот: уведомление о rejected.** Расширение существующего `*telegram.Notifier` (chunk 8) — после commit'а reject-перехода fire-and-forget Telegram-сообщение. Текст статичный, вшит одной константой, итерируется отдельным PR'ом. Lookup chat_id через `creator_application_telegram_link`; при отсутствии link — warn в логе сервиса, без fallback-каналов. Зависит от 12. PR #59, спека архивирована — `_bmad-output/implementation-artifacts/archive/2026-05-04-spec-creator-application-reject-notify.md`
- [x] **14. Фронт-админка: action reject в drawer на verification-экране.** Кнопка «Отклонить заявку» с подтверждением (без feedback-поля — body endpoint'а пустой, текст для креатора статичный из chunk 13). Зависит от 12 и 13. E2E — отдельный spec рядом с 6.5. PR #60, спека архивирована — `_bmad-output/implementation-artifacts/archive/2026-05-04-spec-creator-application-reject-frontend.md`

### Группа 4. Админка-экран модерации

Сейчас в админке есть только экран `verification` (chunk 6). После manual/auto verify заявка уходит в `moderation` и пропадает из видимости — для approve и reject модератор должен видеть отдельный список. Прототип Айданы содержит макет — переносим в реальный фронт.

- [ ] **15. (Опционально) Бэк: list/detail-ручка enhancement для модерации.** Аудит того, что уже отдаётся в item-shape (chunk 4) и detail (chunk 7) — достаточно ли модератору для принятия решения. Возможные расширения: verified-поля соцсетей в list-item, что-то по аудитории/линк-агрегатам. Если ничего не нужно — пустой PR не делаем, чанк закрываем как N/A
- [x] **16. Фронт-админка: экран списка заявок на модерации + drawer.** Перенос из прототипа Айданы в реальный `features/creatorApplications/`. По аналогии с chunk 6: список заявок в `moderation` + карточка-drawer + RoleGuard(ADMIN) + бейдж в sidebar. Action reject — переиспользует обработчик из chunk 14, ставится сразу в этом drawer'е. Action approve добавлена как disabled placeholder с tooltip «Скоро» (по фидбеку — чтобы сразу видеть финальный layout footer'а; реальная реализация — chunk 19). PR #61 смержен. Спека — `_bmad-output/implementation-artifacts/archive/2026-05-04-spec-admin-creator-applications-moderation.md`. Включает 5 фидбек-фиксов: live badge через refetchInterval, label фильтра «Период подачи заявки», скрытие telegram-фильтра на moderation, колонки (добавлена «Город», убрана «Telegram», переименование «В этапе» = updatedAt), approve placeholder
- [x] **16.5. Browser e2e на admin moderation flow + reject action.** Два Playwright spec'а в `frontend/e2e/web/`: `admin-creator-applications-moderation.spec.ts` (15 shape-тестов: drawer-поля с verified-бейджами IG-auto и TT-manual, точные ассерты ячеек таблицы, sort cycle с детерминизмом по updated_at, все 5 фильтров через UI, скрытие telegram-фильтра, sidebar live badge, RoleGuard) + `admin-creator-applications-moderation-reject-action.spec.ts` (3 reject-сценария — с TG / без TG / cancel, fromStatus=moderation). Расширен `frontend/e2e/helpers/api.ts` с `manualVerifyApplicationSocial` для TT-only пути в moderation. CI fix: `SENDPULSE_WEBHOOK_SECRET` пробрасывается во frontend e2e jobs (раньше передавался только в backend e2e). PR #63. Спека — `_bmad-output/implementation-artifacts/spec-admin-creator-applications-moderation-e2e.md`

### Группа 5. Approve

- [ ] **17. State-machine v2.** Один чанк: design-doc (`creator-application-state-machine.md` + `creator-verification-concept.md`) и forward-миграция. Drop из enum значений `awaiting_contract` / `contract_sent` / `signed`. Add `approved`. `withdrawn` оставляем как зарезервированный терминал без объявленных переходов (на будущее). Active-set partial unique index по IIN = `{verification, moderation}`. Обновить domain-константы / `creatorApplicationAllowedTransitions` / unit / e2e
- [ ] **18. Бэк: approve заявки админом.** Admin-endpoint, переход `moderation → approved`. В одной `WithTx` — переход статуса + создание row в `users` с `role=creator`, привязанной к этой заявке + audit + state-history. `approved` — окончательный, обратного перехода нет. Зависит от 17
- [ ] **19. Фронт-админка: action approve в drawer на moderation-экране.** Кнопка «Одобрить заявку» с подтверждением. Зависит от 16 и 18. E2E — расширение spec'а из 16.5 или новый рядом
- [ ] **20. Бот: уведомление об approved.** Расширение notify-сервиса. Креатор получает сообщение, что одобрен и ждёт деталей сотрудничества. Дёргается после commit на approve-переходе. Зависит от 9 и 18

### Группа 6. Автомодерация (в самом конце — подход не определён)

- [ ] **21. Автомодерация — LiveDune метрики.** Атрибут заявки, не отдельный статус. Фоновый воркер тянет метрики у заявок без них. Подсказка модератору, не блокер. Точная схема подтягивания (или альтернатива для TikTok), пороги и UX в админке — спецификация при старте чанка

## Граница онбординга

После chunk 20 креатор виден в общем реестре одобренных (row в `users` с `role=creator`) и доступен будущему campaign-roadmap (договор по конкретной кампании, ТЗ, рассылки, согласие/отказ, reminders события). Chunk 21 (LiveDune) ускоряет ручную модерацию, но не блокирует pipeline и может выкатываться позже.

## Тестирование

Критически важно. Автотесты — base layer, не опция: они защищают бизнес-логику при последующих правках. Каждый chunk покрывается полностью по стандартам проекта — `docs/standards/` (файлы `backend-testing-*`, `frontend-testing-*`)

## Открытые вопросы

Не фиксируем здесь — решаем при старте конкретного chunk. Реестр — `docs/open-questions.md`

## Связанные документы

- PRD: `_bmad-output/planning-artifacts/prd.md` (FR1–FR22, FR50)
- Эпики: `_bmad-output/planning-artifacts/epics.md` (Epic 2)
- UX: `_bmad-output/planning-artifacts/ux-design-specification.md`
- Стейт-машина заявки: `_bmad-output/planning-artifacts/creator-application-state-machine.md`
- Концепция верификации: `_bmad-output/planning-artifacts/creator-verification-concept.md`
- Стандарты: `docs/standards/`
