# Чеклист ревью PR

Источник правды для ревьюер-агента (`/review`). Каждый раздел — обязательная стадия. Standards-auditor явно отмечает в отчёте незатронутые слои (см. инструкцию субагента).

Чеклист = роутинг по слоям → набор стандартов. Сами правила (с severity-метками) живут в секции `## Что ревьюить` каждого стандарта. Здесь — только handle + ссылка.

## Принципы

- **VETO Алихана.** Может приказать reclassify любого finding в `fix-now` без обоснования, поверх любых критериев. Выше всех правил.
- **Default = fix-in-PR.** Tech-debt issue — исключение, см. 5 закрытых критериев в `.claude/commands/review.md`.
- **Каждый слой обязателен.** Все разделы пройти, даже если diff пустой в нём.
- **Все стандарты `docs/standards/` — hard rules.** Отклонение = finding; severity берётся из `[severity]`-метки буллета `## Что ревьюить` соответствующего стандарта (`[blocker]` / `[major]` / `[minor]` / `[nitpick]`).

## Hard rules (cross-cutting)

Сквозные правила, применимые к нескольким слоям. Полный текст и `## Что ревьюить` — в соответствующем стандарте.

- Миграции на staging/prod — не in-place → `backend-repository.md` § Миграции
- Coverage gate на всю поверхность → `backend-testing-unit.md` § Coverage
- PII в stdout / error.Message → `security.md` § PII — где запрещена
- Аудит-лог в той же tx что и mutate → `backend-transactions.md` § Аудит-лог
- Business defaults в коде → `backend-repository.md` § Целостность данных
- Format checks в коде → `backend-repository.md` § Целостность данных

## Backend (Go)

- handler/ → `backend-architecture.md`, `backend-codegen.md`, `backend-errors.md`, `naming.md`, `security.md`
- service/ → `backend-architecture.md`, `backend-design.md`, `backend-transactions.md`, `backend-errors.md`, `backend-libraries.md`, `security.md`
- repository/ → `backend-architecture.md`, `backend-repository.md`, `backend-constants.md`, `backend-libraries.md`
- middleware/ + authz → `backend-architecture.md`, `security.md`
- domain/ → `backend-design.md`, `backend-errors.md`
- libraries/ → `backend-libraries.md`

## Frontend (web / tma / landing)

- API → `frontend-api.md`
- Types → `frontend-types.md`
- Components → `frontend-components.md`, `naming.md`
- State → `frontend-state.md`, `security.md` (auth flow / токены применимы только к web/tma; landing — статика без auth, эти разделы не применять)
- Quality → `frontend-quality.md`

## Database / Migrations

- SQL-файлы в `backend/migrations/` → `backend-repository.md` § Миграции, § Целостность данных. Прочие правила `## Что ревьюить` (про repo-код) к SQL-файлам не применяй — фильтруй по релевантности.

## API contracts (OpenAPI)

- → `backend-codegen.md`, `frontend-types.md`

## Tests

- Backend unit → `backend-testing-unit.md`, `backend-constants.md`
- Backend e2e → `backend-testing-e2e.md`
- Frontend unit → `frontend-testing-unit.md`
- Frontend e2e → `frontend-testing-e2e.md`

## Security

- → `security.md`

## Process / Artifacts

Не покрывается стандартами `docs/standards/` — это требования к процессу PR, не к коду. Ревьюер-агент проверяет в каждом PR:

- [major] Стандарты `docs/standards/` обновлены, если поменялись правила (living docs).
- [major] Артефакты планирования (`_bmad-output/planning-artifacts/`) — без Q&A истории, только итоговое состояние.
- [major] План/scout содержит преамбулу с требованием полной загрузки `docs/standards/`.
- [blocker] Source of truth для legal docs — `legal-documents/`. Копии в лендосе через `make sync-legal`. CI проверяет идентичность.
- [major] `CLAUDE.md` обновлён, если поменялся workflow.
- [minor] Коммит-сообщения concise, English, описывают «почему».
- [major] CI gate для нового инварианта добавлен (lint/test/sync-проверка).

## Расширение чеклиста

Чеклист — роутинг. Новое правило → пополни конкретный **стандарт** (раздел основной части + `## Что ревьюить`). Чеклист подтягивается автоматом, здесь правки не нужны.

Правки чеклиста требуются только когда:
- Появился новый слой (новый каталог в `backend/` или `frontend/`) → добавить строку в роутинг.
- Появился новый cross-cutting hard rule → добавить в «Hard rules».
- Появился новый стандарт (`docs/standards/<file>.md`) → подключить к роутингу слоёв, для которых он применим.

**Источники пополнения стандартов:**

1. **Сами `docs/standards/`** — формализованные правила. Любое новое правило в стандарте автоматом покрыто роутингом.
2. **`[чеклист-кандидат]` маркеры в живых PR-ревью** — субагенты ревьюера помечают повторяющиеся паттерны, которые тянут на проектное правило. Извлекаются через `gh api repos/.../pulls/<N>/comments | jq '.[] | select(.body | contains("чеклист-кандидат"))'`. Каждый кандидат — оценить и (если применимо) добавить в соответствующий стандарт.
3. `_bmad-output/decisions/` — архитектурные ADR (когда появятся).
