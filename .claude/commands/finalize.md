---
description: Финализировать ветку — архив спеки, merge PR, обновить main. Прод не трогает.
---

Закрывающий workflow после approve PR. Только merge feature-ветки + локальный cleanup. **Прод не деплоим** — пользователь делает это руками после.

## Pre-checks (стоп при провале)

1. Текущая ветка не `main` (`git branch --show-current`).
2. Working tree чистый (`git status -s` пусто).
3. Открытый PR для текущей ветки (`gh pr view --json state,number`, `state == "OPEN"`).

Любой провал — стоп, отчитаться, дальше не идти.

## Алгоритм

1. **Архив спеки.** Найти `_bmad-output/implementation-artifacts/spec-<slug>.md` (где `<slug>` — имя ветки минус префикс `<user>/`). Переместить в `_bmad-output/implementation-artifacts/archive/$(date +%F)-spec-<slug>.md`. Если файл-приёмник уже существует — стоп.
2. **Deferred-work.** Если `_bmad-output/implementation-artifacts/deferred-work.md` есть — `git rm`.
3. **Commit + push.** Один коммит `chore(artifacts): archive spec-<slug>` (без `--no-verify`). `git push`.
4. **CI на PR.** `gh pr checks <num> --watch --interval 30 --fail-fast`. Если упёрся в 10-минутный Bash-таймаут — повторить, пока не дождётся финального статуса. Любой не-зелёный — стоп.
5. **Merge.** `gh pr merge <num> --merge --delete-branch` (использует merge commit, как в репо принято; remote-ветка удаляется).
6. **CI на main.** `git fetch origin main`, взять `git rev-parse origin/main` как merge-SHA. Найти workflow run для этого SHA: `gh run list --branch main --commit <sha> --limit 5 --json databaseId,status,conclusion`. Если runs ещё не появились — `sleep 30` и повторить (до 5 минут). Когда появился — `gh run watch <id> --exit-status`. Не-зелёный — стоп.
7. **Локальный cleanup.**
   - `git checkout main`
   - `git pull --ff-only origin main` (только fast-forward; non-FF — стоп)
   - `git branch -d <feature-branch>` (без `-D`; отказался — стоп)

## Стоп-условия — что делать при остановке

CI красный, merge conflict, non-FF pull, `branch -d` отказался, любой git/gh non-zero exit — **остановиться, отчитаться кратко (что упало + последний вывод команды), ждать инструкций**. Не пытаться чинить, не делать workaround'ов.

**Запрещено:** `git push --force` (любой формы), `--no-verify`, `git branch -D`, любые действия за рамками алгоритма, любое касание прод-окружения.
