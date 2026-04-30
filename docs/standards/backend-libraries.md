# Библиотеки вместо велосипедов

Перед написанием утилитарного кода — проверить реестр ниже + Awesome Go / pkg.go.dev. Самописное решение — только если нет адекватной альтернативы; обоснование (нет аналогов, заброшены, неприемлемый транзитивный граф) — в комментарии или PR-описании. Кастомные хелперы для стандартных задач не пишем.

## Каноничный реестр библиотек

| Задача | Библиотека |
|---|---|
| `*T` из значения (опциональные поля) | `github.com/AlekSi/pointer` |
| Конфиг из env | `github.com/caarlos0/env/v11` |
| `.env` в local | `github.com/joho/godotenv` |
| HTTP-роутер | `github.com/go-chi/chi/v5` |
| OpenAPI codegen (Go) | `github.com/oapi-codegen/oapi-codegen` |
| Postgres-драйвер и пул | `github.com/jackc/pgx/v5` |
| SQL-builder | `github.com/Masterminds/squirrel` |
| Маппинг struct→map по тегам (INSERT) | `github.com/elgris/stom` |
| JWT | `github.com/golang-jwt/jwt/v5` |
| Хеширование паролей | `golang.org/x/crypto/bcrypt` |
| Cron | `github.com/robfig/cron/v3` |
| Тестовые ассерты + моки | `github.com/stretchr/testify` |
| Кодогенерация моков | `github.com/vektra/mockery` |
| Pgx-моки в unit-тестах репо | `github.com/pashagolub/pgxmock/v4` |
| Telegram Bot API клиент | `github.com/go-telegram/bot` |
| Случайные значения (id, токены, фикстуры) | `crypto/rand` (stdlib) |

### Правила

- **Поинтеры — `AlekSi/pointer`.** `pointer.ToString`, `pointer.GetString` для безопасного дереференса. Кастомные `ptrTo[T]`, `strptr`, инлайн `&local` — не писать.
- **Случайность — `crypto/rand`.** `math/rand` и `math/rand/v2` забанены в `golangci.yml` (depguard). Детерминированный seeded PRNG (fuzz, property-based) — `//nolint:depguard` с комментарием почему.
- Новая библиотека — через PR. Если в реестре уже есть библиотека для смежной задачи — расширяем, не добавляем вторую.
- Удаляя зависимость — обновить реестр тем же PR'ом.

## Что ревьюить

- [blocker] `math/rand` или `math/rand/v2` (depguard banned — использовать `crypto/rand`).
- [major] Самописная утилита для задачи, для которой есть библиотека в реестре (без обоснования в комментарии кода или в PR-описании).
- [major] Ручной `&local := value; &local` или `ptrTo[T]` вместо `pointer.ToString` / `pointer.GetString`.
- [minor] Зависимость удалена из `go.mod`, но реестр не обновлён тем же PR'ом.
