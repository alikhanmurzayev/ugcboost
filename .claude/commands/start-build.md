---
description: Старт реализационной сессии — prime + standards + bmad-quick-dev
---

Шорткат для запуска реализационной сессии. Выполни последовательно три шага. После каждого — краткий отчёт о готовности, потом сразу следующий, без паузы на подтверждение.

1. **Prime.** Прочитай и выполни инструкции из `.claude/commands/prime.md`.
2. **Standards.** Прочитай и выполни инструкции из `.claude/commands/standards.md`.
3. **Bmad-quick-dev.** Запусти skill `bmad-quick-dev`. Skill в step-01-clarify-and-route сам обнаружит готовую спецификацию (status `ready-for-dev` или `in-progress`) в `_bmad-output/implementation-artifacts/` и перейдёт к реализации.

После шага 3 управление переходит к skill bmad-quick-dev — продолжай по его workflow.
