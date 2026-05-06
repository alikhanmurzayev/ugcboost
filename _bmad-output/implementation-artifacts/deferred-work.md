# Deferred work

Findings из ревью, не закрываемые в рамках исходного PR'а.

## Из chunk #6 (`spec-campaign-list-backend.md`, ревью 2026-05-06)

### oapi-codegen v2 enum-naming heuristic

**Находка** (Acceptance auditor #1, Blind hunter #9): добавление `CampaignListSortField` в `openapi.yaml` спровоцировало переименование констант существующего `CreatorListSortField` с длинных имён (`CreatorListSortFieldCreatedAt`) на короткие (`CreatedAt`) — oapi-codegen v2 для enum'ов с пересекающимися значениями делегирует префикс одному из enum'ов по непрозрачной эвристике. Текущий PR подвинул эту эвристику и пришлось руками править `creator_test.go`/`creators/list_test.go`.

**Стабилизатор найден:** `compatibility.always-prefix-enum-values: true` в config-файле `oapi-codegen` восстанавливает префиксы для ВСЕХ enum'ов, но это глобально включает префиксы для уже-используемых коротких имён (`api.Admin`, `apiclient.Instagram`, `apiclient.Tiktok`, `testclient.Campaign` и т.п.) — ~50+ мест по codebase, требующих массового переименования (`api.Admin` → `api.UserRoleAdmin`, и т.д.).

**Что сделать:** отдельным PR'ом
1. Добавить `backend/api/oapi-codegen.yaml` с `compatibility.always-prefix-enum-values: true`.
2. Обновить `Makefile` `generate-api` на использование `-config`.
3. Применить mass-rename по всему backend codebase (`api.Admin` → `api.UserRoleAdmin`, `apiclient.Instagram` → `apiclient.SocialPlatformInstagram` и т.п.).
4. Обновить `docs/standards/backend-codegen.md` правилом «всегда use full enum-prefix через config».

Это уберёт класс drift'а навсегда.

### Cross-cutting list-pagination invariants

**Edge case hunter #6 + #10 + Blind hunter #11:**
- Decoupled count+page без атомарного `COUNT(*) OVER ()` может рассинхронизировать total/items при concurrent insert/delete (window race). Применимо ко всем list-эндпоинтам (creators, applications, campaigns).
- E2E sort-сценарии полагаются на `time.Sleep(1100ms)` между seed'ами для разделения created_at — на медленной CI это может схлопнуться, и tie-breaker `id ASC` поменяет ожидаемый порядок.
- При появлении `DELETE /campaigns/{id}` (chunk #7) marker-scoped тест `isDeleted=true` может перестать давать стабильный 0 если сосед-тест seed'ит soft-deleted ряд.

**Что сделать:** документировать count↔page race в `docs/standards/backend-repository.md` § Пагинация; либо ввести стандарт «list через `COUNT(*) OVER ()` window»; рассмотреть e2e-helper для разделения created_at через явный SQL (`set CURRENT_TIMESTAMP`).

### Search trim не покрывает zero-width Unicode

**Edge case hunter #2:** `strings.TrimSpace` оставляет `​` / `‌` / `﻿` / RTL-marks; если admin вставит из IDE/Telegram такой токен в поиск — фильтр НЕ отключится и выдача будет пустой при наличии данных.

**Что сделать:** добавить utility `trimUnicodeWhitespace` или явный фильтр zero-width characters; покрыть unit-тестом. Применимо ко всем search-полям (creator_application, creator, campaign).
