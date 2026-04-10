# Библиотеки вместо велосипедов [REQUIRED]

Go-экосистема богата проверенными библиотеками. Писать свой инфраструктурный код — значит брать на себя ответственность за edge cases, которые уже решены в библиотеках с тысячами звёзд и годами production-использования.

---

## CS-12: Конфигурация — использовать проверенную библиотеку

**Scope:** backend

**Почему:** Кастомные хелперы `envOrDefault`, `envInt` (`config/config.go:84+`) — велосипед без валидации типов, без вложенных структур, без автодокументации, без поддержки файлов `.env`.

**Плохо** (internal/config/config.go):
```go
func envOrDefault(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}

func envInt(key string, fallback int) int {
    v := os.Getenv(key)
    if v == "" {
        return fallback
    }
    i, err := strconv.Atoi(v)
    if err != nil {
        return fallback  // ошибка парсинга молча игнорируется!
    }
    return i
}
```

**Хорошо:** использовать одну из проверенных библиотек:
- [caarlos0/env](https://github.com/caarlos0/env) (~5k звёзд) — struct tags + автопарсинг
- [kelseyhightower/envconfig](https://github.com/kelseyhightower/envconfig) (~5k звёзд) — аналогично
- [knadh/koanf](https://github.com/knadh/koanf) (~3k звёзд) — если нужны файлы конфигов

```go
type Config struct {
    Port        int           `env:"PORT" envDefault:"8080"`
    DatabaseDSN string        `env:"DATABASE_DSN,required"`
    JWTSecret   string        `env:"JWT_SECRET,required"`
    JWTExpiry   time.Duration `env:"JWT_EXPIRY" envDefault:"15m"`
    LogLevel    LogLevel      `env:"LOG_LEVEL" envDefault:"info"`
    Environment Environment   `env:"ENVIRONMENT" envDefault:"local"`
    BcryptCost  int           `env:"BCRYPT_COST" envDefault:"12"`
}

cfg := Config{}
if err := env.Parse(&cfg); err != nil {
    log.Fatal(err) // невалидный конфиг = не стартуем
}
```

**Правило:** конфигурация загружается через проверенную библиотеку со struct tags. Кастомные хелперы для env vars запрещены.

---

## CS-13: Graceful shutdown / lifecycle — использовать проверенную библиотеку

**Scope:** backend

**Почему:** `internal/closer/closer.go` — самописный lifecycle manager. Не покрывает: таймауты на отдельные close-операции, логирование каждого шага, graceful drain HTTP-соединений.

**Плохо:**
```go
// internal/closer/closer.go — 56 строк кастомного кода
type Closer struct {
    mu    sync.Mutex
    funcs []func() error
}
```

**Хорошо:** использовать одну из:
- [oklog/run](https://github.com/oklog/run) (~4k звёзд) — простой actor model
- [sourcegraph/conc](https://github.com/sourcegraph/conc) (~9k звёзд) — structured concurrency
- stdlib `errgroup` + signal handling — минимальный вариант

**Правило:** lifecycle и graceful shutdown через проверенную библиотеку или stdlib errgroup. Самописный closer удалить.

---

## CS-14: Перед написанием инфраструктурного кода — проверить Awesome Go

**Scope:** backend

**Почему:** Каждая строка кастомного инфраструктурного кода — это строка, которую надо поддерживать, тестировать и дебажить. Библиотека с 1000+ звёзд уже прошла через это.

**Правило:** если задача **инфраструктурная** (не бизнес-логика), перед написанием кода:

1. Проверить [Awesome Go](https://github.com/avelino/awesome-go) — есть ли готовое решение
2. Сравнить топ-3 библиотеки по звёздам, свежести последнего коммита, наличию issues
3. Выбрать библиотеку с ≥1000 звёзд и активной поддержкой
4. Если подходящей библиотеки нет — написать свой код, но задокументировать почему

**Бизнес-логика** (хендлеры, сервисы, доменные правила) — пишется самостоятельно. Инфраструктура (config, logging, lifecycle, HTTP middleware, SQL utilities) — берётся из библиотек.
