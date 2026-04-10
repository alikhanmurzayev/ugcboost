# Фронтенд: состояние и авторизация [REQUIRED]

Управление состоянием и авторизация — фундамент фронтенд-приложения. Ошибки здесь приводят к утечкам данных и сломанному UX.

---

## CS-54: i18n — все пользовательские строки через систему локализации

**Scope:** frontend

**Почему:** В проекте есть `shared/i18n/ru.json` с переводами, но он **не используется**. Все строки захардкожены в компонентах: "Войдите в кабинет" (LoginPage:46), "Загрузка..." (везде), "Бренды" (DashboardLayout:6), "Удалить бренд?" (BrandsPage:122). Когда понадобится казахский или английский — придётся лазить по каждому компоненту.

**Плохо** (рассыпано по всем компонентам):
```typescript
// LoginPage.tsx:46
<h1>Войдите в кабинет</h1>

// BrandsPage.tsx:122
if (!confirm("Удалить бренд?")) return;

// AuditLogPage.tsx:5-13
const ACTION_LABELS: Record<string, string> = {
  "user.login": "Вход",
  "brand.create": "Создание бренда",
  // ...
};

// AuthGuard.tsx:35
<p>Загрузка...</p>
```

**Хорошо:**
```typescript
// shared/i18n/ru.json — единственный источник всех строк
{
  "auth": {
    "loginTitle": "Войдите в кабинет",
    "loginButton": "Войти",
    "loginLoading": "Вход..."
  },
  "brands": {
    "title": "Бренды",
    "deleteConfirm": "Удалить бренд?",
    "createError": "Не удалось создать бренд"
  },
  "common": {
    "loading": "Загрузка...",
    "error": "Ошибка загрузки",
    "retry": "Повторить"
  },
  "audit": {
    "actions": {
      "user.login": "Вход",
      "brand.create": "Создание бренда"
    }
  }
}

// Использование через хук:
const { t } = useTranslation();
<h1>{t("auth.loginTitle")}</h1>
```

**Правило:** все пользовательские строки (labels, messages, tooltips, placeholders) — через систему i18n. Строковые литералы на русском/казахском/английском в JSX запрещены. Исключение: технические строки (CSS-классы, data-атрибуты).

---

## CS-55: Авторизация роутов — role-based guards

**Scope:** frontend

**Почему:** `AuthGuard` проверяет только наличие `user`, но не его роль. Роут `/audit` доступен всем аутентифицированным пользователям, хотя на бэкенде он admin-only. Brand manager зайдёт на `/audit` → увидит 403 → плохой UX.

**Плохо** (web/src/features/auth/AuthGuard.tsx):
```typescript
// Проверяет только "залогинен ли"
if (!user) {
  return <Navigate to="/login" replace />;
}
return <Outlet />;
```

**Хорошо:**
```typescript
// shared/components/RoleGuard.tsx
interface RoleGuardProps {
  allowedRoles: UserRole[];
  children: React.ReactNode;
}

export function RoleGuard({ allowedRoles, children }: RoleGuardProps) {
  const user = useAuthStore((s) => s.user);

  if (!user) return <Navigate to={ROUTES.LOGIN} replace />;
  if (!allowedRoles.includes(user.role)) return <Navigate to={ROUTES.DASHBOARD} replace />;

  return <>{children}</>;
}

// App.tsx
<Route element={<RoleGuard allowedRoles={[Roles.ADMIN]} />}>
  <Route path={ROUTES.AUDIT} element={<AuditLogPage />} />
</Route>
```

**Правило:** роуты, доступные не всем ролям, защищаются `RoleGuard`. Навигация отображает только те ссылки, роуты которых доступны текущей роли. Фронтенд-гард — UX-слой, не замена серверной авторизации.

---

## CS-56: Кнопки мутаций — disabled во время выполнения

**Scope:** frontend

**Почему:** Кнопка "Сохранить" в BrandDetailPage.tsx:106 не блокируется во время `updateMut.isPending`. Двойной клик — два запроса — потенциальный конфликт. При этом кнопка "Назначить" (BrandDetailPage:193) блокируется правильно — непоследовательность.

**Плохо:**
```typescript
// BrandDetailPage.tsx:106 — НЕТ disabled
<button onClick={handleUpdate}>Сохранить</button>

// BrandsPage.tsx:122 — нет защиты от двойного клика
<button onClick={() => { if (confirm("...")) deleteMut.mutate(id) }}>Удалить</button>
```

**Хорошо:**
```typescript
<Button
  onClick={handleUpdate}
  disabled={updateMut.isPending}
>
  {updateMut.isPending ? t("common.saving") : t("actions.save")}
</Button>
```

**Правило:** каждая кнопка, вызывающая мутацию, обязана быть `disabled` пока `isPending === true`. Текст кнопки меняется на loading-состояние ("Сохранение...", "Удаление...").

---

## CS-57: Дублирование кода между web и tma — выносить в shared пакет

**Scope:** frontend

**Почему:** `shared/i18n/errors.ts` и `runtime-config.d.ts` идентичны в web и tma. При изменении в одном — забудут обновить в другом. По мере роста дублирование будет нарастать.

**Текущее дублирование:**
- `src/shared/i18n/errors.ts` — одинаковые маппинги ошибок
- `src/runtime-config.d.ts` — одинаковые типы
- `tailwind.config.ts` — почти одинаковые конфигурации (обе импортируют preset)

**Правило:** код, идентичный в web и tma, выносится в корневой shared-пакет при первом обнаружении дубля. Структура:

```
packages/
  shared/
    src/
      i18n/errors.ts
      types/runtime-config.ts
      constants/roles.ts
      constants/routes.ts   # если пересекаются
```

Или проще — на этапе MVP использовать symlinks или общий путь в tsconfig `paths`.
