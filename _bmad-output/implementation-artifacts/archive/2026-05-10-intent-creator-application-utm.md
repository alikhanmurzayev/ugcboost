# Intent: UTM-метки в заявках креаторов с лендинга

> Стандарты проекта (`docs/standards/`) загружаются полностью реализующим
> агентом перед началом работы. Этот intent-файл — living, итерируется в
> spec-mode и передаётся bmad-quick-dev.

## Контекст

Маркетинг хочет различать источники трафика (специальные чаты / рассылки),
по которым креаторы попадают на лендинг и подают заявку. Решение —
стандартные `utm_*` параметры в URL: лендинг ловит их при заходе,
сохраняет, шлёт с заявкой; админ видит их в деталях заявки на этапе
верификации и модерации.

## Тезис

Лендинг ловит стандартные `utm_*` параметры (source/medium/campaign/term/content)
из query-string при заходе, кладёт в sessionStorage и шлёт вместе с submit.
Бэк добавляет 5 nullable text-колонок в `creator_applications` через новую
миграцию, отдаёт их только в `CreatorApplicationDetailData` (не в list),
админский drawer показывает блок «Источник трафика» с непустыми парами под
основными полями — единое место отображения покрывает и верификацию, и
модерацию.

## Решённые вопросы

- Поля: 5 стандартных `utm_*` (source / medium / campaign / term / content),
  все nullable, `maxLength=256`.
- Хранение: 5 отдельных колонок (без JSONB).
- Видимость: только в `CreatorApplicationDetailData` (детальный endpoint),
  НЕ в list / counts / TMA.
- UI-точка: общий `ApplicationDrawer` — покрывает обе страницы (модерация + верификация).

## User-facing flow

**Лендинг (Astro):**
- На `DOMContentLoaded` парсим 5 ключей `utm_*` из `window.location.search`.
  Хотя бы один непустой → перезаписываем `sessionStorage.ugc_utm` JSON-объектом
  (last-click model). Reload без UTM не чистит storage.
- `collectFormData` дочитывает `sessionStorage.ugc_utm`, кладёт непустые поля
  в payload submit-запроса.

**Web admin (`ApplicationDrawer.tsx`):**
- Новая секция «Источник трафика» сразу после блока контактов, до соцсетей.
- Все 5 полей `null` → секцию не рендерим.
- Иначе — список `label: value` для непустых пар, лейблы через
  `t("creatorApplications.utm.*")`. Контейнер и значения с `data-testid`
  (`utm-section`, `utm-source-value`, …).

## API + поля + валидация

**OpenAPI (`backend/api/openapi.yaml`):**
- `CreatorApplicationSubmitRequest` += 5 опциональных nullable strings:
  `utmSource`, `utmMedium`, `utmCampaign`, `utmTerm`, `utmContent`,
  `maxLength: 256` каждое. Pattern не задаём.
- `CreatorApplicationDetailData` += те же 5 полей (nullable).
- `CreatorApplicationListItem` — НЕ трогаем.
- `openapi-test.yaml` — НЕ трогаем (UTM не нужен в test-API).
- Flat поля, без вложенного `utm: {...}` — единообразно с `categoryOtherText`.
- После правок YAML — `make generate-api` (server.gen.go, schema.ts × 3
  фронта, e2e-клиенты).

**Backend handler:**
- Trim + empty-after-trim → nil для каждого UTM-поля.
- `maxLength: 256` валидирует strict-server из спеки.

**Domain input + сервис:**
- Domain input расширяется 5 `*string`, сервис проксирует в
  `CreatorApplicationRow` без бизнес-логики (plain metadata).

## Миграция + repository

**Новая goose-миграция `creator_applications_utm.sql`:**

Up — `ALTER TABLE creator_applications ADD COLUMN utm_source TEXT, ... utm_content TEXT;`
(5 nullable text-колонок без default).

Down — симметричный `DROP COLUMN` × 5.

Без CHECK на длину (валидация в спеке/handler), без индексов
(UTM не используется в WHERE/ORDER BY).

**Repository (`creator_application.go`):**
- `CreatorApplicationRow` += 5 `*string` с тегами `db:"utm_*"` + `insert:"utm_*"`
  — автоматически попадают в `creatorApplicationSelectColumns` и в стандартный INSERT.
- 5 новых констант `CreatorApplicationColumnUTM{Source,Medium,Campaign,Term,Content}`.
- `CreatorApplicationListRow` не трогаем.

## Audit

- Submit-событие audit_log пишет UTM в `details` JSON (в той же tx, что и
  INSERT в `creator_applications` — без изменений транзакционной модели).
- В `details` включаем только non-null UTM-поля, чтобы не раздувать payload
  прямых заходов без меток. Формат — flat-ключи `utm_source`, `utm_medium`, ...
  (зеркало именования колонок БД).

## Тесты

**Backend unit (handler/service/repository — coverage-gate ≥80%):**
- `handler/creator_application_test.go` — captured-input ассерт: UTM-поля
  прокидываются из request в domain-input; trim+empty→nil для `"   "`;
  полный набор / partial / отсутствие UTM.
- `service/creator_application_test.go` — UTM прокидывается в `CreatorApplicationRow`
  без модификации; audit-call получает `details` JSON с непустыми UTM-полями
  (`mock.Run` + `JSONEq`).
- `repository/creator_application_test.go` — INSERT включает 5 utm-колонок в
  SQL и в args; SELECT-проекция включает их; round-trip Row сохраняет UTM.

**Backend e2e (`backend/e2e/creator_application/`):**
- В существующий happy-path submit-тест добавить вариант с полным набором UTM:
  detail-эндпоинт возвращает их обратно, `testutil.AssertAuditEntry` видит UTM
  в `details` (расширить хелпер при необходимости). Отдельный `t.Run` без UTM:
  поля в response = `null`, audit `details` не содержит UTM-ключей.

**Frontend unit:**
- `landing` (vitest) — функция parse+persist UTM из `location.search`
  (mock `window.location` + `sessionStorage`): полный / частичный набор /
  пустой query / последовательные заходы (last-click перезаписывает).
- `web` — `ApplicationDrawer.test.tsx`: рендерит секцию когда есть UTM,
  не рендерит когда все null, рендерит только непустые ключи. i18n через
  реальный `I18nextProvider` с переводами.

**Frontend e2e:**
- `landing/e2e` — открыть лендинг с
  `?utm_source=test_chat&utm_campaign=spring`, заполнить форму, submit,
  через `helpers/api.ts` дёрнуть detail-API и проверить UTM в response.
- `web/e2e` — необязательно (drawer-секция покрыта unit-тестом).

## i18n

`frontend/web/src/locales/ru/creatorApplications.json` += ключи:
- `utm.title` («Источник трафика»)
- `utm.source` / `utm.medium` / `utm.campaign` / `utm.term` / `utm.content`
  (русские лейблы стандартных UTM-параметров).

## Out of scope

- Фильтрация / сортировка по UTM в админ-таблице (по требованию — только детали).
- Аналитика / агрегации по UTM на стороне бэка (хранение и отдача — да; counts — нет).
- TMA-фронт — не показывает UTM креатору.
- `openapi-test.yaml` / тестовое API — не трогаем.

## История итераций

- 2026-05-10 — тезис принят.
- 2026-05-10 — user-facing flow зафиксирован (last-click sessionStorage, drawer-секция).
- 2026-05-10 — API: 5 flat nullable полей `utm{Source,Medium,Campaign,Term,Content}`, `maxLength=256`.
- 2026-05-10 — миграция: 5 nullable TEXT-колонок без CHECK/индексов, repository — расширение Row + константы.
- 2026-05-10 — audit: UTM пишется в `details` JSON submit-события (только non-null поля).
- 2026-05-10 — тесты по слоям зафиксированы.
- 2026-05-10 — cohesion check пройден (i18n, data-testid, generate-api, TMA-scope, openapi-test scope).
