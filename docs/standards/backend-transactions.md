# Транзакции [REQUIRED]

## Принцип

Если сервисный метод выполняет 2+ операции записи, которые должны выполниться атомарно — они оборачиваются в транзакцию. Без транзакции частичный сбой оставляет данные в несогласованном состоянии: пользователь создан, но не привязан к бренду; пароль сменён, но старые сессии живы.

Одна операция записи не требует транзакции — Postgres гарантирует атомарность отдельного запроса.

## Владение транзакциями

- **Service** — единственный слой, который открывает и управляет транзакциями. Бизнес-логика определяет, какие операции должны быть атомарными
- **Handler** — не знает о транзакциях. Не вызывает `Begin()`, не передаёт tx в сервис
- **Repository** — не знает, выполняется ли он внутри транзакции или нет. Работает с `dbutil.DB`, который прозрачен к pool и tx. Никогда не вызывает `Begin()`

## dbutil.WithTx

`dbutil.WithTx` — единственный способ начать транзакцию. Прямой вызов `pool.Begin()` в сервисах запрещён.

```go
err := dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
    // tx — тот же dbutil.DB, что и pool, но привязан к транзакции
    // все операции через repos, созданные с этим tx
    return nil // → commit
    // return err → rollback
})
```

Сервис не управляет lifecycle транзакции. Callback просто возвращает свои обычные бизнес-ошибки. Всё остальное (begin, commit, rollback, `errors.Join` при двойном сбое) — ответственность хелпера, реализовано один раз.

## Repository

### Структура repo

Repo — приватная структура, хранит `db dbutil.DB`. Конструктора на уровне пакета нет — создание через `RepoFactory`.

```go
type entityRepository struct {
    db dbutil.DB
}

func (r *entityRepository) DoSomething(ctx context.Context, ...) (..., error) { ... }
```

### Интерфейс repo

Рядом с repo лежит экспортируемый интерфейс — перечисляет все публичные методы:

```go
type EntityRepo interface {
    DoSomething(ctx context.Context, ...) (..., error)
    // ...все публичные методы repo
}
```

### RepoFactory

`RepoFactory` — экспортируемая stateless структура в пакете `repository`. Без полей, без хранения соединения. Каждый метод принимает `dbutil.DB` и возвращает интерфейс repo:

```go
type RepoFactory struct{}

func NewRepoFactory() *RepoFactory { return &RepoFactory{} }

func (f *RepoFactory) NewEntityRepo(db dbutil.DB) EntityRepo {
    return &entityRepository{db: db}
}
```

### Интерфейс RepoFactory в сервисе

Каждый сервис объявляет свой интерфейс `RepoFactory` с только нужными ему конструкторами (Go convention: accept interfaces, return structs):

```go
type RepoFactory interface {
    NewEntityARepo(db dbutil.DB) repository.EntityARepo
    NewEntityBRepo(db dbutil.DB) repository.EntityBRepo
    // ...только те repos, которые нужны этому сервису
}

type SomeService struct {
    pool        dbutil.TxStarter
    repoFactory RepoFactory
}
```

## Использование в сервисе

Сервис хранит `pool` (для начала транзакций) и `repoFactory` (для создания repos). Внутри `WithTx` сервис передаёт `tx` в фабрику и получает repos, привязанные к транзакции:

```go
func (s *SomeService) SomeOperation(ctx context.Context, ...) error {
    return dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
        entityARepo := s.repoFactory.NewEntityARepo(tx)
        entityBRepo := s.repoFactory.NewEntityBRepo(tx)

        // все операции через repos на одной tx
        return nil
    })
}
```

Вне транзакции сервис передаёт `pool` напрямую:

```go
func (s *SomeService) ReadOnly(ctx context.Context, ...) (..., error) {
    entityARepo := s.repoFactory.NewEntityARepo(s.pool)
    return entityARepo.GetSomething(ctx, ...)
}
```

## Аудит-лог

Аудит-лог **обязательно** пишется внутри той же транзакции, что и изменение данных. Если изменение откатилось — аудит-лог откатится тоже. Если изменение закоммитилось — аудит-лог гарантированно существует. Fire-and-forget запись аудита запрещена.

```go
return dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
    entityRepo := s.repoFactory.NewEntityRepo(tx)
    auditRepo := s.repoFactory.NewAuditRepo(tx)

    // изменение данных
    // ...

    // аудит-лог в той же транзакции
    return auditRepo.Create(ctx, ...)
})
```

## Ограничения

- **Nested transactions (savepoints)** — запрещены. Если два сервиса нуждаются в общей транзакции — рефакторинг в один orchestrating сервис
