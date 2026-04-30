# Безопасность

Secure by default, не secure by configuration.

## Environment — явный enum

Окружение задаётся через `ENVIRONMENT` env var (`local`, `staging`, `production`). Поведение, зависящее от окружения (secure cookies, test endpoints, debug logging), определяется от этого enum явно. Никаких `insecure`, `disable_security`, `skip_auth` флагов. Production — всегда максимально secure, без возможности ослабить через конфиг.

## PII — где запрещена

PII (ИИН, ФИО, телефон, адрес, handle, email в связке с другими данными) запрещена **во всех ветках вывода**, не только в `logger.Info`:

- **Стандартные stdout-логи** (`logger.Info` / `Debug` / `Warn`) — запрещено. Audit-логи в БД (`audit_logs`) — допустимо (это специализированное хранилище с отдельным retention).
- **Текст `error.Message`** — попадает в response body, доступен злоумышленнику.
- **URL params** (query string или path segment) — попадают в access-логи backend'а, в browser history, в Referer header.
- **Любые structured-log поля.**

Допустимо в логах: HTTP method/path/status/duration, user ID (UUID), request/trace ID, IP-адрес, error messages без sensitive context.

## Anti-fingerprinting

Сообщения и timing для классов "пользователь не существует", "неверный пароль", "пароль слишком короткий" — одинаковые. Иначе атакующий по разным response'ам перебором определит, какой email уже зарегистрирован.

Применимо ко всем ручкам, где невалидный ввод может выдать существование сущности: login, password reset, registration с уникальным полем, любые public lookup'ы.

## Внешние URL — escape user-input

User-controlled string (handle, email, имя) в построении внешних URL (Telegram deep-link, redirect URL, OAuth state) — обязательно через `url.QueryEscape` или `url.PathEscape`. Без escape пользователь с handle `?evil=1` инжектит параметры во внешний URL и компрометирует redirect / share-link.

Internal URL (наши собственные ручки) — тоже escape по умолчанию.

## Limits и rate-limiting

- **Length bounds для free-text.** Поля, принимающие произвольный пользовательский текст (UserAgent, address, comment, bio) — ограничены длиной (например, UA → 1024, address → 256). Без bound'а атакующий отправит body размером в мегабайты и сложит instance.
- **Rate-limiting на критичных публичных ручках.** Submit, login, password-reset, registration — лимит запросов с IP / per-user. Без лимита публичная ручка — DoS-вектор.

## Секреты — только в env vars

- Все секреты — через env vars. Хардкод секретов в коде запрещён
- `.env` файлы — только для локальной разработки, в `.gitignore`
- CI/CD — через GitHub Secrets
- Production — через Dokploy env vars
- При обнаружении секрета в коде — немедленно ротировать и исправить

## Что ревьюить

- [blocker] Hardcoded секрет в коде (token, password, API key) — не env var.
- [blocker] PII (ИИН, ФИО, телефон, адрес, handle) в `logger.Info` / `logger.Debug` / `logger.Warn`.
- [blocker] PII в тексте `error.Message` (попадает в response body).
- [blocker] PII в URL params (query string или path segment).
- [blocker] CORS_ORIGINS включает `*` (wildcard).
- [blocker] `insecure_*` / `disable_*` / `skip_*` флаг конфигурации для production-поведения.
- [major] Anti-fingerprinting нарушение: разный текст ошибки или timing для login user-not-found vs wrong-password vs short-password.
- [major] Free-text поле без length bound (UA, address, comment) — DoS через гигантский body.
- [major] User-controlled string в построении внешних URL без `url.QueryEscape` / `url.PathEscape`.
- [major] Refresh token не httpOnly cookie.
- [minor] Rate-limiting на критичной публичной ручке отсутствует.
