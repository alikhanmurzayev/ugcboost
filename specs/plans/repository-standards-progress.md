# Прогресс: Приведение репозитория к стандартам

## Выполнено
- [x] Шаг 1: Добавить константы в user.go (RefreshToken: ID, CreatedAt; ResetToken: ID, CreatedAt)
- [x] Шаг 2: Добавить константы в brand.go (BrandManager: ID, CreatedAt)
- [x] Шаг 3: Заменить 6 строковых литералов на константы в brand.go (List, ListByUser, ListManagers)
- [x] Шаг 4: Переписать UserRepository.Create на SetMap
- [x] Шаг 5: Создать helpers_test.go, перенести captureQuery/captureExec, добавить scalarRows
- [x] Шаг 6: Добавить TestBrandRepository_GetBrandIDsForUser
- [x] Шаг 7: Добавить TestAuditRepository_List (3 сценария: count no filters, count all filters, data with pagination)
- [x] Шаг 8: build-backend — компиляция без ошибок
- [x] Шаг 9: test-unit-backend — все тесты зелёные
- [x] Шаг 10: test-e2e-backend — все E2E зелёные

## Блокеры
Нет

## Заметки
- Семантическая ошибка в ListManagers исправлена: BrandColumnCreatedAt → BrandManagerColumnCreatedAt
- SetMap не изменил SQL — тесты прошли без модификации
- scalarRows реализует pgx.Rows для тестирования двух-запросных методов
