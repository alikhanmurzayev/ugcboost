---
title: "Deferred work — found by review, not caused by this story"
type: backlog
status: living
created: "2026-05-06"
---

Findings, всплывшие во время review-loop'ов, но НЕ относящиеся к текущей истории. Решаем отдельными PR'ами, когда дойдут руки. Каждая запись = `<кратко> + <проявление> + <место первой встречи>`.

## Frontend / API wrappers

- **`response.status` без null-coalesce.** Все обёртки в `frontend/web/src/api/*.ts` (`listCampaigns`, `createCampaign`, `audit.ts`, `brands.ts`, …) делают `throw new ApiError(response.status, …)` напрямую. При network failure `response` может быть `undefined` → `TypeError` вместо аккуратного `ApiError`. Cross-cutting: чинится один раз на уровне generated client wrapper или хелпера. Найдено: blind-hunter + edge-case-hunter в chunk 8a.

## Frontend / mutation lifecycle

- **`navigate()` / `setState()` после unmount.** `useMutation` `onSuccess` / `onError` могут стрелять после того как пользователь ушёл со страницы (back-link, browser back). React 18 молча проглатывает, но логически некорректно — пользователя может дёрнуть с другой страницы; AbortController через RQ `signal` не передаётся. Cross-cutting: касается всех форм с `useMutation` + redirect/back-кнопкой (CampaignCreatePage, LoginPage, CreateBrandForm, …). Найдено: edge-case-hunter в chunk 8a.

- **Unmapped error code → молчаливый «Произошла ошибка» без логов.** `getErrorMessage` (errors.ts) возвращает `errors.unknown` если ключа нет, но не пишет `console.warn`/`data-error-code`. Дебаг production-инцидентов с новым backend-кодом — невозможен без full network trace. Найдено: edge-case-hunter в chunk 8a.

## Frontend / accessibility

- **`role="alert"` без re-mount при смене текста.** Если форма-level alert меняет `formError` без перемонтирования узла, screen-reader может не объявить новый текст. Касается всех форм с persistent alert-узлом. Найдено: edge-case-hunter в chunk 8a.

## E2E / cleanup robustness

- **`withTimeout(fn(), …)` без try/catch теряет хвост `cleanupStack`.** Если первый cleanup упадёт, остальные не выполнятся → утечка тестовых пользователей / row'ов. Один и тот же паттерн в `admin-campaigns-list.spec.ts`, `admin-campaign-create.spec.ts`, и в моде ration / verification specs. Cross-cutting: вынести в общий `helpers/cleanup.ts` с try/catch + AggregateError. Найдено: blind-hunter + edge-case-hunter в chunk 8a.

## Frontend / UX polish (low priority)

- **`<input type="text">` для URL-полей без `inputMode="url"` / `autoCapitalize="off"` / `spellCheck={false}`.** На мобильных браузерах URL'ы коверкаются автокоррекцией. Web-админка — desktop-only, поэтому не блокер; вернёмся когда/если включим mobile-flow. Найдено: edge-case-hunter в chunk 8a.
