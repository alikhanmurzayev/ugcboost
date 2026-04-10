# Фронтенд: компоненты и UI [REQUIRED]

Компоненты — строительные блоки UI. Качество компонентов определяет скорость разработки, тестируемость и UX.

---

## CS-49: Route paths — константы в одном файле

**Scope:** frontend

**Почему:** Пути `"/brands"`, `"/audit"`, `"/brands/:brandId"` определены как строковые литералы в App.tsx (роутинг) и в DashboardLayout.tsx (навигация). Переименовали роут в одном месте — забыли в другом — мёртвая ссылка.

**Плохо** (web/src/App.tsx:25-32 + web/src/shared/layouts/DashboardLayout.tsx:5-18):
```typescript
// App.tsx — роуты
<Route path="/brands" element={<BrandsPage />} />
<Route path="/brands/:brandId" element={<BrandDetailPage />} />
<Route path="/audit" element={<AuditLogPage />} />

// DashboardLayout.tsx — навигация (ДУБЛИРОВАНИЕ!)
const adminNav = [
  { label: "Бренды", path: "/brands" },
  { label: "Креаторы", path: "/creators" },      // ← нет такого роута!
  { label: "Кампании", path: "/campaigns" },      // ← нет такого роута!
  { label: "Модерация", path: "/moderation" },    // ← нет такого роута!
  { label: "Аудит", path: "/audit" },
];
```

**Хорошо:**
```typescript
// shared/constants/routes.ts
export const ROUTES = {
  LOGIN: "/login",
  DASHBOARD: "/",
  BRANDS: "/brands",
  BRAND_DETAIL: (id: string) => `/brands/${id}`,
  AUDIT: "/audit",
  // Добавлять по мере реализации:
  // CREATORS: "/creators",
  // CAMPAIGNS: "/campaigns",
} as const;

// App.tsx
<Route path={ROUTES.BRANDS} element={<BrandsPage />} />

// DashboardLayout.tsx
const adminNav = [
  { label: t("nav.brands"), path: ROUTES.BRANDS },
  { label: t("nav.audit"), path: ROUTES.AUDIT },
];

// Навигация в компонентах:
navigate(ROUTES.BRAND_DETAIL(brand.id));
```

**Правило:** все route paths определяются в `shared/constants/routes.ts`. Строковые литералы путей в компонентах и роутере запрещены. Навигация (`adminNav`, `brandNav`) содержит только существующие роуты — мёртвые ссылки запрещены.

---

## CS-50: Роли пользователей — константы, не строковые литералы

**Scope:** frontend

**Почему:** `user?.role === "admin"` в DashboardLayout.tsx:26 — та же проблема что на бэкенде (CS-01). Опечатка `"admim"` — навигация сломана.

**Плохо:**
```typescript
// DashboardLayout.tsx:26
const nav = user?.role === "admin" ? adminNav : brandNav;

// api/auth.ts:6
role: "admin" | "brand_manager";
```

**Хорошо:**
```typescript
// Импортировать из generated schema (роли определены в OpenAPI)
import type { components } from "@/api/generated/schema";
type UserRole = components["schemas"]["User"]["role"];

// Или определить константы:
export const Roles = {
  ADMIN: "admin",
  BRAND_MANAGER: "brand_manager",
} as const;

// Использование:
const nav = user?.role === Roles.ADMIN ? adminNav : brandNav;
```

**Правило:** роли сравниваются через константы или типы из кодогенерации. Строковые литералы ролей в компонентах запрещены. Это зеркало бэкенд-правила CS-01.

---

## CS-51: Компонент > 150 строк — разбивать на подкомпоненты

**Scope:** frontend

**Почему:** `BrandDetailPage.tsx` (225 строк) содержит: отображение бренда, форму редактирования, список менеджеров, форму назначения менеджера. Четыре ответственности в одном компоненте — сложно тестировать, сложно переиспользовать, сложно читать.

**Плохо:**
```typescript
// BrandDetailPage.tsx — 225 строк, 4 ответственности
export default function BrandDetailPage() {
  // 8 hooks
  // 4 мутации
  // 3 обработчика форм
  // Рендер: информация о бренде + форма редактирования + список менеджеров + форма назначения
}
```

**Хорошо:**
```
features/brands/
├── BrandDetailPage.tsx        # Оркестратор: собирает подкомпоненты, <80 строк
├── components/
│   ├── BrandInfo.tsx          # Отображение + редактирование бренда
│   ├── ManagerList.tsx        # Список менеджеров + удаление
│   └── AssignManagerForm.tsx  # Форма назначения менеджера
```

**Правило:** компонент > 150 строк — сигнал к декомпозиции. Страница (Page) — оркестратор, который собирает подкомпоненты. Бизнес-логика (мутации, обработчики) живёт в подкомпонентах рядом с UI, который их использует.

---

## CS-52: Нативные confirm()/alert() — запрещены, использовать UI-компоненты

**Scope:** frontend

**Почему:** `confirm("Удалить бренд?")` (BrandsPage.tsx:122) и `confirm("Удалить менеджера?")` (BrandDetailPage.tsx:162) — нативные диалоги. Их нельзя стилизовать, нельзя локализовать, они блокируют поток, выглядят чужеродно.

**Плохо:**
```typescript
if (!confirm("Удалить бренд?")) return;
deleteMut.mutate(id);
```

**Хорошо:**
```typescript
// Используем AlertDialog из shadcn/ui (уже в проекте как зависимость дизайн-системы)
<AlertDialog>
  <AlertDialogTrigger asChild>
    <Button variant="destructive">{t("actions.delete")}</Button>
  </AlertDialogTrigger>
  <AlertDialogContent>
    <AlertDialogHeader>
      <AlertDialogTitle>{t("brands.deleteConfirmTitle")}</AlertDialogTitle>
      <AlertDialogDescription>{t("brands.deleteConfirmDescription")}</AlertDialogDescription>
    </AlertDialogHeader>
    <AlertDialogFooter>
      <AlertDialogCancel>{t("actions.cancel")}</AlertDialogCancel>
      <AlertDialogAction onClick={() => deleteMut.mutate(id)}>
        {t("actions.delete")}
      </AlertDialogAction>
    </AlertDialogFooter>
  </AlertDialogContent>
</AlertDialog>
```

**Правило:** `window.confirm()`, `window.alert()`, `window.prompt()` запрещены. Все диалоги — через компоненты UI-библиотеки (shadcn/ui AlertDialog, Dialog и т.д.).

---

## CS-53: Loading/Error/Empty states — обязательны и единообразны

**Scope:** frontend

**Почему:** Во всех страницах (AuditLogPage:73, BrandsPage:93, BrandDetailPage:56, AuthGuard:35) loading state — просто текст `"Загрузка..."`. Нет skeleton loaders, нет спиннеров, нет empty states. UX страдает.

**Плохо:**
```typescript
if (isLoading) return <p>Загрузка...</p>;
if (error) return <p>Ошибка загрузки</p>;
```

**Хорошо:**
```typescript
// shared/components/ — переиспользуемые компоненты состояний
import { Skeleton } from "@/shared/components/Skeleton";
import { ErrorState } from "@/shared/components/ErrorState";
import { EmptyState } from "@/shared/components/EmptyState";

if (isLoading) return <Skeleton variant="table" rows={5} />;
if (error) return <ErrorState error={error} onRetry={refetch} />;
if (data.length === 0) return <EmptyState icon={<BoxIcon />} message={t("brands.empty")} />;
```

**Правило:** каждый запрос данных (useQuery) обязан обрабатывать три состояния: loading (skeleton/spinner), error (с кнопкой retry), empty (с понятным сообщением). Использовать переиспользуемые компоненты из `shared/components/`. Голый текст "Загрузка..." запрещён.
