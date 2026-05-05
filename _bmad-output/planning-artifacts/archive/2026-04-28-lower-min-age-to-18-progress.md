# Прогресс: понизить минимальный возраст с 21 до 18

## Выполнено

- [x] Шаг 1: Создана ветка `alikhan/lower-creator-min-age` от `origin/main` (4e8cfc9), отслеживает `origin/main`. Untracked-файлы (scout/plan + `infra/_step.sh`) не затронуты.
- [x] Шаг 2: `backend/internal/domain/iin.go` — `MinCreatorAge = 18`, godoc-комментарий перефразирован (PRD FR3 + legal floor RK, без журнала изменений).
- [x] Шаг 3: `backend/internal/domain/iin_test.go` — даты подтестов `under MinCreatorAge by one day` и `exactly MinCreatorAge today` привязаны к константе (`time.Date(2005+MinCreatorAge, …)`); прокомментированы кратко («Anchored on MinCreatorAge…»).
- [x] Шаг 4: `backend/e2e/creator_application/creator_application_test.go:580` — `const minAge = 18`; комментарий `mirrors domain.MinCreatorAge` сохранён.
- [x] Шаг 5 (добавлен follow-up'ом по review): `frontend/e2e/helpers/api.ts:15` — `MIN_CREATOR_AGE = 18`. Shared helper landing/web/tma e2e; landing-spec под-возрастной валидации полагается на эту константу. Был пропущен в первоначальном scope (scout/plan не вышел из `frontend/{web,tma,landing}/src/`); найден review'ом, исправлен здесь.
- [x] Шаг 6: `frontend/landing/src/content.ts:94,97` — обе строки `"Тебе 21+"` → `"Тебе 18+"`.
- [x] Шаг 7: `frontend/landing/src/api/client.ts:25` — пример в комментарии «Возраст менее 21 лет» → «Возраст менее 18 лет».
- [x] Шаг 8: `_bmad-output/implementation-artifacts/spec-creator-application-submit.md:204,228` — пункт 4 переписан в финальную форму («MinCreatorAge = 18, EFW-фильтр был включён 2026-04-25 и снят 2026-04-28»); значение константы в § «Сопутствующие изменения на бэке» обновлено.
- [x] Шаг 9: Локальные гейты — см. ниже.

## В процессе

— (нет)

## Осталось

- [ ] Шаг 10: Передать на ревью Alikhan. По memory `feedback_no_commits` / `feedback_no_merge` — Claude не коммитит и не пушит сам. Изменения в working tree, ждут ручного review/commit/push.

## Блокеры

— нет.

## Заметки

- `feedback_no_commits` соблюдён: ни одного `git commit` не выполнено. Все 6 файлов лежат в working tree готовыми к ревью.
- Backend lint: `0 issues`. Landing lint: tsc + eslint без сообщений. Landing build: 3 страницы за 1.85s.
- Backend unit-тесты: все пакеты `ok` (с `-race`).
- Coverage gate: handler 95.3%, service 96.2%, repo 99.5%, middleware/authz/closer/logger 100%. Domain 75.4% — но domain исключён из per-method gate (фильтр в Makefile только на `handler|service|repository|middleware|authz`).
- E2E backend: пакеты `creator_application` (7.34s) и `dictionary` (1.05s) — PASS. Ключевой подтест `TestSubmitCreatorApplicationValidation/under_MinCreatorAge_rejected_with_UNDER_AGE` — PASS, подтверждает что `buildUnderageIIN()` корректно строит ИИН под 18 (16 лет назад).
- Финальный grep по всему репо: нет остаточных вхождений `21+`, `21 лет`, `MinCreatorAge = 21`, `minAge = 21` (кроме исторических цитат в спеке — оставлены сознательно).
- Спека `spec-creator-application-submit.md` — пункт 4 в § «Изменения контракта» переписан в финальную форму: «MinCreatorAge = 18. EFW-фильтр (порог 21) был включён 2026-04-25 и снят 2026-04-28». Хронология сохранена, но без Q&A-стиля.

## Команды для воспроизведения гейтов

```bash
make lint-backend                   # 0 issues
cd /home/alikhan/projects/ugcboost/frontend/landing && npx tsc --noEmit && npx eslint src/   # clean
cd /home/alikhan/projects/ugcboost/frontend/landing && npm run build   # 3 pages built
cd /home/alikhan/projects/ugcboost/backend && go test ./... -count=1 -race -timeout 5m   # all ok
make test-unit-backend-coverage     # all packages ≥80% per-method
make test-e2e-backend               # PASS, includes under_MinCreatorAge_rejected_with_UNDER_AGE
make test-e2e-landing               # PASS — обязательный гейт при правке backend constant'ов, отзеркаленных в frontend/e2e/helpers (landing under-age spec)
```
