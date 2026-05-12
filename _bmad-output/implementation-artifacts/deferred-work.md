# Deferred work

Findings из review-loop'ов и других контекстов, которые не входят в текущий
scope, но заслуживают внимания позже. Каждая запись — дата, источник,
описание, минимальное обоснование «почему отложили».

## 2026-05-11 — spec-tma-decision-buttons-visibility

### Anti-fingerprint в TMA endpoints отдаёт 401/403/404 разными кодами

**Источник:** review (blind hunter).
**Где:** `backend/api/openapi.yaml` для `tmaAgree` / `tmaDecline` / `tmaGetParticipation`; реальная логика в `backend/internal/authz/tma.go` и handlers `tma_campaign_creator.go`.

Сейчас три разных HTTP-кода (401 для битого initData, 403 для not-in-campaign / not-creator, 404 для невалидного / несуществующего secretToken). По принципу anti-fingerprint злоумышленник может различить «такая кампания есть, но я к ней не привязан» (403) от «такого secretToken нет» (404). Pattern унаследован от существующих agree/decline и не введён этой задачей — но при добавлении read-only ручки enumeration становится дешевле (read не требует state change). Возможный фикс — коллапс 403/404 в один код и стандартизованный generic body. Требует широкого ревью security-инвариантов: захватывает 4 эндпоинта, надо проверить frontend error-handling.

### `useParticipationStatus` не рефетчит при возврате в TMA

**Источник:** review (edge case hunter).
**Где:** `frontend/tma/src/features/campaign/useParticipationStatus.ts`.

Дефолт React Query сейчас `retry: false`, без `staleTime` / `refetchOnMount` / `refetchOnWindowFocus`. Сценарий: креатор открыл бриф (status=invited, кнопки видны), свернул TMA, админ перевёл его в declined, креатор вернулся к открытой странице → видит кнопки, кликает «Согласиться» → 422 NEED_REINVITE на бэке. Сейчас защита — только серверная state-machine, UI не понимает что данные устарели. Возможный фикс: `staleTime: 0, refetchOnMount: "always", refetchOnWindowFocus: true`. Решение про обширность правки тaнут уйти после первых прод-наблюдений (если будут жалобы на этот UX).

### `secretTokenFormat` дублируется backend ↔ frontend

**Источник:** review (blind hunter).
**Где:** `backend/internal/domain/<...>.go` (SecretTokenRegex), `frontend/tma/src/features/campaign/campaigns.ts:387` (`secretTokenFormat`).

Сейчас оба регулярных выражения `^[A-Za-z0-9_-]{16,256}$` — captured как литералы в обоих репозиториях. Изменение в одном месте не ловится тестами в другом. Дёшево было бы экспортировать константу из OpenAPI parameter pattern и сгенерировать TS-константу, либо добавить CI-проверку идентичности. Не блокер сейчас (regex редко меняется), но кандидат на формализацию контракта.
