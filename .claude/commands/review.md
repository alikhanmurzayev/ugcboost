---
description: Параллельный multi-subagent код-ревью PR с уклоном на fix-in-PR
---

# Ревью PR

Прогон ревью для: $ARGUMENTS

`$ARGUMENTS` — номер PR (`19`), путь к diff, или пусто (тогда `git diff $(git merge-base HEAD main)..HEAD` текущей ветки).

## Назначение

Оркестратор спавнит 3-4 параллельных read-only субагента с разными оптиками, собирает их findings, дедупает, выводит структурированный отчёт. Цель — довести PR до состояния "грамотно реализовано", а не плодить tech-debt.

## CRITICAL RULES (NO EXCEPTIONS)

- **Subagent isolation.** Каждый субагент стартует холодным через Agent tool. Inline-ревью запрещено.
- **Параллельный запуск.** Все применимые субагенты — в **ОДНОМ message** с несколькими `Agent` tool_use блоками. Sequential — только если есть зависимости (нет в этом флоу).
- **DEFAULT = fix in this PR.** Любое finding по умолчанию = `fix-now`. Punt в issue — только по 5 критериям ниже.
- **Self-check defer'ов перед HALT'ом.** Каждый предложенный `defer-to-issue` оркестратор сам валидирует против критериев. Не прошёл — переклассифицируй в `fix-now` молча, без обсуждения.
- **VETO Алихана** — выше всех правил. Его слово "fix-now" → fix-now независимо от severity.
- **READ-ONLY на код.** Edit/Write/NotebookEdit на исходники запрещены. Можно: Read, Bash, gh. После approval Алихана — обновлять файлы в `docs/standards/` (чеклист и/или конкретные стандарты) и постить inline-комменты в PR.
- **АВТОНОМНОСТЬ.** Шаги 1-6 выполняются non-stop, без промежуточных подтверждений. Единственная HALT-точка — Шаг 7 (финальный диалог).

## VETO Алихана (безусловный приоритет)

Алихан в любой момент может приказать исправить любое finding в текущем PR — без обоснования, поверх любых критериев. Reclassify в `fix-now` без обсуждения. Это правило выше всех других в этой команде.

## Когда `defer-to-issue` ДОПУСТИМ

5 закрытых критериев:

1. **Architectural refactor** — меняет публичный контракт между слоями (handler↔service↔repository) или cross-cutting concern (auth, logging, transactions, codegen pipeline, RepoFactory shape). Backstop: либо >15 файлов, либо >800 строк net diff. Правка 5–15 файлов БЕЗ изменения межслойного контракта — `fix-now`.
2. **Data migration** — требует миграции существующих production данных (не схемы — данных).
3. **External dependency** — блокировано внешним сервисом / контрактом / другим репо.
4. **Out-of-PR-scope** — finding про код, который не затронут diff'ом и не вытекает напрямую из изменений PR.
5. **Operational task** — задача без кодового изменения (конвертация ассетов, ротация секрета).

При сомнении — всегда `fix-now`. Список закрытый. Расширение — через явное согласование с Алиханом + правка этой секции.

`blocker`/`major` нельзя классифицировать как `defer-to-issue` без явного одобрения Алихана.

## Шкала severity

- **blocker** — нарушение security/PII, regression, нарушение `[CRITICAL]` стандарта, mutate без аудита, утечка секрета.
- **major** — нарушение `[REQUIRED]` стандарта, мёртвый код, дублирование, говнокод, недотест критичной ветки.
- **minor** — стилистика в рамках стандартов, точечная неоптимальность, missing edge-case test.
- **nitpick** — субъективная стилистика, naming, комментарии.

## Шаги выполнения

### Шаг 1 — Сбор контекста

1. **Зафиксируй SHA-диапазон ревью.** Это «снимок»: все субагенты будут ревьюить одно и то же, даже если кто-то пушнёт в процессе.

   ```bash
   git fetch origin main
   HEAD_SHA=$(git rev-parse HEAD)
   ```

   `BASE_SHA` определяется в зависимости от входа:
   - **Путь к diff передан** — особый случай: SHA не нужны, читай напрямую `cat <path>`. Передай субагентам путь, не команду `git diff`. Дальнейшие шаги про PR/маркер/треды пропускаются.
   - **PR номер передан, baseline-маркер есть** (см. п.2) — `BASE_SHA` берётся из маркера.
   - **PR номер передан, маркера нет** или **PR номера нет** — `BASE_SHA=$(git merge-base HEAD origin/main)`. Это full pass.

2. **Incremental baseline** (только если есть PR номер):
   - Найди самый свежий маркер по `created_at`:
     ```bash
     gh api repos/{owner}/{repo}/issues/<N>/comments --paginate \
       --jq '[.[] | select(.body | startswith("[🤖 review-agent] baseline"))] | sort_by(.created_at) | last | {id, body}'
     ```
     `{owner}/{repo}` — литерал, `gh` сам подставляет из git remote.
   - Если маркер найден — извлеки `<SHA>` из первой строки тела (`baseline: <SHA>`).
   - **Проверь, что SHA достижим:** `git rev-parse --verify --quiet <SHA>^{commit}`. Если не существует (force-push в ветку обнулил историю) — игнорируй маркер, делай full pass (`BASE_SHA=$(git merge-base HEAD origin/main)`).
   - Если маркер ОК → `BASE_SHA=<SHA>`. Запомни `marker_comment_id` для Шага 8.
   - **Проверь что diff не пустой:** `git diff --quiet <BASE_SHA>..<HEAD_SHA>`. Если exit 0 — сообщи «ничего нового с baseline `<SHA>`, выхожу» и завершись.

3. **Существующие треды PR** (только если есть PR номер):
   - `gh api repos/{owner}/{repo}/pulls/<N>/comments --paginate --jq '.[] | {id, path, line: (.line // .original_line), user: .user.login, body, in_reply_to_id, created_at}'`
   - Группируй по `in_reply_to_id` в треды.
   - **Закрытый тред** = последний коммент Алихана содержит одобрение (`ок`, `согласен`, `норм`, `пойдёт`, `гуд`, `принято`, `✅` и т.д.).
   - **Живой тред** = последний коммент Алихана — вопрос или возражение (`?`, `почему`, `что`, `не согласен`, `поправь`, `проверь` и т.д.).
   - Из закрытых тредов извлеки `[чеклист-кандидат]` маркеры (regex `\[чеклист-кандидат\]`) — это input для пополнения чеклиста (выводится в финальном отчёте).

4. **Метаданные PR и спека** (если PR номер передан):
   - `gh pr view <N> --json number,title,body,baseRefName,headRefName` — для отчёта.
   - Спека/PRD: проверь `_bmad-output/implementation-artifacts/spec-*.md`, `_bmad-output/planning-artifacts/*-plan.md`, `docs/prd*.md`. Если есть — запомни путь(и).

### Шаг 2 — Параллельный multi-subagent ревью

В **ОДНОМ message** запусти все применимые субагенты через `Agent` tool. Каждому передай **self-contained prompt**, явно указав:

- `BASE_SHA` и `HEAD_SHA` из Шага 1.
- Команду для diff'а: `git diff <BASE_SHA>..<HEAD_SHA>` (для подмножества — `... -- <path>`; для полного файла на момент HEAD — `git show <HEAD_SHA>:<path>`).
- Тип pass'а: `full` или `incremental` (для последнего — упомяни baseline-SHA, чтобы субагент учитывал, что часть требований/кода уже жила до baseline).
- Если на входе **путь к diff-файлу** (не PR/ветка) — передай этот путь вместо `BASE_SHA`/`HEAD_SHA`/команды.

Минимальный набор — всегда:

- **`standards-auditor`**.
- **`blind-hunter`**.
- **`edge-case-hunter`** — добавь путь к спеке, если найдена в Шаге 1.4.

Условный:

- **`acceptance-auditor`** — ТОЛЬКО если найдена спека/PRD. Передай путь к спеке.

Каждый возвращает structured findings (формат — в его описании).

### Шаг 3 — Дедуп и сборка

1. Собрать findings со всех субагентов в один список.
2. **Дедуп:** одинаковый `(path, line, нормализованный title)` — оставить одну запись. В поле `sources` указать всех агентов, которые нашли (например, `[standards-auditor, blind-hunter]`).
3. **Cross-validation:** findings, найденные 2+ агентами — сильный сигнал. Помечай их и выводи **первыми** в отчёте.
4. Сортировка внутри каждой группы: severity (blocker → nitpick), потом по числу sources (больше → выше).

### Шаг 4 — Triage

Каждое finding → одна категория:

- **fix-now** (default) — нужно исправить в этом PR.
- **clarify** — требует уточнения от Алихана (ambiguity в спеке/контракте).
- **defer-to-issue** — попадает в один из 5 критериев. Обязательно укажи **под какой критерий**.

### Шаг 5 — Self-check defer'ов

Прежде чем выводить отчёт — пройди каждый предложенный `defer-to-issue` против критериев из секции **«Когда `defer-to-issue` ДОПУСТИМ»** выше. Жёстко: критерий не выполнен буквально (не "почти", не "близко") → переклассифицируй в `fix-now` молча, без обсуждения.

Правило тайбрейкера из той же секции применяется здесь: при сомнении — всегда `fix-now`.

В отчёт идут только те `defer-to-issue`, что прошли self-check.

### Шаг 6 — Output (markdown в чате)

Структура отчёта:

```markdown
# Ревью <PR <N> | ветка <branch> | файл <path>> (<incremental|full>, baseline=<SHA или "first pass">)

## Сводка
- fix-now: <N>  (blocker: <X>, major: <Y>, minor: <Z>, nitpick: <W>)
- clarify: <N>
- defer-to-issue: <N>  (после self-check)

## Findings, найденные несколькими агентами

(сильный сигнал — выводятся первыми, с пометкой sources)

1. [blocker] <file:line> — <title>
   sources: [standards-auditor, blind-hunter]
   rationale: ...
   **Fix:** ...

## Findings: fix-now

(остальные, сортировка по severity)

1. [major] <file:line> — <title>
   source: <agent>
   rationale: ...
   **Fix:** ...

## Findings: clarify

1. <file:line> — <title>
   **Вопрос:** <что нужно решить>
   rationale: ...

## Findings: defer-to-issue

1. <file:line> — <title>
   **Критерий:** Architectural refactor (затрагивает 18 файлов handler-слоя + миграцию контракта strict-server)
   rationale: ...

## Живые треды PR (информационно)

(не флагую повторно; ждут твоего решения)

- thread #<id> на <file:line>: "<краткая суть>"

## Кандидаты в чеклист

(из `[чеклист-кандидат]` аннотаций в закрытых тредах + новые предложения от субагентов)

- "<правило>" — обоснование (источник: thread #<id> или standards-auditor)
```

### Шаг 7 — HALT, диалог

После вывода отчёта:

1. Жди решения Алихана:
   - По каждому `clarify` — его ответ.
   - По каждому `defer-to-issue` — confirm / reject / reclassify в `fix-now`.
   - Опционально по `fix-now` — отдельные правки приоритета или reclassify в `clarify`.

2. После approval — собери решения по follow-up действиям через `/interview`. По каждому смысловому решению — **один вопрос** (не дробить на подвопросы; формулируй компактно с понятными вариантами). Если число релевантных объектов для решения = 0 (например, 0 кандидатов) — соответствующий вопрос не задаётся.

   Смысловые решения:
   - **Inline-комменты в PR** для fix-now findings.
   - **GitHub issues** для одобренных defer-to-issue.
   - **Куда вписать кандидатов** — варианты на каждого кандидата: в `docs/standards/review-checklist.md`, в конкретный `docs/standards/<file>.md`, или отбросить. Субагенты в маркере `[чеклист-кандидат]` уже подсказывают `куда:` — используй как дефолт-предложение, но финальное решение за Алиханом.

   Формулировки и варианты — на твоё усмотрение, опираясь на конкретику отчёта.

3. Только при явном выборе действия (не "Нет"/"отбросить"):
   - **Inline-комменты:** для каждого fix-now finding'а — `gh api --method POST repos/{owner}/{repo}/pulls/<N>/comments -f body="<rationale + fix>" -f path=<path> -f commit_id=$(git rev-parse HEAD) -f line=<line> -f side=RIGHT`. `commit_id` берётся на момент постинга (не на момент Шага 1 — между ревью и approval'ом могли пушнуть).
   - **Issues:** `gh issue create --label tech-debt --label <релевантный: backend / frontend / infra / docs / ...> --title "<кратко>" --body "<rationale>\n\nLink: <discussion-url>"`. На русском. `--label` можно повторять, если нужно несколько лейблов. `<discussion-url>` — для finding'ов из живых тредов это `html_url` коммента из Шага 1.3; для новых — URL PR (`gh pr view <N> --json url --jq .url`).
   - **Кандидаты:** правка `docs/standards/review-checklist.md` и/или конкретных `docs/standards/<file>.md` согласно решению по каждому кандидату. Это разрешённая запись (не код).

### Шаг 8 — Обновить baseline-маркер

Только если PR номер передан. Иначе — пропусти шаг.

1. `NEW_HEAD_SHA=$(git rev-parse HEAD)`. Если ничего не пушили после Шага 1, совпадает с `HEAD_SHA`.

2. Сформируй тело маркера через heredoc + переменную (single-quoted строка с `\n` не сработает; backtick'и поломают команду):

   ```bash
   BODY=$(cat <<EOF
   [🤖 review-agent] baseline: ${NEW_HEAD_SHA}
   прошлые pass-ы: ${SHA1}, ${SHA2}
   EOF
   )
   ```

   Где `SHA1, SHA2, ...` — список из строки `прошлые pass-ы:` старого маркера + бывший `baseline:` старого маркера. Если маркера не было — строку `прошлые pass-ы:` опусти.

3. Применение:
   - **Маркер был** (`marker_comment_id` есть): `gh api --method PATCH repos/{owner}/{repo}/issues/comments/<marker_comment_id> -f body="$BODY"`.
   - **Маркера не было:** `gh api --method POST repos/{owner}/{repo}/issues/<N>/comments -f body="$BODY"`.

## После ревью

Финальный отчёт одной строкой: `<N> fix-now (<X> постены inline), <M> clarified, <K> deferred to issues #<...>; baseline marker → <SHA>`.

Не коммитить, не пушить.
