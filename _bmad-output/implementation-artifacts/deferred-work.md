---
title: "Deferred work — отложенный тех-долг и сторонние сценарии"
type: backlog
status: living
created: "2026-05-02"
---

# Deferred work

Список заметок «не сейчас», накопленных в ходе adversarial-review-циклов чанков. Каждая запись — кандидат на отдельный chunk/PR, не блокирующий текущую работу.

## 2026-05-02 — adversarial review chunk 4 (admin list endpoint)

Источник — review subagents (acceptance-auditor + blind-hunter + edge-case-hunter) на `_bmad-output/implementation-artifacts/spec-creator-applications-list.md`. Patch-findings закрыты в этом же PR; defer-findings зафиксированы здесь.

- **Read-only TX wrapper для list/count.** Сейчас `creatorApplicationRepository.List` делает count и page query двумя независимыми round-trip'ами. Между ними параллельный admin/Submit может изменить набор → total и `len(items)` рассходятся. То же касается `NOW()::date` в age-фильтре: на пересечении полуночи count и page видят разную "сегодня". Решение — обернуть `List` в `BEGIN; SET TRANSACTION ISOLATION LEVEL REPEATABLE READ; ... COMMIT` в repo (или service). Не критично для админ-UI на текущих масштабах, но снимает нагрузку с UI на повторные запросы.
- **EXISTS subquery + cross-column equality через builder API.** `applyCreatorApplicationListFilters` сейчас собирает условие `cac.application_id = ca.id` как сырую строку (`Where(cacAppID + " = " + caID)`). Squirrel поддерживает это идиоматичнее. Также вся `search`-ветка собрана как один большой `sq.Expr`. Можно перевести на `sq.Or{}` со связанными `sq.Expr` для каждой ILIKE-проверки.
- **`creatorApplicationListSelectColumns` через stom.** Сейчас projection строится вручную (`alias.col AS col`) вместо предвычисленного списка через stom над `CreatorApplicationListRow`. Для базы существующих (`creatorApplicationSelectColumns` через stom) это inconsistency.
- **Dictionary cache в `DictionaryService.List`.** На каждом /list-запросе сейчас два доп. SQL-запроса (categories + cities). На 200 RPS это 400 dictionary queries/s. Кеш на 10–60 секунд снимает нагрузку и не теряет консистентность для админ-UI.
- **NTP drift в e2e dateFrom-тесте.** `marker := time.Now().UTC()` сравнивается с server-side `created_at`. На NTP drift > 1 сек тест может флакать. Решение — отдать server-time через `/test/now` (test-only endpoint) и использовать его как marker. Сейчас покрыто sleep'ом 1100ms, что снижает риск, но не устраняет.
- **`UniqueIIN` modulo 10000 collision.** При параллельных `go test -p N` процессах счётчик в каждом процессе свой, и теоретически два процесса могут попытаться вставить ряд с одним IIN → 409. Маловероятно на текущих масштабах, но pattern увеличивает риск с ростом числа e2e helper'ов. Решение — заменить modulo на nanoTimestamp + atomic counter, либо подмешать pid в seed.
- **PII guard test для Submit-ручки.** Стандарт `backend-testing-e2e.md` § PII guard test требует guard для mutate-ручек, принимающих PII. Submit (POST /creators/applications) подпадает, но сейчас guard'а нет ни в `creator_application/`, ни в моём чанке (т. к. /list — read-only). Нужен отдельный chunk: либо `docker logs --since` (как был prototype), либо test-only endpoint, читающий буфер `logger.Logger`-а из in-memory adapter'а. Второй путь надёжнее в CI без docker.
- **Test-boilerplate в handler `TestServer_ListCreatorApplications`.** ~14 t.Run-кейсов с одинаковой обвязкой `authz/creator/dict/router`. Helper-функция вроде `newListEndpointHarness(t, opts)` сократит файл в 4 раза. Не баг — refactor opportunity.
- **Default branch в `applyCreatorApplicationListOrder`.** Сейчас silent fallback на `created_at DESC`. Service+handler уже rejects unknown sort, поэтому ветка недостижима. Альтернатива — `panic("creator_application_repo: unwhitelisted sort %q", sort)` для loud failure при будущем drift'е. Текущее silent-поведение consistent с защитой, но скрывает регресс. Tradeoff.
- **`creator_applications_test`-package с подчёркиванием.** Локальное исключение из `naming.md` (короткие имена пакетов без `_`), наследует существующий pattern в `creator_application/`. Вариант — переименовать оба каталога в `creatorapplications/` и `creatorapplication/` единым PR'ом.

## Кандидаты в стандарты `docs/standards/`

Свежие правила, всплывшие в ходе review. Не добавлены в `docs/standards/` сейчас, чтобы не размазывать scope chunk 4 — собираются здесь как pull для отдельного PR.

- **Handler enforce'ит OpenAPI bounds.** `oapi-codegen` не enforce'ит `minimum/maximum/maxLength/minLength/maxItems` на runtime. Каждый numeric/string/array параметр должен быть проверен явно перед использованием в SQL/математике. Кандидат в `docs/standards/security.md` или новый `docs/standards/api-validation.md`.
- **Read-only TX для list+count round-trip'ов.** Любой list-эндпоинт с count + page без shared snapshot имеет race с concurrent writes. Кандидат в `docs/standards/backend-repository.md` § Pagination или § Read consistency.
- **Bounds guard в repo entry.** Repo не имеет права полагаться на bounds, навязанные верхними слоями. `int → uint64` cast без проверки ≥ 0 — finding `[blocker]`. Кандидат в `docs/standards/backend-repository.md` § Целостность данных.
- **ILIKE wildcards escape.** User-controlled input в `LIKE/ILIKE` через `'%' || input || '%'` без escape `%`/`_`/`\` ломает обещанную семантику "case-insensitive substring search". Кандидат в `docs/standards/backend-repository.md` или `docs/standards/security.md`.
- **`_ := ToSql()` discard error.** Сабильдер может теоретически провалиться (новый аргумент → invalid type, например). Discarding error через blank даёт silent broken SQL. Кандидат в `docs/standards/backend-errors.md`.
- **PII guard для mutate-ручек: docker logs vs in-memory buffer.** `docker logs --since` хрупок (silent skip без docker, не работает в нек-CI). Альтернатива — test-only endpoint, возвращающий буфер `logger.Logger` из in-memory adapter. Кандидат в `docs/standards/backend-testing-e2e.md` § PII guard test (расширить раздел практическими решениями).
