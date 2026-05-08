# Deferred Work

Findings, отложенные на отдельные итерации, найденные при review chunk'ов BMad. Living, дополняется по мере появления.

## Из chunk 13 (campaign-notifications-front), 2026-05-08

- **404 CAMPAIGN_NOT_FOUND отдельной веткой `parseSettled`.** Сейчас всё, что не 422 batch-invalid и не 422 other, схлопывается в `network_error`. Если кампанию soft-delete'нули из другой вкладки, админ зацикливается на ретраях вместо явного «Кампания удалена». Нужно расширить `parseSettled` под 404 → отдельный inline-результат + invalidate `campaignKeys.detail` чтобы страница перерендерилась как deleted.

- **Shared `mutation.isPending` блокирует кнопки соседних групп.** `useCampaignNotifyMutations(campaignId)` отдаёт один `notify` mutation на `planned` и `declined`. Пока `planned` шлёт, кнопка `declined` тоже disabled. Спека требует «один экземпляр», но UX-нюанс. Возможные решения: per-group hook или сравнение `mutation.variables`. (Result-lift в parent уже изолировал submitting-state per status, но `isPending` mutation всё ещё shared.)

- **e2e race-422 контракт.** Backend e2e уже фиксирует поведение в `backend/e2e/campaign_creator/campaign_notify_test.go` (`batch-invalid: not_in_campaign + wrong_status`). Frontend race-422 тест полагается на тот же контракт. Drift минимален, но если backend поменяет «один wrong_status → весь batch отказ» → нужна пересинхронизация с фронтом.

- **escapeValue: false → true миграция.** `frontend/web/src/shared/i18n/config.ts`. Pre-existing. React автоэкранирует children, но `<Trans>` / `dangerouslySetInnerHTML` в будущем останутся unescaped. Прямое flipping ломает clipboard-flow в `VerificationPage` (URL-слеши становятся `&#x2F;`), поэтому нужен отдельный PR: пройтись по всем `t()`-callsites, выделить sanitised wrapper для clipboard, потом включить escape.

- **Sort внутри группы.** В `invited`/`declined` логично «по времени смены статуса» (`decidedAt`/`invitedAt` desc), сейчас порядок наследуется от `listCreators` (по profile created_at). Не chunk-13 проблема, но стоит chunk 15.

- **Dead exports `CampaignNotifyUndeliveredReason`, `CampaignCreatorBatchInvalidError`.** Re-export'нуты в `frontend/web/src/api/campaignCreators.ts` под будущих consumer'ов. `CampaignCreatorBatchInvalidDetail` теперь используется в `notifyResult.ts`, а первые два — нет. Если chunk 14+ их так и не подберёт — удалить.

- **Cleanup-стек избыточен** в `campaign-notify.spec.ts`. `removeCampaignCreator` push'ы могут быть лишними, если `cleanupCampaign` каскадит campaign_creators. Проверить и либо удалить, либо явно прокомментировать «test-эндпоинт не каскадит».

- **`<section>` отдельно от `aria-labelledby` для group sub-elements.** Heading-блок группы теперь связан через `aria-labelledby`, но внутренние подсекции (action button, result block) — раздельные landmarks без явной связи. Если поднимем landmark-discipline по проекту, формализовать единый паттерн.
