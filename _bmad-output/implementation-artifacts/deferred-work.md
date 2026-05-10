# Отложенные работы (deferred-work)

Список pre-existing проблем, обнаруженных по ходу работы, но не относящихся напрямую к текущим спекам. Каждый — кандидат на отдельный chunk / спеку.

## chunk-18 (campaign creators UI statuses)

- **STATUSES_WITHOUT_REMOVE drift с backend.** `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx:34-43` скрывает trash для `agreed/signing/signed/signing_declined`. Backend (`backend/internal/service/campaign_creator.go:130`) блокирует remove ТОЛЬКО для `agreed`. Если кто-то соберётся ужесточить либо ослабить серверную проверку, фронт молча разойдётся. Решение: расширить backend-guard на signing/signed/signing_declined одной forward-миграцией, либо вынести список «нельзя удалять» в backend и подтянуть на фронт через generated.

- **TrustMe spy не чистится между e2e-тестами.** `frontend/e2e/web/admin-campaign-creators-trustme.spec.ts` плюс существующие специи опираются на `findTrustMeSpyByIIN(iin)`. IIN практически уникален (`uniqueIIN()` ~70M пула), но spy store глобальный per backend-процесс — collision теоретически возможна при retry-prone сценариях или при контаминированной staging-БД. Решение: добавить вызов `POST /test/trustme/spy-clear` в `beforeEach` (или общий test-fixture).

- **`waitForCcStatus` не детектит terminal-overshoot.** `frontend/e2e/helpers/api.ts:594-629` ждёт ровно `expectedStatus`, но при leftover-webhook'е из предыдущего прогона ряд может сразу оказаться в другом терминале. Сейчас тест уходит в 5-секундный timeout; полезнее fail-fast с конкретным сообщением «overshoot: got X, expected Y».
