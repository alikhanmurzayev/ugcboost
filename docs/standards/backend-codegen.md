# Использование кодогенерации

Contract-first подход: OpenAPI YAML → кодогенерация для Go (oapi-codegen), TypeScript (openapi-typescript), E2E-клиентов. Сгенерированный код **должен использоваться**. Ручные дубликаты запрещены.

## Принцип

Если что-то определено в OpenAPI-контракте и сгенерировано — используй сгенерированное. Ручные структуры, роуты, парсинг параметров, интерфейсы для API — всё это создаёт второй источник правды, который неизбежно рассинхронизируется с контрактом.

## Backend

- **Роутинг** — только через `api.HandlerFromMux()`. Ручная регистрация `r.Get/Post/Route` запрещена для API-эндпоинтов (исключение: health check)
- **Типы запросов/ответов** — только сгенерированные типы из `api/`. Анонимные структуры в хендлерах запрещены
- **Query/path параметры** — парсятся автоматически через ServerInterfaceWrapper. Ручной `r.URL.Query().Get()` и `chi.URLParam()` запрещены для API-эндпоинтов
- **Моки** — mockery с `all: true` для автообнаружения интерфейсов. Ручные моки запрещены
- **Strict-server Set-Cookie — одно значение на ответ.** Сгенерированный wrapper пишет header через `w.Header().Set("Set-Cookie", value)` — повторный `Set` тихо перетрёт предыдущий. Если эндпоинту нужно вернуть две cookie сразу (CSRF + refresh) — strict-server не подойдёт; нужен либо отдельный middleware, дописывающий cookie через `w.Header().Add(...)` после strict-handler'а, либо chi-handler в обход generated wrapper'а

## Frontend

- **API-типы** — только из `generated/schema.ts`. Ручные interface/type для API request/response запрещены

## Что ревьюить

- [blocker] Ручной struct для API request/response в handler (вместо типов из `api/`).
- [blocker] Ручной `json.NewDecoder(r.Body).Decode` в handler (вместо ServerInterfaceWrapper / strict-server).
- [major] Ручной `r.URL.Query().Get(...)` / `chi.URLParam(...)` для API-эндпоинта (должно идти через сгенерированные параметры).
- [major] Хардкод-список enum-значений в switch / error message вместо `req.X.Valid()` от сгенерированного типа.
- [minor] Ручной мок вместо mockery с `all: true`.
- [blocker] Generated файл (`*.gen.go`, `frontend/*/generated/*` и др.) изменён в diff'е без правки yaml-источника (`api/openapi.yaml`) — нарушение codegen pipeline.
- [major] Strict-server response отдаёт две Set-Cookie через `Headers` — generated wrapper делает `w.Header().Set(...)`, второе значение перетрёт первое; нужен workaround через middleware с `Add(...)`.
