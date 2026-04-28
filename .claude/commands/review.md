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
- **Параллельный запуск.** Все применимые субагенты — в **ОДНОМ message** с несколькими `Agent` tool_use блоками. Sequential — только Шаги 7→8.
- **DEFAULT = fix in this PR.** Любое finding по умолчанию = `fix-now`. Punt в issue — только по 5 критериям ниже.
- **Self-check defer'ов перед HALT'ом.** Каждый предложенный `defer-to-issue` оркестратор сам валидирует против критериев. Не прошёл (severity `minor`/`nitpick`) — переклассифицируй в `fix-now` молча. Не прошёл (`blocker`/`major`) — оставь в отчёте с пометкой `[требует одобрения]` для Шага 7.
- **VETO Алихана** — выше всех правил. Его слово "fix-now" → fix-now независимо от severity.
- **READ-ONLY на код.** Edit/Write/NotebookEdit на исходники запрещены. Можно: Read, Bash, gh. После approval Алихана разрешено: (a) обновлять `docs/standards/<стандарт>.md` (правила) и/или `docs/standards/review-checklist.md` (только при новом слое / Hard rule / стандарт-файле); (b) постить inline-комменты в PR через `gh api`. Это явно одобренная запись для этого workflow.
- **АВТОНОМНОСТЬ.** Шаги 1-6 выполняются non-stop, без промежуточных подтверждений. Единственная HALT-точка — Шаг 7.

## Когда `defer-to-issue` ДОПУСТИМ

5 закрытых критериев:

1. **Architectural refactor** — меняет публичный контракт между слоями (handler↔service↔repository) или cross-cutting concern (auth, logging, transactions, codegen pipeline, RepoFactory shape) **И** один из backstop'ов: либо >15 файлов, либо >800 строк net diff. Правка 5–15 файлов БЕЗ изменения межслойного контракта — `fix-now`.
2. **Data migration** — требует миграции существующих production данных (не схемы — данных).
3. **External dependency** — блокировано внешним сервисом / контрактом / другим репо.
4. **Out-of-PR-scope** — finding про код, который не затронут diff'ом и не вытекает напрямую из изменений PR. **Исключение:** `regression`-finding от acceptance-auditor (diff сломал существующее требование) — всегда `fix-now`, даже если сломанный код вне diff'а.
5. **Operational task** — задача без кодового изменения (конвертация ассетов, ротация секрета).

При сомнении — всегда `fix-now`. Список закрытый. Расширение — через явное согласование с Алиханом + правка этой секции.

`blocker`/`major`-defer'ы выводятся в отчёт с пометкой `[требует одобрения]` — Алихан confirm/reject/reclassify на Шаге 7. Без явного действия по умолчанию = `fix-now`.

## Шкала severity

Severity finding'а определяется `[severity]`-меткой буллета `## Что ревьюить` соответствующего стандарта. Subagent может escalate / downgrade с обоснованием в `rationale` (например, «буллет [major], но здесь PII в production-логе → escalate to [blocker]»).

Метка в стандарте — приоритетна. Общие категории ниже — справочные:

- **blocker** — security/PII, regression, mutate без аудита, утечка секрета, `[blocker]`-буллет стандарта.
- **major** — `[major]`-буллет стандарта, мёртвый код, дублирование, говнокод, недотест критичной ветки.
- **minor** — `[minor]`-буллет стандарта, стилистика, точечная неоптимальность, missing edge-case test.
- **nitpick** — субъективная стилистика, naming, комментарии, `[nitpick]`-буллет стандарта.

## Шаги выполнения

### Шаг 1 — Сбор контекста

#### 1.1 Зафиксируй repo + identity

```bash
git fetch origin               # обновляет ВСЕ remote refs (origin/main и т.п.)
REPO=$(gh repo view --json nameWithOwner --jq .nameWithOwner)
OWNER_LOGIN=$(gh repo view --json owner --jq .owner.login)   # GitHub login владельца репо = Алихан в этом проекте; используется на Шаге 1.5 для определения «коммент Алихана»
```

`REPO` (формат `owner/repo`) подставляется во все `gh api`-вызовы дальше. **Литерал `{owner}/{repo}` в `gh api` НЕ работает** — gh api'шка не делает substitution (только подкоманды `gh repo`/`gh pr`/`gh issue` поддерживают placeholder'ы).

#### 1.2 Определи `HEAD_SHA` и `BASE_SHA`

В зависимости от входа:

- **Путь к diff передан** — особый случай: SHA не нужны, читай напрямую `cat <path>`. Передай субагентам путь, не команду `git diff` (см. Шаг 2). **Empty-check:** `[ -s <path> ]` — если файл пустой, сообщи «diff-файл `<path>` пустой, выхожу» и завершись. Шаги 1.3-1.6 пропускаются. Иди к Шагу 2.

- **PR номер передан** →
  ```bash
  HEAD_SHA=$(gh pr view <N> --json headRefOid --jq .headRefOid)
  git fetch origin "+refs/pull/<N>/head:refs/remotes/origin/pr-<N>"   # гарантирует, что PR HEAD достижим локально
  ```
  Берём актуальный PR HEAD на GitHub (НЕ `git rev-parse HEAD` — Алихан может быть на любой локальной ветке, локальный HEAD не равен PR HEAD'у). Второй `git fetch` — чтобы коммит был достижим: автор PR мог пушнуть между Шагом 1.1 и `gh pr view`, и `git diff $HEAD_SHA` иначе упал бы с `bad revision`.

  Дальше — ищи baseline-маркер (см. 1.3). Если найден — `BASE_SHA = <SHA из маркера>`. Если нет — `BASE_SHA=$(git merge-base $HEAD_SHA origin/main)`.

- **PR номера нет, текущая ветка** →
  ```bash
  HEAD_SHA=$(git rev-parse HEAD)
  BASE_SHA=$(git merge-base HEAD origin/main)
  ```
  Full pass на локальной ветке.

#### 1.3 Incremental baseline (только если есть PR номер)

```bash
gh api repos/$REPO/issues/<N>/comments --paginate \
  --jq '[.[] | select(.body | startswith("[🤖 review-agent] baseline"))] | sort_by(.created_at) | last | {id, body}'
```

- Если маркер найден — извлеки `<SHA>` из строки `baseline: <SHA>` по regex `^\[🤖 review-agent\] baseline:\s*([0-9a-f]{7,40})\s*$`. Если regex не матчится (маркер испорчен) — игнорируй, full pass.
- **Проверь, что SHA достижим:** `git rev-parse --verify --quiet <SHA>^{commit}`. Если не существует (force-push в ветку обнулил историю) — игнорируй маркер, full pass.
- Если маркер ОК → `BASE_SHA=<SHA>`. Запомни для Шага 8:
  - `marker_comment_id` — id маркера (для PATCH в Шаге 8).
  - `OLD_BASELINE` = `<SHA>` (войдёт в `прошлые pass-ы:` нового маркера).
  - `OLD_PAST` = содержимое строки `прошлые pass-ы: ...` старого маркера (или пустая строка, если такой строки не было).

#### 1.4 Проверь что diff не пустой (после финализации `BASE_SHA`)

```bash
git diff --quiet $BASE_SHA..$HEAD_SHA
```

Если exit 0:
- **Incremental empty** (был маркер, BASE_SHA из него): сообщи «ничего нового с baseline `<SHA>` — нечего ревьюить, выхожу» и завершись (это нормально).
- **Full empty** (маркера не было / не PR, BASE_SHA = merge-base): сообщи «diff между `merge-base origin/main` и HEAD пустой — ветка идентична main, проверь локальный checkout» и завершись.

#### 1.5 Существующие треды PR (только если есть PR номер)

```bash
gh api repos/$REPO/pulls/<N>/comments --paginate \
  --jq '.[] | {id, path, line: (.line // .original_line), user: .user.login, body, in_reply_to_id, created_at, html_url}'
```

`html_url` нужен на Шаге 7.3 для discussion-link.

- Группируй по `in_reply_to_id` в треды.
- **«Коммент Алихана»** = коммент, где `.user.login == $OWNER_LOGIN` (см. Шаг 1.1).
- **Закрытый тред** = последний коммент Алихана начинается с approval-маркера как отдельного слова. Regex (case-insensitive): `^\s*(ок|согласен|норм|пойдёт|гуд|принято|✅)([\s.,!]|$)`. Это исключает ложные `не ок`, `я не согласен с подходом`, `норм если переделать`.
- **Живой тред** = последний коммент Алихана НЕ approval (вопрос, возражение, неоднозначность).
- **Последний коммент не от Алихана** (другой ревьюер, бот, автор PR):
  - Если в треде есть прежний approval Алихана — считай закрытым.
  - Если approval'а нет — выведи в «Живые треды» с пометкой `(ожидает реакции Алихана)`. Иначе непрочитанный blocker от прошлой итерации может потеряться.
- **Неоднозначность** (есть и approval, и вопрос в одном комменте) — считай живым.
- Из закрытых тредов извлеки `[чеклист-кандидат]` маркеры (regex `\[чеклист-кандидат\]`).

#### 1.6 Метаданные PR и спека (если PR номер)

- `gh pr view <N> --json number,title,body,baseRefName,headRefName,url` — для отчёта.
- Спека/PRD: проверь `_bmad-output/implementation-artifacts/spec-*.md`, `docs/prd*.md`. Plan-файлы (`*-plan.md`, `*-fixes-plan.md`) спеками НЕ считаются.
- **Матчинг спеки с PR.** Найденная спека должна быть релевантна PR: упоминание в PR body, совпадение тематики с branch name (например, `creator-application-submit` ↔ `spec-creator-application-submit.md`), или явная ссылка из плана. Если совпадения нет — НЕ передавай спеку acceptance-auditor (он применит чужую спеку и выдаст мусорный отчёт).
- Если матчинг с низкой уверенностью — отметь в отчёте под «Сводка»: `(спека: <path>, низкая уверенность матчинга, не передана субагентам)`. **НЕ передавай** такую спеку acceptance-auditor / edge-case-hunter — риск мусорных findings от чужой спеки выше пользы. Если Алихан хочет всё-таки прогнать — повторит /review с явной командой.

### Шаг 2 — Параллельный multi-subagent ревью

В **ОДНОМ message** запусти все применимые субагенты через `Agent` tool. Каждому передай **self-contained prompt**, явно указав:

- `BASE_SHA` и `HEAD_SHA` из Шага 1 (для PR/ветки) **или** путь к diff-файлу.
- Команду для diff'а: `git diff <BASE_SHA>..<HEAD_SHA>` (для подмножества — `... -- <path>`; для полного файла на момент HEAD — `git show <HEAD_SHA>:<path>`). Если на входе путь к diff-файлу — команда `cat <path>` вместо `git diff`.
- **Тип pass'а** (всем субагентам, включая acceptance-auditor): `full` или `incremental`. Для incremental — упомяни baseline-SHA, чтобы субагент учитывал, что часть требований/кода жила до baseline.

Минимальный набор — всегда:

- **`standards-auditor`**.
- **`blind-hunter`**.
- **`edge-case-hunter`** — добавь путь к спеке, ТОЛЬКО если она сматчена с PR (Шаг 1.6). Без матчинга — не передавай (чужая спека приведёт к мусорным findings).

Условный:

- **`acceptance-auditor`** — ТОЛЬКО если спека/PRD найдена и сматчена с PR (Шаг 1.6). Передай путь к спеке + тип pass'а.

Каждый возвращает structured findings (формат — в его описании).

### Шаг 3 — Дедуп и сборка

1. Собрать findings со всех субагентов в один список.
2. **Дедуп:** одинаковый `(path, line, нормализованный title)` — оставить одну запись. **Нормализация title:** `trim` + `lowercase` + удаление trailing-пунктуации (`.`, `!`, `?`). В поле `sources` указать всех агентов, которые нашли. **Severity** — берётся максимальный из найденных (`blocker > major > minor > nitpick`).
3. **Семантический дедуп — best-effort.** Если титлы разных субагентов очевидно описывают одно (`"Hardcoded секрет"` vs `"Plain-text token"` на одной строке) — мержь вручную в `(sources)`. Если сомнение — оставь раздельно, отметь в Сводке `(возможный семантический дубль: 2 finding'а на одной строке)`.
4. **Cross-validation:** findings, найденные 2+ агентами — сильный сигнал. Помечай `**[cross-validated]**` в заголовке finding'а.
5. **Сортировка** внутри triage-секций: severity (blocker → nitpick) → cross-validated в начале своей severity-группы (по числу sources DESC).

### Шаг 4 — Triage

Каждое finding → одна категория:

- **fix-now** (default) — нужно исправить в этом PR.
- **clarify** — требует уточнения от Алихана (ambiguity в спеке/контракте).
- **defer-to-issue** — попадает в один из 5 критериев. Обязательно укажи **под какой критерий**.

Спецслучаи:
- `regression`-finding от acceptance-auditor — всегда `fix-now` (не подходит под Out-of-PR-scope, см. Critical Rules / критерий 4).
- `not-implemented [out-of-PR-scope]` от acceptance-auditor — автоматически `defer-to-issue` (критерий 4 «Out-of-PR-scope»), без пометки `[требует одобрения]` (acceptance-auditor уже подтвердил scope).

### Шаг 5 — Self-check defer'ов

Прежде чем выводить отчёт — пройди каждый предложенный `defer-to-issue` против критериев:

- Критерий не выполнен буквально (не "почти", не "близко"), severity `minor`/`nitpick` → переклассифицируй в `fix-now` молча.
- Критерий не выполнен буквально, severity `blocker`/`major` → оставь в `defer-to-issue` с пометкой `[не прошёл self-check, требует одобрения]`.
- Критерий выполнен, severity `blocker`/`major` → пометка `[требует одобрения]`.
- Критерий выполнен, severity `minor`/`nitpick` → defer без пометки (готов к auto-issue после approval).

Правило тайбрейкера: при сомнении — `fix-now`.

### Шаг 6 — Output (markdown в чате)

Структура отчёта:

```markdown
# Ревью <PR <N> | ветка <branch> | файл <path>> (<incremental|full>, baseline=<SHA или "n/a">)

## Сводка
- fix-now: <N>  (blocker: <X>, major: <Y>, minor: <Z>, nitpick: <W>)
- clarify: <N>
- defer-to-issue: <N>  (после self-check; из них <K> требуют одобрения)
- не затронутые слои: <список из standards-auditor «Не затронуто», или «—»>
- спека: <path или «—»; пометить если матчинг с низкой уверенностью>

## Findings: fix-now

(сортировка: severity → cross-validated в начале группы → остальные)

1. [blocker] **[cross-validated]** <file:line> — <title>
   sources: [standards-auditor, blind-hunter]
   rationale: ...
   **Fix:** ...

## Findings: clarify

1. <file:line> — <title>
   **Вопрос:** <что нужно решить>
   rationale: ...

## Findings: defer-to-issue

1. [blocker] **[требует одобрения]** <file:line> — <title>
   **Критерий:** Architectural refactor (затрагивает 18 файлов handler-слоя)
   rationale: ...

## Живые треды PR (информационно)

- thread #<id> на <file:line>: "<краткая суть>" (<html_url>)

## Кандидаты в стандарты

- "<правило>" — обоснование (источник: thread #<id> или standards-auditor; тематика: "security" / "tests" / ...)
```

Если в категории 0 findings — секция выводится с пометкой «(нет)».

### Шаг 7 — HALT, диалог

После вывода отчёта:

#### 7.1 Жди решения Алихана

- По каждому `clarify` — его ответ.
- По каждому `defer-to-issue [требует одобрения]` — варианты:
  - **confirm** — создаём issue.
  - **reject** — отбрасываем finding целиком (false positive).
  - **reclassify в fix-now** — оставляем finding, чиним в этом PR.
- Опционально по любому `fix-now` — отдельные правки приоритета или reclassify в `clarify`.

Без явного решения по `defer-to-issue [требует одобрения]` — по умолчанию `fix-now` (см. Critical Rules).

#### 7.2 Сбор follow-up через `/interview`

Применяй `/interview` (`.claude/commands/interview.md`) — компактные вопросы с 2-4 вариантами. Минимум вопросов, но не любой ценой: гранулярность важна.

**Inline-комменты в PR.**
- `blocker`/`major`-fix-now — постим автоматически (без вопроса), это критично.
- `minor`/`nitpick`-fix-now — отдельный вопрос: «постить minor/nitpick тоже? Yes / No / только конкретные (multiSelect)». Если minor+nitpick = 0 — вопрос пропускается.

**GitHub issues для одобренных defer-to-issue** — `Yes` (создаём все), `No` (только конкретные, multiSelect), `Skip`.
- Для `blocker`/`major`-defer'ов ответ `Other` обрабатывается как «переспросить» (не «отбросить») — потеря blocker'а на random-инпуте недопустима.
- Для `minor`/`nitpick`-defer'ов `Other` = «отбросить».

**Кандидаты в стандарты.** Если кандидатов = 0 — пропусти. Если 1-2 — по одному вопросу на каждого с вариантами «в `<тематический-стандарт>.md` / в Hard rules / в новый стандарт-файл / отбросить». Если 3+ — сначала MultiSelect «принять [N1, N2, ...] / отбросить остальные», потом по каждому принятому — вопрос «куда».

Если число релевантных объектов = 0 — соответствующий вопрос не задаётся.

#### 7.3 Применение действий (только при явном выборе)

**Inline-комменты.** Findings бывают двух типов:

1. **С конкретной строкой** (`path:line` есть) — постим как inline-коммент:
   ```bash
   COMMIT_ID=$(gh pr view <N> --json headRefOid --jq .headRefOid)
   gh api --method POST repos/$REPO/pulls/<N>/comments \
     -f body="<rationale + fix>" \
     -f path=<path> \
     -f commit_id=$COMMIT_ID \
     -f line=<line> \
     -f side=RIGHT
   ```

2. **Без конкретной строки** (`line` отсутствует или `<line>` = "не реализовано", "функция целиком" и т.п.) — gh API inline-коммент НЕ примет (422). Группируй такие findings в **один общий PR-коммент**:
   ```bash
   GENERAL_BODY=$(cat <<EOF
   ## Review findings без конкретной строки

   1. [blocker] **<title>** (sources: ...)
      <rationale>
      Fix: <fix>

   2. [major] **<title>** ...
   EOF
   )
   gh pr comment <N> --body "$GENERAL_BODY"
   ```

Пояснения:
- `commit_id` — берётся свежим перед каждым batch'ем (между approval и постингом могли пушнуть).
- `side=RIGHT` для finding'а на новой/изменённой строке. Если finding на удалённой строке (была до diff'а) — `side=LEFT`.
- При 422 на inline (commit устарел) — рефетч `headRefOid` и retry один раз. Если снова 422 — пометка в финальном отчёте «N inline-комментов не запостились (race с force-push)».
- В `body` экранируй `"` и `$` (bash подстановка); используй heredoc для многострочного.
- **Важно:** между Шагом 5 и постингом Алихан может pushed правки — line numbers в finding'ах могут больше не указывать на проблемный код. Inline-коммент уйдёт на тот же commit/line, но содержание может уже быть исправленным. После постинга упомяни в финальном отчёте: «N inline-комментов запостены на commit `<SHA>`, M общих PR-комментов. Если автор PR пушнул правки между ревью и постингом — content inline-комментов мог уйти на не ту строку, требует ручной верификации».

**Issues:**
```bash
ISSUE_BODY=$(cat <<EOF
<rationale>

Link: <discussion-url>
EOF
)
gh issue create --label tech-debt --label <релевантный> \
  --title "<кратко>" \
  --body "$ISSUE_BODY"
```

**Важно:** `\n` в bash-строке `--body "..."` остаётся литералом «`\n`», а не переносом строки — markdown в issue сломается. Используй heredoc как выше.

- На русском. `--label` можно повторять.
- `<discussion-url>` — для finding'ов из живых тредов это `html_url` коммента из Шага 1.5; для новых — URL PR (`gh pr view <N> --json url --jq .url`).
- **Отсутствующий label** — `gh issue create` упадёт. При первой ошибке создай label: `gh label create tech-debt --color "ededed" --description "Tech debt for later iteration"` и retry. Аналогично для `backend`/`frontend`/etc.

**Кандидаты:** правка конкретного `docs/standards/<file>.md` (основная часть и/или `## Что ревьюить`). Чеклист `docs/standards/review-checklist.md` правится только если кандидат — новый Hard rule (cross-cutting, 3+ слоя), новый слой (новый каталог) или новый стандарт-файл.

### Шаг 8 — Обновить baseline-маркер

Только если PR номер передан. Иначе — пропусти.

`baseline:` маркера = `HEAD_SHA` из Шага 1.2 (то, что реально проревьюено субагентами). **НЕ** свежий `headRefOid` — если автор пушнул правки между Шагом 1 и Шагом 8, свежий SHA будет ссылаться на коммиты, которых субагенты не видели; следующая итерация пропустит ревью этих коммитов.

Сформируй тело маркера через heredoc:

```bash
BODY=$(cat <<EOF
[🤖 review-agent] baseline: ${HEAD_SHA}
прошлые pass-ы: ${PAST_SHAS}
EOF
)
```

`EOF` без кавычек — `${NEW_HEAD_SHA}` подставляется. Для литералов `${...}` используй `<<'EOF'`.

`PAST_SHAS` — список из строки `прошлые pass-ы:` старого маркера + бывший `baseline:` старого маркера, через запятую с пробелом. **Лимит — последние 10 SHA**:

```bash
PAST_SHAS=$(echo "$OLD_PAST, $OLD_BASELINE" | tr ',' '\n' | sed 's/^ *//;s/ *$//' | grep -v '^$' | tail -10 | paste -sd ', ')
```

`tail -10` (НЕ `head -10`) — самые свежие SHA в конце потока, надо сохранять их, отбрасывая старые. `head` бы отбросил наоборот.

Если маркера не было — строку `прошлые pass-ы:` опусти.

Применение:
- **Маркер был:** `gh api --method PATCH repos/$REPO/issues/comments/<marker_comment_id> -f body="$BODY"`.
- **Маркера не было:** `gh api --method POST repos/$REPO/issues/<N>/comments -f body="$BODY"`.

## После ревью

Финальный отчёт одной строкой: `<N> fix-now (<X> постены inline, <Y> race-failed), <M> clarified, <K> deferred to issues #<...>; baseline marker → <SHA>`.

Не коммитить, не пушить.
