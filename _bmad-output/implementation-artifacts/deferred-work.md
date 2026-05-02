---
title: "Deferred work — отложенный тех-долг и сторонние сценарии"
type: backlog
status: living
created: "2026-05-02"
---

# Deferred work

Список заметок «не сейчас», накопленных в ходе adversarial-review-циклов чанков. Каждая запись — кандидат на отдельный chunk/PR, не блокирующий текущую работу.

## 2026-05-02 — adversarial review chunk 4 (admin list endpoint)

Источник — review subagents (acceptance-auditor + blind-hunter + edge-case-hunter) на `_bmad-output/implementation-artifacts/spec-creator-applications-list.md`. Большинство finding'ов закрыто в этом же PR; ниже — то, что осталось как отдельный долг.

## Кандидаты в стандарты `docs/standards/`

- **Handler enforce'ит OpenAPI bounds.** `oapi-codegen` не enforce'ит `minimum/maximum/maxLength/minLength/maxItems` на runtime. Каждый numeric/string/array параметр должен быть проверен явно перед использованием в SQL/математике. Кандидат в `docs/standards/security.md` или новый `docs/standards/api-validation.md`. Также нужен аудит существующих эндпоинтов: где OpenAPI задаёт bounds, а handler не проверяет.
- **Bounds guard в repo entry.** Repo не имеет права полагаться на bounds, навязанные верхними слоями. `int → uint64` cast без проверки ≥ 0 — finding `[blocker]`. Кандидат в `docs/standards/backend-repository.md` § Целостность данных.
- **ILIKE wildcards escape.** User-controlled input в `LIKE/ILIKE` через `'%' || input || '%'` без escape `%`/`_`/`\` ломает обещанную семантику "case-insensitive substring search". Кандидат в `docs/standards/backend-repository.md` или `docs/standards/security.md`.
