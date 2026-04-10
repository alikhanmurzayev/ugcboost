# Фронтенд: типы и кодогенерация [CRITICAL]

Единственный источник правды для API-типов — OpenAPI-спецификация. Сгенерированные типы в `api/generated/schema.ts` существуют именно для этого. Ручные интерфейсы рядом с ними — прямой путь к рассинхрону.

---

## CS-41: Все API-типы — только из generated/schema.ts

**Scope:** frontend

**Почему:** `User` определён вручную в трёх местах (api/auth.ts:3, stores/auth.ts:3, api/brands.ts через ManagerInfo). Если в OpenAPI добавится поле `phone` — сгенерированный тип обновится, а три ручных — нет. Фронтенд молча потеряет данные.

**Плохо** (web/src/api/auth.ts:3-7):
```typescript
interface User {
  id: string;
  email: string;
  role: "admin" | "brand_manager";
}
```

**Плохо** (web/src/stores/auth.ts:3-6 — дубль):
```typescript
interface User {
  id: string;
  email: string;
  role: "admin" | "brand_manager";
}
```

**Плохо** (web/src/api/brands.ts:3-53 — целая россыпь ручных типов):
```typescript
interface Brand { ... }
interface BrandListItem { ... }
interface ManagerInfo { ... }
interface BrandWithManagers { ... }
```

**Хорошо:**
```typescript
import type { components } from "@/api/generated/schema";

type User = components["schemas"]["User"];
type Brand = components["schemas"]["Brand"];
type BrandWithManagers = components["schemas"]["BrandResult"];
type AuditLogEntry = components["schemas"]["AuditLogEntry"];
type LoginRequest = components["schemas"]["LoginRequest"];

// Реэкспорт из одного файла для удобства:
// api/types.ts
export type { User, Brand, ... };
```

**Правило:** запрещено объявлять `interface`/`type` для API-сущностей вручную. Все типы импортируются из `api/generated/schema.ts`. Допускается файл `api/types.ts` для реэкспорта с человекочитаемыми именами.

---

## CS-42: Типы ответов API — из schema, не из обёрток

**Scope:** frontend

**Почему:** `web/src/api/auth.ts:9-18` определяет `LoginResponse`, `UserResponse` как ручные обёртки `{ data: { ... } }`. Эта обёртка уже описана в OpenAPI. Если формат ответа изменится — ручная обёртка не обновится.

**Плохо:**
```typescript
// api/auth.ts:9-14
interface LoginResponse {
  data: {
    accessToken: string;
    user: User;
  };
}
```

**Хорошо:**
```typescript
import type { paths } from "@/api/generated/schema";

type LoginResponse = paths["/auth/login"]["post"]["responses"]["200"]["content"]["application/json"];
type BrandListResponse = paths["/brands"]["get"]["responses"]["200"]["content"]["application/json"];
```

**Правило:** типы ответов извлекаются из `paths` сгенерированной схемы. Ручные `{ data: { ... } }` обёртки запрещены.

---

## CS-43: TypeScript strict mode — обязателен

**Scope:** frontend

**Почему:** В `tsconfig.app.json` включены `noUnusedLocals` и `noUnusedParameters`, но **не включён `strict: true`**. Это значит что `strictNullChecks`, `noImplicitAny`, `strictFunctionTypes` и другие критические проверки **выключены**. TypeScript работает в полсилы.

**Плохо:**
```json
{
  "compilerOptions": {
    "noUnusedLocals": true,
    "noUnusedParameters": true
    // strict: true ОТСУТСТВУЕТ!
  }
}
```

**Хорошо:**
```json
{
  "compilerOptions": {
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "noUncheckedIndexedAccess": true
  }
}
```

**Правило:** `"strict": true` обязателен в обоих приложениях (web, tma). Это включает: `strictNullChecks`, `noImplicitAny`, `strictFunctionTypes`, `strictBindCallApply`, `strictPropertyInitialization`, `alwaysStrict`.

---

## CS-44: Non-null assertions (!) — запрещены, использовать type guards

**Scope:** frontend

**Почему:** `brandId!` в `BrandDetailPage.tsx:23,28,37,49` — это обход компилятора. Если `brandId` окажется `undefined` (например, при прямом переходе по сломанному URL) — будет runtime crash.

**Плохо** (web/src/features/brands/BrandDetailPage.tsx:23):
```typescript
const { data: brand } = useQuery({
  queryKey: ["brand", brandId],
  queryFn: () => getBrand(brandId!),  // crash если brandId undefined
  enabled: !!brandId,
});
```

**Хорошо:**
```typescript
const { brandId } = useParams<{ brandId: string }>();

if (!brandId) {
  return <Navigate to={ROUTES.BRANDS} replace />;
}

// После проверки TypeScript знает что brandId — string, не undefined
const { data: brand } = useQuery({
  queryKey: [QueryKeys.BRAND, brandId],
  queryFn: () => getBrand(brandId),
});
```

**Правило:** оператор `!` (non-null assertion) запрещён. Использовать early return, type guards или conditional rendering. Единственное исключение — `document.getElementById('root')!` в main.tsx (bootstrapping).
