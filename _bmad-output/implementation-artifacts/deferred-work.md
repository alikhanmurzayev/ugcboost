# Deferred Work

Findings, отложенные на отдельные итерации, найденные при review chunk'ов BMad. Living, дополняется по мере появления.

## Из chunk 13 (campaign-notifications-front), 2026-05-08

- **`onSettled` → setState на размонтированном компоненте.** `CampaignCreatorGroupSection` вызывает `setCheckedCreatorIds`/`setIsSubmitting`/`setResult` в `onSettled`, но если пользователь ушёл со страницы или группа размонтировалась после refetch, callback всё равно сработает — React выкинет warning. Нужен `isMountedRef` или `AbortController`. Низкий приоритет — warning, не баг.

- **404 CAMPAIGN_NOT_FOUND отдельной веткой `parseSettled`.** Сейчас всё, что не 422 batch-invalid, схлопывается в `network_error` («Не удалось разослать. Попробуйте снова.»). Если кампанию soft-delete'нули из другой вкладки, админ зацикливается на ретраях вместо явного «Кампания удалена». Нужно расширить `parseSettled` под 404 → отдельный inline-результат + invalidate `campaignKeys.detail` чтобы страница перерендерилась как deleted. Соответствует спеке («network/5xx → generic»), но UX страдает.

- **Shared `mutation.isPending` блокирует кнопки соседних групп.** `useCampaignNotifyMutations(campaignId)` отдаёт один `notify` mutation на `planned` и `declined`. Пока `planned` шлёт, кнопка `declined` тоже disabled. Спека требует «один экземпляр», но UX-нюанс. Возможные решения: per-group hook или сравнение `mutation.variables`.

- **Стейл-id в `checkedCreatorIds` после refetch соседней группы.** Если invalidate из `declined`-submit перерисовал `planned` без выбранного креатора, его id остался в `checkedCreatorIds`. Submit отправит стейл-id → бэк ответит 422 BATCH_INVALID. UX-шум. Фикс: `useEffect` или `useMemo` с пересечением selection и текущих rows.

- **`as`-cast в `extractErrorParts`.** `frontend/web/src/api/campaignCreators.ts` — legacy паттерн, существовал до chunk 13, нарушает `frontend-quality.md` (`as` запрещён). Надо переписать на type guards. Tracking-issue: пройти все API-обёртки одним PR.

- **e2e race-422 контракт.** Тест `campaign-notify.spec.ts` полагается на backend-семантику «весь batch отказывается на одном wrong_status». Сейчас работает, но нет contract-теста, который зафиксирует это явно. При backend-рефакторе тест может неожиданно мигать. Defer пока chunk 14+ не тронет batch-семантику.
