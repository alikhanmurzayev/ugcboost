# Deferred work

Список отложенных задач, обнаруженных по ходу реализации других веток.

## Frontend e2e: PATCH /campaigns/{id}/creators/{creatorId} (ticketSent toggle)

**Origin:** ветка `alikhan/campaign-ticket-sent` (PR на фичу
«чекбокс ‟Билет отправлен”»).

**Why deferred:** UI-flow требует креатора со `status='signed'`, что
по конвейеру TrustMe в e2e достигается через
`testutil.SetupCampaignWithSigningCreator` + webhook signed payload
(см. `backend/e2e/contract/webhook_test.go`). На frontend это
эквивалентно «прогнать TMA agree → outbox tick → TrustMe webhook»,
для чего нет готового composable-хелпера в `frontend/e2e/helpers/`.
Под жёсткий дедлайн фичи покрытие закрыли тремя слоями: backend e2e
полностью пройден через TrustMe-webhook, frontend unit покрывает
optimistic / rollback / toast, backend unit — repo+service+handler.

**Что нужно сделать:**

1. Добавить в `frontend/e2e/helpers/api.ts` helper
   `seedCampaignWithSignedCreator(request, API_URL, adminToken)` —
   симметрично backend-helper'у `SetupCampaignWithSigningCreator`,
   плюс вызов test endpoint для эмуляции TrustMe webhook signed
   (через `/test/...` ручку, если будет создана; иначе — прямой
   POST на `/trustme/webhook` с тестовым токеном).
2. Дополнить `frontend/e2e/web/admin-campaign-creators-mutations.spec.ts`
   единственным `test()` на happy-flow: открыть кампанию,
   найти `[data-testid="campaign-creator-ticket-sent-{creatorId}"]`,
   кликнуть, дождаться `checked=true` после ответа.

## Findings из review-цикла фичи `campaign-ticket-sent` (2026-05-11)

Накопленные defer-findings из трёх review-агентов
(Blind Hunter / Edge Case Hunter / Acceptance Auditor). Severity указан
из reviewer'а. Реально не блокируют фичу — все backend/frontend инварианты
покрыты текущим кодом, но стоит вернуться при первой же расширительной
доработке этого слоя.

- **[major] Single-string `ticketSentError`** —
  `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx`.
  Конкурентные toggle на разных креаторах перетирают друг другу error.
  Когда появится первый toast/notification helper в shared — переключить
  на per-row attribution.
- **[major] Future-field gotcha в `PatchCampaignCreatorInput` empty-check** —
  `backend/internal/handler/campaign_creator.go` проверяет `body.TicketSent == nil`.
  Когда добавим второе поле, надо переключить на проверку всех полей
  (либо struct-level `IsEmpty()` метод на типе).
- **[minor] Stale closure для двойного клика в одном render-tick** —
  `CampaignCreatorsSection.handleToggleTicketSent`. Если admin кликает
  быстрее одного React-ре-рендера, `ticketSentPending.has(creatorId)`
  guard видит stale Set. На практике сегодня сейчас не воспроизводится
  (mutate синхронна), но при росте задержек стоит переехать на
  `useRef<Set>`.
- **[minor] React `setState` после unmount в `onError`/`onSettled`** —
  тот же файл. React 18 не warning'ует, но cosmetic — wrap'нуть через
  `isMountedRef` при первом отчёте о visible bug.
- **[minor] Multi-tab cache desync** — react-query кэш per-tab; toggle в Tab1
  не транслируется в Tab2 без `broadcastQueryClient` или refetch-on-focus.
  Пока админов мало и UX не выходил с этим в проде — defer.
- **[minor] A11y: `role="presentation"` на div с onClick/onKeyDown** в
  `TicketSentCheckbox` (`CampaignCreatorsTable.tsx`). a11y-линтеры могут
  ругаться. Лучше event-stop напрямую на `<input>`, либо снять role.
- **[minor] Test brittleness: assert через `Contains "ticket_sent_at":"`** —
  `backend/e2e/campaign_creator/campaign_creator_test.go` happy-flow проверяет
  audit OldValue substring'ом. Если в будущем уберут `omitempty` у
  `domain.CampaignCreator.TicketSentAt`, тест silent pass (получит
  `"ticket_sent_at":null`). При следующем рефакторинге audit-payload —
  переключиться на полную struct-deserialize + сравнение полей.
