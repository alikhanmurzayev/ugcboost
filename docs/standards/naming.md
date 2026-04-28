# Нейминг и стиль

## Backend (Go)

### Файлы и пакеты
- Файлы: `snake_case.go` (`brand_service.go`, `audit_handler.go`)
- Пакеты: короткие, одно слово, lowercase (`handler`, `service`, `repository`). Не `brandService`, не `brand_service`
- Тест-файлы: `{name}_test.go` рядом с тестируемым файлом

### Структуры и методы
- Зависимости в структурах — с суффиксом слоя: `authService`, `brandRepo`
- Интерфейсы — с суффиксом слоя: `AuthService`, `BrandRepo`. Для простых — Go-конвенция суффикс `-er` (`TokenValidator`, `PasswordHasher`)
- Receiver — сокращённое имя типа: `func (s *BrandService)`, `func (r *UserRepo)`, `func (h *AuthHandler)`

### Константы
- Колонки БД: `{Entity}Column{Field}` — `UserColumnEmail`, `BrandColumnName` (подробнее в стандарте репозитория)
- Таблицы: `Table{Entity}` — `TableUsers`, `TableBrands`
- Коды ошибок: `Code{Error}` — `CodeValidation`, `CodeNotFound`
- Общий принцип: имя константы отражает домен + назначение, без сокращений

## Frontend (TypeScript/React)

### Файлы
- Компоненты: `PascalCase.tsx` (`BrandDetailPage.tsx`, `ManagerList.tsx`)
- Хуки: `camelCase.ts` с префиксом `use` (`useAuth.ts`, `useBrands.ts`)
- Утилиты, константы: `camelCase.ts` (`formatDate.ts`, `routes.ts`)
- Тесты: рядом с файлом, `.test.ts` (`useAuth.test.ts`, `formatDate.test.ts`)

### Компоненты и props
- Event handlers внутри компонента: `handle{Action}` — `handleSubmit`, `handleDelete`, `handleEmailChange`
- Callback props: `on{Action}` — `onClick`, `onDelete`, `onBrandSelect`
- Булевы переменные и props: префикс `is`/`has`/`can` — `isLoading`, `hasError`, `canEdit`, `isOpen`

## TODO — только с номером issue

- Прежде чем написать TODO — создать issue в GitHub
- Формат: `TODO(#N): описание`
- TODO без номера issue запрещены
- При закрытии issue — удалить TODO из кода

## Комментарии

- На английском (код и комментарии на одном языке)
- Не комментировать очевидное — комментировать **почему** принято решение, а не **что** делает код
- Godoc-комментарии для экспортированных функций/типов обязательны (backend)

## Что ревьюить

- [major] Файл `.go` не snake_case.
- [major] Receiver — full name типа (вместо сокращённого `s` / `r` / `h`).
- [major] Интерфейс без суффикса слоя (`Brand` вместо `BrandService`).
- [major] TODO без `(#issue-N)`.
- [major] Комментарий объясняет ЧТО, а не ПОЧЕМУ.
- [minor] Компонент-файл не PascalCase (`brandList.tsx` вместо `BrandList.tsx`).
- [minor] Хук без префикса `use`.
- [minor] Event handler не `handle{Action}` / callback prop не `on{Action}`.
- [minor] Булева переменная / prop без префикса `is` / `has` / `can`.
