# Deferred work

Поток откладываемых findings — surface'd review'ами, но не сфера текущей задачи. Каждый пункт — кандидат на отдельный intent.

---

## state-machine guard для terminal webhook'ов из non-`signing` cc.status

**Дата:** 2026-05-11
**Источник:** review `spec-trustme-webhook-revoked` (edge-case-hunter).

**Симптом:** Если TrustMe webhook `status=3/4/9` придёт когда `cc.status='agreed'` (контракт не дошёл до `signing`), `webhook_service.applyCampaignCreatorTransition` слепо переключит `cc.status` в `signed`/`signing_declined` через `UpdateStatus` — минуя промежуточное `signing`. CHECK-constraint таблицы разрешает оба перехода, но state-machine инвариант нарушается.

**Почему отложено:** Pre-existing — тот же риск был для `status=9` до фикса. По нормальному pipeline невозможно: webhook прилетает только когда `contracts.trustme_document_id` задан, что выставляется outbox-worker'ом одновременно с `cc.status='signing'`. Отдельный intent — для defensive guard'а (`UpdateStatus` принимает `WHERE current_status = 'signing'`), чтобы non-`signing` webhook возвращал domain-error.

**Триггер для последующего intent'а:** появится в продакшене расхождение состояний из-за race между outbox'ом и webhook'ом, или начнём слать webhook'и из других caller'ов.
