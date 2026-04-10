# Фронтенд: качество и надёжность [REQUIRED]

Правила, обеспечивающие надёжность, доступность и поддерживаемость фронтенд-кода.

---

## CS-58: ESLint — строгие правила обязательны

**Scope:** frontend

**Почему:** Текущий ESLint-конфиг минимален: только `recommended` от TypeScript-ESLint + react-hooks. Не ловит: console.log в production, неупорядоченные импорты, неявные `any` в catch, неиспользуемые переменные в деструктуризации.

**Текущий конфиг** (web/eslint.config.js, tma/eslint.config.js — идентичны):
```javascript
// Только базовый recommended
...tseslint.configs.recommended,
```

**Необходимые правила:**
```javascript
rules: {
  // Запрет console.log (допустимо console.error, console.warn)
  "no-console": ["error", { allow: ["warn", "error"] }],

  // Запрет неиспользуемых переменных (с исключением для _prefix)
  "@typescript-eslint/no-unused-vars": ["error", {
    argsIgnorePattern: "^_",
    varsIgnorePattern: "^_",
  }],

  // Запрет any
  "@typescript-eslint/no-explicit-any": "error",

  // Запрет non-null assertion
  "@typescript-eslint/no-non-null-assertion": "error",

  // Обязательные return types для экспортируемых функций
  "@typescript-eslint/explicit-module-boundary-types": "warn",
}
```

**Правило:** ESLint-конфиг единый для web и tma (вынести в корень или shared). Правила должны ловить проблемы из этого документа на этапе lint, а не на code review.

---

## CS-59: Формы — валидация с feedback пользователю

**Scope:** frontend

**Почему:** Формы в проекте используют минимальную HTML5-валидацию (`required`, `type="email"`). Email менеджера в BrandDetailPage.tsx:185 не проверяется вообще — только `trim()`. Пользователь не получает внятного feedback при ошибке.

**Плохо** (web/src/features/brands/BrandDetailPage.tsx:74,185):
```typescript
// Валидация — только trim
const handleAssign = () => {
  if (!managerEmail.trim()) return;  // молча ничего не делает
  assignMut.mutate();
};

// Input без валидации email-формата
<input
  type="text"  // даже не type="email"!
  value={managerEmail}
  onChange={(e) => setManagerEmail(e.target.value)}
/>
```

**Хорошо:**
```typescript
// С валидацией и feedback
const [emailError, setEmailError] = useState<string | null>(null);

const handleAssign = () => {
  const email = managerEmail.trim();
  if (!email) {
    setEmailError(t("validation.emailRequired"));
    return;
  }
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
    setEmailError(t("validation.emailInvalid"));
    return;
  }
  setEmailError(null);
  assignMut.mutate();
};

// Input с error state
<div>
  <input
    type="email"
    value={managerEmail}
    onChange={(e) => { setManagerEmail(e.target.value); setEmailError(null); }}
    aria-invalid={!!emailError}
    aria-describedby={emailError ? "email-error" : undefined}
  />
  {emailError && <p id="email-error" role="alert" className="text-red-500 text-sm">{emailError}</p>}
</div>
```

**Правило:** каждое поле формы с ограничениями имеет валидацию на клиенте с понятным сообщением об ошибке. Молчаливый `return` без feedback запрещён. При ошибке — показать что не так и где.

---

## CS-60: Accessibility — базовые требования

**Scope:** frontend

**Почему:** `<select>` без `aria-label` (AuditLogPage:42-48), таблицы без `scope` на заголовках (BrandsPage:97-104, AuditLogPage:79-87), кликабельные `<tr>` без keyboard navigation (BrandsPage:111). Скринридеры не смогут работать с интерфейсом.

**Минимальные требования:**

```typescript
// Все интерактивные элементы — с aria-label если нет видимого текста
<select aria-label={t("audit.filterByAction")}>

// Таблицы — scope на заголовках
<th scope="col">{t("brands.name")}</th>

// Кликабельные строки — через ссылку внутри, не onClick на <tr>
// ПЛОХО:
<tr onClick={() => navigate(`/brands/${brand.id}`)} className="cursor-pointer">

// ХОРОШО:
<tr>
  <td><Link to={ROUTES.BRAND_DETAIL(brand.id)}>{brand.name}</Link></td>
</tr>

// Формы — label привязан к input
<label htmlFor="email">{t("auth.email")}</label>
<input id="email" type="email" ... />

// Ошибки — role="alert"
{error && <p role="alert">{error}</p>}

// Кнопки-иконки — aria-label
<button aria-label={t("actions.delete")}><TrashIcon /></button>
```

**Правило:**
1. Все `<input>`, `<select>`, `<textarea>` имеют связанный `<label>` или `aria-label`
2. Все `<th>` имеют `scope="col"` или `scope="row"`
3. Все кнопки-иконки (без текста) имеют `aria-label`
4. Ошибки валидации имеют `role="alert"` и связаны через `aria-describedby`
5. Интерактивные элементы доступны с клавиатуры (Tab, Enter, Escape)

---

## CS-61: Error boundaries — обязательны

**Scope:** frontend

**Почему:** В проекте нет ни одного Error Boundary. Если React-компонент бросит исключение при рендере — белый экран. Пользователь потеряет весь контекст, единственный выход — перезагрузка.

**Хорошо:**
```typescript
// shared/components/ErrorBoundary.tsx
class ErrorBoundary extends React.Component<Props, State> {
  state = { hasError: false, error: null };

  static getDerivedStateFromError(error: Error) {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    // Логирование в будущем (Sentry, etc.)
    console.error("React error boundary caught:", error, info);
  }

  render() {
    if (this.state.hasError) {
      return (
        <div>
          <h2>{t("errors.unexpected")}</h2>
          <Button onClick={() => window.location.reload()}>
            {t("actions.reload")}
          </Button>
        </div>
      );
    }
    return this.props.children;
  }
}

// App.tsx — оборачиваем роуты
<ErrorBoundary>
  <RouterProvider router={router} />
</ErrorBoundary>
```

**Правило:** корневой `ErrorBoundary` оборачивает всё приложение. По мере роста — дополнительные границы на уровне feature-секций, чтобы ошибка в одном модуле не роняла весь UI.

---

## CS-62: Runtime config — с валидацией

**Scope:** frontend

**Почему:** `window.__RUNTIME_CONFIG__?.apiUrl` (web/src/api/client.ts:3-10) — optional chaining без валидации. Если конфиг не подгрузился — молча fallback на `/api`. В production с неправильным Nginx-конфигом — запросы уходят не туда.

**Плохо:**
```typescript
const BASE = window.__RUNTIME_CONFIG__?.apiUrl ?? "/api";
```

**Хорошо:**
```typescript
// shared/lib/config.ts
function getApiUrl(): string {
  const url = window.__RUNTIME_CONFIG__?.apiUrl;
  if (!url) {
    if (import.meta.env.DEV) {
      return "/api"; // Допустимый fallback только в dev (Vite proxy)
    }
    throw new Error("Runtime config missing: __RUNTIME_CONFIG__.apiUrl is required in production");
  }
  return url;
}

export const API_BASE = getApiUrl();
```

**Правило:** runtime config валидируется при инициализации приложения. В production отсутствие обязательного конфига — fatal error (лучше не запуститься, чем работать неправильно). В dev — допустимы fallback-значения.
