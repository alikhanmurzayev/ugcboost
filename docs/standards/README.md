# Стандарты кода UGCBoost

## Как использовать этот документ

- **Обязателен к прочтению** перед написанием любого кода в проекте
- При code review ссылаться на ID правила (например, CS-04)
- Правила извлечены из ревью PR #14 (Epic 1) и аудита фронтенд-кода, фиксируют ошибки, которые не должны повторяться

## Уровни строгости

| Уровень | Значение |
|---------|----------|
| **[CRITICAL]** | Нарушение = баг, дыра безопасности или потеря данных. Блокирует merge |
| **[REQUIRED]** | Нарушение = технический долг, хрупкость кода. Блокирует merge |
| **[STANDARD]** | Нарушение = снижение читаемости. Рекомендуется исправить, не блокирует merge |

## Scope

- **backend** — правило относится к Go-коду (backend/, e2etest/)
- **frontend** — правило относится к TypeScript/React (web/, tma/)
- **both** — применяется везде

## Оглавление

| Файл | Правила | Уровень | Описание |
|------|---------|---------|----------|
| [01-constants.md](01-constants.md) | CS-01 — CS-05 | CRITICAL | Константы вместо строковых литералов |
| [02-codegen.md](02-codegen.md) | CS-06 — CS-11 | REQUIRED | Использование кодогенерации |
| [03-libraries.md](03-libraries.md) | CS-12 — CS-14 | REQUIRED | Библиотеки вместо велосипедов |
| [04-architecture.md](04-architecture.md) | CS-15 — CS-18 | REQUIRED | Слои архитектуры |
| [05-error-handling.md](05-error-handling.md) | CS-19 — CS-23 | CRITICAL | Обработка ошибок |
| [06-design.md](06-design.md) | CS-24 — CS-28 | REQUIRED | Архитектура и проектирование |
| [07-repository.md](07-repository.md) | CS-29 — CS-31 | REQUIRED | Паттерны репозитория |
| [08-testing.md](08-testing.md) | CS-32 — CS-34 | REQUIRED | Тестирование |
| [09-naming.md](09-naming.md) | CS-35 — CS-37 | STANDARD | Нейминг и стиль |
| [10-security.md](10-security.md) | CS-38 — CS-40 | CRITICAL | Безопасность |
| | | | **Фронтенд** |
| [11-frontend-types.md](11-frontend-types.md) | CS-41 — CS-44 | CRITICAL | Типы и кодогенерация |
| [12-frontend-api.md](12-frontend-api.md) | CS-45 — CS-48 | REQUIRED | API-слой |
| [13-frontend-components.md](13-frontend-components.md) | CS-49 — CS-53 | REQUIRED | Компоненты и UI |
| [14-frontend-state.md](14-frontend-state.md) | CS-54 — CS-57 | REQUIRED | Состояние и авторизация |
| [15-frontend-quality.md](15-frontend-quality.md) | CS-58 — CS-62 | REQUIRED | Качество и надёжность |
