---
title: "Frontend: action reject в drawer на verification-экране"
type: feature
created: "2026-05-04"
status: done
baseline_commit: f08bc29bb87eb14c0373db1bf760535221dd1d1a
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/creator-onboarding-roadmap.md
---

> Перед реализацией обязательно полностью загрузить все файлы `docs/standards/` — это hard rules, спека не дублирует.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Бэк-ручка `POST /creators/applications/{id}/reject` смержена (PR #58, baseline `6020535`); параллельный бэк-чанк добавляет Telegram-уведомление об отказе и пишется прямо сейчас. На фронте админ открывает drawer заявки на `/creator-applications/verification`, но дёрнуть reject нечем. Pipeline стопорится на заявках, не подходящих по контенту/аудитории/документам — остановить их нельзя.

**Approach:** Расширить `ApplicationDrawer` опциональным sticky-footer'ом (паттерн из прототипа `_prototype/.../ApplicationDrawer.tsx:88-95`). На verification-экране в footer — outlined-red кнопка «Отклонить заявку». Click → confirm-modal с упоминанием Telegram-уведомления (или warning'ом, если TG не привязан) → `POST /creators/applications/{id}/reject` → invalidate `creatorApplicationKeys.all()` + закрытие dialog'а и drawer'а. Контейнер действий (`ApplicationActions`) проектируется так, чтобы chunk 19 (approve) добавил вторую кнопку без правок Drawer/Page.

## Boundaries & Constraints

**Always:**
- Бэк-зависимости: reject endpoint (бэк `verification`+`moderation`, baseline `6020535`) и Telegram-нотифайер на rejected (`spec-creator-application-reject-notify.md`) **смержены в main** до старта реализации этого чанка. Этот front-чанк предполагает, что бот реально шлёт сообщение об отказе через `Sender` после успешного `WithTx`.
- `ApplicationDrawer` получает новый prop `footer?: ReactNode`. Sticky-footer рендерится только если prop передан (`border-t border-surface-300 bg-white px-6 py-3`, `data-testid="drawer-footer"`). Backward-compatible — текущие callers без `footer` не ломаются.
- Контейнер `ApplicationActions` решает по `application.status` какие кнопки показать. Сейчас — только reject на `verification`. На любом другом статусе компонент рендерит `null`, и `VerificationPage` передаёт `footer={null}` → `Drawer` вообще не рендерит footer-секцию. Защищает от пустой сероватой полосы при URL-открытии чужой заявки через `?id=`.
- Кнопка-триггер `data-testid="reject-button"`, outlined red (`border border-red-600 px-4 py-2 text-sm font-semibold text-red-600 hover:bg-red-50 disabled:opacity-50`) — точное зеркало `_prototype/.../ModerationActions.tsx:86-95`. Текст: `t("actions.reject") = "Отклонить заявку"`.
- Confirm-dialog `data-testid="reject-confirm-dialog"` (z-[60]):
  - title `t("rejectDialog.title") = "Отклонить заявку?"`
  - body conditional: `application.telegramLink ? t("rejectDialog.body") : t("rejectDialog.bodyNoTg")`. Тексты: «Заявка перейдёт в статус "Отклонена". Креатор получит уведомление в Telegram-боте.» / «Заявка перейдёт в статус "Отклонена". Креатор не привязал Telegram — уведомление об отклонении не будет отправлено.»
  - submit: red filled (`bg-red-600 hover:bg-red-700 text-white`), `data-testid="reject-confirm-submit"`, текст `t("rejectDialog.submit") = "Отклонить"` / pending `t("actions.rejecting") = "Отклонение..."`.
  - cancel: text-only, `data-testid="reject-confirm-cancel"`, текст `t("common:cancel")`.
  - backdrop click + Escape — закрыть, если не pending. Escape ловится в capture-фазе с `e.stopImmediatePropagation()` (как `VerifyManualDialog.tsx:67-75`), чтобы не закрылся drawer.
- Mutation pattern (зеркало `VerifyManualDialog.tsx`):
  - `useMutation({ mutationFn: () => rejectApplication(applicationId) })`
  - `onSuccess`: `queryClient.invalidateQueries({ queryKey: creatorApplicationKeys.all() })` → `onClose()` (close dialog) → `onCloseDrawer()` (delete `?id=` из searchParams).
  - `onError`: `ApiError && status 4xx && code !== "INTERNAL_ERROR"` → invalidate `all()` + `onApiError(getErrorMessage(err.code))` (banner в drawer, как в chunk 11) + close dialog; на 404 — также close drawer. 5xx/network — inline `setInlineError(t("rejectDialog.retryError"))`, dialog остаётся, drawer остаётся.
  - `onSettled`: reset `isSubmitting`.
  - `disabled` на submit/cancel/backdrop/Escape пока `isPending = mutation.isPending || isSubmitting`. Double-submit guard: external `isSubmitting` flag в local state.
- API-функция `rejectApplication(applicationId)` в `frontend/web/src/api/creatorApplications.ts` поверх openapi-fetch (`POST /creators/applications/{id}/reject`, без body, operationId `rejectCreatorApplication`). Бросает `ApiError` по образцу `verifyApplicationSocialManually`.
- Локализация: новые ключи в `creatorApplications.json` (`actions.reject`, `actions.rejecting`, `rejectDialog.{title,body,bodyNoTg,submit,retryError}`); `errors.CREATOR_APPLICATION_NOT_REJECTABLE` в `common.json` рядом с другими `CREATOR_APPLICATION_*`. Текст: «Заявку нельзя отклонить в текущем статусе».
- Все строки через `t("...")`, литералы testid'ов — константы при необходимости. Никаких хардкод-строк в JSX.
- Browser e2e — отдельный spec `frontend/e2e/web/admin-creator-applications-reject-action.spec.ts`. Helper `collectTelegramSent` (сейчас inline в `verify-action.spec.ts:343-367`) выносится в `frontend/e2e/helpers/telegram.ts` для шеринга.

**Ask First:** ничего — все ключевые UX-решения (размещение, поведение без TG, текст диалога, e2e-ассерты, стиль кнопки) зафиксированы на старте.

**Never:**
- Триггер reject из строки таблицы. Только из drawer'а — admin должен видеть ФИО, ИИН, контент, причину перед action'ом.
- Textarea для комментария / выбор категории / любое поле в dialog'е, кроме статичного текста. Бэк-чанк 12 решил: body пустой, текст бота — фикс-шаблон, итерируется отдельным PR'ом.
- Optimistic update (`setQueryData`) — invalidate-only.
- Кнопка reject при `application.status !== "verification"`. Бэк защищён 422, но визуально admin не должен видеть кнопку, которая упадёт.
- Undo / переход `rejected → verification`. Forward-only по state-machine.
- Bulk-reject. Каждая заявка — отдельное action и audit-row на бэке.
- Toast / snackbar после success. В web-app нет toast-инфраструктуры; закрытие drawer'а + исчезновение row из списка — достаточно. Banner используется только для error-веток (как chunk 11).
- WhatsApp-ссылки / призывы связаться вручную в warning'е. Warning короткий и без императива.
- Изменение `VerifyManualDialog`, `SocialAdminRow`, `ApplicationsTable`, фильтров, sort'а или любых файлов вне scope.
- Локальный mock контракта `rejectApplication`. Юзаем реальный generated-schema через openapi-fetch.

## I/O & Edge-Case Matrix

| Сценарий | Состояние | Поведение |
|---|---|---|
| Happy с TG | `verification`, `telegramLink !== null` | Footer виден, кнопка enabled. Click → dialog с TG-телом → submit → 200 → invalidate `all()` → drawer закрыт, `?id=` ушёл, row пропала, `verification-total` -1, counts обновился. В chunk 14 nofifier шлёт сообщение в TG. |
| Happy без TG | `verification`, `telegramLink === null` | То же, но dialog показывает warning-вариант body. Submit → 200, всё как выше. Сообщение в TG не уходит (нечем). |
| Cancel | dialog открыт | Cancel → dialog закрыт, drawer открыт, БД не изменилась. |
| Pending | submit отправлен, ответ ещё не пришёл | Submit/Cancel/backdrop/Escape — все `disabled`. Submit-text «Отклонение...». |
| 422 NOT_REJECTABLE | race: другой admin или manual-verify уже сменил статус | Modal закрыт, banner в drawer «Заявку нельзя отклонить в текущем статусе», invalidate `all()` (drawer обновится, заявка пропадёт из verification-списка). |
| 404 NOT_FOUND | заявка удалена бэкендом | Banner «Ресурс не найден» (общий код), invalidate `all()`, close dialog + close drawer. |
| 403 FORBIDDEN | brand_manager bypassed RoleGuard | Banner «Доступ запрещён», invalidate. (RoleGuard в норме не пускает.) |
| Network / 5xx | offline, 500, INTERNAL_ERROR | Inline error в dialog «Не удалось отклонить, попробуйте ещё раз», dialog остаётся, retry. Drawer остаётся. |
| Footer без actions | drawer detail в любом не-`verification` статусе (открыт через `?id=` чужой заявки) | `ApplicationActions` рендерит пустой fragment → Drawer всё равно показывает пустой footer-блок (visual cost минимальный, защищает от false-positive «нет действий»). Можно ужать до `null` если нужен кondicional render footer'а. |

</frozen-after-approval>

## Code Map

- `frontend/web/src/api/creatorApplications.ts` — функция `rejectApplication(applicationId: string)` поверх openapi-fetch.
- `frontend/web/src/features/creatorApplications/components/ApplicationDrawer.tsx` — новый prop `footer?: ReactNode`, sticky-footer-секция (зеркало `_prototype/.../ApplicationDrawer.tsx:88-95`), `data-testid="drawer-footer"`.
- `frontend/web/src/features/creatorApplications/components/ApplicationActions.tsx` — **новый**. Тонкий контейнер: switch по `application.status` → набор кнопок. Chunk 13: `verification` → `<RejectApplicationDialog/>`. Chunk 19 расширит на `moderation` (reject + approve).
- `frontend/web/src/features/creatorApplications/components/RejectApplicationDialog.tsx` — **новый**. Кнопка-триггер + confirm-modal + mutation. Структурный шаблон — `VerifyManualDialog.tsx`.
- `frontend/web/src/features/creatorApplications/VerificationPage.tsx` — пробросить `<ApplicationActions application={detailQuery.data?.data}/>` в `<ApplicationDrawer footer={...}/>`. Banner с error'ом из mutation подаётся через `onApiError` (state локально в Drawer, как в chunk 11).
- `frontend/web/src/shared/i18n/locales/ru/creatorApplications.json` — `actions.reject`, `actions.rejecting`, `rejectDialog.{title,body,bodyNoTg,submit,retryError}`.
- `frontend/web/src/shared/i18n/locales/ru/common.json` — `errors.CREATOR_APPLICATION_NOT_REJECTABLE`.
- `frontend/web/src/features/creatorApplications/components/RejectApplicationDialog.test.tsx` — **новый**. Сценарии: open / cancel / submit success → invalidate+close / 422-body / 404-body+close-drawer / 403 / 5xx-retry-inline / pending-disables-all / double-submit-guard / TG-warning conditional (telegramLink null vs filled).
- `frontend/web/src/features/creatorApplications/components/ApplicationActions.test.tsx` — **новый**. На `verification` рендерит reject; на любом другом — пустой fragment.
- `frontend/web/src/features/creatorApplications/components/ApplicationDrawer.test.tsx` — расширить: footer rendered only when prop передан (regression-safe для текущих тестов); click reject-button открывает dialog.
- `frontend/e2e/web/admin-creator-applications-reject-action.spec.ts` — **новый**. 3 сценария.
- `frontend/e2e/helpers/telegram.ts` — **новый**. `collectTelegramSent` вынести из inline в `verify-action.spec.ts:343-367` для переиспользования. Исходный spec обновить на импорт.
- `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` — фронт-чанк reject (после reorder группы 3 — текущий chunk 14 в roadmap'е) `[~]` при старте, `[x]` при merge PR.

## Tasks & Acceptance

**Pre-execution gates:**
- [x] Reject endpoint смержен в main (PR #58, baseline `6020535`). `frontend/web/src/api/generated/schema.ts` уже содержит `rejectCreatorApplication` op + `CREATOR_APPLICATION_NOT_REJECTABLE` error code + `CreatorApplicationDetailData.rejection` field.
- [x] `spec-creator-application-reject-notify.md` смержен в main (PR #59, commit `f08bc29`); нотифайер реально шлёт сообщение через `Sender` после commit.

**Execution:**
- [x] `api/creatorApplications.ts` — `rejectApplication`.
- [x] `ApplicationDrawer.tsx` — prop `footer?: ReactNode`, sticky-footer, не сломав существующие тесты (footer optional). DrawerContext + apiError state lifted up.
- [x] `RejectApplicationDialog.tsx` + unit.
- [x] `ApplicationActions.tsx` + unit (verification → reject visible; иное → пусто).
- [x] Wire в `VerificationPage.tsx`: `footer={<ApplicationActions application={detailQuery.data?.data} />}` (callbacks приходят через `useDrawerContext()`).
- [x] i18n-ключи в `creatorApplications.json` + `common.json`.
- [x] e2e helpers: вынос `collectTelegramSent` в `helpers/telegram.ts`, обновление импорта в `verify-action.spec.ts`.
- [x] e2e spec — 3 сценария (см. AC).
- [x] `make lint-web build-web test-unit-web test-e2e-frontend` — все зелёные (lint OK, build OK, unit 132/132, e2e 17/17).
- [~] Roadmap: фронт-чанк reject `[~]` при старте → `[x]` при merge. Сейчас `[~]`.

**Acceptance Criteria:**
- Given admin Bearer, заявка `verification` с TG-link, when admin открывает drawer, then footer-bar виден, кнопка `reject-button` enabled outlined-red.
- Given то же, when admin кликает reject → submit, then 200; drawer закрылся, `?id=` ушёл, row пропала из таблицы; admin GET detail → `status=rejected`, `rejection.fromStatus="verification"`, `rejection.rejectedByUserId=adminID`; через `/test/telegram/sent?since={ISO_до_submit}&chatId={tgUserId}` появилась хотя бы одна запись в течение 5s окна.
- Given заявка `verification` БЕЗ TG-link, when admin открывает drawer и кликает reject, then dialog body показывает текст с warning'ом «...уведомление не будет отправлено», submit идёт, 200; в `/test/telegram/sent` записей нет.
- Given dialog открыт, when admin click cancel, then dialog закрыт, drawer открыт, `?id=` сохранён, admin GET detail → `status=verification`, БД без изменений.
- Given race: другой admin reject'нул заявку секунду назад, when admin click submit, then получаем 422; modal закрыт; banner «Заявку нельзя отклонить в текущем статусе»; invalidate `all()`; row пропала из таблицы.
- `make lint-web build-web test-unit-web test-e2e-frontend` — все зелёные.

## Spec Change Log

- **2026-05-04 — review-loop 1 (no spec amendment).** 3 субагента (blind-hunter / edge-case-hunter / acceptance-auditor) прогнаны, классификация: 4 patch (применены), 3 defer (записаны в `deferred-work.md`), остальное reject.
  - **patch:** Esc race в `RejectApplicationDialog` — при `isPending=true` Escape проскакивал на drawer и закрывал его посреди мутации. Исправлено: listener теперь `stopImmediatePropagation` всегда (если open), а `setOpen(false)` только при `!isPending`.
  - **patch:** Exhaustive switch в `ApplicationActions` — добавлен default branch с `_exhaustive: never` чтобы при добавлении нового статуса в OpenAPI ловить compile-error.
  - **patch:** `useMemo` для DrawerContext value — стабилизирует reference, защищает потенциальных будущих consumer'ов с `useEffect([onApiError])` от бесконечного цикла.
  - **patch (откатан, defer):** auto-reset `apiError` при смене `application.id` (для prev/next в drawer) — пробовал через `key={applicationId}`, сломал `VerificationPage.test.tsx:272 closes drawer when close button clicked`. ESLint правило `react-hooks/set-state-in-effect` блокирует useEffect-вариант. Записано в `deferred-work.md` как minor edge.
  - **defer:** `collectTelegramSent` три edge cases (deadline-pass / silent-non-200 / dedup-collision) — pre-existing код, я только перенёс из inline в helper, чинится отдельным PR'ом.
  - **defer:** `as ApplicationDetail` в `ApplicationActions.test.tsx:56` — повторяет project-wide pattern из других fixture-builder'ов, замена project-wide задачкой.

## Design Notes

**Footer-pattern**. Прототип Айданы (`_prototype/.../ApplicationDrawer.tsx:88-95`) использует sticky-footer через `children`-slot. В реальной реализации делаем явный prop `footer?: ReactNode` — семантичнее, чем generic children, и не конфликтует с возможным будущим body-children.

**Контейнер `ApplicationActions`** — тонкий switch по статусу. Цель: chunk 19 (approve на moderation) и chunk 16 (moderation-screen) не должны трогать `Drawer` или `VerificationPage` — только дополнить `ApplicationActions`. На chunk 13 — единственная ветка `verification → reject`.

**TG-warning conditional**: одно текстовое поле в body dialog'а, разные ключи перевода. Без отдельного yellow-alert-блока — текст и так предупреждает, лишний UI-слой не нужен.

**Submit-кнопка reject-dialog'а** — red filled (по прототипу). Это primary-кнопка модалки, она должна быть выделена — не путать с trigger-кнопкой в footer (outlined red): trigger открывает диалог, submit подтверждает действие.

**`/test/telegram/sent?since=...&chatId=...`** — точная семантика как в `verify-action.spec.ts:343-367` (helper `collectTelegramSent`). ISO timestamp перед click submit, polling каждые 250ms в течение 5s, накопительный seen-set по `${chatId}|${sentAt}|${text}`. Для chunk 13 — assert `length >= 1` (happy с TG) или `length === 0` (без TG); проверять точный текст не нужно — это забота chunk 14 e2e.

**Z-index**: drawer `z-50`, confirm-dialog `z-[60]` (как `VerifyManualDialog.tsx:93`). Escape capture-listener в dialog'е с `stopImmediatePropagation` — чтобы Escape не закрыл drawer-listener'ом drawer (см. `VerifyManualDialog.tsx:67-75`).

## Verification

**Commands:**
- `make lint-web build-web test-unit-web test-e2e-frontend` — все зелёные.

**Manual smoke (локально):**
- `make compose-up migrate-up && make start-backend start-web`. Создать заявку через лендос (`:3003`), привязать TG через бот, login admin на `:3001`, открыть `/creator-applications/verification`, выбрать заявку → drawer → footer-bar «Отклонить заявку» (outlined red) → confirm-dialog с текстом «Креатор получит уведомление в Telegram-боте» → Submit. Заявка пропала с экрана; в БД `app.status=rejected`, transition row есть; в Telegram у тестового аккаунта пришло сообщение об отклонении (отправлено chunk 14 нотифайером).

## Suggested Review Order

**Боевой код — drawer и actions**

- entry point: thin switch по статусу — каркас для будущих approve/withdraw actions, exhaustive default ловит compile-error при добавлении статусов в OpenAPI.
  [`ApplicationActions.tsx:1`](../../frontend/web/src/features/creatorApplications/components/ApplicationActions.tsx#L1)

- триггер-кнопка + confirm-modal + mutation в одном компоненте; зеркало `VerifyManualDialog`, но без выбора соцсети и с conditional-body по `hasTelegram`.
  [`RejectApplicationDialog.tsx:28`](../../frontend/web/src/features/creatorApplications/components/RejectApplicationDialog.tsx#L28)

- `footer?: ReactNode` prop + поднятый `apiError` state + DrawerContext через `useMemo` — единый banner для verify/reject ошибок, OpenDrawer-split обходит ESLint `react-hooks/set-state-in-effect`.
  [`ApplicationDrawer.tsx:33`](../../frontend/web/src/features/creatorApplications/components/ApplicationDrawer.tsx#L33)

- Context в отдельном файле — иначе `react-refresh/only-export-components` ругается на смешанный default-export-component + named-export-hook.
  [`drawerContext.ts:1`](../../frontend/web/src/features/creatorApplications/components/drawerContext.ts#L1)

- проброс footer + `ApplicationActions`, callbacks приходят через context (не через VerificationPage props).
  [`VerificationPage.tsx:14`](../../frontend/web/src/features/creatorApplications/VerificationPage.tsx#L14)

**API + i18n**

- thin wrapper над openapi-fetch, без body, throws `ApiError` — паттерн `verifyApplicationSocialManually` 1:1.
  [`creatorApplications.ts:60`](../../frontend/web/src/api/creatorApplications.ts#L60)

- `actions.reject`, `actions.rejecting`, `rejectDialog.{title,body,bodyNoTg,submit,retryError}`. Тексты согласованы с пользователем (без призывов в WhatsApp, простые формулировки).
  [`creatorApplications.json:84`](../../frontend/web/src/shared/i18n/locales/ru/creatorApplications.json#L84)

- `errors.CREATOR_APPLICATION_NOT_REJECTABLE` рядом с другими `CREATOR_APPLICATION_*`.
  [`common.json:39`](../../frontend/web/src/shared/i18n/locales/ru/common.json#L39)

**Тесты**

- 13 сценариев unit: open / cancel / TG-conditional body / 422 / 403 / 404 / 500 / network / pending-disable / double-submit-guard.
  [`RejectApplicationDialog.test.tsx:1`](../../frontend/web/src/features/creatorApplications/components/RejectApplicationDialog.test.tsx#L1)

- exhaustive: для каждого не-`verification` статуса (6 значений) рендерится null.
  [`ApplicationActions.test.tsx:80`](../../frontend/web/src/features/creatorApplications/components/ApplicationActions.test.tsx#L80)

- расширение существующих тестов: footer-slot regression-safe + DrawerContext provides callbacks + единый banner.
  [`ApplicationDrawer.test.tsx:172`](../../frontend/web/src/features/creatorApplications/components/ApplicationDrawer.test.tsx#L172)

- 3 e2e: Happy-with-TG (assert TG-message ≥1) / Happy-without-TG (assert 0 messages) / Cancel.
  [`admin-creator-applications-reject-action.spec.ts:75`](../../frontend/e2e/web/admin-creator-applications-reject-action.spec.ts#L75)

- pre-existing `collectTelegramSent` вынесен из inline в shared helper для переиспользования.
  [`telegram.ts:14`](../../frontend/e2e/helpers/telegram.ts#L14)

- импорт обновлён, локальная копия удалена.
  [`admin-creator-applications-verify-action.spec.ts:46`](../../frontend/e2e/web/admin-creator-applications-verify-action.spec.ts#L46)

**Roadmap**

- chunk 13 (notify) → `[x]` (PR #59); chunk 14 (фронт-reject) → `[~]` (этот PR).
  [`creator-onboarding-roadmap.md:71`](../planning-artifacts/creator-onboarding-roadmap.md#L71)
