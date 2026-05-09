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
| Retry с backoff'ом (transient errors) | `github.com/cenkalti/backoff/v5` |
| PDF text + bbox extraction | `github.com/ledongthuc/pdf` |
| PDF overlay rendering (TrustMe outbox) | `github.com/signintech/gopdf` |
| Rate-limit для исходящих HTTP-вызовов | `golang.org/x/time/rate` |

### Правила

- **Поинтеры — `AlekSi/pointer`.** `pointer.ToString`, `pointer.GetString` для безопасного дереференса. Кастомные `ptrTo[T]`, `strptr`, инлайн `&local` — не писать.
- **Случайность — `crypto/rand`.** `math/rand` и `math/rand/v2` забанены в `golangci.yml` (depguard). Детерминированный seeded PRNG (fuzz, property-based) — `//nolint:depguard` с комментарием почему.
- **PDF extraction — `ledongthuc/pdf`.** Используется только в `internal/contract/Extractor` для валидации шаблонов договора и chunk-16 outbox-render'а. Достаёт глифы с координатами per word (X/Y/FontSize), что критично для overlay-рендера. Mainstream pure-Go альтернатив без CGo нет: `pdfcpu` не отдаёт bbox per глиф, MuPDF доступен только через CGo. Production-код — только в этом пакете; тестовые фикстуры PDF генерируются `github.com/jung-kurt/gofpdf` в test files (не production dep).
- **PDF overlay — `signintech/gopdf`.** Парная библиотека к `ledongthuc/pdf` для chunk-16 `ContractPDFRenderer`. Импортирует страницу шаблона через `ImportPage(file, n, "/MediaBox")` и рисует поверх белые прямоугольники + текст значений. Pure-Go без CGo, без sidecar'ов; шрифт читается из embedded `internal/contract/fonts/LiberationSerif-Regular.ttf`. Альтернатива `pdfcpu` для overlay не подходит — нет bbox-aware rendering API.
- **Rate-limit — `golang.org/x/time/rate`.** Используется в `internal/trustme/RealClient` под TrustMe blueprint требование «не более 4 RPS». `rate.NewLimiter(rate.Limit(4), 1)` — sliding window без накопления budget'а. Самописная реализация (channel + ticker) даст ту же семантику, но проигрывает в тестируемости (rate.Limiter принимает ctx и корректно отменяется). Обязательно использовать `Limiter.Wait(ctx)` перед каждым исходящим запросом.
- Новая библиотека — через PR. Если в реестре уже есть библиотека для смежной задачи — расширяем, не добавляем вторую.
- Удаляя зависимость — обновить реестр тем же PR'ом.

## Что ревьюить

- [blocker] `math/rand` или `math/rand/v2` (depguard banned — использовать `crypto/rand`).
- [major] Самописная утилита для задачи, для которой есть библиотека в реестре (без обоснования в комментарии кода или в PR-описании).
- [major] Ручной `&local := value; &local` или `ptrTo[T]` вместо `pointer.ToString` / `pointer.GetString`.
- [minor] Зависимость удалена из `go.mod`, но реестр не обновлён тем же PR'ом.
