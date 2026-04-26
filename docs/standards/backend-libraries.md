# Библиотеки вместо велосипедов [REQUIRED]

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
| Случайные значения (id, токены, фикстуры) | `crypto/rand` (stdlib) |

### Правила

- **Поинтеры — `AlekSi/pointer`.** `pointer.ToString`, `pointer.GetString` для безопасного дереференса. Кастомные `ptrTo[T]`, `strptr`, инлайн `&local` — не писать.
- **Случайность — `crypto/rand`.** `math/rand` и `math/rand/v2` забанены в `golangci.yml` (depguard). Детерминированный seeded PRNG (fuzz, property-based) — `//nolint:depguard` с комментарием почему.
- Новая библиотека — через PR. Если в реестре уже есть библиотека для смежной задачи — расширяем, не добавляем вторую.
- Удаляя зависимость — обновить реестр тем же PR'ом.
