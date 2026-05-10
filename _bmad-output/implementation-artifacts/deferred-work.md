# Deferred work

Накопительный список находок, которые review-цикл собрал, но они не блокируют
текущий слайс. Каждая запись — это либо pre-existing проблема, surfaced
incidentally, либо follow-up к завершённой фиче, требующий отдельного решения.

## Из ревью UTM-меток (2026-05-10)

### PII-policy для UTM-полей
- **Severity**: major
- **Контекст**: спека и комментарий `auditNewValue` декларируют UTM как «non-PII
  tracking metadata», но никакой backend-валидации формата нет. Атакующий или
  маркетолог может вложить телефон / email / IIN в `utm_term` через прямой
  API-вызов — значение попадёт в БД и в `audit_logs.new_value`.
- **Что нужно решить**: либо добавить charset whitelist в OpenAPI / handler
  (`^[A-Za-z0-9_.\-]+$` — стандартный UTM формат), либо обновить
  `docs/standards/security.md` записью «UTM-поля считаются user-controlled и
  могут содержать произвольные данные до cap'а».
- **Why deferred**: требует policy-decision на уровне продукта (Aidana) и
  потенциального обновления стандарта — это не зона текущего dev-слайса.

### Дублирующиеся UTM-ключи в URL не ассертятся
- **Severity**: minor
- **Контекст**: `?utm_source=A&utm_source=B` → `URLSearchParams.get` берёт
  первое значение. Поведение разумное, но не покрыто тестом и не
  документировано.
- **Что нужно**: добавить кейс в `frontend/landing/src/lib/utm.test.ts` либо
  явный `getAll().length > 1` warning в `captureUTM()`.
- **Why deferred**: маркетинг сейчас не генерирует ссылки с дублями, риск
  низкий.

### `noUncheckedIndexedAccess` отключён в landing tsconfig
- **Severity**: minor
- **Контекст**: `frontend/landing/tsconfig.json` extends `astro/tsconfigs/strict`,
  который не включает `noUncheckedIndexedAccess` (web/tma его включают). По
  стандарту `frontend-quality.md` флаг обязателен.
- **Что нужно**: добавить `"compilerOptions": {"noUncheckedIndexedAccess": true}`
  в `frontend/landing/tsconfig.json` и пройтись по landing-исходникам, починив
  возникшие type errors.
- **Why deferred**: правка cross-cutting, может вскрыть несвязанные с UTM
  места — отдельный slice.
