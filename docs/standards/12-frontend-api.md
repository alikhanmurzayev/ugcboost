# Фронтенд: API-слой [REQUIRED]

API-слой — граница между фронтендом и бэкендом. Здесь критичны единообразие, типобезопасность и корректная обработка ошибок.

---

## CS-45: Все HTTP-запросы — через API-клиент, никаких raw fetch()

**Scope:** frontend

**Почему:** `web/src/api/auth.ts:50` и `web/src/api/client.ts:33` используют `fetch()` напрямую, минуя API-клиент (`api()` функцию). Это значит — свой error handling, свои заголовки, свои credentials. Любое изменение в формате ошибок или auth-логике нужно менять в N местах вместо одного.

**Плохо** (web/src/api/auth.ts:50):
```typescript
// restoreSession() обходит api() клиент
const res = await fetch(`${BASE}/auth/me`, {
  headers: { Authorization: `Bearer ${token}` },
  credentials: "include",
});
```

**Плохо** (web/src/api/client.ts:33):
```typescript
// refreshToken() обходит api() клиент
const res = await fetch(`${BASE}/auth/refresh`, {
  method: "POST",
  credentials: "include",
});
```

**Хорошо:**
```typescript
// Все запросы через единый клиент
export async function restoreSession(): Promise<User> {
  return api<UserResponse>("/auth/me");
}

// Для refresh — допустимо внутри самого клиента (это инфраструктура клиента),
// но должно быть отмечено комментарием
```

**Правило:** все HTTP-запросы к API идут через единый клиент (`api()` функцию). Raw `fetch()` допустим только внутри самого API-клиента для инфраструктурных нужд (refresh token) с комментарием почему.

---

## CS-46: API-пути — константы, не строковые литералы

**Scope:** frontend

**Почему:** `/auth/login`, `/brands`, `/brands/${id}/managers` и т.д. разбросаны по файлам api/auth.ts, api/brands.ts, api/audit.ts как строковые литералы. Изменили путь на бэке — ищем по всему фронту grep'ом.

**Плохо:**
```typescript
// api/auth.ts
return api<LoginResponse>("/auth/login", { method: "POST", body: data });

// api/brands.ts
return api<BrandListResponse>("/brands");
return api<BrandResponse>(`/brands/${id}`);
return api<void>(`/brands/${brandId}/managers`, { method: "POST", body: { email } });
```

**Хорошо:**
```typescript
// api/endpoints.ts — единственное место определения путей
export const API = {
  AUTH: {
    LOGIN: "/auth/login",
    LOGOUT: "/auth/logout",
    REFRESH: "/auth/refresh",
    ME: "/auth/me",
    PASSWORD_RESET_REQUEST: "/auth/password-reset-request",
    PASSWORD_RESET: "/auth/password-reset",
  },
  BRANDS: {
    LIST: "/brands",
    ONE: (id: string) => `/brands/${id}`,
    MANAGERS: (brandId: string) => `/brands/${brandId}/managers`,
    MANAGER: (brandId: string, userId: string) => `/brands/${brandId}/managers/${userId}`,
  },
  AUDIT: {
    LIST: "/audit-logs",
  },
} as const;

// Использование:
return api<LoginResponse>(API.AUTH.LOGIN, { method: "POST", body: data });
return api<BrandListResponse>(API.BRANDS.LIST);
return api<void>(API.BRANDS.MANAGERS(brandId), { method: "POST", body: { email } });
```

**Правило:** все API-пути определяются в одном файле (`api/endpoints.ts`) как константы. Строковые литералы путей в коде запрещены.

---

## CS-47: Query keys — константы с фабрикой

**Scope:** frontend

**Почему:** `["brands"]`, `["brand", brandId]`, `["audit-logs", ...]` как строковые литералы в 10+ местах. Опечатка `"brads"` — кэш не инвалидируется, данные устаревшие.

**Плохо** (рассыпано по features/):
```typescript
// BrandsPage.tsx:14
useQuery({ queryKey: ["brands"], ... })

// BrandDetailPage.tsx:22
useQuery({ queryKey: ["brand", brandId], ... })

// BrandDetailPage.tsx:31
queryClient.invalidateQueries({ queryKey: ["brands"] })

// AuditLogPage.tsx:22
useQuery({ queryKey: ["audit-logs", page, perPage, filters], ... })
```

**Хорошо:**
```typescript
// api/queryKeys.ts
export const QueryKeys = {
  brands: {
    all: ["brands"] as const,
    one: (id: string) => ["brands", id] as const,
  },
  auditLogs: {
    list: (params: AuditLogParams) => ["audit-logs", params] as const,
  },
} as const;

// Использование:
useQuery({ queryKey: QueryKeys.brands.all, ... })
useQuery({ queryKey: QueryKeys.brands.one(brandId), ... })
queryClient.invalidateQueries({ queryKey: QueryKeys.brands.all })
```

**Правило:** все query keys определяются в одном файле с фабричными функциями. Строковые литералы query keys в компонентах запрещены. Иерархическая структура ключей обеспечивает правильную инвалидацию (инвалидация `["brands"]` затрагивает `["brands", id]`).

---

## CS-48: Каждая мутация — обработка onError

**Scope:** frontend

**Почему:** `deleteMut` в BrandsPage.tsx:29, `updateMut` и `removeMut` в BrandDetailPage.tsx:27,48 — НЕТ `onError`. Пользователь нажал "Удалить", запрос упал — ничего не произошло. Пользователь не знает, удалилось или нет.

**Плохо** (web/src/features/brands/BrandsPage.tsx:29-34):
```typescript
const deleteMut = useMutation({
  mutationFn: (id: string) => deleteBrand(id),
  onSuccess: () => {
    queryClient.invalidateQueries({ queryKey: ["brands"] });
  },
  // onError ОТСУТСТВУЕТ — ошибка молча проглочена
});
```

**Плохо** (web/src/features/brands/BrandDetailPage.tsx:48-54):
```typescript
const removeMut = useMutation({
  mutationFn: (userId: string) => removeManager(brandId!, userId),
  onSuccess: () => {
    queryClient.invalidateQueries({ queryKey: ["brand", brandId] });
  },
  // onError ОТСУТСТВУЕТ
});
```

**Хорошо:**
```typescript
const deleteMut = useMutation({
  mutationFn: (id: string) => deleteBrand(id),
  onSuccess: () => {
    queryClient.invalidateQueries({ queryKey: QueryKeys.brands.all });
  },
  onError: (error) => {
    toast.error(getErrorMessage(error));
    // или: setError(getErrorMessage(error));
  },
});
```

**Правило:** каждый `useMutation` обязан иметь `onError` handler. Ошибки мутаций всегда отображаются пользователю — через toast, inline-сообщение или модалку. Молчаливые мутации запрещены.
