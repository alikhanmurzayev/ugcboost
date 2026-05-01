# Deferred Work

Найденное в ревью, но вне scope конкретных PR'ов. Каждая запись — кандидат
на отдельный спринт-чанк или backlog-issue.

## 2026-05-01 — review of `creator-application-state-machine`

### CONCURRENTLY для перестройки partial unique index в миграциях
- **Источник:** blind hunter, edge-case hunter (review chunk 3 state-machine)
- **Что:** `DROP INDEX` / `CREATE UNIQUE INDEX` / `ALTER TABLE ADD CONSTRAINT`
  без `CONCURRENTLY` берут ACCESS EXCLUSIVE LOCK на `creator_applications`,
  блокируя R/W-трафик на время миграции.
- **Почему отложено:** проект pre-launch, прод-трафика по этой таблице нет.
  Паттерн распространяется на все миграции — это архитектурное решение, не
  специфичное для конкретной фичи. Также backfill требует атомарной транзакции
  (между транзитным и финальным CHECK), а `CREATE INDEX CONCURRENTLY` в
  транзакции не работает. Решение: разбивать миграцию на 2-3 файла с
  `-- +goose NO TRANSACTION` либо вводить операционную процедуру для
  data-миграций enum'ов.
- **Когда поднять:** перед public launch / перед первым ростом трафика по
  заявкам.

### Прямой каст domain-string в OpenAPI enum (`handler/creator_application.go:202`)
- **Источник:** blind hunter, edge-case hunter
- **Что:** `api.CreatorApplicationDetailDataStatus(d.Status)` беззвучно пропустит
  невалидное значение в response, нарушая контракт OpenAPI клиента.
- **Почему отложено:** pre-existing код, не введён этим PR. Сейчас CHECK + домен
  гарантируют синхронизацию; рассинхрон возможен только при ручном UPDATE или
  частично применённой миграции.
- **Когда поднять:** при следующем расширении state machine (chunks 4-7) — там
  поверхность каста будет шире, и валидация через `.Valid()` станет осмысленной
  частью контракта.

### Concurrent race-test для partial unique index по ИИН
- **Источник:** acceptance auditor (gap по `backend-testing-e2e.md`)
- **Что:** `TestSubmitCreatorApplicationDuplicate` — последовательный duplicate-test,
  не concurrent. Стандарт `backend-testing-e2e.md` требует настоящий race-тест
  (две goroutine с одинаковым ИИН), чтобы partial unique index фактически
  гарантировал отсутствие дублей.
- **Почему отложено:** pre-existing gap; pure unit-test покрывает обработку
  pgErr 23505 → ErrCreatorApplicationDuplicate.
- **Когда поднять:** в чанке тестов / отдельным PR на enforcement стандарта.

### Migration-level integration tests
- **Источник:** edge-case hunter
- **Что:** Нет автоматизированного теста, проверяющего поведение миграций на
  непустой БД (backfill `pending → verification`, fail-fast на `approved/blocked`,
  Down с rejected).
- **Почему отложено:** проект сейчас полагается на manual smoke и e2e после Up.
  Отдельная инфраструктура для миграционных тестов (testcontainers + старые
  схемы) — большая работа.
- **Когда поднять:** после первого инцидента с миграцией на staging/prod, либо
  при добавлении следующих enum-расширений.
