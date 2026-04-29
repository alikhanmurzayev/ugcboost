---
title: "Прогресс: привязка Telegram-аккаунта к заявке (chunk 1 онбординга креатора)"
type: progress
status: in-progress
created: "2026-04-29"
plan: "_bmad-output/implementation-artifacts/29_04_2026_creator-telegram-link-plan.md"
scout: "_bmad-output/implementation-artifacts/29_04_2026_creator-telegram-link-scout.md"
---

# Прогресс: привязка Telegram-аккаунта к заявке

## В процессе
- [~] Шаг 1: обновить `docs/standards/backend-libraries.md`

## Осталось
- [ ] Шаг 2: OpenAPI prod + test, запустить generate-api
- [ ] Шаг 3: Config + .env
- [ ] Шаг 4: миграция creator_application_telegram_links
- [ ] Шаг 5: domain — типы, ошибки, константы
- [ ] Шаг 6: repository creator_application_telegram_link + pgxmock-тесты
- [ ] Шаг 7: service creator_application — расширить GetByID + RepoFactory
- [ ] Шаг 8 + 9: service creator_application_telegram + audit constants + unit-тесты
- [ ] Шаг 10: telegram package — client (real/noop/spy) + unit-тест
- [ ] Шаг 11: telegram package — dispatcher + unit-тест
- [ ] Шаг 12: telegram package — start handler + unit-тесты
- [ ] Шаг 13: telegram package — messages.go
- [ ] Шаг 14: telegram package — runner + unit-тесты
- [ ] Шаг 15: testapi.SendTelegramUpdate
- [ ] Шаг 16: cmd/api/main.go wiring
- [ ] Шаг 17: .mockery.yaml + generate-mocks
- [ ] Шаг 18: coverage gate (Makefile awk)
- [ ] Шаг 19: e2e/testutil/telegram.go
- [ ] Шаг 20: e2e тесты telegram_link_test.go
- [ ] Шаг 20a: PII inventory в legal-documents
- [ ] Шаг 21: roadmap chunk 1 → [~] + ссылки на артефакты
- [ ] Шаг 22: финальные проверки (build, lint, unit, coverage, e2e, PII-grep)
- [ ] Review №1 (`/review`)
- [ ] Apply fix-now из review №1
- [ ] Review №2 (`/review`)
- [ ] Финальный отчёт

## Блокеры
Пока нет.

## Заметки
- Ветка: `alikhan/creator-telegram-link` создана, локально, без push.
- Стандарты `docs/standards/` загружены полностью (20 файлов, по преамбуле плана).
- Коммитим по этапам для diff'а в `/review`. Push на remote запрещён.
- Для review №1 / №2 — режим `branch` (`git diff $(git merge-base origin/main HEAD)..HEAD`).
- HALT-точка `/review` пройдена в автоматическом режиме: применяем все fix-now (включая minor/nitpick); clarify/defer оставляем в отчёте без действия; inline-комменты в PR не постим (PR'а нет); кандидатов в стандарты не правим автоматически.
