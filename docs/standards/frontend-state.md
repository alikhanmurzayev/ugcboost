# Фронтенд: состояние и авторизация

## State management

Три типа состояния, три инструмента:

- **Серверное состояние** (данные API: бренды, кампании, пользователи) — React Query (TanStack Query). Кэширование, инвалидация, refetch — всё через него
- **Глобальное клиентское состояние** (auth, текущий пользователь) — Zustand. Минимальный store, без бизнес-логики
- **Локальное состояние** (формы, UI-тоглы, модалки) — `useState`. Не выносить в глобальный store то, что нужно только одному компоненту

Context — только для провайдеров (i18n, theme). Не использовать Context для часто обновляемого состояния — вызывает ре-рендер всего дерева.

## Auth flow

### Хранение токенов
- **Access token** — только в памяти (Zustand store). Не в localStorage, не в sessionStorage — XSS уязвимость
- **Refresh token** — httpOnly cookie, устанавливается бэкендом. Недоступен из JS

### Login
1. `POST /auth/login` с email/password
2. Ответ: access token в body, refresh token в httpOnly cookie (автоматически)
3. Access token и данные пользователя сохраняются в Zustand store

### Authenticated requests
- API-клиент добавляет access token в `Authorization: Bearer` заголовок
- При 401 — автоматический refresh (см. ниже)

### Token refresh
1. Access token истёк → API-клиент перехватывает 401
2. `POST /auth/refresh` (refresh token отправляется автоматически через cookie)
3. Получаем новый access token → обновляем store → повторяем оригинальный запрос
4. Если refresh тоже 401 — сессия истекла, очистить store, редирект на login

### Session restore (перезагрузка страницы)
- Access token в памяти — теряется при reload
- При инициализации приложения: `POST /auth/refresh` для получения нового access token
- Успех → пользователь остаётся залогиненным
- Ошибка → редирект на login

### Logout
1. `POST /auth/logout` (сервер инвалидирует refresh token)
2. Очистить Zustand store
3. Редирект на login

## Авторизация роутов — role-based guards

Роуты, доступные не всем ролям, защищаются `RoleGuard` — проверяет не только наличие пользователя, но и его роль. Навигация отображает только те ссылки, роуты которых доступны текущей роли. Фронтенд-гард — UX-слой, не замена серверной авторизации.

## Кнопки мутаций — disabled во время выполнения

Каждая кнопка, вызывающая мутацию, обязана быть `disabled` пока `isPending === true`. Текст кнопки меняется на loading-состояние. Защита от двойного клика.

**Submit-lock с async prereqs.** Если submit зависит от async-данных (загрузка dictionaries, lookup'а, init-запроса) — кнопка `disabled` пока prereqs не загрузились, не только при `isPending`. Иначе пользователь нажмёт submit до того как зависимости резолвятся, и form отправится с пустыми / дефолтными значениями.

**Double-submit guard.** Помимо `disabled`, держать external boolean flag (`isSubmitting` в local state), который явно сбрасывается в `onSettled`. Защищает от race между React-ре-рендером disabled и быстрыми кликами.

## Общий код между web и tma

Код, идентичный в web и tma, выносится в shared-пакет (`@ugcboost/shared`) через pnpm workspaces. Дублирование одних и тех же файлов между приложениями запрещено.

```
frontend/
├── package.json              # workspace root
├── packages/
│   └── shared/
│       ├── package.json      # name: @ugcboost/shared
│       └── src/
├── web/
│   └── package.json          # depends on @ugcboost/shared
├── tma/
│   └── package.json          # depends on @ugcboost/shared
└── landing/
```

## Что ревьюить

- [blocker] Access token в `localStorage` / `sessionStorage` (XSS уязвимость — должен быть только в памяти Zustand).
- [major] Refresh token не httpOnly cookie (доступен из JS).
- [major] Глобальный store для локального UI-state (модалка, тоггл).
- [major] Context для часто обновляемого state (re-render всего дерева).
- [major] Кнопка мутации не disabled при `isPending` (двойной submit).
- [major] Submit зависит от async prereqs (dictionaries, lookup), но кнопка disabled только при `isPending` — отправка пройдёт до резолва зависимостей.
- [major] Double-submit guard отсутствует (нет external `isSubmitting` flag, сбрасываемого в `onSettled`).
- [major] Дублирование одного и того же файла между web/tma (вместо `@ugcboost/shared`).
