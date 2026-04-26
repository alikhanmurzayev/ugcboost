# Unit-тесты бэкенда [REQUIRED]

## Нейминг

`Test{Struct}_{Method}` — одна функция на метод. Сценарии внутри через `t.Run`.

```go
// Порядок t.Run повторяет порядок исполнения кода в методе:
// сначала ранние выходы (ошибки, валидация), в конце — happy path
func TestAuthService_Login(t *testing.T) {
    t.Parallel()
    t.Run("empty email", func(t *testing.T) { ... })         // валидация входных данных
    t.Run("user not found", func(t *testing.T) { ... })      // первый вызов зависимости — repo.GetByEmail
    t.Run("wrong password", func(t *testing.T) { ... })      // следующая проверка — bcrypt.Compare
    t.Run("token generation error", func(t *testing.T) { ... }) // ошибка генерации токена
    t.Run("success", func(t *testing.T) { ... })              // все проверки пройдены
}
```

## Сценарии

Комбо подход:
- Table-driven — для однотипных сценариев (валидация разных полей)
- Отдельные `t.Run` — для сценариев со сложным setup

Стремимся к высокому покрытию — каждая ветка кода покрыта.

## Изоляция

- `t.Parallel()` в каждом тесте и каждом `t.Run`
- Новый мок на каждый `t.Run` — `mock := NewMockService(t)` внутри сценария, без протекания между тестами
- Данные создаются inline в каждом `t.Run`, без общих фабрик. Дублирование допустимо ради изоляции

## Моки

- mockery + testify. Ручные моки запрещены
- Проверять точные аргументы каждого вызова — `mock.EXPECT().Create(mock.AnythingOfType("*context.valueCtx"), "exact-name", "exact-logo").Return(...)`
- Для сложных проверок аргументов — `.Run(func(args mock.Arguments) { ... })`: достать аргумент, привести к типу напрямую (без проверки ok — паника в корректном тесте невозможна), проверить через `require`

## Assertions

- `require` везде — первый провал останавливает тест
- Динамические поля (UUID, время и т.п.) — сначала проверить отдельно (UUID не пустой, время через `require.WithinDuration` с запасом ~1 мин), затем подменить на ожидаемые значения, затем `require.Equal` всей структуры целиком
- Этот паттерн применяется везде: результаты вызовов сервисов, аргументы моков, HTTP response
- JSON-поля (`json.RawMessage`, `[]byte` с сериализованным JSON) сравниваются через `require.JSONEq` — порядок ключей в `json.Marshal(map[...]...)` не детерминирован, `require.Equal` на сырые байты создаст флаки. При сравнении содержащей структуры целиком: сначала `JSONEq` на JSON-поле, затем обнулить его перед `require.Equal` на структуру

## Проверка ошибок

Строгая проверка:
- Кастомный тип ошибки → `require.ErrorAs` для проверки типа
- Обёртка ошибки с контекстом → `require.ErrorContains` для проверки текста обёртки
- Конкретная ошибка → `require.ErrorIs`

Если в коде добавлен контекст в ошибку — тест обязан это проверить.

## По слоям

### Repository
- Мок: MockDB (pgx interface)
- SQL assertion: проверяем точную SQL-строку + аргументы через хелперы (`captureQuery`, `captureExec`). Хелперы перехватывают SQL и аргументы через `mock.Run()` callback
- Строковые литералы в SQL намеренно — двойная проверка правильности констант (подробнее в стандарте констант)
- Проверяем маппинг row → struct: мокаем `rows.Scan()`, убеждаемся что поля маппятся корректно
- Проверяем error propagation: ошибки БД корректно оборачиваются с контекстом и пробрасываются наверх

### Service
- Моки на все зависимости (repository interfaces, token generator и т.д.)
- Новый мок на каждый `t.Run`
- Точные аргументы в каждом mock expectation
- Бизнес-логика тестируется изолированно от HTTP и БД: на вход — данные, на выход — результат или ошибка

### Handler
- Чёрный ящик через HTTP. Моки: service interfaces. Handler не знает про repository и БД
- Запрос: формируем тело как сгенерированную структуру из `api/` → `json.Marshal` → `httptest.NewRequest` → прогоняем через роутер с ServerInterfaceWrapper
- Ответ: `httptest.ResponseRecorder` → `json.Unmarshal` в сгенерированную структуру из `api/` → проверка динамических полей → `require.Equal` целиком (см. Assertions)
- Сырой JSON в тестах запрещён — request body и response body через типизированные структуры кодогенерации. Query params — строкой в URL
- HTTP status code проверяется всегда
- Заголовки и cookies проверяются где релевантно (например, auth flow — refresh token cookie)

### Middleware
- Формируем handler chain: `middleware(okHandler)`, где `okHandler` — простой handler возвращающий 200
- Проверяем response: HTTP status code, заголовки (CORS, security headers и т.п.)
- Проверяем context: через next handler убеждаемся что middleware корректно прокидывает значения в context (user ID, role и т.п.)
- Мок: MockTokenValidator для auth middleware

## Хелперы

Общие хелперы для тестов пакета — в `helpers_test.go`. Каждый пакет имеет свой `helpers_test.go`.

## Coverage

Целевой порог — 80% **на каждой публичной и приватной функции/методе** в покрываемых пакетах (handler/service/repository/middleware/authz). Gate в `make test-unit-backend-coverage` падает, если хотя бы один identifier ниже 80%. Исключения по файлам: generated code (`*.gen.go`), mockery-моки (`*/mocks/`), `cmd/`, trivial bootstrap (`handler/health.go`, `middleware/logging.go`, `middleware/json.go`).

## Race detector

`go test -race` включён во всех make-таргетах и в CI. Отключение запрещено. Если тест не проходит с `-race` — это баг в коде, не в тесте.

## Комментарии

Общий комментарий в начале файла не нужен — unit-тесты выразительны сами по себе. Точечные комментарии допустимы для неочевидной логики или нестандартных трюков.
