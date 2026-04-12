# План: приведение web к стандартам

## Обзор

Привести web-приложение к стандартам `docs/standards/frontend-*`, `naming`, `security`. 22 нарушения (6 критичных / 13 важных / 3 мелких), сгруппированных в 10 последовательных шагов. Каждый шаг — один коммит, билд не ломается между шагами.

## Шаги

### Шаг 1: Реструктуризация — frontend/ + pnpm workspace
- **Файлы**: `web/`, `tma/`, `landing/`, `tailwind.preset.ts`, `web/e2e/`, `Dockerfile`-ы, `docker-compose*.yml`, `.github/workflows/ci.yml`, `Makefile`
- **Что делаем**:
  1. Создать `frontend/` в корне
  2. `git mv web frontend/web`, `git mv tma frontend/tma`, `git mv landing frontend/landing`
  3. `git mv tailwind.preset.ts frontend/tailwind.preset.ts`
  4. Создать `frontend/e2e/web/` — перенести `frontend/web/e2e/auth.spec.ts` → `frontend/e2e/web/auth.spec.ts`. Удалить `frontend/web/e2e/` и корневой `e2e/web/` (пустой дубль)
  5. Создать `frontend/package.json` — pnpm workspace root (`"workspaces": ["web", "tma", "landing", "packages/*"]`)
  6. Создать `frontend/pnpm-workspace.yaml`
  7. Обновить `web/Dockerfile`: COPY paths (`COPY frontend/tailwind.preset.ts`, `COPY frontend/web/`)
  8. Обновить `tma/Dockerfile` и `landing/Dockerfile` аналогично
  9. Обновить `docker-compose.yml`: `dockerfile: frontend/web/Dockerfile`, `dockerfile: frontend/tma/Dockerfile`
  10. Обновить `docker-compose.test.yml`: аналогичные пути
  11. Обновить `docker-compose.ci.yml`: image overrides (без build — OK, но проверить)
  12. Обновить `.github/workflows/ci.yml`: `working-directory: frontend/web` (lint-web), `working-directory: frontend/tma` (lint-tma), Playwright paths
  13. Обновить `Makefile`: все `cd web` → `cd frontend/web`, `cd tma` → `cd frontend/tma`, `cd landing` → `cd frontend/landing`
  14. Обновить `frontend/web/playwright.config.ts`: `cd ../backend` → `cd ../../backend`
  15. Обновить `frontend/web/tailwind.config.ts`: `../tailwind.preset` → остаётся `../tailwind.preset` (preset перенесён в frontend/)
- **Стандарт**: `docs/standards/frontend-state.md`, `docs/standards/frontend-testing-e2e.md`
- **Проверка**: `make build` и `make lint` проходят. CI jobs обращаются к правильным путям. `docker compose up` стартует все сервисы. Структура: `frontend/web/`, `frontend/tma/`, `frontend/landing/`, `frontend/e2e/web/`

---

### Шаг 2: TypeScript strict mode
- **Файлы**: `frontend/web/tsconfig.app.json`, `frontend/web/src/api/client.ts` и файлы с ошибками компиляции
- **Что делаем**:
  1. Добавить в `tsconfig.app.json` compilerOptions: `"strict": true`, `"noUncheckedIndexedAccess": true`
  2. Запустить `npx tsc --noEmit` — собрать все ошибки
  3. Исправить `client.ts:55`: убрать `as Record<string, string>` — принимать headers как `Record<string, string>` в сигнатуре `api()` или использовать type-safe spread
  4. Исправить все остальные ошибки компиляции (null checks, indexed access, etc.)
- **Стандарт**: `docs/standards/frontend-quality.md`
- **Проверка**: `npx tsc --noEmit` проходит без ошибок. Нет `as` assertions (кроме main.tsx root element)

---

### Шаг 3: ESLint конфиг — обязательные правила
- **Файлы**: `frontend/web/eslint.config.js`
- **Что делаем**:
  1. Добавить правило `"no-console": ["error", { allow: ["warn", "error"] }]`
  2. Добавить `"@typescript-eslint/no-non-null-assertion": "error"`
  3. Добавить `"@typescript-eslint/no-unused-vars": ["error", { argsIgnorePattern: "^_" }]`
  4. Запустить `npx eslint src/` — исправить нарушения (если есть)
- **Стандарт**: `docs/standards/frontend-quality.md`
- **Проверка**: `npx eslint src/` проходит чисто

---

### Шаг 4: ErrorBoundary + Spinner + ErrorState (shared UI)
- **Файлы**: новые `frontend/web/src/shared/components/ErrorBoundary.tsx`, `Spinner.tsx`, `ErrorState.tsx`; обновить `App.tsx`, `BrandsPage.tsx`, `BrandDetailPage.tsx`, `AuditLogPage.tsx`, `AuthGuard.tsx`
- **Что делаем**:
  1. Создать `ErrorBoundary.tsx` — class component, fallback UI с кнопкой перезагрузки
  2. Обернуть `<App>` (или содержимое внутри providers) в `<ErrorBoundary>`
  3. Создать `Spinner.tsx` — CSS анимация, переиспользуемый
  4. Создать `ErrorState.tsx` — сообщение об ошибке + кнопка "Повторить" (принимает `onRetry` prop)
  5. Заменить `<p>Загрузка...</p>` → `<Spinner />` во всех компонентах
  6. Добавить обработку `isError`/`error` + `<ErrorState onRetry={refetch} />` в `BrandsPage` и `AuditLogPage`
- **Стандарт**: `docs/standards/frontend-quality.md`, `docs/standards/frontend-components.md`
- **Проверка**: Билд проходит. ErrorBoundary ловит ошибки (можно проверить ручным throw в dev). Loading/Error/Empty states присутствуют во всех query-компонентах

---

### Шаг 5: openapi-fetch + query keys + runtime config + derived types
- **Файлы**: `frontend/web/package.json`, `frontend/web/src/api/client.ts`, `auth.ts`, `brands.ts`, `audit.ts`; новый `frontend/web/src/shared/constants/queryKeys.ts`
- **Что делаем**:
  1. `npm install openapi-fetch` в `frontend/web/`
  2. Переписать `client.ts`:
     - Создать типизированный клиент: `createClient<paths>({ baseUrl })`
     - Middleware для auth header (Bearer token из Zustand)
     - Middleware для 401 → refresh → retry
     - Runtime config валидация: в production без `__RUNTIME_CONFIG__` → fatal error (показать UI, не запускать)
  3. Переписать `auth.ts`: `client.POST("/auth/login", ...)`, `client.POST("/auth/logout", ...)`
  4. Переписать `brands.ts`: `client.GET("/brands", ...)`, `client.POST("/brands", ...)`, etc.
  5. Переписать `audit.ts`: `client.GET("/audit-logs", ...)`. Удалить ручной `AuditLogsParams` — использовать `operations["listAuditLogs"]["parameters"]["query"]`
  6. Создать `queryKeys.ts`:
     ```ts
     export const brandKeys = {
       all: () => ["brands"] as const,
       detail: (id: string) => ["brands", id] as const,
     };
     export const auditKeys = {
       list: (filters: Record<string, unknown>) => ["audit-logs", filters] as const,
     };
     ```
  7. Обновить все `useQuery`/`useMutation` в компонентах — использовать query key factory
- **Стандарт**: `docs/standards/frontend-api.md`, `docs/standards/frontend-quality.md`, `docs/standards/frontend-types.md`
- **Проверка**: Билд проходит. Нет raw `fetch()` в API-слое (кроме refresh внутри middleware). Query keys не дублируются. `AuditLogsParams` удалён. Runtime config валидируется

---

### Шаг 6: RoleGuard + route constants
- **Файлы**: новый `frontend/web/src/features/auth/RoleGuard.tsx`, обновить `App.tsx`, `routes.ts`
- **Что делаем**:
  1. Создать `RoleGuard.tsx`:
     - Props: `allowedRoles: UserRole[]`
     - Проверяет `user.role` из Zustand
     - Если роль не в списке → показать "Нет доступа" (или редирект)
     - Использует `Roles` из `shared/constants/roles.ts`
  2. Обернуть admin-only роуты в `App.tsx`: `<Route element={<RoleGuard allowedRoles={[Roles.ADMIN]} />}>`
  3. Добавить в `routes.ts` pattern для роутера: `BRAND_DETAIL_PATTERN: "brands/:brandId"`
  4. Использовать `ROUTES.BRAND_DETAIL_PATTERN` в `App.tsx` вместо `ROUTES.BRANDS + "/:brandId"`
- **Стандарт**: `docs/standards/frontend-state.md`, `docs/standards/frontend-components.md`
- **Проверка**: Билд проходит. Brand manager при прямом переходе на `/audit` видит "Нет доступа". Все route paths — из констант, нет конкатенаций

---

### Шаг 7: Декомпозиция + a11y + формы + таблица
- **Файлы**: `frontend/web/src/features/brands/BrandDetailPage.tsx`, `BrandsPage.tsx`, `AuditLogPage.tsx`; новые подкомпоненты в `features/brands/components/`
- **Что делаем**:
  1. Вынести из `BrandDetailPage.tsx` (259 строк):
     - `components/BrandEditForm.tsx` — форма редактирования названия + мутация
     - `components/ManagerList.tsx` — таблица менеджеров + удаление
     - `components/AssignManagerForm.tsx` — форма назначения + temp password display
     - `BrandDetailPage` остаётся оркестратором: query + layout подкомпонентов
  2. Вынести из `BrandsPage.tsx` (173 строки):
     - `components/CreateBrandForm.tsx` — форма создания + мутация
     - `components/BrandList.tsx` — таблица + удаление
  3. Заменить `<tr onClick>` в BrandList на `<Link>` внутри `<td>`:
     ```tsx
     <td><Link to={ROUTES.BRAND_DETAIL(b.id)}>{b.name}</Link></td>
     ```
  4. Добавить валидацию с feedback: `if (!name.trim()) { setError("Название обязательно"); return; }`
  5. Добавить `aria-label` на `<select>` в `AuditLogPage.tsx`
  6. Добавить `data-testid` на все интерактивные элементы и ключевые контейнеры:
     - Login: `data-testid="login-form"`, `"email-input"`, `"password-input"`, `"login-button"`, `"login-error"`
     - Brands: `"create-brand-button"`, `"brand-name-input"`, `"brands-table"`, `"brand-row-{id}"`
     - BrandDetail: `"edit-brand-button"`, `"brand-name-input"`, `"assign-manager-input"`, `"manager-row-{id}"`
     - Audit: `"entity-type-filter"`, `"action-filter"`, `"audit-table"`
     - Layout: `"sidebar"`, `"logout-button"`, `"nav-link-{route}"`
- **Стандарт**: `docs/standards/frontend-components.md`, `docs/standards/frontend-quality.md`
- **Проверка**: Билд проходит. Все компоненты < 150 строк. `data-testid` на каждом интерактивном элементе. Нет `onClick` на `<tr>`. Формы показывают ошибки при пустом вводе. Все `<select>` имеют `aria-label`

---

### Шаг 8: i18n
- **Файлы**: `frontend/web/package.json`; новый `frontend/web/src/shared/i18n/config.ts`; обновить `App.tsx`, все компоненты; JSON-файлы переводов
- **Что делаем**:
  1. `npm install react-i18next i18next` в `frontend/web/`
  2. Создать `shared/i18n/config.ts` — инициализация i18next, русский по умолчанию
  3. Создать JSON-файлы переводов по неймспейсам фич:
     - `locales/ru/common.json` — общие строки (ошибки, кнопки Отмена/Сохранить/Удалить)
     - `locales/ru/auth.json` — логин, пароль, ошибки авторизации
     - `locales/ru/brands.json` — бренды, менеджеры
     - `locales/ru/audit.json` — журнал действий, фильтры, лейблы действий
     - `locales/ru/dashboard.json` — дашборд
  4. Обновить `App.tsx` — добавить `I18nextProvider`
  5. Заменить все захардкоженные строки на `t("namespace.key")`:
     - `"Загрузка..."` → `t("common.loading")`
     - `"Войти"` → `t("auth.login.submit")`
     - `"Бренды"` → `t("brands.title")`
     - и т.д. для всех компонентов
  6. Удалить или реорганизовать `shared/i18n/ru.json` (старый неиспользуемый файл)
  7. Обновить `shared/i18n/errors.ts` — использовать i18next вместо ручного маппинга (или оставить как утилиту, если удобнее)
- **Стандарт**: `docs/standards/frontend-components.md`
- **Проверка**: Билд проходит. `grep -rn '"[А-Яа-яЁё]' src/` — нет хардкоженных русских строк в JSX (только в JSON-файлах переводов и в errors.ts маппинге). Все строки через `t()`

---

### Шаг 9: E2E тест — комментарий + cleanup
- **Файлы**: `frontend/e2e/web/auth.spec.ts`
- **Что делаем**:
  1. Добавить комментарий-описание flow в начало файла:
     ```ts
     /**
      * Auth flow E2E tests — web application.
      *
      * Шаги:
      * 1. Seed admin user via /test/seed-user
      * 2. Happy login → dashboard with sidebar
      * 3. Wrong password → error, stay on login
      * 4. Session restore → F5 keeps user logged in
      * 5. Logout → redirect to login, session destroyed
      * 6. Protected route → unauthenticated user redirected
      *
      * Данные: один тестовый admin (TEST_EMAIL).
      * Cleanup: удаление через /test/cleanup после всех тестов.
      */
     ```
  2. Добавить `afterAll` cleanup:
     ```ts
     test.afterAll(async ({ request }) => {
       if (process.env.E2E_CLEANUP !== "false") {
         await request.delete(`${API_URL}/test/users/${TEST_EMAIL}`);
       }
     });
     ```
  3. Обновить E2E тесты для использования `data-testid` локаторов (из шага 7):
     ```ts
     await page.getByTestId('email-input').fill(TEST_EMAIL);
     await page.getByTestId('login-button').click();
     ```
- **Стандарт**: `docs/standards/frontend-testing-e2e.md`
- **Проверка**: E2E тесты проходят. Cleanup удаляет seed-данные (при `E2E_CLEANUP=true`)

---

### Шаг 10: Unit-тест инфраструктура + первые тесты
- **Файлы**: `frontend/web/package.json`, `frontend/web/vite.config.ts`; новые `.test.ts` файлы
- **Что делаем**:
  1. `npm install -D vitest @testing-library/react @testing-library/user-event @testing-library/jest-dom jsdom`
  2. Добавить в `vite.config.ts` конфиг vitest:
     ```ts
     test: {
       globals: true,
       environment: "jsdom",
       setupFiles: ["./src/test-setup.ts"],
     }
     ```
  3. Создать `src/test-setup.ts` — import `@testing-library/jest-dom`
  4. Добавить script в package.json: `"test": "vitest"`
  5. Написать тесты по слоям:
     - **Utils**: `shared/i18n/errors.test.ts` — `getErrorMessage()` с разными кодами
     - **Store**: `stores/auth.test.ts` — `setAuth`, `clearAuth`, initial state
     - **Components**: `features/auth/LoginPage.test.tsx` — рендер, submit, error state, loading state
     - **Guard**: `features/auth/AuthGuard.test.tsx` — redirect when no user, render outlet when user exists
     - **Guard**: `features/auth/RoleGuard.test.tsx` — allowed/denied roles
  6. Добавить vitest в Makefile `test-unit` и CI
- **Стандарт**: `docs/standards/frontend-testing-unit.md`
- **Проверка**: `npm test -- --run` проходит. Coverage > 0%. Тесты изолированы, моки очищаются

---

## Порядок выполнения

```
Шаг 1 → Шаг 2 → Шаг 3 → Шаг 4 → Шаг 5 → Шаг 6 → Шаг 7 → Шаг 8 → Шаг 9 → Шаг 10
```

Строго последовательный — каждый шаг зависит от предыдущих:
- **Шаг 1** первым — все пути меняются, дальше работаем с `frontend/web/`
- **Шаг 2** до шага 5 — strict mode выявляет проблемы, которые openapi-fetch решает
- **Шаг 4** до шага 7 — shared UI компоненты используются при декомпозиции
- **Шаг 5** до шага 7 — API-слой стабилен перед рефакторингом компонентов
- **Шаг 7** до шага 8 — подкомпоненты с data-testid до i18n (i18n проще на маленьких файлах)
- **Шаг 8** до шага 9 — i18n до E2E (тесты ищут переведённый текст)
- **Шаг 9** до шага 10 — E2E стабильны до unit-тестов
- **Шаг 10** последним — код стабилен, тестируем финальное состояние

## Проверка после всех шагов

- [ ] `cd frontend/web && npx tsc --noEmit` — нет ошибок
- [ ] `cd frontend/web && npx eslint src/` — нет ошибок
- [ ] `cd frontend/web && npm test -- --run` — unit-тесты проходят
- [ ] `make build` — Docker images собираются
- [ ] `make test-all` — все тесты (unit + backend E2E + browser E2E) проходят
- [ ] CI pipeline зелёный
- [ ] Нет хардкоженных русских строк в JSX: `grep -rn '"[А-Яа-яЁё]' frontend/web/src/ --include='*.tsx'` — только в import paths и comments
- [ ] Все компоненты < 150 строк
- [ ] `data-testid` на всех интерактивных элементах
- [ ] strict mode включён
- [ ] ErrorBoundary оборачивает приложение
- [ ] RoleGuard защищает admin-only роуты
