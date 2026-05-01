# Deferred Work

Findings из ревью, не относящиеся к текущей сторе, но реальные. Раскручивать отдельными тикетами по мере приоритезации.

## 2026-05-01 — InsertMany категорий не транслирует SQLSTATE 23503

**Источник:** edge-case-hunter review для PR `gh-25-dictionary-code-pk` (`spec-gh-25-dictionary-code-pk.md`).

**Файл:** `backend/internal/repository/creator_application_category.go:46-58` (`InsertMany`).

**Суть:** между service-уровневой проверкой `resolveCategoryCodes` (SELECT внутри tx) и `InsertMany` (INSERT в той же tx) под READ COMMITTED остаётся узкое окно. Если другой админ DELETE'нет категорию между двумя операциями, INSERT получит SQLSTATE 23503 (`creator_application_categories_category_code_fkey`) — repo возвращает raw ошибку, handler выдаёт 500.

Аналогичный gap существовал и до миграции `gh-25` (FK был на `categories(id)` с тем же поведением), просто стал заметнее после переноса конвенции на code.

**Что сделать:** в `InsertMany` ловить `*pgconn.PgError` с `Code == "23503"` и `ConstraintName == "creator_application_categories_category_code_fkey"`, транслировать в `domain.NewValidationError(domain.CodeUnknownCategory, …)` или sentinel `domain.ErrUnknownCategory`. По симметрии — рассмотреть аналогичный guard в `creator_application.go` Create на FK `creator_applications_city_code_fkey` (на случай race между `resolveCityCode` и Create).

**Severity:** major (race редкий, surface = 500 вместо 422).
