---
title: "Прогресс: привязка Telegram-аккаунта к заявке (chunk 1 онбординга креатора)"
type: progress
status: ready-for-review
created: "2026-04-29"
plan: "_bmad-output/implementation-artifacts/29_04_2026_creator-telegram-link-plan.md"
scout: "_bmad-output/implementation-artifacts/29_04_2026_creator-telegram-link-scout.md"
---

# Прогресс: привязка Telegram-аккаунта к заявке

## Выполнено
- [x] Шаг 1: обновлён `docs/standards/backend-libraries.md`
- [x] Шаг 2: OpenAPI prod + test, `make generate-api`
- [x] Шаг 3: Config + .env (`TELEGRAM_BOT_TOKEN`, `TELEGRAM_POLLING_TIMEOUT`)
- [x] Шаг 4: миграция `20260429224431_creator_application_telegram_links.sql`
- [x] Шаг 5: domain — типы, ошибки, константы (TelegramLink, TelegramLinkInput, TelegramLinkResult, sentinels, codes)
- [x] Шаг 6: repository `creator_application_telegram_link.go` + 9 pgxmock-тестов
- [x] Шаг 7: service `creator_application` — расширен `GetByID` + `RepoFactory`
- [x] Шаг 8 + 9: service `creator_application_telegram` + 14 unit-тестов + audit constant
- [x] Шаг 10: telegram package — client (real/noop/spy)
- [x] Шаг 11: telegram package — dispatcher (regex-routing, lower+original-text)
- [x] Шаг 12: telegram package — start handler
- [x] Шаг 13: telegram package — messages.go (русские reply-тексты)
- [x] Шаг 14: telegram package — PollingRunner (recover, retry-after, monotonic offset)
- [x] Шаг 15: `testapi.SendTelegramUpdate` (полное покрытие валидаций)
- [x] Шаг 16: cmd/api/main.go wiring (defer runnerCancel, switch-based reason logging)
- [x] Шаг 17: `.mockery.yaml` расширен; `make generate-mocks`
- [x] Шаг 18: coverage gate включает `integration/telegram`
- [x] Шаг 19: `e2e/testutil/telegram.go` (TelegramUpdateParams + DefaultTelegramUpdateParams + SendTelegramUpdate)
- [x] Шаг 20: e2e `telegram_link_test.go` — все 9 сценариев
- [x] Шаг 20a: `legal-documents/PII_INVENTORY.md`
- [x] Шаг 21: roadmap chunk 1 → `[~]` со ссылками на scout/plan
- [x] Шаг 22: финальные проверки (build/lint/unit/coverage/e2e/PII-grep — все зелёные)
- [x] Review №1: 17 + 19 + 27 findings, 32 fix-now применены
- [x] Apply fix-now из review №1
- [x] Review №2: 6 + 18 + 19 findings, 27 fix-now применены
- [x] Apply fix-now из review №2

## Шаг 23 (ручной)
- [ ] Roadmap финал: chunk 1 → `[x]` после merge — пользователю на ревью.

## Сводка изменений

**Итого 3 коммита на ветке `alikhan/creator-telegram-link`:**
1. `feat(telegram): bind Telegram account to creator application via /start` — base реализация по плану (60 файлов, +6323/-38).
2. `fix(telegram): apply review-1 findings` — 32 fix-now (security, runtime, validation, audit, tests).
3. `fix(telegram): apply review-2 findings` — 27 fix-now (panic-resilience, defensive guards, test coverage, sanitisation).

**Всего:** 6865 insertions, 38 deletions, 62 файла.

**Контракт:**
- Новая таблица `creator_application_telegram_links` (PK на `application_id`, UNIQUE+CHECK на `telegram_user_id`, ON DELETE CASCADE).
- Новый POST `/test/telegram/send-update` (test-endpoint).
- Расширенный GET `/creators/applications/{id}` (поле `telegramLink`).

**Новые тесты:** 14 unit (service) + 9 unit (repository) + 6 unit (dispatcher) + 8 unit (start_handler) + 5 unit (runner) + 9 unit (factory/messages/spy) + 7 unit (testapi handler) + 1 unit (middleware WithClientIP) + 9 e2e сценариев = **68 новых тестовых случаев**.

## Финальные проверки (зелёные)

| Шаг | Команда | Результат |
|---|---|---|
| Build | `make build-backend` | OK |
| Lint | `make lint-backend` | 0 issues |
| Unit | `make test-unit-backend` | OK (race detector on) |
| Coverage gate | `make test-unit-backend-coverage` | OK (≥80% per-method на handler/service/repository/middleware/authz/integration/telegram) |
| E2E | `make test-e2e-backend` | все пакеты PASS (auth/audit/brand/creator_application/dictionary/telegram) |
| PII guard | `docker logs --since 60s ugcboost-backend-1` после happy path | 0 совпадений по IIN/phone/handle/username/first_name/last_name |
| Migration | `goose -dir backend/migrations ... up` | OK |
| Mockery | `make generate-mocks` | OK (новый telegram-пакет добавлен в .mockery.yaml) |
| Generated | `make generate-api` | diff только в gen-файлах |

## Зафиксированные решения (в дополнение к плану)

- **`go-telegram/bot` библиотека отброшена** — публичный API не предоставляет `GetUpdates(offset, timeout)` напрямую, polling-цикл закрытый. Реализация через stdlib `net/http` (см. addendum в plan.md и обновлённый реестр библиотек).
- **`linked_at` без DB DEFAULT** — service stamps `now` явно (per backend-repository.md § business defaults).
- **`telegram_user_id > 0`** — defensive CHECK в миграции.
- **`audit_logs.ip_address = "telegram-bot"`** — постоянный маркер для бот-пути (бот не имеет реального user IP, перетираем contextually).
- **Recovered panic**: offset продвигается, но логируется warn о возможной потере reply (at-least-once + idempotency покрывают повторное /start от пользователя).
- **`allowed_updates: ["message"]`** в getUpdates — фильтрация на стороне Telegram, экономит трафик.
- **Token sanitisation**: `*url.Error` редактируется (URL → `<redacted>`) перед возвратом, чтобы slog не залогил token.

## Открытые вопросы (для ревью с Алиханом)

Эти findings из review'а оставлены как clarify (бизнес/design-решения, не подходят под автономный fix-now):

1. **`messages.go: ApplicationAlreadyLinked` vs `AccountAlreadyLinked`** — тонкая разница в формулировке для user UX. Бизнес-копирайт.

2. **`cmd/api/main.go: runner не стартует при `EnableTestEndpoints=true`** — на staging бот через `t.me/staging_bot` физически молчит. Намеренно (test-endpoint-only) или баг? Если намеренно — добавить warning + докку. Если баг — выкинуть условие `!cfg.EnableTestEndpoints` и сделать отдельный флаг для бота.

3. **`isLinkableStatus` разрешает `blocked`** — заблокированный креатор может привязать новый Telegram. Anti-fraud bypass или ОК (link нужен админу, чтобы написать заблокированному)?

4. **Idempotent /start не обновляет username/first_name/last_name** — фиксируем link-time или latest. Сейчас link-time. Тест на этот контракт явный.

5. **Client SRP** — interface объединяет SendMessage + GetUpdates, dispatcher/start_handler используют только Send. Рефактор на Sender + Updater? Сейчас single Client + spy comment "GetUpdates noop in test mode" — приемлемо, но можно почистить.

6. **`looksLikeCanonicalUUID` + `uuid.Parse` двойная защита** — оставлено как defensive (комментарий объясняет). Можно полностью заменить на `uuid.ValidateStrict` если выйдет в библиотеке.

## Блокеры
Нет.

## Следующие шаги
- **Алихан**: ручное ревью кода, тестирование через реальный @BotFather-токен на staging (deploy этой ветки + установка `TELEGRAM_BOT_TOKEN`), решение 5 clarify-вопросов выше.
- **После approve**: создать PR (push нужен), code review, merge в main → перевести chunk 1 → `[x]` в roadmap.
- **Smoke-test после merge**: открыть `t.me/<staging_bot>?start=<application_id>`, убедиться что link создаётся и приходит "Заявка успешно связана…".

## Заметки
- Все артефакты chunk 1 в `_bmad-output/implementation-artifacts/29_04_2026_creator-telegram-link-{scout,plan,progress}.md`.
- Ветка `alikhan/creator-telegram-link` создана локально, не push'нута.
- Build phase выполнен Claude Code в автономном режиме (билд → review №1 → fix-now → review №2 → fix-now); все коммиты с маркером `Co-Authored-By: Claude Opus 4.7`.
- HALT-точки `/review` пропущены автоматически; clarify-вопросы (5 штук) накоплены в этом файле для решения Алихана.
