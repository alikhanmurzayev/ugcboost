---
title: 'Browser e2e — admin moderation screen + reject action'
type: 'feature'
created: '2026-05-05'
status: 'done'
context: []
baseline_commit: 3fc16906f35eb787e36e7bbf7e7b183ccf279859
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Экран admin-moderation (`/creator-applications/moderation`, чанк 16) и reject в статусе moderation покрыты только unit-тестами и ручным smoke. Нет e2e, который доказывает, что специфика moderation (новые колонки, скрытый TG-фильтр, verified-бейджи на соцсетях, approve-placeholder, reject из moderation) работает с реальным backend'ом.

**Approach:** Два spec-файла в `frontend/e2e/web/`. Первый — shape: admin сидит данные через composable-хелперы, прогоняет UI-сценарии, ассертит точные значения каждой ячейки таблицы и каждого поля drawer'а, всех фильтров, sort'а, sidebar-бейджа, RoleGuard. Второй — reject в moderation, симметричен `admin-creator-applications-reject-action.spec.ts` (status `moderation` вместо `verification`).

## Boundaries & Constraints

**Always:**
- Seed только через хелперы `frontend/e2e/helpers/api.ts` + один inline-helper `setupModerationApplication` внутри spec-файла (НЕ в `api.ts` — YAGNI).
- Русский JSDoc-нарратив в начале каждого spec-файла (`frontend-testing-e2e.md` § Комментарий).
- Локаторы — только `data-testid`. `getByText` для копирайт-зависимых строк запрещён.
- Уникальность через uuid в `lastName` + `uniqueIIN()`.
- Cleanup-стек fail-fast с per-call 5s таймаутом (паттерн как в соседних спецах).
- `test.use({ timezoneId: "UTC" })` в обоих spec'ах.
- Точные ассерты значений, не `toContainText` на фрагментах.

**Ask First:**
- Live-badge тест требует ожидания > 5s (refetchInterval 15s) — HALT и предложить `page.reload()`.
- Любое отклонение manual-verify endpoint от OpenAPI-контракта (ожидаемо 200 в verification → 422 в moderation).

**Never:**
- НЕ дублировать тесты, уже покрытые соседним verification-spec'ом (drawer ArrowKeys, drawer-backdrop click, reject-modal-body wording — assert один раз).
- НЕ выносить новые helpers в `api.ts` пока spec-вызовов меньше двух (исключение: `manualVerifyApplicationSocial` — нужен обоим новым spec'ам).
- НЕ тестировать pagination (PER_PAGE=50 — нереалистично).
- НЕ ассертить CSS / layout, точный текст TG-нотификации (контракт chunk-14 backend e2e).

## I/O & Edge-Case Matrix

| Сценарий | Setup | Ожидаемое поведение |
|---|---|---|
| Single IG auto verified в drawer | seed `[IG]` + linkTG + SendPulse webhook → moderation | `verified-badge-{id}` text="Подтверждено · авто" |
| Single TT manual verified в drawer | seed `[TT]` + linkTG + manual-verify API → moderation | `verified-badge-{id}` text="Подтверждено · вручную" |
| Sort cycle на "В этапе" | default updated_at asc; click 1 → desc; click 2 → asc | URL: первый клик `?sort=updated_at&order=desc`; повторный — без sort/order params |
| Все 5 фильтров (search/date/age/city/categories) | каждый — отдельный test, 2 заявки (1 matches, 1 нет) | в таблице остаются только matching строки (точный список); URL содержит param; active-count = 1 |
| Telegram-фильтр в popover | open popover на moderation | `filter-telegram-linked` отсутствует в DOM |
| Multi-filter reset | 2 активных фильтра → click `filters-reset` | URL очищен от фильтров, active-count badge исчез |
| Empty filtered | search = свежий random uuid | `applications-table-empty` виден, table в DOM нет |
| Live badge | seed moderation-заявку → page.reload() | sidebar-бейдж count ≥ 1 |
| RoleGuard | brand_manager → goto /moderation | редирект на `/`, nav-link отсутствует |
| Reject с TG | drawer → reject → confirm-modal TG-body → submit | status=rejected, rejection.fromStatus=moderation, ≥1 TG message |
| Reject без TG | без linkTG → reject → confirm-modal warning-body → submit | status=rejected, fromStatus=moderation, telegram/sent пустой |
| Reject cancel | drawer → reject → cancel | modal закрылся, drawer остался, status=moderation без изменений |

</frozen-after-approval>

## Code Map

- `frontend/e2e/web/admin-creator-applications-moderation.spec.ts` -- НОВЫЙ shape-spec.
- `frontend/e2e/web/admin-creator-applications-moderation-reject-action.spec.ts` -- НОВЫЙ reject-spec.
- `frontend/e2e/helpers/api.ts` -- расширить: `manualVerifyApplicationSocial(request, apiUrl, applicationId, socialId, token)` → POST /creators/applications/{id}/socials/{socialId}/verify.
- `frontend/e2e/web/admin-creator-applications-verification.spec.ts` -- референс по стилю/cleanup/timezone.
- `frontend/e2e/web/admin-creator-applications-reject-action.spec.ts` -- референс по reject-сценариям + `collectTelegramSent`.
- `frontend/e2e/helpers/telegram.ts` -- существующий `collectTelegramSent`.
- `frontend/web/src/features/creatorApplications/ModerationPage.tsx` -- источник правды по data-testid'ам page (`creator-applications-moderation-page`, `moderation-total`).
- `frontend/web/src/features/creatorApplications/components/ApplicationDrawer.tsx` -- testid'ы drawer-полей.
- `frontend/web/src/features/creatorApplications/components/SocialAdminRow.tsx` -- `verified-badge-{id}`.
- `frontend/web/src/features/creatorApplications/components/ApplicationActions.tsx` -- `approve-button` (disabled).
- `frontend/web/src/features/creatorApplications/components/ApplicationFilters.tsx` -- testid'ы фильтров.
- `frontend/web/src/shared/layouts/DashboardLayout.tsx` -- nav-link + sidebar-бейдж.
- `backend/api/openapi.yaml:641-717` -- контракт verify-endpoint'а.

## Tasks & Acceptance

**Execution:**
- [x] `frontend/e2e/helpers/api.ts` -- добавить `manualVerifyApplicationSocial` с bearer-token + типизацией ответа (200) + типизированными exceptions.
- [x] `frontend/e2e/web/admin-creator-applications-moderation.spec.ts` -- НОВЫЙ. Inline-helper `setupModerationApplication(opts)` оборачивает seed → linkTG (опц.) → IG-webhook ИЛИ manual-verify TT. Все shape-кейсы из I/O matrix + AC ниже.
- [x] `frontend/e2e/web/admin-creator-applications-moderation-reject-action.spec.ts` -- НОВЫЙ. Три reject-сценария, переиспользует setupModerationApplication.
- [x] `make test-e2e-frontend` локально — оба зелёные.
- [x] `make lint` — без warnings.

**Acceptance Criteria:**

- Given Happy seed (IG-only verified-auto, middleName, address, 2 categories + Other, city=almaty, TG привязан), when admin кликает по строке, then drawer показывает: `drawer-full-name`="Last First Middle", timeline="Подана: <full date>", `drawer-birth-date` точно по IIN с pluralYears, `drawer-iin` точный, `application-phone` href="tel:+77...", `drawer-city`="Алматы", все category-чипы (включая `drawer-category-other-text`), `drawer-telegram-linked`="@<username>", footer = reject-кнопка + `approve-button` disabled с title="Скоро".
- Given Happy seed без middleName и без linkTG, then drawer-full-name = "Last First" без trailing-space, `drawer-telegram-not-linked` + `drawer-copy-bot-message` видны.
- Given таблица отрендерена с одной заявкой, then проверяем точные значения каждой ячейки строки: `№`="1", ФИО="Last First", socials с handle, categories chips + Other, city="Алматы", Подана-дата (короткий формат), `hours-badge` (с проверкой формата "<1ч"/"Nч"/"Nд").
- Given `<thead>`, then ячейка "Город" присутствует, ячейка "Telegram" отсутствует.
- Given любая moderation-заявка, when admin click `approve-button`, then click игнорируется (drawer state без изменений, no API request).
- Given 3 заявки с разными `lastName`/`city`/`createdAt`/`updatedAt`, when click sort-headers ФИО/Подана/В этапе/Город, then URL получает соответствующий sort-param + порядок строк меняется. Дополнительно: default sort moderation = updated_at asc подтверждён (URL без params на загрузке).
- Given для каждого фильтра — два сидованных набора, when admin применяет фильтр через UI, then в таблице остаются только matching строки (точный список по `lastName`-uuid), URL содержит param, active-count badge = "1".
- Given popover открыт, then `filter-telegram-linked` отсутствует в DOM.
- Given 2 активных фильтра + reset, then URL без фильтров, active-count badge исчезает.
- Given свежий random-uuid в search, then `applications-table-empty` виден с текстом `t("emptyFiltered")`, ApplicationsTable не в DOM.
- Given seed moderation-заявки + open page + `page.reload()`, then sidebar-бейдж на `nav-link-creator-applications/moderation` показывает count ≥ 1.
- Given brand_manager logged in, when goto `/creator-applications/moderation`, then редирект на `/`, nav-link на moderation отсутствует.
- Given moderation-заявка с TG → reject через drawer, then admin GET → `status=rejected`, `rejection.fromStatus=moderation`, `rejection.rejectedByUserId=adminID`; `collectTelegramSent` вернёт ≥1 message за 5s.
- Given moderation-заявка без TG → reject, then status=rejected/fromStatus=moderation, telegram/sent пустой.
- Given moderation-заявка с TG → reject → cancel, then drawer остался, status=moderation без изменений.

## Spec Change Log

## Verification

**Commands:**
- `make test-e2e-frontend` -- оба новых spec-файла зелёные, соседние не сломаны.
- `make lint` -- 0 warnings/errors во frontend/e2e/.

## Suggested Review Order

**Композабельный setup данных в moderation**

- Header JSDoc — связный нарратив всех 15 сценариев и точных ассертов.
  [`admin-creator-applications-moderation.spec.ts:1`](../../frontend/e2e/web/admin-creator-applications-moderation.spec.ts#L1)

- Главный seed-helper: seed → linkTG → IG-webhook + post-promotion verify (защита от silent webhook no-op).
  [`admin-creator-applications-moderation.spec.ts:1016`](../../frontend/e2e/web/admin-creator-applications-moderation.spec.ts#L1016)

- Новый API-helper для admin manual-verify endpoint'а — bearer + `data: {}` для Content-Type.
  [`api.ts:492`](../../frontend/e2e/helpers/api.ts#L492)

**Точные ассерты UI ↔ data (Happy path)**

- Resolve city/categories через dictionary endpoint, не hardcoded copy — устойчиво к изменению копи.
  [`admin-creator-applications-moderation.spec.ts:159`](../../frontend/e2e/web/admin-creator-applications-moderation.spec.ts#L159)

- Каждая ячейка строки таблицы сравнивается точно; hours-badge через regex — robust к границе <1ч/1ч.
  [`admin-creator-applications-moderation.spec.ts:218`](../../frontend/e2e/web/admin-creator-applications-moderation.spec.ts#L218)

- Все поля drawer'а через UTC-safe форматтеры; verified-badge "Подтверждено · авто" + approve disabled "Скоро".
  [`admin-creator-applications-moderation.spec.ts:240`](../../frontend/e2e/web/admin-creator-applications-moderation.spec.ts#L240)

**Variants verified-бейджа**

- TT-only через manual-verify API → drawer показывает "Подтверждено · вручную"; assert `verification` перед manualVerify (422 на moderation).
  [`admin-creator-applications-moderation.spec.ts:296`](../../frontend/e2e/web/admin-creator-applications-moderation.spec.ts#L296)

- Без middleName и без TG → ФИО collapse'ится без trailing-space + drawer-copy-bot-message виден.
  [`admin-creator-applications-moderation.spec.ts:374`](../../frontend/e2e/web/admin-creator-applications-moderation.spec.ts#L374)

**Sort + фильтры через UI**

- Sort cycle с явным sleep 60ms между webhooks для детерминизма по updated_at; все 4 sort-headers.
  [`admin-creator-applications-moderation.spec.ts:490`](../../frontend/e2e/web/admin-creator-applications-moderation.spec.ts#L490)

- Filter age — wait между двумя fill'ами против stale-closure race в setSearchParams.
  [`admin-creator-applications-moderation.spec.ts:680`](../../frontend/e2e/web/admin-creator-applications-moderation.spec.ts#L680)

- Filter cities/categories — explicit visibility wait для async dict load + `.click()` вместо `.check()`.
  [`admin-creator-applications-moderation.spec.ts:749`](../../frontend/e2e/web/admin-creator-applications-moderation.spec.ts#L749)

- Telegram-фильтр СКРЫТ; structural assert `<thead>` count=7 + th-telegram отсутствует.
  [`admin-creator-applications-moderation.spec.ts:441`](../../frontend/e2e/web/admin-creator-applications-moderation.spec.ts#L441)

**Reject в moderation**

- Setup-helper для reject-spec'а с post-promotion verify; разделён от shape-spec'а (reuse внутри файла, YAGNI вне).
  [`admin-creator-applications-moderation-reject-action.spec.ts:342`](../../frontend/e2e/web/admin-creator-applications-moderation-reject-action.spec.ts#L342)

- Happy с TG: status=rejected + fromStatus=moderation + ≥1 TG-сообщение через collectTelegramSent.
  [`admin-creator-applications-moderation-reject-action.spec.ts:91`](../../frontend/e2e/web/admin-creator-applications-moderation-reject-action.spec.ts#L91)

- Без TG / Cancel — симметрично verification-spec'у, но fromStatus=moderation.
  [`admin-creator-applications-moderation-reject-action.spec.ts:188`](../../frontend/e2e/web/admin-creator-applications-moderation-reject-action.spec.ts#L188)

**TZ-safe форматтеры и dictionary lookup (peripherals)**

- formatShortDate / formatLongDateTime / formatBirthDateUTC — `timeZone:"UTC"` или manual UTC math; защита от Node-runner ≠ browser TZ.
  [`admin-creator-applications-moderation.spec.ts:1165`](../../frontend/e2e/web/admin-creator-applications-moderation.spec.ts#L1165)

- fetchDictionary + lookupOrThrow — паттерн из verification-spec'а, разрешает копи через live API.
  [`admin-creator-applications-moderation.spec.ts:1187`](../../frontend/e2e/web/admin-creator-applications-moderation.spec.ts#L1187)

**Boundaries и sidebar**

- Live badge: `>= 1` вместо `before+1` — защита от race с параллельным cleanup'ом другого воркера.
  [`admin-creator-applications-moderation.spec.ts:945`](../../frontend/e2e/web/admin-creator-applications-moderation.spec.ts#L945)

- RoleGuard: brand_manager не видит nav-link, прямой goto редиректит на `/`.
  [`admin-creator-applications-moderation.spec.ts:981`](../../frontend/e2e/web/admin-creator-applications-moderation.spec.ts#L981)
