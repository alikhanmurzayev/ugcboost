# Фронтенд: качество и надёжность

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

## Landing (Astro) specifics

- **Form input masks.** Поля известного формата (телефон `+7 ___ ___ __ __`, ИИН — 12 цифр) получают input mask на клиенте. Без маски пользователь вводит мусор → валидация выдаёт UX-ошибки задним числом, после submit.
- **WebP по дефолту для растровых ассетов.** Hero, benefits, logos и другие статичные изображения — WebP. Fallback на PNG/JPG только если нужна совместимость с конкретным потребителем. Без WebP лендос отдаёт 2-5x лишних байт на изображение.

## Что ревьюить

- [blocker] Runtime config не валидируется при инициализации в production — приложение запустится с пропавшим обязательным конфигом.
- [major] `any` / `!` (non-null assertion) / `as` (type assertion) в коде. Исключение `document.getElementById('root')!` — единственное.
- [major] `console.log` в коде (только `console.error` / `console.warn` допустимы).
- [major] Молчаливый return формы без error message.
- [major] `<input>` / `<select>` / `<textarea>` без связанного `<label>` / `aria-label`.
- [major] Ошибка валидации без `role="alert"` / без `aria-describedby` на поле.
- [minor] Form input без mask для известного формата (phone, IIN).
- [minor] Растровый ассет не WebP (jpg/png без обоснования).
