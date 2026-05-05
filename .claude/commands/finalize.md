---
description: Финализировать ветку — архив спеки и интента, merge PR, обновить main. Прод не трогает.
---

Закрывающий workflow после approve PR. Только merge feature-ветки + локальный cleanup. **Прод не деплоим** — пользователь делает это руками после.

## Pre-checks (стоп при провале)

1. Текущая ветка не `main` (`git branch --show-current`).
2. Working tree не имеет tracked-модификаций — `git diff --quiet HEAD`.
   Untracked-файлы под `_bmad-output/` допускаются (артефакты параллельных агентов).
   **НЕ использовать `git status -s`** как gate — он ловит untracked и валится на чужих артефактах.
3. Открытый PR для текущей ветки (`gh pr view --json state,number`, `state == "OPEN"`).
4. CI на PR уже зелёный (`gh pr checks <num> --required` — все `pass`).
   Любая `pending`/`fail` — стоп, не делать никаких правок и пушей. Команда запускается, когда работа реально завершена.

Любой провал — стоп, отчитаться, дальше не идти.

## Алгоритм

1. **Slug ветки.** `slug = current-branch-name минус префикс `<user>/`. Пример: `alikhan/foo-bar` → `foo-bar`.

2. **Архив своих артефактов.** Точечно по slug, оба файла независимо:
   - Если `_bmad-output/implementation-artifacts/spec-{slug}.md` существует — `git mv` в `_bmad-output/implementation-artifacts/archive/$(date +%F)-spec-{slug}.md`.
   - Если `_bmad-output/implementation-artifacts/intent-{slug}.md` существует — `git mv` в `_bmad-output/implementation-artifacts/archive/$(date +%F)-intent-{slug}.md`.
   - Если файл-приёмник в `archive/` уже существует — стоп.
   - **Чужие** `spec-*.md` / `intent-*.md` (от параллельных агентов) НЕ ТРОГАТЬ. Только точное совпадение по slug текущей ветки.
   - Если ни spec, ни intent для текущего slug нет — это норм (чанк без формальной спеки), шаг no-op, идём дальше.

3. **Deferred-work.** Если `_bmad-output/implementation-artifacts/deferred-work.md` существует — `git rm`.

4. **Commit + push.** Один коммит `chore(artifacts): archive spec-{slug}` (без `--no-verify`). **Использовать `git commit` без `git add .`** — только staged-renames из шага 2 + `git rm` из шага 3 попадут в коммит, untracked параллельных агентов — нет. Затем `git push`.

5. **Wait A — появление CI checks после push.** Двухступенчатое ожидание необходимо: `--watch` сам по себе НЕ ждёт появления checks (вернёт «no checks reported» с exit 0).
   Monitor tool, until-loop, timeout 5 минут:
   ```
   until gh pr checks <num> 2>&1 | grep -qv 'no checks reported'; do sleep 15; done
   ```

6. **Wait B — завершение CI checks.**
   ```
   gh pr checks <num> --watch --interval 30 --fail-fast
   ```
   Если Bash 10-минутный таймаут сработает раньше — **повторить ту же команду**. Не подменять инструмент.
   Любой не-зелёный — стоп.

7. **Merge.** `gh pr merge <num> --merge --delete-branch` (merge commit как принято в репо; remote-ветка удаляется; локальная feature-ветка обычно удаляется автоматически).

8. **Merge-SHA на main.** `git fetch origin main`, `merge_sha = $(git rev-parse origin/main)`.

9. **Wait C — появление run на main.** Monitor tool, until-loop, timeout 5 минут:
   ```
   until [[ -n "$(gh run list --branch main --commit <merge_sha> --limit 1 --json databaseId -q '.[0].databaseId' 2>/dev/null)" ]]; do sleep 15; done
   ```

10. **Wait D — завершение run на main.** Когда run появился, взять его id:
    ```
    gh run watch <run-id> --exit-status
    ```
    Bash timeout — повторить ту же команду. Не-зелёный — стоп.

11. **Локальный cleanup.**
    - `git checkout main`
    - `git pull --ff-only origin main` (только fast-forward; non-FF — стоп)
    - Снова проверить tracked-чистоту: `git diff --quiet HEAD`. Untracked под `_bmad-output/` — игнорировать.
    - `git branch -d <feature-branch>` (без `-D`; если уже удалена `--delete-branch`'ем в шаге 7 — no-op, не ошибка). Любой другой отказ — стоп.

## Стоп-условия — что делать при остановке

CI красный, merge conflict, non-FF pull, `branch -d` отказался, любой git/gh non-zero exit — **остановиться, отчитаться кратко (что упало + последний вывод команды), ждать инструкций**. Не пытаться чинить, не делать workaround'ов.

## Запрещено

- `git push --force` (любой формы)
- `--no-verify`
- `git branch -D`
- `git status -s` как working-tree gate (использовать `git diff --quiet HEAD`)
- `git add .` или wildcard-add при коммите архива
- Альтернативные ожидания CI: внешний `watch`, кастомные shell-loop'ы вне Monitor tool, `jq` для парсинга CI-статуса (только встроенные `gh ... -q` jq-выражения)
- Любые действия за рамками алгоритма
- Любое касание прод-окружения
