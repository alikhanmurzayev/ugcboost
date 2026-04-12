# Разведка по стандартам: web

**Дата:** 2026-04-12
**Область:** web (frontend)
**Стандарты:** frontend-components, frontend-api, frontend-state, frontend-types, frontend-quality, frontend-testing-unit, frontend-testing-e2e, naming, security

---

## Нарушения стандартов

### 1. Критичные

#### 1.1 Структура проекта: фронтенды лежат в корне, а не в frontend/
- **Файлы:** `web/`, `tma/`, `landing/`, `e2e/`
- **Стандарт:** `docs/standards/frontend-state.md`, `docs/standards/frontend-testing-e2e.md`
- **Что не так:** `web/`, `tma/`, `landing/` лежат в корне проекта рядом с `backend/`. По стандарту все фронтенд-проекты должны быть внутри `frontend/` — это workspace root с pnpm workspaces, shared-пакетом и единой E2E-директорией. Текущая структура блокирует создание `@ugcboost/shared` и единого E2E-каталога
- **Как исправить:** Создать `frontend/` в корне. Перенести `web/`, `tma/`, `landing/` внутрь. Настроить pnpm workspace root (`frontend/package.json`). Перенести E2E в `frontend/e2e/`. Обновить все импорты, Docker-конфиги, CI workflows, vite конфиги

#### 1.2 TypeScript strict mode не включён
- **Файл:** `web/tsconfig.app.json`
- **Стандарт:** `docs/standards/frontend-quality.md` — "strict: true обязателен"
- **Что не так:** Отсутствуют `strict: true` и `noUncheckedIndexedAccess: true`. Без strict mode типы не гарантируют корректность (null/undefined пропускаются, неявные any допускаются)
- **Как исправить:** Добавить `"strict": true` и `"noUncheckedIndexedAccess": true` в compilerOptions. Исправить ошибки компиляции, которые появятся

#### 1.3 ErrorBoundary отсутствует
- **Файл:** `web/src/App.tsx`
- **Стандарт:** `docs/standards/frontend-quality.md` — "Корневой ErrorBoundary оборачивает всё приложение"
- **Что не так:** Нет ErrorBoundary. При ошибке рендера — белый экран
- **Как исправить:** Создать `shared/components/ErrorBoundary.tsx`, обернуть App. Fallback UI с кнопкой перезагрузки

#### 1.4 i18n не настроен — все строки захардкожены
- **Файлы:** Все компоненты (LoginPage, BrandsPage, BrandDetailPage, DashboardPage, AuditLogPage, DashboardLayout)
- **Стандарт:** `docs/standards/frontend-components.md` — "Хардкод пользовательского текста в JSX запрещён. Все строки — через react-i18next"
- **Что не так:** `react-i18next` и `i18next` не установлены (нет в package.json). Все UI-строки на русском захардкожены прямо в JSX. Файл `shared/i18n/ru.json` существует, но не используется
- **Как исправить:** Установить `react-i18next`, `i18next`. Настроить провайдер. Вынести все строки в JSON-файлы по неймспейсам фич. Заменить хардкод на `t("key")`

#### 1.5 API-клиент ручной, не openapi-fetch
- **Файл:** `web/src/api/client.ts`
- **Стандарт:** `docs/standards/frontend-api.md` — "Все HTTP-запросы — через сгенерированный клиент (openapi-fetch)"
- **Что не так:** Используется самописная `api()` функция с raw `fetch()`. Пути передаются строками без типизации. `openapi-fetch` не установлен. Опечатка в пути = ошибка в рантайме, не при компиляции
- **Как исправить:** Установить `openapi-fetch`, создать типизированный клиент из `schema.ts`. Переписать API-функции через `client.GET()`, `client.POST()` и т.д.

#### 1.6 Нет RoleGuard — admin-only роуты не защищены по роли
- **Файл:** `web/src/App.tsx:28-34`, `web/src/features/auth/AuthGuard.tsx`
- **Стандарт:** `docs/standards/frontend-state.md` — "Роуты, доступные не всем ролям, защищаются RoleGuard"
- **Что не так:** AuthGuard проверяет только наличие пользователя. Роут `/audit` (admin-only) доступен brand_manager через прямой URL. Навигация в DashboardLayout фильтрует ссылки по роли, но роутер не защищён
- **Как исправить:** Создать `RoleGuard` компонент. Обернуть admin-only роуты (audit). Brand_manager при прямом переходе должен видеть "Нет доступа", а не контент

---

### 2. Важные

#### 2.1 Query keys — инлайн строки, нет фабрики
- **Файлы:** `web/src/features/brands/BrandsPage.tsx:18`, `web/src/features/brands/BrandDetailPage.tsx:27`, `web/src/features/audit/AuditLogPage.tsx:22`
- **Стандарт:** `docs/standards/frontend-api.md` — "Все query keys — в одном файле с фабричными функциями"
- **Что не так:** Query keys захардкожены: `["brands"]`, `["brand", brandId]`, `["audit-logs", ...]`. Дублируются в queryFn и invalidateQueries. Риск рассинхрона при изменении ключей
- **Как исправить:** Создать `shared/constants/queryKeys.ts` с фабрикой: `brandKeys.all()`, `brandKeys.detail(id)`, `auditKeys.list(params)`

#### 2.2 data-testid отсутствуют
- **Файлы:** Все компоненты
- **Стандарт:** `docs/standards/frontend-components.md` — "Каждый интерактивный элемент получает data-testid"
- **Что не так:** Ни один элемент не имеет `data-testid`. E2E тесты используют `getByRole`/`getByText`, но стандарт E2E требует `data-testid` как стабильный контракт
- **Как исправить:** Добавить `data-testid` на все кнопки, инпуты, ссылки навигации, таблицы, ключевые контейнеры

#### 2.3 Unit-тесты полностью отсутствуют
- **Файл:** `web/package.json`
- **Стандарт:** `docs/standards/frontend-testing-unit.md` — "Vitest + React Testing Library. Целевой порог — 80%"
- **Что не так:** Vitest не установлен. Нет ни одного `.test.ts`/`.test.tsx` файла. Нет `@testing-library/react` в зависимостях
- **Как исправить:** Установить `vitest`, `@testing-library/react`, `@testing-library/user-event`. Написать тесты для utils, hooks, компонентов

#### 2.4 Компоненты превышают 150 строк — нужна декомпозиция
- **Файлы:** `web/src/features/brands/BrandDetailPage.tsx` (259 строк), `web/src/features/brands/BrandsPage.tsx` (173 строки)
- **Стандарт:** `docs/standards/frontend-components.md` — "Компонент > 150 строк — сигнал к декомпозиции"
- **Что не так:** BrandDetailPage содержит edit form, manager list, assign form, temp password display — всё в одном компоненте. BrandsPage содержит create form, list, delete confirmation
- **Как исправить:** Вынести в подкомпоненты: `BrandEditForm`, `ManagerList`, `AssignManagerForm`, `CreateBrandForm`, `BrandDeleteButton`

#### 2.5 Loading states — голый текст, нет skeleton/spinner
- **Файлы:** `web/src/features/brands/BrandsPage.tsx:104`, `web/src/features/brands/BrandDetailPage.tsx:76`, `web/src/features/audit/AuditLogPage.tsx:74`, `web/src/features/auth/AuthGuard.tsx:35`
- **Стандарт:** `docs/standards/frontend-components.md` — "Loading — skeleton или spinner, не голый текст"
- **Что не так:** Все loading states — `<p>Загрузка...</p>`. Нет визуального индикатора (skeleton, spinner)
- **Как исправить:** Создать `shared/components/Skeleton.tsx` и `shared/components/Spinner.tsx`. Использовать вместо текста

#### 2.6 Error state отсутствует для query-запросов
- **Файлы:** `web/src/features/brands/BrandsPage.tsx:17`, `web/src/features/audit/AuditLogPage.tsx:21`
- **Стандарт:** `docs/standards/frontend-components.md` — "Error — сообщение с возможностью retry"
- **Что не так:** `useQuery` в BrandsPage и AuditLogPage не обрабатывают `error` состояние. При ошибке запроса пользователь видит пустую страницу
- **Как исправить:** Добавить обработку `isError` с сообщением и кнопкой "Повторить" (`refetch`)

#### 2.7 `as` type assertion в API-клиенте
- **Файл:** `web/src/api/client.ts:55`
- **Стандарт:** `docs/standards/frontend-quality.md` — "`as` (type assertion) — запрещён"
- **Что не так:** `...(options.headers as Record<string, string>)` — type assertion для обхода типа Headers
- **Как исправить:** Принимать headers как `Record<string, string>` в сигнатуре или использовать type guard

#### 2.8 Таблица брендов — onClick на `<tr>` вместо `<Link>` в ячейке
- **Файл:** `web/src/features/brands/BrandsPage.tsx:119-122`
- **Стандарт:** `docs/standards/frontend-quality.md` — "Кликабельные строки таблиц — через `<Link>` внутри ячейки, не onClick на `<tr>`"
- **Что не так:** `<tr onClick={() => navigate(...)}>` — не доступно с клавиатуры, нет визуального индикатора ссылки, stopPropagation на кнопках внутри
- **Как исправить:** Заменить на `<Link>` внутри `<td>`, убрать onClick со строки

#### 2.9 Валидация форм — молчаливый return без feedback
- **Файл:** `web/src/features/brands/BrandsPage.tsx:49`, `web/src/features/brands/BrandDetailPage.tsx:88-89`
- **Стандарт:** `docs/standards/frontend-quality.md` — "Молчаливый return без feedback пользователю запрещён"
- **Что не так:** `if (!newName.trim()) return;` — пустое имя бренда игнорируется без сообщения
- **Как исправить:** Показать inline-ошибку "Название обязательно"

#### 2.10 Runtime config без валидации
- **Файл:** `web/src/api/client.ts:3-8`
- **Стандарт:** `docs/standards/frontend-quality.md` — "Runtime config валидируется при инициализации. В production — fatal error"
- **Что не так:** `getApiBase()` молча возвращает `/api` fallback. В production без конфига приложение будет отправлять запросы не туда
- **Как исправить:** Валидировать при инициализации. В production если `__RUNTIME_CONFIG__` отсутствует — показать ошибку, не запускать

#### 2.11 AuditLogsParams — ручной интерфейс вместо производного от кодогенерации
- **Файл:** `web/src/api/audit.ts:7-15`
- **Стандарт:** `docs/standards/frontend-types.md` — "Ручные interface/type для API request/response запрещены"
- **Что не так:** `AuditLogsParams` полностью дублирует query-параметры из OpenAPI спецификации
- **Как исправить:** Извлечь тип из `operations["listAuditLogs"]["parameters"]["query"]`

#### 2.12 E2E: нет комментария-описания flow, нет cleanup
- **Файл:** `web/e2e/auth.spec.ts`
- **Стандарт:** `docs/standards/frontend-testing-e2e.md` — "Комментарий в начале файла обязателен", "Defer-based cleanup stack"
- **Что не так:** Нет комментария с описанием flow. Seed-данные (тестовый пользователь) не удаляются после тестов
- **Как исправить:** Добавить комментарий-описание. Добавить `afterAll` с cleanup через API

#### 2.13 Route для brand detail — конкатенация строк вместо фабричной функции
- **Файл:** `web/src/App.tsx:32`
- **Стандарт:** `docs/standards/frontend-components.md` — "Для динамических путей — фабричные функции"
- **Что не так:** `ROUTES.BRANDS + "/:brandId"` — ручная конкатенация. `ROUTES.BRAND_DETAIL` — фабрика для навигации, но нет отдельного паттерна для роутера
- **Как исправить:** Добавить `BRAND_DETAIL_PATTERN: "brands/:brandId"` в ROUTES

---

### 3. Мелкие

#### 3.1 ESLint конфиг неполный
- **Файл:** `web/eslint.config.js`
- **Стандарт:** `docs/standards/frontend-quality.md` — обязательные правила ESLint
- **Что не так:** Нет явных правил: `no-console`, `@typescript-eslint/no-non-null-assertion`, `@typescript-eslint/no-unused-vars` с исключением `_`
- **Как исправить:** Добавить правила в конфиг. `tseslint.configs.recommended` покрывает часть, но не все

#### 3.2 Accessibility: select без label
- **Файл:** `web/src/features/audit/AuditLogPage.tsx:43-68`
- **Стандарт:** `docs/standards/frontend-quality.md` — "Все input, select, textarea имеют label или aria-label"
- **Что не так:** Два `<select>` (фильтры entity type и action) не имеют `<label>` или `aria-label`
- **Как исправить:** Добавить `aria-label="Тип сущности"` и `aria-label="Действие"` соответственно

#### 3.3 E2E тесты внутри web/e2e/, не в стандартной структуре
- **Файл:** `web/e2e/auth.spec.ts`
- **Стандарт:** `docs/standards/frontend-testing-e2e.md` — структура `e2e/web/`, `e2e/tma/`
- **Что не так:** E2E тесты для web живут в `web/e2e/`, а не в `e2e/web/` как в стандарте. При этом `e2e/web/` на верхнем уровне тоже существует
- **Как исправить:** Решить какая структура финальная. Перенести тесты в `e2e/web/` если выбрана стандартная, или обновить стандарт

---

## Статистика

- **Всего файлов проверено:** 26 (все tracked файлы web/ включая конфиги, типы, компоненты, API, тесты)
- **Нарушений найдено:** 22 (6 критичных / 13 важных / 3 мелких)

---

## Рекомендация

Порядок исправлений:

1. **Перенос фронтендов в frontend/** — структурный фундамент. Всё остальное (workspace, shared, e2e) строится на этом. Делать первым, пока мало зависимостей. Включает: создание `frontend/`, перенос `web/`, `tma/`, `landing/`, настройку pnpm workspaces, обновление Docker/CI/импортов.

2. **strict mode + ErrorBoundary** — минимальный effort, максимальная защита. strict mode выявит скрытые баги. ErrorBoundary предотвратит белый экран.

3. **openapi-fetch + query key factory** — фундамент API-слоя. Без этого все остальные API-улучшения строятся на неправильной базе. Переписать client.ts, перевести API-функции.

4. **RoleGuard** — минимальная правка (один компонент), закрывает дыру в авторизации. Быстро.

5. **i18n setup** — большой рефакторинг (все компоненты), но стандарт помечен REQUIRED. Можно инкрементально: установить i18next, настроить провайдер, переводить по фиче за итерацию.

6. **Декомпозиция + loading/error states + data-testid** — можно совмещать с i18n рефакторингом по фичам.

7. **Unit тесты** — писать после стабилизации API-слоя и компонентов. Начать с utils и hooks, потом компоненты.

8. **Мелочи** (ESLint, a11y, E2E cleanup) — фиксить попутно.
