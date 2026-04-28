# Слои архитектуры

Бэкенд организован по слоям: **handler → service → repository**. Каждый слой имеет чёткую ответственность. Нарушение границ приводит к дублированию логики, невозможности тестирования и цепным багам.

## Направление зависимостей

```
handler  →  service  →  repository  →  DB
  (HTTP)     (бизнес)     (SQL/data)
```

- handler зависит от service interfaces — **никогда** от repository напрямую
- service зависит от repository interfaces — **никогда** от handler
- repository зависит от DB interface — **никогда** от service или handler
- Интерфейсы определяются в пакете-потребителе (Go convention: accept interfaces, return structs)

## Ответственности слоёв

- **Handler** — точка входа запроса. HTTP-парсинг выполняется кодогенерацией (ServerInterfaceWrapper), хендлер получает уже типизированные данные. Хендлер выполняет всю валидацию, которую можно выполнить, имея только данные запроса: формат, обязательность, очевидные бизнес-ограничения (дата не в прошлом, значение в допустимом диапазоне и т.п.). До сервиса доходят только провалидированные данные, со всеми обязательными полями domain-input, заполненными — включая middleware-derived (IP, UserAgent, AgreementVersion из `context.WithValue`)
- **Service** — бизнес-логика, которая требует данных из БД или других сервисов для принятия решения (уникальность, лимиты, зависимости между сущностями). Дублирование валидации из хендлера запрещено
- **Repository** — единственный слой, который строит SQL-запросы и обращается к БД. Прямой SQL вне repository запрещён

## Service helpers — методы vs free functions

Helper, привязанный к state сервиса (`s.repoFactory`, `s.config`, `s.logger`), — **метод** `*XService` (lowercase, приватный). Helper, чисто generic (не зависит от state), реально переиспользуемый между сервисами — **package-level free function**. Проброс `repo` параметром в free function — finding: значит helper привязан к state и должен быть методом.

## Транзакции

Границы транзакций определяет service layer — он знает, какие операции должны быть атомарными. Handler не знает о транзакциях. Repository прозрачен к pool и tx — работает с `dbutil.DB`, не различая их. Детали — в стандарте backend-transactions.

## Авторизация

Вся авторизационная логика — в отдельном сервисе (AuthzService). Прямые сравнения ролей в хендлерах запрещены. Хендлер вызывает один метод авторизации — не размазывает проверки по бизнес-логике.

## Что ревьюить

- [blocker] Прямой SQL вне `repository/` (в handler или service).
- [blocker] Прямые role-сравнения в handler (вместо AuthzService).
- [major] handler не валидирует формат / обязательность / тривиальные границы — невалидированные данные доходят до service.
- [major] handler не заполняет обязательные поля domain-input из middleware-context (IP, UserAgent, AgreementVersion).
- [major] service дублирует валидацию handler (которую можно сделать без обращения к БД).
- [major] Helpers сервиса, привязанные к state (`s.repoFactory`, `s.config`, `s.logger`), оставлены package-level free functions — должны быть методами `*XService`.
- [major] Helper-free-function принимает `repo` параметром — значит привязан к state, должен быть методом.
- [minor] Generic утилиты (`trim`, `parseDate`) сделаны методами сервиса — должны быть package-level.
