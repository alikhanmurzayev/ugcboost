---
title: 'Deferred work — not-this-story findings'
created: '2026-05-11'
status: 'living'
---

# Отложенные findings

Сюда складываются review-findings, помеченные `defer` — pre-existing issues или мелкие улучшения, не блокирующие текущий PR. Каждый пункт — кандидат на отдельный тикет.

## Из доп. раунда ревью PR #105 (creator-campaigns-list), 2026-05-11

- **Loading vs Empty state в drawer-блоке кампаний.** Сейчас `CampaignsBlock` показывает empty-state и для случая «detail ещё грузится», и для случая «у креатора 0 кампаний». Acceptance auditor + Blind hunter (`m14`, `A6`, `D7`). Нужен отдельный Loading-индикатор. Источник: `frontend/web/src/features/creators/CreatorDrawerBody.tsx`, секция `CampaignsBlock`. Стандарт: `frontend-components.md` § «Loading / Error / Empty states — обязательны».

- **Regression-тест на LIFO-cleanup soft-deleted кампании.** Текущий `SetupCampaign` + `SoftDeleteCampaign` paired pattern не покрыт прямым тестом; если `RegisterCampaignCleanup` когда-нибудь начнёт skip'ать soft-deleted — обе e2e-фичи фейлятся silently. Test auditor (`T5`). Локация: `backend/e2e/testutil/campaign.go`.

- **`RegisterCampaignCreatorForceCleanup` — один testclient на cleanup-стек.** Сейчас создаётся новый `testclient` per cleanup invocation. Blind hunter (`M8`). Хотя cleanup happens once, накапливает file descriptors / DNS entries в параллельных suite'ах. Локация: `backend/e2e/testutil/campaign_creator.go:1244`.

- **Explicit IN-list cap в `CampaignCreatorRepo.ListByCreatorIDs`.** Сейчас защищает `perPage = 50` (CreatorListPerPageMax), но в коде нет явного guard'а. PG parameter limit ~65535. Blind hunter (`M10`). Локация: `backend/internal/repository/campaign_creator.go:1914`.

- **Defensive UI fallback для unknown CampaignCreatorStatus.** Если бэкенд однажды добавит статус, а фронт не задеплоится — row тихо исчезнет из drawer'а (`filterCampaignsForGroup` не матчит ни одну группу). Edge case hunter (`E2`). Нужен «unknown» bucket или warn в console. Локация: `frontend/web/src/features/creators/CreatorDrawerBody.tsx:248-255`.

- **Audit log для test-only `MarkCampaignDeleted` — открытый вопрос.** Test endpoint меняет business state без аудита; в текущем codebase audit для read-only test endpoint'ов не предусмотрен, но soft-delete мутирует. Blind hunter (`m10`). Локация: `backend/internal/handler/testapi.go:248`. Решить, нужен ли audit для test-only мутаций.

- **Дополнить Code Map спеки упоминанием `CreatorRepoFactory.NewCampaignCreatorRepo/NewCampaignRepo`.** Acceptance auditor (`A5`). Локация: `_bmad-output/implementation-artifacts/spec-creator-campaigns-list.md` § Code Map. Cosmetic, но Code Map должна быть полной.

- **`openapi-test.yaml` — явный 500 response для `/test/campaign-creators/force-cleanup`.** Blind hunter (`M7`). В коде handler возвращает 500 при repo-error, но контракт декларирует только 204/404. Локация: `backend/api/openapi-test.yaml`.
