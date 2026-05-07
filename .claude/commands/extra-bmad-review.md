---
description: Доп. раунд ревью bmad-quick-dev — тесты, frontend codegen, security, manual QA
---

Запусти ещё один раунд step-04 review из `bmad-quick-dev`. Помимо стандартных трёх ревьюеров — параллельно запусти этих дополнительных субагентов с тем же diff'ом и доступом к `docs/standards/`. Их findings обработай в общем дедупе/классификации:

- **test-auditor** — ревью тестов в diff'е: полнота покрытия (каждая ручка / ветка / public e2e), конкретность assert'ов (поля сверены с **конкретными ожидаемыми значениями**, не на «не пусто» / «есть поле») и в unit, и в e2e, код-стайл по `docs/standards/{backend,frontend}-testing-*.md`.
- **frontend-codegen-auditor** — фронт строго использует только сгенерированные API-типы из `api/generated/schema.ts` и openapi-fetch клиент: никаких ручных `interface`/`type` для request/response/query/path-params, никаких raw `fetch()`.
- **security-auditor** — кибербезопасность по `docs/standards/security.md` и смежным.
- **manual-qa** — поднимает релевантные сервисы (`make start-*`), открывает приложение через Playwright MCP, тыкает изменённые user flows, ловит регрессии в смежных фичах. По завершении — стопает сервисы.
