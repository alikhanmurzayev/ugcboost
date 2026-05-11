package telegram

// User-facing reply texts for the synchronous handler path. Russian, official
// tone, no emojis. Tests assert against these constants verbatim, so any copy
// change is a deliberate code change. Async post-commit notifications live in
// notifier.go alongside their HTML-formatted templates.
const (
	// MessageFallback covers no-payload, malformed payload and any unknown
	// command — the actionable next step is the same: submit at the website.
	MessageFallback = "Если есть вопросы, можете обратиться к @aizerealzair"

	// MessageApplicationNotFound is sent when /start carries a syntactically
	// valid UUID but no application exists with that id.
	MessageApplicationNotFound = "Заявка не найдена. Подайте новую на ugcboost.kz"

	// MessageApplicationAlreadyLinked is sent when the application is bound to
	// a different Telegram account than the one issuing /start.
	MessageApplicationAlreadyLinked = "Эта заявка уже связана с другим Telegram. " +
		"Если это ошибка — обратитесь в поддержку"

	// MessageInternalError covers unexpected failures (DB, network) so the
	// user does not face silence.
	MessageInternalError = "Произошла внутренняя ошибка. Попробуйте позже"
)
