# Backend E2E тесты [REQUIRED]

Чёрный ящик. Отдельный Go module — физически не может импортировать internal. HTTP-клиент сгенерирован из OpenAPI.

## Нейминг

По бизнес-сценарию: `TestLogin`, `TestBrandCRUD`, `TestAuditLogFiltering`. Одна функция покрывает все edge cases эндпоинта, сценарии внутри через `t.Run`.

## Структура файлов

По доменам в подпапках:

```
e2e/
├── testutil/          # экспортированные хелперы
│   ├── seed.go        # setup хелперы
│   ├── cleanup.go     # cleanup stack, env-managed
│   └── client.go      # API client setup, retry, auth
├── auth/
│   └── auth_test.go
├── brand/
│   └── brand_test.go
└── campaign/
    └── campaign_test.go
```

## Создание тестовых данных

Максимально через реальные бизнес-ручки. Тестовое API (`/test/*`) — только когда нет бизнес-flow (создание admin, получение raw токенов).

Composable хелперы в `testutil/`. Принцип — конструктор, из которого можно собрать любой набор данных:

```go
// Примеры для понимания паттерна, не конкретные функции
adminToken := testutil.SetupAdmin(t)
brandID := testutil.SetupBrand(t, adminToken, ...)
managerToken := testutil.SetupManagerWithLogin(t, adminToken, brandID, ...)
```

Хелперы composable — могут делать HTTP-запросы, вызывать другие хелперы, комбинировать шаги. `require` внутри — упал setup = весь тест упал.

## HTTP-клиент

Сгенерированный из OpenAPI с retry на транспортном уровне:
- **Retry**: connection refused, timeout, DNS resolution, 502/503/504
- **Не retry**: 4xx (валидация, forbidden, not found), 500, любой ответ от приложения

Retry только когда запрос не дошёл до приложения или приложение было временно недоступно.

## Cleanup

Defer-based cleanup stack с управлением через env var `E2E_CLEANUP` (по умолчанию `true`):
- `E2E_CLEANUP=true` — данные удаляются после каждого теста в обратном порядке (LIFO, уважает foreign keys)
- `E2E_CLEANUP=false` — данные остаются для анализа состояния БД при дебаге упавших тестов

Удаление — через бизнес-ручки приоритетно. Если бизнес-ручки удаления нет — тестовая ручка (`/test/*`): принимает тип сущности (enum) + идентификатор. Тестовые ручки доступны только в тестовых средах и локально, не в проде.

## Изоляция

- `t.Parallel()` на всех `Test*` функциях
- Каждый тест создаёт полный набор данных с нуля
- Уникальность через generated emails/names — тесты идемпотентны, работают на любой БД с любыми данными
- Тест проходит и с чистой БД, и с накопленными данными от предыдущих прогонов

## Assertions

- `require` везде
- Сгенерированный клиент сам выполняет marshal/unmarshal — на выходе типизированная структура
- Динамические поля (UUID, время) — проверить отдельно, подменить, затем `require.Equal` целиком
- HTTP status code + полный response body + заголовки/cookies где релевантно

## Проверка ошибок

Строгая проверка HTTP error responses — status code + error code в body + message.

## Сценарии

Все edge cases каждого endpoint: success, validation errors, forbidden, not found, business rule violations. Покрываем полностью.

## Комментарий в начале файла

Обязателен. Полное описание: что тестируется, каждый шаг, какие данные создаются, что ожидаем.
