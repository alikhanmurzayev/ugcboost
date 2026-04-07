---
stepsCompleted: [1, 2, 3, 4, 5, 6, 7, 8]
lastStep: 8
status: 'complete'
completedAt: '2026-04-07'
inputDocuments:
  - '_bmad-output/planning-artifacts/prd.md'
  - '_bmad-output/planning/product-brief-ugcboost-distillate.md'
  - '_bmad-output/planning-artifacts/market-research.md'
  - '_bmad-output/planning-artifacts/domain-research.md'
  - '_bmad-output/planning-artifacts/product-backlog.md'
  - 'docs/open-questions.md'
  - 'docs/research/oq15-encryption-at-rest.md'
workflowType: 'architecture'
project_name: 'ugcboost'
user_name: 'Alikhan'
date: '2026-04-07'
---

# Architecture Decision Document

_This document builds collaboratively through step-by-step discovery. Sections are appended as we work through each architectural decision together._

## Анализ контекста проекта

### Обзор требований

**Функциональные требования (5 доменов, 16 блоков):**

1. **Онбординг креаторов:** веб-форма заявки (ФИО, ИИН, соцсети, категория, адрес, 4 чекбокса ПД, проверка 18+ по ИИН), верификация владения аккаунтом (метод TBD), автоскрининг через LiveDune API (подписчики, ER, просмотры, фейки), ручная модерация с фидбэком, подписание рамочного договора через TrustMe, Telegram-бот для уведомлений
2. **Кампании:** создание 3 типов (продукт/услуга/ивент), модерация кампаний UGCBoost, категорийная фильтрация для креаторов, система заявок с подтверждением ознакомления с ТЗ + отзыв заявки, ранжированный список заявок с метриками, сдача работ (ссылки + чеклист маркировки), приёмка/отклонение, dispute flow с эскалацией
3. **Управление:** админ-панель (модерация креаторов + кампаний), кросс-кампанийная видимость (настраивается per-brand), бан/чёрный список, система напоминаний (2нед/1нед/3дня/1день) с кнопкой подтверждения, история инцидентов
4. **Бренды:** веб-кабинет (аккаунты создаются вручную в MVP), каталог креаторов с фильтрами и метриками, массовое одобрение/отклонение, отслеживание статусов кампаний, галерея контента (MVP: ссылки с метаданными, без oEmbed)
5. **Инфраструктура:** RBAC (креатор / менеджер бренда / админ), аудит-лог (упрощённый: user_id, action, entity_type, entity_id, timestamp, details_json), нотификации (Telegram-бот + email)

**Убрано из MVP (→ Growth):**
- Q&A по кампаниям (креаторы спрашивают через Telegram-бот напрямую)
- oEmbed галерея контента (MVP: ссылки с метаданными)
- Формальный i18n framework (MVP: строки в отдельных файлах, не хардкод)

**Нефункциональные требования (архитектурно значимые):**

- **Безопасность:** TLS 1.2+, RBAC на каждом API-эндпоинте, OWASP Top 10, изоляция данных между тенантами, шифрование at rest не требуется для MVP
- **Производительность:** TMA < 3с, веб-каталог 200 записей < 2с, 200 concurrent users, уведомления Telegram < 1 мин, email < 5 мин
- **Надёжность:** zero-downtime deploys, continuous backup (WAL), RTO ≤ 4ч, юридически значимые данные — потеря недопустима, backwards-compatible миграции БД
- **Тестируемость:** 100% покрытие user flows (unit + integration + E2E + UI), идемпотентные тесты на непустой БД (factories/fixtures), CI < 10 мин (параллельный запуск бэкенд + фронтенд)
- **Observability:** health check, метрики (response time, error rate), алертинг, application-логи
- **Локализация:** русский (MVP), строки вынесены в отдельные файлы. Формальный i18n framework — Growth

### Фронтенд-архитектура: 2 приложения, не 3

**Решение (Party Mode):** Веб-бренд и веб-админ объединены в одно приложение с ролевым доступом.

**Обоснование:** ~80% общих компонентов (таблицы кампаний, списки креаторов, статусы). Разница — scope данных (бэкенд фильтрует по brand_id) + 2-3 экрана модерации только для admin. Два отдельных приложения = дублирование кода при одном разработчике.

| Приложение | Пользователи | Платформа |
|---|---|---|
| **Лендинг** | Все посетители | Статический сайт (Astro), SEO |
| **Telegram Mini App** | Креаторы | Мобайл (TMA SDK) |
| **Веб-кабинет** | Бренды + Админы | Десктоп (ролевой доступ) |

### Внешние зависимости и интеграции

| Интеграция | Назначение | Статус API | Fallback |
|---|---|---|---|
| TrustMe/TrustContract | E-подпись договоров (SMS, юрсила по ГК РК) | Нужен запрос — тариф «Профи» с API | Ручная отправка через web-интерфейс TrustMe |
| LiveDune | Метрики креаторов (IG: подписчики, ER, просмотры, фейки) | Документация изучена, доступ нужен | Ручной скрининг |
| Telegram Bot API | Уведомления + Mini App фронтенд | Стабильно | WhatsApp (ручной) для уведомлений |
| Email (transactional) | Уведомления брендам | Выбор сервиса на этапе техдизайна | — |

**Паттерн интеграций (порты и адаптеры):** Каждая внешняя интеграция = интерфейс + реальная реализация + мок-реализация. Переключение через env var. Мок-реализации функциональные (реалистичные данные), не stub-заглушки.

**Критичные открытые вопросы (влияют на архитектуру):**
- OQ-1: TikTok через LiveDune — не подтверждён
- OQ-2: API TrustMe — доступ, лимиты, цены
- OQ-3: Метод верификации креаторов — OAuth / верификационный пост / иное

### Масштаб и сложность

- **Тип проекта:** Managed Marketplace (2 фронтенда: TMA + Web с ролевым доступом)
- **Сложность:** Высокая — 2 фронтенда, 4 внешних интеграции, юридический compliance (авторское право, ПД, маркировка рекламы), трёхуровневая договорная схема
- **Ресурсы:** 1 full-stack разработчик, дедлайн ~20 апреля 2026
- **Нагрузка MVP:** ~200 креаторов, несколько брендов, десятки кампаний
- **Ограничения:** серверы в РК, только бартер (без платежей), TikTok-аналитика не подтверждена

### Cross-Cutting Concerns

1. **Аутентификация:** два механизма — Telegram auth (креаторы) + логин/пароль (бренды/админы)
2. **Авторизация:** RBAC с проверкой принадлежности данных на каждом запросе
3. **Аудит-лог:** упрощённый (user_id, action, entity_type, entity_id, timestamp, details_json), отдельно от application-логов
4. **Изоляция данных:** бренд видит только свои кампании, креатор — только свои заявки
5. **Нотификации:** два канала (Telegram + email), разные для разных ролей
6. **File/media storage:** объектное хранилище для документов, превью профилей
7. **Background jobs:** scheduler для напоминаний по дедлайнам, автоскрининга LiveDune, email-рассылок
8. **Миграции БД:** backwards-compatible (zero-downtime deploys)
9. **Порты и адаптеры:** каждая интеграция за интерфейсом + мок-реализация, переключение через env
10. **Тестовая инфраструктура:** factories/fixtures, уникальные данные, параллельный CI

### Архитектурные решения (зафиксированы)

| Решение | Выбор | Обоснование |
|---|---|---|
| Репозиторий | Монорепа | 1 разработчик, общие типы и конфиги |
| Бэкенд | Один сервис | Микросервисы при одном человеке — overkill |
| API | REST | Проще для TMA, меньше overhead, GraphQL не даёт выигрыша |
| БД | PostgreSQL | Реляционные данные, транзакции, юридически значимые записи |
| Фронтенд-стратегия | TMA + единый веб-кабинет (ролевой доступ) | ~80% общих компонентов бренд/админ |

## Стартовый стек

### Бэкенд

| Компонент | Выбор | Обоснование |
|---|---|---|
| Язык | Go | Опыт Alikhan, простой для AI-кодинга и ревью |
| Роутер | Chi v5 | Идиоматичный, net/http совместимый, composable, ~1000 LOC ядра |
| API контракт | OpenAPI 3.x YAML | Contract-first: единственный источник правды для бэка и фронта |
| Go codegen | oapi-codegen | Генерирует Chi handler interface + request/response types из OpenAPI |
| БД-драйвер | pgx | Нативный PostgreSQL, быстрее database/sql |
| SQL builder | squirrel | Динамические запросы, фильтры, читаемый Go-код |
| DB utility | internal/dbutil | Интерфейс DB (мокается в unit-тестах), generic-хелперы: One[T], Many[T], Val[T], Exec — принимают sq.Sqlizer |
| Миграции | goose | SQL-миграции, backwards-compatible |
| Валидация | oapi-codegen (из OpenAPI spec) + go-playground/validator | Валидация на уровне handler, ошибки → 400 с деталями |
| Логирование | slog (stdlib) | Structured JSON logs, без зависимостей |

### DB Utility Layer (internal/dbutil)

Тонкая обёртка (~100-200 строк) для унификации работы с БД:

**Интерфейс:**
```
type DB interface {
    Query(ctx, sql, args...) (pgx.Rows, error)
    QueryRow(ctx, sql, args...) pgx.Row
    Exec(ctx, sql, args...) (pgconn.CommandTag, error)
}
```
pgxpool.Pool и pgx.Tx реализуют этот интерфейс — zero overhead.

**Generic-хелперы (принимают sq.Sqlizer):**
- `One[T](ctx, db, query)` — одна строка → struct
- `Many[T](ctx, db, query)` — много строк → []struct
- `Val[T](ctx, db, query)` — одно значение (scalar)
- `Vals[T](ctx, db, query)` — много значений → []T
- `Exec(ctx, db, query)` — INSERT/UPDATE/DELETE → rows affected (int64)

INSERT ... RETURNING → используй `One[T]`.

**Тестируемость:** unit-тесты мокают интерфейс DB, интеграционные тесты работают с реальной PostgreSQL.

### Фронтенд

| Компонент | Выбор | Обоснование |
|---|---|---|
| Framework | React + TypeScript | Доминирует в TMA-экосистеме, AI-friendly |
| Bundler | Vite | Быстрая сборка, HMR, стандарт 2026 |
| TS codegen | openapi-typescript | Генерирует TS types + type-safe fetch client из того же OpenAPI YAML |
| State management | TanStack Query + Zustand | React Query = серверный стейт (кэш, refetch). Zustand = UI-стейт |
| UI библиотека | shadcn/ui + Tailwind CSS | Копируемые компоненты, AI пишет отличный Tailwind |
| Роутинг | React Router v7 | Стандарт, AI знает идеально |
| TMA SDK | @telegram-apps/sdk-react (tma.js) | Официальный SDK, mock-env для разработки |
| TMA стартер | Telegram-Mini-Apps/reactjs-template | Официальный шаблон React + tma.js + TS + Vite |

### Лендинг (ugcboost.kz)

| Компонент | Выбор | Обоснование |
|---|---|---|
| Framework | Astro | Статический output (HTML/CSS/JS), zero JS by default, компоненты без copy-paste |
| Стили | Tailwind CSS (shared preset) | Единый дизайн-язык с web и tma |
| Output | Статический HTML | SEO из коробки, максимальная скорость загрузки |
| Интерактивность | Astro Islands (при необходимости) | Формы и динамические элементы — точечно, без нагрузки на всю страницу |

Лендинг — отдельное приложение, не часть web-кабинета. Деплоится независимо. Конкретные страницы определяются по мере необходимости.

### Инфраструктура

| Компонент | Выбор | Обоснование |
|---|---|---|
| Деплой | Dokploy (self-hosted PaaS) | Docker-based, auto-SSL, мониторинг, Git-деплой |
| Оркестрация | Docker Compose | Один VPS на среду |
| БД | PostgreSQL (на том же VPS через Docker) | Zero latency, одна точка администрирования. Отдельный сервер — Growth |
| Reverse proxy | Traefik (встроен в Dokploy) | Автоматический SSL, роутинг |
| CDN / DDoS / WAF | Cloudflare (бесплатный план) | Скрытие IP, DDoS-защита L3-L7, WAF, bot protection |
| Хостинг | 3× VPS у KZ-провайдера (выбор позже) | Закон N 94-V: ПД граждан РК хранятся в РК |

### Серверная инфраструктура

```
                    Cloudflare (DDoS, WAF, скрытие IP)
                         │
              ┌──────────┴──────────┐
              ▼                     ▼
         VPS-1 (prod)          VPS-2 (staging)
         ├── Dokploy           ├── Dokploy
         ├── Traefik           ├── Traefik
         ├── backend           ├── backend
         ├── web               ├── web
         ├── tma               ├── tma
         ├── landing           ├── landing
         ├── PostgreSQL        ├── PostgreSQL
         └── backup container  └── (бэкапы не нужны)
              │
              ▼ rsync daily
         VPS-3 (backup) — только хранение бэкапов
```

**Среды:**
- **Production** (VPS-1): ugcboost.kz, app.ugcboost.kz, api.ugcboost.kz
- **Staging** (VPS-2): staging.ugcboost.kz — доступ через Cloudflare Access или basic auth
- **Backup** (VPS-3): дешёвый VPS, только диск, rsync с VPS-1

Все три VPS у одного KZ-провайдера (требование локализации данных). Провайдер выбирается отдельно.

**CI/CD pipeline:**
```
push → lint → test-unit → build Docker image :sha-xxx
  → deploy staging (тот же image, staging env vars)
  → E2E tests на staging
  → (ручной approve) → deploy prod (тот же image, prod env vars)
```

### Кибербезопасность

**Сетевой уровень:**
- Cloudflare проксирует весь трафик — реальный IP сервера скрыт
- Firewall (ufw): HTTP/HTTPS только с Cloudflare IP ranges, SSH с кастомного порта только по ключу
- Все остальные порты закрыты (PostgreSQL 5432, Dokploy UI — только через SSH tunnel)

**SSH hardening:**
- Аутентификация только по ключу (пароль отключён)
- Кастомный порт (не 22)
- fail2ban — автобан после 3 неудачных попыток
- Root логин отключён

**Docker security:**
- Контейнеры работают от non-root user
- Без `--privileged`
- Docker socket не проброшен в приложения
- Source maps не включаются в production builds

**Secrets management:**
- `.env` файлы не в Git (gitignore)
- Production secrets: Dokploy environment variables (шифрование)
- CI/CD secrets: GitHub Secrets → передаются в Actions
- Никаких секретов в Docker images, логах, error messages

**Не палить стек:**
- Убрать `Server` и `X-Powered-By` headers (Traefik config)
- Error pages без stack traces (generic 500)
- Без source maps на проде

**Бэкапы:**
- PostgreSQL: pg_dump daily (контейнер postgres-backup-local) + WAL archiving
- Локальные бэкапы на VPS-1 (быстрое восстановление)
- rsync daily на VPS-3 (disaster recovery)
- Rolling: 7 дней daily + 4 недели weekly
- Staging-бэкапы не нужны (данные фейковые, seed-скрипты)

**Dependency security:**
- Dependabot (GitHub) — автоматические PR на уязвимые зависимости
- `npm audit` в CI — fail на critical/high
- `go.sum` верифицируется автоматически

**Security logging:**
- Все auth-попытки (успешные и неуспешные) → slog
- Все 403 (попытки доступа к чужим данным) → slog с userId
- Rate limit срабатывания → slog
- Отдельно от бизнес аудит-лога

**Staging-данные:**
- Только seed-скрипты с фейковыми данными
- Никогда не копировать prod-данные на staging

### Структура монорепы

```
ugcboost/
├── backend/
│   ├── cmd/api/           — Entry point
│   ├── internal/
│   │   ├── dbutil/        — DB interface + generic helpers (sq.Sqlizer)
│   │   ├── handler/       — HTTP handlers (Chi routes)
│   │   ├── service/       — Business logic
│   │   ├── repository/    — Data access (squirrel queries)
│   │   ├── integration/   — Ports & adapters (TrustMe, LiveDune)
│   │   └── middleware/     — Auth, RBAC, audit
│   └── go.mod
├── web/                   — React + Vite + TS (бренды + админы)
├── tma/                   — React + Vite + TS + tma.js (креаторы)
├── migrations/            — SQL-миграции (goose)
├── api/
│   └── openapi.yaml         — OpenAPI 3.x spec (единственный источник правды)
├── docker-compose.yml
├── Makefile
└── .env.example
```

### Contract-First API Workflow

```
api/openapi.yaml → make generate →
  ├── backend/internal/api/     (Go: Chi handler interface + types)
  ├── web/src/api/generated/    (TS: types + fetch client)
  └── tma/src/api/generated/    (TS: types + fetch client)
```

Swagger UI доступен автоматически из того же `openapi.yaml`.

## Архитектурные решения

### Data Architecture

| Решение | Выбор | Обоснование |
|---|---|---|
| Кэширование | Без кэша для MVP | 200 юзеров — PostgreSQL справится. Redis — Growth |
| Soft delete | Нет | Настоящий DELETE + аудит-лог. Восстановление из лога |

### Authentication & Security

| Решение | Выбор | Обоснование |
|---|---|---|
| Сессии/токены | JWT (short-lived access + refresh в httpOnly cookie) | Stateless, не нужен session store |
| Telegram auth | Валидация initData HMAC-SHA256 → выдача JWT | Стандарт TMA, криптографическая верификация |
| Единый auth | Один JWT формат для всех ролей, поле `role` в payload | Один middleware, одна логика |
| Пароли брендов | bcrypt, сброс через email-ссылку | Стандарт |
| CORS | Whitelist: TMA origin + web domain | Строго, не `*` |
| Rate limiting | Chi middleware, in-memory (golang.org/x/time/rate), по IP + user | Без Redis, достаточно для одного инстанса |

### API & Communication

| Решение | Выбор | Обоснование |
|---|---|---|
| Подход | Contract-first (OpenAPI 3.x YAML) | Гарантированный синхрон типов бэк/фронт |
| Go codegen | oapi-codegen → Chi handler interface + types | Нативная Chi-совместимость |
| TS codegen | openapi-typescript → types + fetch client | Type-safe клиент из того же YAML |
| Swagger | Автоматически из OpenAPI YAML | Бесплатная документация |
| Версионирование | Нет для MVP, префикс `/api/` | Один клиент, breaking changes → обнови клиент |
| Формат ответов | Единый JSON envelope: `{ "data": ..., "error": { "code": "...", "message": "..." } }` | Предсказуемый контракт |
| Коды ошибок | Строковые domain-коды: `CREATOR_NOT_FOUND`, `CAMPAIGN_FULL` + HTTP status | Фронт парсит код, показывает локализованное сообщение |

### Frontend Architecture

| Решение | Выбор | Обоснование |
|---|---|---|
| State management | TanStack Query (серверный) + Zustand (UI) | Кэш, refetch, loading states + легковесный UI store |
| UI библиотека | shadcn/ui + Tailwind CSS | Копируемые компоненты, AI-friendly |
| Роутинг | React Router v7 | Стандарт |
| HTTP клиент | Сгенерированный из openapi-typescript | Type-safe, из контракта |

### Infrastructure & Deployment

| Решение | Выбор | Обоснование |
|---|---|---|
| CI/CD | GitHub Actions (lint + test + build + deploy) | GitHub уже настроен, бесплатные минуты |
| Background jobs | robfig/cron внутри основного процесса | Напоминания, LiveDune polling — по расписанию. Отдельный worker — overkill для MVP |
| Логирование | slog (stdlib Go 1.21+), structured JSON | Встроен в Go, Dokploy показывает stdout |
| File storage | Локальная ФС (Docker volume) для MVP | Минимальный объём. S3-совместимое — Growth |
| Env config | .env файл, Dokploy управляет env variables | Стандарт для Docker-деплоя |

### Отложенные решения (Growth)

| Решение | Когда | Причина |
|---|---|---|
| Redis (кэш, rate limiting) | При росте >1000 юзеров | MVP справляется без |
| S3 file storage | При загрузке медиа | MVP: только ссылки |
| API versioning (/v1/) | При внешних потребителях | Один клиент = не нужно |
| Формальный i18n | Казахский язык | MVP: русский, строки в файлах |
| oEmbed галерея | После EFW | MVP: ссылки с метаданными |
| Q&A по кампаниям | После EFW | Креаторы спрашивают через Telegram |

## Паттерны и правила консистентности

### Naming

**БД (PostgreSQL):**
- Таблицы: `snake_case`, множественное число — `creators`, `campaigns`, `applications`
- Колонки: `snake_case` — `full_name`, `brand_id`, `created_at`
- Foreign keys: `{referenced_table_singular}_id` — `creator_id`, `campaign_id`
- Индексы: `idx_{table}_{columns}` — `idx_creators_status`
- Constraints: `{table}_{type}_{columns}` — `creators_pkey`, `creators_iin_unique`

**Go:**
- Стандартные конвенции: `CamelCase` экспорты, `camelCase` internal
- Файлы: `snake_case.go` — `campaign_handler.go`, `creator_service.go`
- Пакеты: одно слово lowercase — `handler`, `service`, `repository`, `authz`

**React/TypeScript:**
- Компоненты: `PascalCase.tsx` — `CampaignList.tsx`, `CreatorCard.tsx`
- Хуки: `camelCase.ts` — `useCampaigns.ts`
- Утилиты/функции: `camelCase`

**JSON (API):**
- Поля: `camelCase` — `{ "fullName": "...", "brandId": 1, "createdAt": "..." }`
- Даты: ISO 8601 — `"2026-04-07T12:00:00Z"`
- Null: `null` (не пустая строка, не 0)

### Структура кода

**Бэкенд — по слоям:**
```
backend/internal/
├── api/           — сгенерированный код oapi-codegen (не трогать руками)
├── handler/       — реализация oapi-codegen interface
├── service/       — бизнес-логика (интерфейсы для мокинга)
├── repository/    — data access (squirrel + dbutil)
├── integration/   — внешние сервисы (порты + адаптеры)
├── middleware/     — auth (JWT decode), logging, CORS, rate limiting
├── authz/         — fine-grained авторизация (CanManageCampaign, CanSubmitWork)
├── domain/        — domain types + domain errors
└── dbutil/        — DB interface + generic helpers
```

**Фронтенд — по фичам:**
```
web/src/  (и tma/src/ аналогично)
├── features/
│   ├── campaigns/
│   │   ├── CampaignList.tsx
│   │   ├── CampaignList.test.tsx
│   │   ├── CampaignForm.tsx
│   │   ├── useCampaigns.ts
│   │   └── index.ts
│   ├── creators/
│   └── auth/
├── shared/
│   ├── components/     — UI (Button, Modal, Table)
│   ├── hooks/          — общие хуки
│   ├── lib/            — утилиты
│   └── i18n/           — маппинг error codes → локализованные строки
└── api/generated/      — сгенерированные типы (не трогать руками)
```

**Тесты — co-located:**
- Go: `creator_service_test.go` рядом с `creator_service.go`
- React: `CampaignList.test.tsx` рядом с `CampaignList.tsx`

### Валидация: Zero Trust

**Принцип:** хендлер обязан валидировать абсолютно все входные данные. Пользователям нельзя доверять. Невалидные данные не должны дойти до сервисного слоя.

**Два уровня валидации:**

1. **OpenAPI-уровень (автоматический):** oapi-codegen генерирует валидацию из спеки — required, типы, enums, min/max, format. Кривой JSON или отсутствующее поле → 400 автоматически.
2. **Бизнес-уровень (в handler):** формат ИИН (12 цифр, дата рождения), проверка 18+, URL соцсетей, и т.д.

**Правило:** сервис получает только провалидированные данные. Сервис никогда не валидирует входные данные сам.

### Авторизация: два слоя

**Middleware (`internal/middleware`)** — грубая проверка роли:
- Decode JWT → достать `userId` + `role` → положить в context
- Проверка: этот эндпоинт доступен для роли `admin`? `brand_manager`? Отсекает сразу → 403

**Пакет `internal/authz`** — fine-grained проверка владения:
```
authz.CanManageCampaign(ctx, db, userID, campaignID) error   — бренд видит только свои
authz.CanApproveCreator(ctx, db, userID) error                — только admin
authz.CanSubmitWork(ctx, db, creatorID, campaignID) error     — только одобренный креатор
```

Handler вызывает `authz.Can...()` до вызова сервиса. Ошибка → 403.

**Почему отдельный пакет:** тестируется изолированно, переиспользуется, не размазан по бизнес-логике, одно место чтобы понять "кто что может".

### Error Handling

**Go (бэкенд):**
- Domain errors: `domain.ErrNotFound`, `domain.ErrForbidden`, `domain.ErrValidation`
- Handler маппит domain error → HTTP status + строковый error code
- Unexpected errors: `slog.Error()` + generic 500, без деталей клиенту
- Бизнес-ошибки: `slog.Warn()`

**API формат ошибки:**
```
{ "error": { "code": "CAMPAIGN_FULL", "message": "Campaign has reached participant limit" } }
```
- `code` — строковый domain-код, фронт парсит
- `message` — английский fallback (если фронт не знает код)

**Локализация ошибок — целиком на фронте:**
```
// shared/i18n/errors.ts
const errorMessages: Record<string, string> = {
  "CAMPAIGN_FULL": "Кампания заполнена",
  "CREATOR_NOT_FOUND": "Креатор не найден",
  "VALIDATION_ERROR": "Проверьте введённые данные",
}
```
- MVP: русский маппинг
- Growth (казахский): второй маппинг
- Бэкенд language-agnostic

**React (фронтенд):**
- TanStack Query `onError` → toast с локализованным сообщением
- React Error Boundary для unexpected crashes

### Process Patterns

**Auth flow:**
- Каждый запрос: JWT в `Authorization: Bearer` header
- Handler достаёт userId/role из context (положено middleware), **никогда** из request body

**Dependency injection:**
- Go: конструкторы, не глобалы — `NewCampaignService(repo, logger)`
- Все зависимости через интерфейсы → мокаемо в unit-тестах

**Сгенерированный код:**
- `backend/internal/api/` и `*/src/api/generated/` — НИКОГДА не редактировать руками
- Изменения только через `openapi.yaml` → `make generate`

## Структура проекта и границы

### Структура слоёв

Конкретные файлы создаются AI-агентом при имплементации каждого vertical slice. Здесь — только слои и их ответственность.

```
ugcboost/
├── api/
│   └── openapi.yaml                    # OpenAPI 3.x — единственный источник правды
│
├── backend/
│   ├── cmd/
│   │   └── api/
│   │       └── main.go                 # Entry point: config.Load(), DI, server start
│   ├── internal/
│   │   ├── api/                        # ⚡ СГЕНЕРИРОВАНО (oapi-codegen) — не трогать
│   │   ├── config/                     # struct Config, Load() — парсинг env vars
│   │   ├── handler/                    # Реализация oapi-codegen ServerInterface
│   │   │   └── handler.go             # Struct Handler → делегирует domain groups
│   │   ├── service/                    # Бизнес-логика (интерфейсы для мокинга)
│   │   ├── repository/                 # Data access (squirrel + dbutil)
│   │   ├── integration/                # Порты и адаптеры (по сервису)
│   │   │   ├── livedune/              # Интерфейс + client.go + mock.go
│   │   │   ├── trustme/               # Интерфейс + client.go + mock.go
│   │   │   ├── telegram/              # Интерфейс + bot.go + mock.go
│   │   │   ├── email/                 # Интерфейс + sender.go + mock.go
│   │   │   └── storage/               # Интерфейс + local.go + mock.go
│   │   ├── middleware/                 # Auth (JWT), logging, CORS, rate limiting, recovery
│   │   ├── authz/                      # Fine-grained авторизация (CanManage*, CanSubmit*)
│   │   ├── domain/                     # Internal types + enums + domain errors (НЕ API types)
│   │   ├── dbutil/                     # DB interface + One/Many/Val/Vals/Exec
│   │   └── testutil/                   # Factories, test DB setup, test JWT, mock re-exports
│   └── go.mod
│
├── web/                                # React + Vite + TS (бренды + админы)
│   ├── src/
│   │   ├── api/generated/              # ⚡ СГЕНЕРИРОВАНО (openapi-typescript) — не трогать
│   │   ├── features/                   # По фичам (создаются при имплементации slice)
│   │   ├── shared/
│   │   │   ├── components/ui/          # shadcn/ui примитивы
│   │   │   ├── hooks/
│   │   │   ├── lib/
│   │   │   └── i18n/                   # error codes → русские сообщения
│   │   └── stores/                     # Zustand (auth, UI state)
│   └── package.json
│
├── tma/                                # React + Vite + TS + tma.js (креаторы)
│   ├── src/
│   │   ├── api/generated/              # ⚡ СГЕНЕРИРОВАНО — не трогать
│   │   ├── features/                   # По фичам
│   │   ├── shared/
│   │   │   ├── components/ui/
│   │   │   ├── hooks/
│   │   │   ├── lib/                    # + tma.ts (TMA SDK init, theme, haptics)
│   │   │   └── i18n/
│   │   └── stores/
│   └── package.json
│
├── landing/                            # Astro — статический лендинг ugcboost.kz
│   ├── src/
│   │   ├── layouts/                    # Общие layouts (header, footer, meta)
│   │   ├── pages/                      # Страницы (создаются по мере необходимости)
│   │   └── components/                 # Переиспользуемые блоки
│   ├── public/                         # Статика (изображения, шрифты)
│   ├── astro.config.mjs
│   ├── tailwind.config.ts              # extends: ../tailwind.preset.ts
│   └── package.json
│
├── e2e/                                # E2E тесты (Playwright)
│   ├── playwright.config.ts
│   ├── web/                            # Тесты веб-кабинета
│   └── tma/                            # Тесты TMA
│
├── migrations/                         # SQL-миграции (goose, timestamp-based)
│
├── .github/
│   └── workflows/
│       └── ci.yml                      # lint + test-unit + test-integration + build
│
├── tailwind.preset.ts                  # Общие цвета, шрифты, spacing для всех фронтов
├── docker-compose.yml                  # Dev: PostgreSQL + backend + web + tma + landing
├── docker-compose.prod.yml             # Production overrides
├── Makefile
├── .env.example
├── .gitignore
└── CLAUDE.md
```

### Архитектурные границы

**Слои бэкенда (строго однонаправленные):**
```
HTTP → middleware → handler → authz → service → repository → DB
                                          ↓
                                    integration (external APIs)
```
- Handler → service (никогда напрямую в repository)
- Service → repository + integration (никогда в handler)
- Repository → dbutil + squirrel (никогда в service)
- Integration изолирована за интерфейсами, `*_MOCK=true` переключает реализацию

**Handler groups:**
```go
type Handler struct {
    auth     *AuthGroup
    creator  *CreatorGroup
    campaign *CampaignGroup
    // ... группы создаются при имплементации
}
```
Каждая группа получает только свои зависимости. Handler реализует единый `api.ServerInterface`.

**Типы — domain vs generated:**
- `internal/domain/` — internal types, enums, domain errors (`ErrNotFound`, `ErrForbidden`)
- `internal/api/*.gen.go` — API request/response types (сгенерированные, не трогать)
- Маппинг domain ↔ API — в handler

**Границы данных:**
- brand_id / creator_id фильтрация в repository
- authz проверяет владение до вызова сервиса
- Аудит-лог — отдельная таблица, записывается в repository

**Границы фронтенда:**
- `web/`, `tma/`, `landing/` — полностью независимые builds (Vite / Astro)
- Общий код между web и tma — только через сгенерированные API-типы
- Общий дизайн-язык — через `tailwind.preset.ts` (цвета, шрифты, spacing)
- Дублирование `shared/` между web и tma допустимо для MVP

### Тестирование

**Go unit-тесты:** co-located (`*_test.go` рядом с кодом)
**Go integration-тесты:** co-located + build tag `//go:build integration`
**Фронтенд unit-тесты:** co-located (`*.test.tsx` рядом с компонентом)
**E2E тесты:** `e2e/` на корневом уровне (Playwright)
**Test utilities:** `backend/internal/testutil/` — factories, test DB, test JWT

### Точки интеграции

| Сервис | Пакет | Env: переключение |
|---|---|---|
| LiveDune | `integration/livedune/` | `LIVEDUNE_MOCK=true` |
| TrustMe | `integration/trustme/` | `TRUSTME_MOCK=true` |
| Telegram Bot | `integration/telegram/` | `TELEGRAM_MOCK=true` |
| Email | `integration/email/` | `EMAIL_MOCK=true` |
| File Storage | `integration/storage/` | `STORAGE_MOCK=true` |

### Makefile targets

```makefile
make generate          # openapi.yaml → Go types + TS types
make dev               # docker-compose up + air (Go) + vite (web) + vite (tma)
make test-unit         # go test ./... (без integration tag) + vitest
make test-integration  # go test -tags=integration ./...
make test              # test-unit + test-integration
make lint              # golangci-lint + eslint
make migrate           # goose -dir migrations/ up
make build             # Docker images
```

### Этапы реализации

Конкретные файлы создаются AI-агентом при реализации каждого этапа. Каждый этап заканчивается проверяемым результатом:

1. **Scaffold + Health check** — config, dbutil, middleware, docker-compose, Makefile, CI, `GET /health`. Результат: запущенный сервер, 200 на health check
2. **Auth + Creator onboarding** — регистрация → скрининг (мок LiveDune) → модерация
3. **Campaigns + Applications** — создание кампании → каталог → заявка креатора
4. **Work submission** — сдача работы → приёмка → dispute
5. **Notifications** — Telegram-бот + email напоминания
6. **Polish** — E2E тесты, production hardening

### Дополнительные паттерны (из валидации)

**Транзакции:**
```go
err := dbutil.WithTx(ctx, pool, func(tx dbutil.DB) error {
    // всё внутри одной транзакции
    // tx передаётся в repository-методы
    return nil
})
```
Сервис вызывает `WithTx` когда нужна атомарность (создание + аудит-лог, одобрение заявки + обновление слотов).

**Pagination (offset для MVP):**
```json
GET /creators?limit=20&offset=40

{ "data": [...], "pagination": { "total": 200, "limit": 20, "offset": 40 } }
```
Единый формат для всех list-эндпоинтов. Cursor-based — Growth.

**Mock-переключение:**
- **При старте приложения (main.go):** env var `*_MOCK=true` → конструктор получает mock-реализацию
- **В тестах:** DI через конструктор, env vars не участвуют. `NewCreatorService(mockRepo, mockLiveDune, logger)`

**OpenAPI файл:**
- MVP: один `api/openapi.yaml` (~30 эндпоинтов — управляемо)
- При необходимости: разделить через `$ref` на `api/paths/*.yaml`

## Валидация архитектуры

### Когерентность решений ✅

- Go + Chi + oapi-codegen: бесшовная совместимость
- squirrel + pgx + dbutil: dbutil оборачивает pgx, принимает sq.Sqlizer
- React + Vite + openapi-typescript: TS types из того же OpenAPI YAML
- JWT + middleware + authz: чистое разделение ролевой и fine-grained проверки
- Contract-first: единый OpenAPI YAML → codegen для обеих сторон
- Dokploy + Docker Compose + Traefik: стандартная цепочка self-hosted
- Противоречий между решениями нет

### Покрытие требований ✅

**Функциональные домены (все 5 из PRD):**

| Домен | Backend | Frontend | Интеграции | Статус |
|---|---|---|---|---|
| Онбординг креаторов | handler → service → repository | TMA: onboarding | LiveDune, TrustMe | ✅ |
| Кампании | handler → service → repository | Web + TMA | — | ✅ |
| Заявки и работы | handler → service → repository | Web + TMA | Storage | ✅ |
| Бренды | handler → service → repository | Web | — | ✅ |
| Модерация (админ) | handler → service | Web: admin/ | — | ✅ |

**Cross-cutting (все 10):** RBAC ✅, аудит-лог ✅, нотификации ✅, изоляция данных ✅, auth ✅, file storage ✅, background jobs ✅, миграции ✅, порты и адаптеры ✅, тестовая инфраструктура ✅

**NFR:** безопасность ✅, производительность ✅, надёжность ✅, тестируемость ✅, observability ✅, локализация ✅

### Готовность к имплементации ✅

**Полнота:** все критичные решения задокументированы — стек, паттерны, границы, naming, валидация, authz, error handling, транзакции, pagination, процессы.

**Минорные пробелы (закрываются при реализации этапа 1):**
- Health check эндпоинт → добавляется в этапе 1
- Graceful shutdown → `signal.NotifyContext` в main.go
- Версии инструментов → фиксируются в go.mod / package.json
- Prometheus/Grafana → Growth

### Чеклист готовности

- [x] Контекст проекта проанализирован
- [x] Стек полностью специфицирован
- [x] Contract-first workflow определён
- [x] Архитектурные решения задокументированы с обоснованиями
- [x] Паттерны консистентности определены
- [x] Структура проекта определена (слои, не конкретные файлы)
- [x] Границы очерчены
- [x] Тестовая стратегия определена
- [x] Этапы реализации спланированы
- [x] Отложенные решения задокументированы

**Статус: ГОТОВО К ИМПЛЕМЕНТАЦИИ**
**Уровень уверенности: высокий**
