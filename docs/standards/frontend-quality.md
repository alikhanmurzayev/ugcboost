# Фронтенд: качество и надёжность [REQUIRED]

## TypeScript strict mode

Обязателен в обоих приложениях (web, tma):

```json
{
  "compilerOptions": {
    "strict": true,
    "noUncheckedIndexedAccess": true,
    "noFallthroughCasesInSwitch": true
  }
}
```

`noUncheckedIndexedAccess` — обязателен, без него `array[0]` имеет тип `T` вместо `T | undefined`.

## Запрещённые конструкции

- **`any`** — запрещён. Использовать `unknown` + type guards
- **`!` (non-null assertion)** — запрещён. Использовать early return, type guards или conditional rendering. Исключение: `document.getElementById('root')!` в main.tsx
- **`as` (type assertion)** — запрещён. Использовать type guards

## ESLint

Конфиг единый для web и tma (вынести в shared). Обязательные правила:
- `no-console` — запрет `console.log` (допустимо `console.error`, `console.warn`)
- `@typescript-eslint/no-explicit-any` — запрет `any`
- `@typescript-eslint/no-non-null-assertion` — запрет `!`
- `@typescript-eslint/no-unused-vars` — с исключением для `_` prefix

## Валидация форм

Каждое поле формы с ограничениями — валидация на клиенте с понятным сообщением об ошибке. Молчаливый `return` без feedback пользователю запрещён. При ошибке — показать что не так и где.

## Error boundaries

Корневой `ErrorBoundary` оборачивает всё приложение. При ошибке рендера — fallback UI с кнопкой перезагрузки, не белый экран. По мере роста — дополнительные границы на уровне feature-секций.

## Accessibility — базовые требования

- Все `<input>`, `<select>`, `<textarea>` имеют связанный `<label>` или `aria-label`
- Все кнопки-иконки (без текста) имеют `aria-label`
- Ошибки валидации имеют `role="alert"` и связаны с полем через `aria-describedby`
- Интерактивные элементы доступны с клавиатуры (Tab, Enter, Escape)
- Кликабельные строки таблиц — через `<Link>` внутри ячейки, не `onClick` на `<tr>`

## Runtime config — с валидацией

Runtime config валидируется при инициализации приложения. В production отсутствие обязательного конфига — fatal error (лучше не запуститься, чем работать неправильно). В dev — допустимы fallback-значения.
