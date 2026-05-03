---
title: "Roadmap: онбординг креатора до договора"
type: roadmap
status: living
created: "2026-04-29"
updated: "2026-05-02"
revisions:
  - "2026-05-02: добавлен chunk 5 — POST /creators/applications/counts для бейджа в админке; нумерация 5–11 сдвинута на 6–12"
  - "2026-05-02: chunk 5 в работе — GET /creators/applications/counts (массив пар, sparse) + API hygiene (dedup enums/responses, BrandInput, DictionaryItem, pagination через PaginationInput, camelCase query везде)"
  - "2026-05-02: добавлен chunk 6.5 — Playwright e2e на admin verification flow (отделён от chunk 6 чтобы не блокировать UI и писать тесты на стабилизированном интерфейсе)"
  - "2026-05-02: chunk 6 готов — PR #49 смержен. Включает ревью-фиксы: единый sidebar, фильтр telegramLinked, колонка Telegram, копирование bot-link сообщения для not-linked"
  - "2026-05-02: chunk 6.5 готов — PR #50 смержен. Включает PR-fix-итерацию: codegen для frontend/e2e/types/, fail-fast cleanup с per-call timeout, crypto.randomBytes для IIN/TG-id, middleName=null/empty покрытие, drawer testid'ы на ФИО/birth-date/iin/city/categories вместо getByText"
  - "2026-05-02: бывшие chunks 7–8 переработаны на основе концепции верификации (`creator-verification-concept.md`) — расщеплены на 9 более мелких чанков 7–15 (бэк first, потом фронт-админка, потом TMA); reject и withdraw вынесены из бывшего chunk 10 в отдельные ранние чанки; прежние 9–12 сдвинуты на 16–19"
  - "2026-05-02: reject/withdraw перенесены ЗА верификационный flow в TMA — приоритет: сначала довести верификацию целиком (7 → 12), потом отказы/отзывы. Бывший 12 (verify+reject в drawer) расщеплён: verify-часть стала chunk 10, reject-часть — chunk 14. Нумерация 10–19 → 10–20."
---

# Roadmap: онбординг креатора до договора

Living document. Покрывает путь от подачи заявки на лендинге до момента, когда заявка превращается в одобренного креатора с подписанным рамочным договором. Обновляется по ходу реализации.

## Как использовать

- Документ — карта маршрута, не story-spec. Уровень — chunk, не экран и не сущность
- Перед стартом каждого chunk — отдельный `/bmad-quick-dev` в `_bmad-output/implementation-artifacts/`, ссылку добавляем сюда
- Отмечаем прогресс чек-боксами: `[ ]` → `[~]` (в работе) → `[x]` (готово, в main)
- Перед любой реализацией агент обязан полностью загрузить `docs/standards/`

## Архитектурные принципы

- **Клиент-агностичный API.** Сейчас клиент — Telegram Mini App. Целевой клиент — нативное мобильное приложение. Бэкенд один и тот же, контракт стабильный. Никакой логики, привязанной только к TMA, в общих эндпоинтах
- **Аутентификация — два режима.** Сейчас: Telegram TMA `initData` как достоверный источник identity (валидация подписи на бэке). Потом: собственная аутентификация с JWT для мобильного приложения. Сервер должен поддерживать оба способа, не переписывая бизнес-логику
- **Telegram-бот.** Клиентская часть бота (TMA, обработчики сообщений) и серверная часть (приём updates, привязка `/start`-payload к заявке, отправка уведомлений) расположены грамотно — бот не размазан по случайным местам, а живёт отдельным модулем с чёткой границей
- **Тон коммуникации с креатором** — официальный, профессиональный, грамотный. Никакого жаргона

## Темп процесса

До недели, но реалистично — день-два. Пушим креатора на быструю верификацию и подпись. Не закладываем длинные таймеры там, где можно дёрнуть напоминанием

## Нелинейность

Учитываем во всех экранах TMA: возможны отклонения с фидбэком, повторные заявки, смена категории (с ремодерацией), отказ от подписи договора. Креатор видит актуальный статус заявки на каждом шаге

## Путь заявки

- [x] **0. Форма заявки на лендинге.** Уже реализовано — заявка сохраняется на бэке
- [x] **1. Привязка Telegram-аккаунта к заявке.** Кнопка на лендинге → автозапуск бота → автовыполнение `/start` со скрытым ID заявки в payload → бот ловит, привязывает Telegram-пользователя к заявке → открытие TMA с экраном статуса. Реализация — PR #35 (минимальный бот на go-telegram/bot), PR #38 (привязка через `/start`-payload + audit), review-fixes — `_bmad-output/implementation-artifacts/archive/spec-review-fixes-telegram-link.md`
- [x] **2. Спроектировать стейт-машину заявки.** Жизненный цикл заявки от подачи до подписания договора, легальные переходы, представление для креатора. Источник истины для БД, бэка, админки и TMA. Документ — `_bmad-output/planning-artifacts/creator-application-state-machine.md`
- [x] **3. Реализация целевой стейт-машины.** Один PR. Миграция: обновить значения существующего enum статусов под целевую модель (см. state-machine doc) + бэкфил существующих заявок `pending` → `verification` + перестроить partial unique index по ИИН на новых активных значениях. Код: domain, repository, openapi, ручка приёма заявок с лендинга (`POST /creators/applications`) теперь пишет `verification`. Unit + e2e тесты под новые статусы. Без таблицы истории переходов и сервиса переходов — появятся с первым реальным переходом. PR #46, спека — `_bmad-output/implementation-artifacts/spec-creator-application-state-machine.md`
- [x] **4. Админка-бэк: ручка списка заявок.** `POST /creators/applications/list` (admin-only, POST из-за PII в search) с фильтрами (status/cities/categories arrays, dateFrom/To, ageFrom/To, telegramLinked, search), сортировкой (created_at|updated_at|full_name|birth_date|city_name × asc|desc) и пагинацией. Item-shape лёгкий (без phone/address/consents) — расширяется новыми optional полями для будущих чанков. PR #47, спека — `_bmad-output/implementation-artifacts/archive/2026-05-02-spec-creator-applications-list.md`
- [x] **5. Админка-бэк: счётчики заявок по статусам.** `GET /creators/applications/counts` (admin-only) — один SQL `GROUP BY status` возвращает массив пар `{status, count}`, sparse (статусы без рядов отсутствуют — фронт лукапит `?? 0`). Без фильтров и пагинации; фронт сам решает какие статусы суммировать в бейдж нотификаций (по умолчанию — `verification + moderation + awaiting_contract`). Read-only, без audit-log. Кеш не закладываем сразу — добавим, если поллинг станет частым. В одном PR с зачисткой openapi-дублей (shared enums/responses/parameters, BrandInput, DictionaryItem, pagination через PaginationInput, camelCase query везде). Спека — `_bmad-output/implementation-artifacts/spec-creator-applications-counts.md`
- [x] **6. Админка-фронт: список + карточка заявки на верификации.** Перенос из прототипа Айданы (`frontend/web/src/_prototype/`) в реальный `features/creatorApplications/`. Один экран — список заявок со статусом `verification` + карточка-drawer. Подключение к реальному API + RoleGuard(ADMIN). Без действий модерации. Бейдж нотификаций в sidebar/header читает counts из chunk 5. Покрытие: unit-тесты + ручной прогон полного flow через Playwright MCP (лендос → подача → админка → таблица → drawer). E2E Playwright — отдельный chunk 6.5. PR #49, спека архивирована — `_bmad-output/implementation-artifacts/archive/2026-05-02-spec-admin-creator-applications-verification.md`. Включает ревью-фиксы: единый sidebar (DashboardLayout поглотил AdminLayout), фильтр `telegramLinked` (URL + UI segmented), колонка Telegram в таблице, кнопка «Скопировать сообщение» в drawer для not-linked заявок (бэк отдаёт `telegramBotUrl` в detail).
- [x] **6.5. Browser e2e на admin verification flow.** Playwright spec в `frontend/e2e/web/admin-creator-applications-verification.spec.ts` — 6 тестов (happy path / creator without middleName / drawer prev-next / filter telegramLinked / empty state / RoleGuard). Расширен `frontend/e2e/helpers/api.ts` с derive'ами из generated `frontend/e2e/types/{schema,test-schema}.ts`, `seedAdmin/seedBrandManager/seedCreatorApplication/linkTelegramToApplication`, `uniqueIIN`/`uniqueTelegramUserId` через `crypto.randomBytes` (зеркалит `backend/e2e/testutil`). PR #50 смержен. Спека — `_bmad-output/implementation-artifacts/archive/2026-05-02-spec-admin-creator-applications-verification-e2e.md`. Включает PR-review-фиксы: codegen для e2e, fail-fast cleanup, drawer testid'ы (`drawer-full-name`, `drawer-iin`, `drawer-birth-date`, `drawer-city`, `drawer-category-{code}`, `drawer-category-other-text`), restored content-asserts на login-error/dashboard/sidebar.
- [x] **7. Бэк-фундамент: storage верификации + verification-код у заявки.** Миграция: поля верификации на соцсетях заявки (`verified` / `method` / `verified_by_user_id` / `verified_at`) + verification-код у заявки + бэкфил кодов для существующих 20 prod-заявок. Domain-типы. Расширение admin-detail ручки заявки — выводит поля верификации соцсетей (без кода). Без action-endpoint'ов. Фундамент для 8/9/11. Концепция — `_bmad-output/planning-artifacts/creator-verification-concept.md`. Спека — `_bmad-output/implementation-artifacts/spec-creator-verification-foundation.md`
- [x] **8. Бэк: webhook от SendPulse для auto-верификации Instagram.** `POST /webhooks/sendpulse/instagram` с bearer-secret middleware (constant-time). Парсинг `UGC-NNNNNN` → lookup заявки в `verification` → проверка IG-социалки → self-fix handle на mismatch → UPDATE social + applyTransition (`verification → moderation`) + audit + transition row в одной WithTx → fire-and-forget Telegram notify (WebApp deeplink) после commit. Создаёт `creator_application_status_transitions` (без бэкфила) + миграцию нормализации существующих IG handle'ов. TMA placeholder text. Зависит от 7. PR #52 смержен. Спека — `_bmad-output/implementation-artifacts/archive/2026-05-03-spec-creator-verification-instagram-webhook.md`. Раунд PR-ревью-фиксов (verificationCode в admin detail, `TELEGRAM_MOCK` switch с config-валидацией, outbound TG observability через `SpyOnlySender`/`TeeSender` + ring-buffer spy-store, WaitGroup drain notify-горутин в closer'е, `cmd/api/telegram.go` для чистоты `main.go`, e2e через generated client) — спека `_bmad-output/implementation-artifacts/archive/2026-05-03-spec-creator-verification-pr52-fixes.md`
- [ ] **9. Бэк: manual verify соцсети админом.** Admin-endpoint, действие из карточки заявки. Помечает конкретную соцсеть как `manual`-верифицированную → авто-переход в `moderation`, если это первая верификация. Audit + state-history. Зависит от 7
- [ ] **10. Фронт-админка: action verify в drawer карточки.** Кнопка «Подтвердить вручную» с выбором соцсети. Зависит от 9. E2E — расширение spec'а из 6.5 или новый рядом
- [ ] **11. TMA: creator-detail ручка + status mapping + чтение verification-данных.** Создание `GET /creators/applications/me` (или эквивалентной creator-detail ручки с initData-auth — auth/identity и access policy по `tma-auth-foundation.md`) — отдаёт код и verified-поля соцсетей для креатора. На стороне TMA — расширение пустого shell'а из chunk 1: visible-mapping 7→6 по state-machine, базовый экран статуса, чтение detail. Зависит от 7
- [ ] **12. TMA: UI верификации.** На статусе `verification` — список соцсетей с признаком verified, для Instagram код + кнопка-deeplink в DM нашего IG + инструкция. Закрывает верификационный flow целиком. Зависит от 8 и 11
- [ ] **13. Бэк: reject заявки админом.** Admin-endpoint, доступен на любом активном статусе (по state-machine). Body — feedback. Перевод в `rejected`. Audit + state-history. Параллелен 7+, не зависит от storage верификации
- [ ] **14. Фронт-админка: action reject в drawer карточки.** Кнопка «Отклонить заявку» с диалогом feedback. Зависит от 13. E2E — расширение spec'а из 6.5 или новый рядом
- [ ] **15. Бэк: withdraw заявки креатором.** TMA-endpoint с initData-auth, доступен на любом активном статусе (по state-machine). Перевод в `withdrawn`. Audit + state-history. Параллелен 7+
- [ ] **16. TMA: UI withdraw.** Кнопка «Отозвать заявку» с подтверждением, доступна на активных статусах. Зависит от 11 и 15
- [ ] **17. Автомодерация.** Подтягивание метрик из LiveDune (или альтернатива для TikTok), автоскрининг по порогам. Атрибут заявки, не отдельный статус — фоновый воркер тянет у заявок без метрик
- [ ] **18. Ручная модерация (действия).** Админка: одобрить / отправить договор / отозвать договор / вернуть на модерацию / восстановить из rejected. Reject вынесен в chunk 13. Креатор получает уведомление в Telegram + видит статус в TMA. Все переходы — через единый сервис с записью в историю переходов и `audit_logs`
- [ ] **19. Подписание рамочного договора.** TrustMe, SMS-код. Админ инициирует отправку из админки (мы подписываем со своей стороны), креатор подписывает в браузере по SMS-ссылке. Вебхук от TrustMe → переход в `signed`
- [ ] **20. Заявка → одобренный креатор.** После подписи открывается доступ к кампаниям. Граница onboarding-флоу, дальше — отдельный roadmap

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
- Фундамент авторизации TMA: `_bmad-output/planning-artifacts/tma-auth-foundation.md`
- Стандарты: `docs/standards/`
