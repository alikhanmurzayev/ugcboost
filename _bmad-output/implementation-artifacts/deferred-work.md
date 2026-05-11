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
  **UPD 2026-05-11.** Закрыт во 2-м раунде review: omitempty убран,
  ассерт переведён на `auditValueMap` (генерик-map) с явной проверкой
  `null` vs not-null.

## Findings из 2-го раунда review фичи `campaign-ticket-sent` (2026-05-11)

Что вернулось в defer после доп. ревью (test/security/frontend-codegen/manual-qa).
Реализация фичи закрывает все AC; перечисленное — known constraints
для следующего касания этого слоя.

- **[blocker→defer] Status check ordering для `unset` на не-`signed` строке.**
  `PatchParticipation` сначала отвергает `status != signed`, потом делает
  no-op early return. Если status уехал из `signed` после set (теоретический
  путь: revoke / future business flow), админ не сможет снять `ticket_sent_at`
  — будут 422. Сейчас status из `signed` не уезжает (нет business flow для
  «откат подписания»), так что practical impact нулевой. Если такой переход
  появится — пересмотреть: либо no-op перед status check, либо отдельный
  «reset ticket» admin-эндпоинт.
- **[major] Non-repeatable read + lost-update window (Patch/Add/Remove).**
  `assertCampaignActive` + `GetByCampaignAndCreator` бегут на pool вне tx.
  Между ними и UPDATE другой админ может soft-delete campaign / сменить
  status, и audit OldValue будет stale. Применимо ко всему campaign_creator
  слою (`ApplyInvite/ApplyRemind/ApplyDecision` тоже так). Лечится единым
  `GetByIDForUpdate` внутри WithTx — реструктуризация, делаем когда придёт
  первый реальный конфликт.
- **[major] Concurrent admin toggles — race без FOR UPDATE.** Два админа
  одновременно нажимают checkbox для одного row: оба читают TicketSentAt=null
  на pool, оба попадают в UPDATE, audit пишет два диффа с одним и тем же
  pre-image. Сейчас single-admin workflow → не воспроизводится.
- **[major] Future-field gotcha в handler empty-body 422.** Когда добавим
  второе toggleable поле, текущий guard `request.Body.TicketSent == nil`
  начнёт выдавать `CAMPAIGN_CREATOR_PATCH_EMPTY` для запроса с другим полем
  — переключить на struct-level `IsEmpty()` или дженерик-check.
- **[minor] Audit JSON snake_case vs API camelCase.** `domain.CampaignCreator`
  serialize'ится в audit_logs как `ticket_sent_at`, а API схема отдаёт
  `ticketSentAt`. Внешний просмотр audit-логов (export, отчёт) удивит
  читателя. Универсальная политика именования audit-payload — отдельная
  задача.
- **[minor] React `setState` после unmount в `usePatchCampaignCreator`.**
  Если admin кликает чекбокс и сразу уходит со страницы — `onSettled`
  попытается обновить state размонтированного компонента. React 18 не
  warning'ует, но cosmetic. Wrap'нуть через `isMountedRef` при первом
  visible bug.
