# Отложенные работы (deferred-work)

Список pre-existing проблем, обнаруженных по ходу работы, но не относящихся напрямую к текущим спекам. Каждый — кандидат на отдельный chunk / спеку.

## chunk-18 (campaign creators UI statuses)

- **`waitForCcStatus` не детектит terminal-overshoot.** `frontend/e2e/helpers/api.ts:594-629` ждёт ровно `expectedStatus`, но при leftover-webhook'е из предыдущего прогона ряд может сразу оказаться в другом терминале. Сейчас тест уходит в 5-секундный timeout; полезнее fail-fast с конкретным сообщением «overshoot: got X, expected Y».

## frontend nginx cache headers — отдельный chunk

- **Cache-Control headers не настроены на frontend-контейнерах.** `frontend/web/nginx.conf` и `frontend/tma/nginx.conf` отдают всё без cache-инструкций — браузер решает эвристикой, returning users тащат бандл заново при каждой загрузке, после деплоя `index.html` может зависать в кэше и показывать старую версию. Корректный фикс: hashed-assets (`*.js`, `*.css`, шрифты) → `public, max-age=31536000, immutable`; `index.html` → `no-cache, no-store, must-revalidate`. Требует проверки прод-топологии (Cloudflare/CDN перед Dokploy?) и manual-QA на staging перед деплоем. Затронет весь returning-traffic.
