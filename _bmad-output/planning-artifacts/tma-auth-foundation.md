---
title: "TMA: фундамент авторизации и identity"
type: concept
status: living
created: "2026-05-02"
---

# TMA: фундамент авторизации и identity

Living document. Закладывает фундамент Telegram Mini App: identity model юзера, auth-пайплайн на бэке, access policy. Конкретная API-surface (имена ручек, shape ответов, OpenAPI security scheme), архитектура фронта и стратегия тестирования — отдельные разделы, добавляются по мере проектирования.

## 1. Identity model

Юзер в TMA идентифицируется через **Telegram user_id** (из initData). Его состояние в нашей системе:

- **Гость** — Telegram-юзер открыл TMA, но в `creator_applications` и `users` его нет.
- **Аппликант** — есть row в `creator_applications` с этим `telegram_user_id`, в `users` — нет. Заявка в активном статусе онбординга.
- **Креатор** — есть row в `users` с `role=creator`, привязанная к этому Telegram-аккаунту.

Признак "креатор" = **наличие row в `users` с `role=creator`**, а не статус заявки. В какой именно момент онбординга создаётся row в `users` (после верификации / модерации / подписания договора) — открытый вопрос, решается при реализации соответствующего chunk'а.

## 2. Auth flow на бэке

Двухпайплайновая stateless схема. Identity-context единый, рефакторинга существующего кода нет.

### TMA pipeline

- TMA-клиент берёт `initData` из Telegram WebApp SDK при загрузке и кладёт её в HTTP-заголовок `X-Telegram-Init-Data` на **каждый** запрос к бэку.
- Новый middleware `TelegramAuthFromScopes` активируется по scope `TelegramInitDataAuth` в OpenAPI. Валидирует HMAC от bot_token (стандарт Telegram) + проверяет `auth_date` не старше N часов (N в Config, защита от replay).
- Никакого обмена initData → JWT, никаких серверных сессий для TMA. initData дешёвая в валидации, обновляется при перезапуске мини-апа.

### JWT pipeline (существующий)

- `AuthFromScopes(validator TokenValidator)` — без изменений. Используется на админских/брендовых ручках через `security: BearerAuth`. Ноль рефакторинга.

### OpenAPI

Две security-схемы рядом:
- `BearerAuth` — для админских/брендовых ручек.
- `TelegramInitDataAuth` — для TMA-ручек.

На каждой ручке — своя схема.

### Identity context — единый

Наполняется **обоими** middleware (JWT и TMA). Бизнес-логика читает через одинаковые хелперы и не знает, кто наполнил:

- `ContextKeyUserID` (UUID) и `ContextKeyRole` — через `middleware.UserIDFromContext` / `RoleFromContext` / `RequireRole`.
- `ContextKeyTelegramUserID` — наполняется только TMA-middleware, всегда (если initData валидный).
- `ContextKeyCreatorApplicationID` — наполняется только TMA-middleware, при наличии активной заявки.

### Будущее: мобилка

Третий middleware рядом, тоже наполняет `ContextKeyUserID + ContextKeyRole` из своих токенов. Бизнес-логика и authz по-прежнему не знают разницы.

## 3. Access policy

Авторизация — единообразно с существующим паттерном проекта: **`AuthzService.CanXxx(ctx, ...)` в начале action хендлера**. Никаких новых middleware-гвардов для TMA не вводим.

### Identity resolution в TMA-middleware

На каждом запросе с валидным initData:
1. Лукап `telegram_user_id → users`. Найден → `ContextKeyUserID` + `ContextKeyRole=creator`.
2. Если user не найден — лукап `telegram_user_id → creator_applications` (активный статус). Найден → `ContextKeyCreatorApplicationID`.
3. `ContextKeyTelegramUserID` — всегда.

### Логические уровни доступа (через AuthzService)

- **Гость+** — достаточно валидной initData (scope `TelegramInitDataAuth` в OpenAPI). Пример ручки: `GET /me/status` (или эквивалент) — фронт узнаёт состояние.
- **Аппликант+** — `AuthzService` проверяет наличие `ContextKeyCreatorApplicationID`. Примеры: `CanViewMyCreatorApplication(ctx)`, `CanWithdrawMyCreatorApplication(ctx)`.
- **Креатор+** — `AuthzService` проверяет `ContextKeyRole == creator` (плюс по желанию `RequireRole(creator)` middleware как лёгкий guard). Примеры — будущие campaigns-методы.

Хендлер вызывает `authzService.CanXxx(ctx, ...)` → если ошибка `domain.ErrForbidden`, возвращает 403. Один и тот же паттерн для админских и TMA-ручек.

В OpenAPI — одна security-схема `TelegramInitDataAuth` для всех TMA-ручек; уровни выше "гость+" — server-side через AuthzService.

## Открытые вопросы

- В какой момент онбординга создаётся row в `users` (после верификации / модерации / подписания) — решим при реализации chunk'а одобрения.
- Конкретное имя/shape ручки `/me/status` (или эквивалент) — отдельным шагом проектирования.
- TTL для `auth_date` (значение N часов) — настраивается через Config; конкретное значение — на момент реализации chunk 11.
- Стратегия тестирования (unit / e2e в CI / e2e на staging для TMA-pipeline, как мокать initData) — отдельным шагом.
- Архитектура фронта TMA (bootstrap, store, API client, error handling) — отдельным шагом.

## Связанные документы

- Roadmap: `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` (chunks 11–12)
- State machine заявки: `_bmad-output/planning-artifacts/creator-application-state-machine.md`
- Концепция верификации: `_bmad-output/planning-artifacts/creator-verification-concept.md`
- Стандарты: `docs/standards/`
