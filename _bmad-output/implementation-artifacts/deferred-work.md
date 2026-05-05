# Deferred work

Issues surfaced during reviews but not fixed in the originating PR. Each
entry: short description, severity, source, and a one-liner on why it was
deferred (usually: pre-existing pattern outside scope, or cross-cutting
concern that needs its own focused fix).

---

## Frontend confirm-dialog pattern (reject + approve)

These all live in the shared confirm-dialog template: introduced in the
reject-flow (chunk 14) and mirrored into approve (chunk 19). Fixing one
should fix both — they're listed once.

- **[major] Escape-handler use of `e.stopImmediatePropagation()` блокирует
  Escape глобально пока dialog открыт.** Покрывает реальное требование (не
  закрывать drawer на Escape), но снимает Escape-listener'ы из других
  popover'ов / nested modals на той же странице. Желательно: вешать
  listener только на сам dialog node либо проверять `e.target` перед
  `stopImmediatePropagation`. Сейчас на этих экранах нет конкурирующих
  Escape-handler'ов, так что баг латентный.
  Source: blind-hunter / edge-case-hunter в chunk 19.

- **[major] Отсутствие ключей `common.errors.UNAUTHORIZED` / `429` / `408`.**
  При истёкшей сессии ApiError(401, "UNAUTHORIZED") пробрасывается через
  `getErrorMessage` → fallback "Произошла ошибка". То же для 429
  (rate-limit) и 408 (request-timeout). Нужно либо добавить ключи и
  actionable hint'ы (для UNAUTHORIZED — «Сессия истекла, войдите заново»),
  либо contract-тест который перебирает все ApiError-коды и проверяет
  наличие переводов.
  Source: edge-case-hunter в chunk 19. **[чеклист-кандидат]** для
  `frontend-api.md` — каждый user-facing ApiError-code обязан иметь явный
  перевод.

- **[minor] `onSettled` setState после unmount.** `mutation.onSuccess`
  вызывает `onCloseDrawer()` → drawer размонтирует dialog → `setIsSubmitting(false)`
  в `onSettled` стреляет на unmounted компонент. React 18 это no-op без
  warning'а, но любая перетасовка callback-порядка превратит в setState
  warning. Желательно: cleanup-flag через `useRef` или пропустить
  `setIsSubmitting` если `mutation.isSuccess`.
  Source: edge-case-hunter в chunk 19.

- **[minor] Backdrop как `<button>` с `aria-label="cancel"` — accessibility
  ложь.** Screen reader увидит «button: Отмена» рядом с настоящим cancel —
  дубликат tabstop'а. Желательно: `<div role="presentation" onClick={...}>`
  + `aria-hidden="true"`, или вынести в shared `<DialogBackdrop />` с
  правильной семантикой.
  Source: blind-hunter в chunk 19.

- **[minor] Mock `ApiError` class drift.** Тесты определяют локальный
  `class ApiError` через `vi.mock('@/api/client', ...)` — если в
  production-коде класс получит новые поля, тесты не отловят регрессию.
  Желательно: `vi.importActual('@/api/client')` и реэкспорт настоящего
  класса.
  Source: blind-hunter в chunk 19.

- **[minor] Отсутствие `I18nextProvider` в unit-тестах диалогов.** Тесты
  работают потому что i18next возвращает ключ как fallback при отсутствии
  bundle (или подцепляется глобальная init'ация из соседнего файла). Не
  даёт гарантию что юзер видит правильный текст.
  Source: acceptance-auditor в chunk 19. Стандарт `frontend-testing-unit.md`
  уже требует `I18nextProvider` — это формально нарушение [major], но
  унаследовано от reject и затрагивает все confirm-dialog тесты, поэтому
  фиксится одним touch'ем во всех файлах сразу.

- **[minor] `w-[420px]` без min-width safety.** На вьюпортах <420px
  `max-w-full` спасает, но padding снаружи + p-5 внутри + жёсткий 420 могут
  давать overflow. Не критично для админки (десктоп-only по факту).
  Source: blind-hunter в chunk 19.

- **[nitpick] Стилистика точек в новых i18n error messages.** Утверждённые
  user'ом тексты `CREATOR_APPLICATION_NOT_APPROVABLE` /
  `CREATOR_ALREADY_EXISTS` / `CREATOR_TELEGRAM_ALREADY_TAKEN`
  заканчиваются точкой, тогда как все остальные записи в `errors.*` без
  точки. Если будем унифицировать стиль — обсудить с user'ом и удалить
  точки одним коммитом.
  Source: edge-case-hunter в chunk 19.
