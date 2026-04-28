# Обработка ошибок

Игнорирование ошибок — одна из самых опасных практик. Молчаливый баг хуже громкого — его находят в проде, когда данные уже потеряны.

## Принципы

- **Каждая ошибка** либо возвращается вызывающему коду, либо логируется с контекстом. `_ = someFunc()` запрещено
- **Невалидный ввод = ошибка**, не тихий fallback. Передали `page=-5` → 422, а не молчаливый `page=1`. Значения по умолчанию допустимы только когда параметр не передан, а не когда передан невалидно
- **Сервисы возвращают ошибки**. Решение о том, как обрабатывать ошибку (откатить, залогировать, продолжить) — принимает вызывающий код, не сервис
- **Транзакции** — сервис не управляет lifecycle транзакции (begin, commit, rollback). Всё инкапсулировано в `dbutil.WithTx` — callback просто возвращает свои бизнес-ошибки. Детали — в стандарте backend-transactions

## Коды и user-facing message

- **Granular error codes.** Класс ошибки с конкретным значением для пользователя — отдельный код (`CodeInvalidIIN`, `CodeUnderAge`, `CodeIINTaken`), не generic `CodeValidation`. Это позволяет фронту показывать заранее подготовленный текст и не делать раундтрипов на расшифровку.
- **Actionable `error.Message`.** Текст user-facing ошибки даёт пользователю понять что делать. Тупиковые ("Already exists", "Invalid input") — finding `major`. Пример хорошего: «Заявка по этому ИИН уже на рассмотрении. Дождитесь решения модератора или, если будет отклонена, подайте новую».
- **Required string-поля проходят trim + non-empty проверку** в handler. `"   "` для required → 422, не silent acceptance.

## Что ревьюить

- [blocker] `_ = someFunc()` (игнор ошибки).
- [blocker] Молчаливый fallback на default при невалидном вводе (`page=-5` → молча `page=1` вместо 422).
- [major] Сервис управляет lifecycle транзакции напрямую (`pool.Begin()`, `tx.Commit/Rollback`) — должен через `dbutil.WithTx`.
- [major] Ошибка пробрасывается без контекста (`return err` вместо `fmt.Errorf("xCreate: %w", err)`).
- [major] Sentinel-ошибка не обёрнута в `domain.NewValidationError` / `domain.NewBusinessError` где нужно user-facing.
- [major] Generic `CodeValidation` вместо granular кода (`CodeInvalidIIN`, `CodeUnderAge`) для известного класса валидационных ошибок.
- [major] `error.Message` тупиковый ("Already exists") — без actionable hint что делать пользователю.
- [major] Required string-поле не проходит trim + non-empty проверку (whitespace-only acceptable).