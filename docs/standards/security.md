# Безопасность [CRITICAL]

Secure by default, не secure by configuration.

## Environment — явный enum

Окружение задаётся через `ENVIRONMENT` env var (`local`, `staging`, `production`). Поведение, зависящее от окружения (secure cookies, test endpoints, debug logging), определяется от этого enum явно. Никаких `insecure`, `disable_security`, `skip_auth` флагов. Production — всегда максимально secure, без возможности ослабить через конфиг.

## Логирование — без чувствительных данных

**Запрещено логировать:** request/response body, заголовок Authorization, cookies с токенами, персональные данные (ИИН, телефон, email в связке с другими данными), секреты.

**Допустимо:** HTTP method/path/status/duration, user ID (UUID), request/trace ID, IP-адрес, error messages без sensitive context.

## Секреты — только в env vars

- Все секреты — через env vars. Хардкод секретов в коде запрещён
- `.env` файлы — только для локальной разработки, в `.gitignore`
- CI/CD — через GitHub Secrets
- Production — через Dokploy env vars
- При обнаружении секрета в коде — немедленно ротировать и исправить
