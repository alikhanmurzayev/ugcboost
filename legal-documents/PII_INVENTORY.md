# PII Inventory — UGCBoost

> **Living document.** Каждая новая колонка БД, содержащая PII или legal-данные,
> обязана попасть сюда тем же PR'ом, в котором она добавлена. Без записи здесь
> compliance не сможет ответить на DSR-запрос — «что вы хранили обо мне».
>
> Источник правды для legal-обработки — `ПОЛИТИКА ОБРАБОТКИ ПЕРСОНАЛЬНЫХ ДАННЫХ UGCBOOST.md`.

## Что считается PII

Любые данные, по которым можно идентифицировать физическое лицо: ФИО, ИИН,
телефон, адрес, email, handle в соцсетях, IP, User-Agent, Telegram identifiers
и метаданные. Бизнес-метаданные без привязки к идентификации (timestamps,
status, document_version) — не PII, но фиксируем для аудита.

## Поля по таблицам

### `creator_applications`

Заявка креатора, поданная через публичную форму на лендинге.

| Поле | Тип | Зачем хранится | Legal basis | Retention |
|---|---|---|---|---|
| `last_name` | TEXT NOT NULL | Идентификация заявителя для модерации и подписания договора | Согласие (privacy-policy §3.1) | Срок жизни заявки + 1 год после отказа / закрытия |
| `first_name` | TEXT NOT NULL | То же | Согласие | То же |
| `middle_name` | TEXT NULL | Опциональное отчество (не у всех есть) | Согласие | То же |
| `iin` | TEXT NOT NULL | Уникальная идентификация налогового резидента РК; контроль 18+; защита от дублей | Согласие + legitimate interest (anti-fraud); ст. 71-73 НК РК | Срок жизни заявки + 1 год после отказа / закрытия |
| `birth_date` | DATE NOT NULL | Деривативно из ИИН — для возрастной проверки (18+) | Legitimate interest (compliance) | То же что `iin` |
| `phone` | TEXT NOT NULL | Контакт креатора, верификация identity при подписании | Согласие | Срок жизни заявки + 1 год |
| `city` | TEXT NOT NULL | Логистика съёмок, маркетинговая аналитика регионов | Согласие | Срок жизни заявки + 1 год |
| `address` | TEXT NULL | Юридический адрес для договора (заполняется ботом/админом, не лендингом) | Согласие | Срок жизни договора + 5 лет (по налоговому учёту) |

### `creator_application_socials`

Список соцсетей с handle, привязанных к заявке.

| Поле | Тип | Зачем хранится | Legal basis | Retention |
|---|---|---|---|---|
| `handle` | TEXT NOT NULL | Публичный идентификатор креатора в соцсети — для верификации владения и интеграций (LiveDune метрики) | Согласие | Срок жизни заявки + 1 год |

### `creator_application_consents`

Per-type запись о принятии legal-документов.

| Поле | Тип | Зачем хранится | Legal basis | Retention |
|---|---|---|---|---|
| `ip_address` | TEXT NOT NULL | Доказательство момента акцепта (anti-fraud, споры по согласию) | Legitimate interest | Срок жизни заявки + 5 лет (по сроку давности гражданских исков) |
| `user_agent` | TEXT NOT NULL (capped 1024) | То же — фиксируем устройство/браузер на момент акцепта | Legitimate interest | То же |
| `document_version` | TEXT NOT NULL | Какая ревизия privacy-policy / user-agreement действовала на момент акцепта | Legal obligation (доказательство какие правила действовали) | То же |

### `creator_application_telegram_links`

Привязка Telegram-аккаунта к заявке (chunk 1 онбординга креатора).

| Поле | Тип | Зачем хранится | Legal basis | Retention |
|---|---|---|---|---|
| `telegram_user_id` | BIGINT NOT NULL UNIQUE | Стабильный идентификатор Telegram-аккаунта — единственная сущность, по которой бот находит заявку для последующих действий (статус, уведомления) | Согласие (через privacy-policy на лендинге, шаг submit) + legitimate interest (бот — операционный канал) | Срок жизни заявки; удаляется через `ON DELETE CASCADE` при cleanup заявки |
| `telegram_username` | TEXT NULL | Удобство модерации (видно админу в админке) | Согласие | То же |
| `telegram_first_name` | TEXT NULL | То же | Согласие | То же |
| `telegram_last_name` | TEXT NULL | То же | Согласие | То же |
| `linked_at` | TIMESTAMPTZ NOT NULL | Аудит момента привязки | Legitimate interest | То же |

### `audit_logs`

Лог всех модифицирующих операций. **Допустимо** содержать PII (это
специализированное хранилище с собственным retention) — см. security.md.

| Поле | Тип | Зачем хранится | Legal basis | Retention |
|---|---|---|---|---|
| `actor_id` | UUID NULL | Кто инициировал действие (NULL для публичных endpoint'ов) | Legitimate interest (audit trail) | 5 лет (compliance) |
| `actor_role` | TEXT NOT NULL | Роль актёра на момент действия | Legitimate interest | 5 лет |
| `entity_type` / `entity_id` | TEXT / UUID | На какую сущность действовали | Legitimate interest | 5 лет |
| `old_value` / `new_value` | JSONB | Снимок изменения; **может содержать PII** (для creator_application_link_telegram несёт telegram_user_id + username + first_name + last_name) | Legitimate interest (доказательство для compliance / споров) | 5 лет |
| `ip_address` | TEXT NOT NULL | IP инициатора действия | Legitimate interest (anti-fraud) | 5 лет |

## Где PII запрещена

- **stdout / structured logs приложения** (`logger.Info` / `Debug` / `Warn`).
  В лог идут только идентификаторы: UUID заявки, user ID, telegram_user_id,
  request/trace ID, IP, HTTP-параметры. Имена/handle/ИИН/телефон — никогда.
- **`error.Message`** (попадает в response body, доступен злоумышленнику).
- **URL params** (query string или path segment — попадают в access-логи).

PII-guard e2e тест grep'ает stdout по известным значениям и фейлится при
утечке (см. `backend/e2e/.../*PIIGuard` тесты).

## Sub-processors

- **Telegram (Bot API)** — обмен сообщениями с креатором. Хранит сообщения
  бота на своих серверах согласно своей privacy policy. Передаём только
  ответы бота, ИИН/имена не отправляем.
- **TrustMe** — подписание договора (chunk 6). Передаются ФИО, ИИН, телефон,
  email — необходимо для электронной подписи.
- **LiveDune** — метрики соцсетей (chunk 4). Передаётся только handle.
- **Postgres (managed на нашем VPS)** — основное хранилище.
- **Dokploy** — оркестратор Docker, хранит env vars и логи stdout.

## Ответы на DSR (data subject request)

Если креатор запрашивает «удалите все мои данные» — DELETE по `creator_applications.id`.
`ON DELETE CASCADE` на FK во всех связанных таблицах (categories, socials,
consents, telegram_links) гарантирует, что после одного DELETE в БД не
остаётся ни одной строки про этого креатора, кроме `audit_logs`. Audit
сохраняем (legal obligation), но при необходимости можем pseudonymise
`actor_id` / `entity_id` через отдельный скрипт.
