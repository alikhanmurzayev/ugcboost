---
title: "Frontend: ручная верификация соцсети из drawer'а"
type: feature
created: "2026-05-04"
status: in-progress
baseline_commit: a6935eb
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/creator-onboarding-roadmap.md
  - _bmad-output/implementation-artifacts/archive/2026-05-04-spec-creator-verification-manual.md
---

> Чанк 11 roadmap'а. Бэк-чанк 10 (`POST /creators/applications/{id}/socials/{socialId}/verify`) смерджен (`a6935eb`); поле `id` (uuid, required) добавлено в `CreatorApplicationDetailSocial`, TS-схема пере-сгенерирована.
>
> Перед реализацией — полностью загрузить `docs/standards/`. Это hard rules, спека не дублирует.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** После чанка 10 у бэка появляется action для ручной верификации соцсети, но в drawer'е заявки (на экране `/creator-applications/verification`) дёрнуть нечем. Pipeline стоит на заявках без Instagram (TikTok / Threads) и где SendPulse-webhook не сработал.

**Approach:** В блок Socials drawer'а добавить inline-кнопку «Подтвердить вручную» рядом с handle каждой не-верифицированной соцсети. Click → confirm-модалка с явным «креатор не получит уведомление» → `POST /creators/applications/{id}/socials/{socialId}/verify` → invalidate `creatorApplicationKeys.all()` + close drawer (заявка пропадает из verification, ушла в moderation). Если `telegramLink === null` — кнопка disabled с подсказкой.

## Boundaries & Constraints

**Always:**
- Кнопка показывается только при `social.verified === false`. Verified — бейдж «Подтверждено · auto / manual» с `data-testid="verified-badge-{socialId}"`, `title` с `verifiedAt`.
- `data-testid="verify-social-{socialId}"` — стабильный ключ для e2e.
- `telegramLink === null` → кнопка `disabled` + рядом `verify-social-{socialId}-disabled-hint` («Сначала креатор должен привязать Telegram»). Click ничего не делает — модалка не открывается.
- Confirm-модалка `data-testid="verify-confirm-dialog"` с заголовком «Подтвердить владение вручную?», телом «Подтверждаете владение @{handle} ({Instagram|TikTok|Threads}). Заявка перейдёт на модерацию. Креатор не получит уведомление о ручной верификации.», testid'ы `verify-confirm-cancel` / `verify-confirm-submit`. Cancel — закрыть без запроса.
- Mutation pattern по образцу `_prototype/.../ModerationActions.tsx`: `useMutation` с `onError` (обязательно, `frontend-api.md`) → `setError(getErrorMessage(err.code))`; `onSuccess`/любая ошибка с invalidate → **всегда** один call `queryClient.invalidateQueries({ queryKey: creatorApplicationKeys.all() })` (prefix-match зацепит list + detail + counts; одно правило для success и error-веток). На success — также закрыть модалку + закрыть drawer (delete `?id=` из searchParams).
- Submit + Cancel + backdrop-close + Escape — все `disabled` пока `mutation.isPending` (по образцу `_prototype/.../ModerationActions.tsx`: cancel-handler делает `() => !isPending && setOpen(false)`). Защита от race «cancel + late success → drawer закрывается без feedback». Submit-текст меняется на «Подтверждение...» (`frontend-state.md` § Кнопки мутаций). Double-submit guard через external `isSubmitting` flag в local state, reset в `onSettled`.
- Маппинг `err.code` → русский текст в `common.json` под `errors.*` (резолвится существующим `getErrorMessage`, лукапает `common:errors.${code}`). Специфичные коды фичи — 4: `CREATOR_APPLICATION_NOT_IN_VERIFICATION`, `CREATOR_APPLICATION_TELEGRAM_NOT_LINKED`, `CREATOR_APPLICATION_SOCIAL_ALREADY_VERIFIED`, `CREATOR_APPLICATION_SOCIAL_NOT_FOUND`. 404 на саму заявку бэк отдаёт как общий `NOT_FOUND` (`backend/internal/handler/response.go:44-45`) — текст уже есть в `common.json:errors.NOT_FOUND`, новый ключ не нужен.
- Все строки через `t("...")`. Литералы testid'ов — константы.
- Browser e2e — отдельный spec `frontend/e2e/web/admin-creator-applications-verify-action.spec.ts` (не расширение 6.5: 6.5 покрывает shape, этот — actions).

**Never:**
- Триггер action из строки таблицы — только из drawer'а (нужен контекст handle).
- Bulk-верификация. Каждая соцсеть — отдельное action и audit-row.
- Новый экран / роут.
- Optimistic update (`setQueryData`).
- Изменение `ApplicationDrawer` props на children-slot. Action-блок встраивается внутрь поля Socials.
- Локальный mock контракта `verifyCreatorApplicationSocial` — гоняем реальный бэк через openapi-fetch.

## I/O & Edge-Case Matrix

| Сценарий | Состояние | Поведение |
|---|---|---|
| Happy path | TG привязан, IG `verified=auto`, TT `verified=false` | У IG бейдж auto, у TT кнопка enabled. Click → modal → Submit → 200 → drawer закрыт, `?id=` ушёл, `verification-total` -1, строка пропала. |
| Cancel | то же | Click verify → modal. Click cancel → modal закрыт, drawer открыт, БД не изменилась. |
| TG не привязан | `telegramLink=null`, IG unverified | Кнопка disabled, `disabled-hint` виден. Click ничего не делает. |
| Verified manual | IG `verified=true / method=manual` | Кнопки нет, бейдж «Подтверждено · вручную». |
| Pending | submit отправлен, ответа ещё нет | Submit/Cancel/backdrop/Escape — все `disabled` (защита от race «cancel + late success»). |
| 409 ALREADY_VERIFIED | race с другим админом | Modal закрыт, error «Эта соцсеть уже подтверждена», invalidate `all()` (бейдж и список обновятся). |
| 422 NOT_IN_VERIFICATION | race на смене статуса | Error «Заявка уже не на верификации», invalidate `all()` (заявка пропадёт). |
| 422 TELEGRAM_NOT_LINKED | креатор отвязал TG | Error «Креатор отвязал Telegram, попросите привязать заново», invalidate `all()`. |
| 404 NOT_FOUND / SOCIAL_NOT_FOUND | заявка или соцсеть удалены | Error через `getErrorMessage(code)` (общий «Ресурс не найден» для заявки, специфичный для соцсети), invalidate `all()` + close drawer. |
| Network / 5xx | offline, 500 | Error «Не удалось подтвердить, попробуйте ещё раз», drawer и modal остаются — retry. |

</frozen-after-approval>

## Code Map

- `frontend/web/src/api/creatorApplications.ts` — `verifyApplicationSocialManually(applicationId, socialId)` поверх openapi-fetch (operationId `verifyCreatorApplicationSocial`).
- `frontend/web/src/features/creatorApplications/components/SocialAdminRow.tsx` — **новый**. `<SocialLink>` + бейдж (verified) или кнопка/disabled-hint (unverified).
- `frontend/web/src/features/creatorApplications/components/VerifyManualDialog.tsx` — **новый**. Confirm-модалка + mutation внутри.
- `frontend/web/src/features/creatorApplications/components/ApplicationDrawer.tsx` — Socials рендерится через `<SocialAdminRow>`. State `selectedVerifyTarget` для модалки.
- `frontend/web/src/shared/i18n/locales/ru/creatorApplications.json` — новые ключи: `actions.verifyManual` / `actions.verifying`, `verifiedAuto` / `verifiedManual`, `verifyDialog.{title,body,submit,cancel,tgRequired}`.
- `frontend/web/src/shared/i18n/locales/ru/common.json` — расширить `errors.*` четырьмя кодами `CREATOR_APPLICATION_*` (рядом с `VALIDATION_ERROR`/`NOT_FOUND`/etc., чтобы существующий `getErrorMessage` их находил). 404 на саму заявку резолвится через уже существующий `errors.NOT_FOUND`.
- `frontend/web/src/features/creatorApplications/components/SocialAdminRow.test.tsx` — **новый**. Verified-auto / verified-manual / unverified+TG / unverified+noTG.
- `frontend/web/src/features/creatorApplications/components/VerifyManualDialog.test.tsx` — **новый**. Open / cancel / submit success / 4 специфичных error-кода + общий `NOT_FOUND` / pending / double-submit guard.
- `frontend/web/src/features/creatorApplications/components/ApplicationDrawer.test.tsx` — расширить интеграционными сценариями (Socials рендерятся через AdminRow; click verify в drawer'е открывает модалку с правильным handle).
- `frontend/e2e/web/admin-creator-applications-verify-action.spec.ts` — **новый**. 3 сценария (см. AC).
- `frontend/e2e/helpers/api.ts` — `loginAsAdmin(request, email, password)` для admin Bearer'а + `fetchApplicationDetail(request, apiUrl, applicationId, token)` для извлечения socialId.
- `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` — chunk 11 `[~]` при старте, `[x]` при merge.

## Tasks & Acceptance

**Execution:**
- [x] API-функция `verifyApplicationSocialManually`.
- [x] `SocialAdminRow` + unit (4 ветки).
- [x] `VerifyManualDialog` + unit (open/cancel/success/4 спец-error + общий NOT_FOUND + pending + double-submit).
- [x] Wire в `ApplicationDrawer` + интеграционные тесты.
- [x] i18n-ключи (`creatorApplications.json` + `common.json`).
- [x] e2e helpers (admin login, fetchDetail, sendpulse-webhook).
- [x] e2e spec — 3 сценария (TT-only заявки; auto+manual coexistence покрыт unit-тестами).
- [x] `make lint-web build-web test-unit-web test-e2e-frontend` — зелёные.
- [ ] Roadmap: `[~]` → `[x]` (ставится после merge PR).

**Acceptance Criteria:**
- Given admin Bearer, заявка `verification` с TG-link и 2 соцсетями (IG `verified=auto`, TT `verified=false`), when admin открывает drawer, then у IG `verified-badge-{igId}.auto`, у TT `verify-social-{ttId}` enabled.
- Given то же, when admin кликает verify TT → submit, then 200; drawer закрылся (`?id=` ушёл); `verification-total` -1; row пропала; admin GET detail — TT `verified=true / method=manual / verifiedByUserId=adminID`; `GET /test/telegram/sent?since={ISO_до_submit}` — пусто (5s polling, см. Design Notes).
- Given то же, when click verify → cancel, then модалка закрыта, drawer открыт, admin GET detail — TT `verified=false`.
- Given заявка БЕЗ TG-link, IG unverified, when admin открывает drawer, then `verify-social-{igId}` disabled; `disabled-hint` виден; click по disabled-кнопке не открывает модалку (`verify-confirm-dialog` count = 0).
- `make lint-web build-web test-unit-web test-e2e-frontend` — зелёные.

## Spec Change Log

(none yet — append-only, populated by step-04 review loops if any)

## Design Notes

**Layout `SocialAdminRow`:** уход с текущего `flex flex-wrap` (горизонтальный compact pack соцсетей в drawer'е). Вертикальный stack — одна соцсеть на строку, `flex items-center justify-between gap-3`: слева существующий `<SocialLink showHandle>`, справа — бейдж (если verified) **или** кнопка/disabled-hint (если нет). Admin-tooling, плотность важнее эстетики компактного списка.

**`/test/telegram/sent?since=...` — точная семантика:** параметр `since` это ISO datetime (см. `openapi-test.yaml`), не магическое слово. Pattern из `backend/e2e/testutil/telegram_sent.go::EnsureNoNewTelegramSent`: захватить `const since = new Date().toISOString()` **перед** click submit, после успеха поллить `GET /test/telegram/sent?since={since}` 5 секунд и валидировать что нет записей с нашим `chatId`. Reference-имплементация — `backend/e2e/creator_applications/manual_verify_test.go` (chunk 10 e2e).

**E2E план (3 сценария):**

1. **Happy path verify TT после auto-verify IG** — seed admin + app с TG-link + IG/TT соцсети. Имитация SendPulse-webhook на IG: `POST /webhooks/sendpulse/instagram` с правильным body + Bearer-секрет из env (готовый pattern в `backend/e2e/webhooks/sendpulse_instagram_test.go` — копировать оттуда). После — IG `verified=auto`. UI: login admin → search uuid → open drawer → assert badges/buttons → захватить `since`-timestamp → click verify TT → modal → submit → drawer закрылся → row пропала → admin GET detail подтверждает manual + moderation; 5s polling `/test/telegram/sent?since={since}` пусто для `chatId`.

2. **Cancel** — тот же seed без auto-verify IG. Click verify TT → modal → cancel → modal закрыт, drawer открыт. Admin GET — TT всё ещё false, app.status=verification.

3. **TG не привязан — disabled** — seed без `linkTelegramToApplication`. Open drawer → button disabled, hint виден, force-click ничего не делает.

**Race 409 — НЕ покрываем e2e** (сложно засидить, заявка после первого verify уходит из экрана). Покрывается unit-тестами `VerifyManualDialog.test.tsx` через mock-error.

**Интерактивный нюанс — Escape:** drawer ловит Escape для close, но при открытой confirm-модалке Escape должен закрывать только модалку. Решение при реализации: либо `e.stopPropagation()` в keydown модалки, либо временно отписаться от drawer-keydown пока модалка открыта.

**Z-index:** drawer на `z-50`. Confirm-modal должна быть выше — `z-60` или новый shared-стек.

## Verification

**Commands:**
- `make generate-api lint-web build-web test-unit-web test-e2e-frontend` — все зелёные.

**Manual smoke (локально):**
- `make compose-up migrate-up && make start-backend start-web`. Создать заявку через лендос (`:3003`), привязать TG через бот, login admin на `:3001`, `/creator-applications/verification` → drawer → «Подтвердить вручную» → Submit. Заявка пропала; в БД `social.verified=true / method=manual`, `app.status=moderation`. В Telegram сообщение **НЕ приходит**.
