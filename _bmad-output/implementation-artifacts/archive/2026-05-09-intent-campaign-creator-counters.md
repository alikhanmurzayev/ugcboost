---
title: "Intent: chunk 15 — счётчики и timestamps попыток на странице кампании"
type: intent
status: living
created: "2026-05-08"
roadmap: "_bmad-output/planning-artifacts/campaign-roadmap.md (chunk 15)"
design: "_bmad-output/planning-artifacts/design-campaign-creator-flow.md"
---

# Intent: chunk 15 — счётчики и timestamps попыток

## Контекст

Roadmap-чанк 15 (campaign-roadmap.md): «Фронт: расширение страницы кампании со
статусами и счётчиками. Колонка статуса (4 значения), счётчики попыток
приглашения и ремайндеров, последние timestamps. Потребляет A3 + расширяет
UI из 11/13.»

Что уже сделано (chunks 11 / 13):

- `CampaignCreatorsSection` группирует креаторов по 4 секциям (`planned` /
  `invited` / `declined` / `agreed`) с заголовком, счётчиком кол-ва и
  кнопкой групповой рассылки. Статус уже визуализирован структурно — через
  группировку секций.
- `CampaignCreatorsTable` показывает: ФИО, соцсети, категории, возраст,
  город, дата создания. Счётчиков попыток / timestamps нет.
- A3 (`GET /campaigns/{id}/creators`) уже отдаёт `invitedCount`,
  `invitedAt`, `remindedCount`, `remindedAt`, `decidedAt` (см.
  `backend/api/openapi.yaml`, schema `CampaignCreator`).

## Тезис

Расширяем `CampaignCreatorsTable` контекстными колонками попыток
коммуникации: набор колонок зависит от секции, в которой рендерится
таблица. Колонка статуса как отдельная — **не добавляется** (статус уже
виден из заголовка секции, дубль информации).

## Решения

- [approved 2026-05-08] Добавляем counters/timestamps в таблицу, без
  отдельной колонки статуса.
- [approved 2026-05-08] Контекстные колонки по секции — таблица принимает
  пропс `variant` (или похожий), набор колонок зависит от секции:

  | Секция | Доп. колонки |
  |---|---|
  | `planned` | — |
  | `invited` | invitedCount + invitedAt, remindedCount + remindedAt |
  | `declined` | invitedCount, decidedAt |
  | `agreed` | invitedCount, decidedAt |

  Альтернативы (отвергнуты): один сквозной набор колонок (много пустых
  ячеек в `planned`); compact-inline (плохо scannable).
- [approved 2026-05-08] Раздельные колонки: count и timestamp — каждое в
  своей колонке. Альтернатива (composite count+at в одной ячейке)
  отвергнута.
- [approved 2026-05-08] Формат timestamp — день+месяц+время (`HH:mm`).
  Конкретный паттерн локали определим в реализации, ориентир — добавить
  время к существующему `formatShortDate` (день+месяц коротко).

  Итоговая раскладка колонок (поверх существующих ФИО / соцсети /
  категории / возраст / город / createdAt):

  | Секция | Доп. колонки |
  |---|---|
  | `planned` | — |
  | `invited` | invitedCount, invitedAt, remindedCount, remindedAt |
  | `declined` | invitedCount, decidedAt |
  | `agreed` | invitedCount, decidedAt |

- [approved 2026-05-08] Заголовки, плейсхолдеры, расположение:

  | Поле | Заголовок | Пусто/null |
  |---|---|---|
  | `invitedCount` | «Приглашений» | `0` (число, не em-dash) |
  | `invitedAt` | «Когда приглашён» | n/a (всегда есть в видимых секциях) |
  | `remindedCount` | «Ремайндеров» | `0` (число) |
  | `remindedAt` | «Когда ремайндер» | `—` (через `deletedPlaceholder`) |
  | `decidedAt` | «Решение от» | n/a (всегда есть в declined/agreed) |

  Порядок колонок: ФИО → соцсети → категории → возраст → город →
  **новые** → `createdAt` → actions.

  Формат timestamp: `6 мая, 14:30` (`day numeric, month short, hour 2-digit,
  minute 2-digit`, локаль `ru`, без года). В проекте уже есть похожий
  `formatDateTime` в `CampaignDetailPage.tsx:264` — целесообразно вынести
  в shared util без года.

- [approved 2026-05-08] Имплементация и тесты:

  **Variant таблицы.** `CampaignCreatorsTable` принимает новый пропс
  `status: CampaignCreatorStatus`. `buildColumns` решает по `status`,
  какие counter/timestamp колонки добавить.
  `CampaignCreatorGroupSection` пробрасывает свой `status` в таблицу.

  **data-testid** (для e2e и unit), на ячейку, привязка по `creatorId`:

  - `campaign-creator-invited-count-{creatorId}`
  - `campaign-creator-invited-at-{creatorId}`
  - `campaign-creator-reminded-count-{creatorId}`
  - `campaign-creator-reminded-at-{creatorId}`
  - `campaign-creator-decided-at-{creatorId}`

  **i18n keys** (добавляем в `campaignCreators.columns.*`):
  `invitedCount`, `invitedAt`, `remindedCount`, `remindedAt`, `decidedAt`.

  **Утилита формата.** Выносим `formatDateTime` (без года) в shared
  utils (`shared/utils/formatDateTime.ts`), используем в новых ячейках.
  Существующий `formatDateTime` в `CampaignDetailPage.tsx:264` мигрирует
  на shared-версию (с годом / без — уточнить в реализации, либо
  параметризовать).

  **Тесты:**

  - **Unit `CampaignCreatorsTable.test.tsx`**: для каждого `status` —
    рендер фикстуры, assert наличия/отсутствия колонок, значений ячеек
    (включая `0` для нулевого `remindedCount` в `invited` и `—` для
    `remindedAt = null`); фикстура для `declined`/`agreed` использует
    реальные ISO timestamps в `decidedAt`.
  - **Unit `CampaignCreatorsSection.test.tsx`**: спот-проверка, что
    каждой группе таблица получает правильный `status`. Existing tests
    не должны сломаться.
  - **Playwright e2e — расширяем `campaign-notify.spec.ts`**: после
    happy-path notify добавляем asserts на `invited-count = 1` /
    `invited-at` (любая непустая дата); после remind — на
    `reminded-count = 1` / `reminded-at`.
  - **Decided-сценарии в e2e** не покрываем сейчас — TMA agree/decline
    (chunk 14) ещё `[ ]`; покроем в spec'е chunk 14.

## Слой

Фронт (web). Бэк / TMA / lендос — out of scope.

## Out of scope

- Изменения бэк-контракта (всё нужное уже в A3).
- Чанк 14 (TMA agree/decline), чанк 13a (уведомление при удалении).
