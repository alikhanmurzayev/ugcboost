package telegram

// User-facing reply texts. Russian, official tone, no emojis.
// Tests assert against these constants verbatim, so any copy change is a
// deliberate code change.
const (
	// MessageFallback covers no-payload, malformed payload and any unknown
	// command — the actionable next step is the same: submit at the website.
	MessageFallback = "Здравствуйте! Чтобы продолжить, подайте заявку на ugcboost.kz"

	// MessageApplicationNotFound is sent when /start carries a syntactically
	// valid UUID but no application exists with that id.
	MessageApplicationNotFound = "Заявка не найдена. Подайте новую на ugcboost.kz"

	// MessageLinkSuccess is sent on a fresh link AND on an idempotent repeat
	// from the same Telegram account.
	MessageLinkSuccess = "Здравствуйте! Заявка успешно связана с вашим Telegram. " +
		"В ближайшее время в этом чате откроется мини-приложение со статусом обработки заявки"

	// MessageApplicationAlreadyLinked is sent when the application is bound to
	// a different Telegram account than the one issuing /start.
	MessageApplicationAlreadyLinked = "Эта заявка уже связана с другим Telegram. " +
		"Если это ошибка — обратитесь в поддержку"

	// MessageInternalError covers unexpected failures (DB, network) so the
	// user does not face silence.
	MessageInternalError = "Произошла внутренняя ошибка. Попробуйте позже"

	// MessageVerificationApproved is posted by the SendPulse Instagram
	// webhook after the verification → moderation transition. The bot
	// surfaces a single "Открыть" button that opens the TMA via WebApp.
	MessageVerificationApproved = "Instagram подтверждён. Заявка ушла на модерацию — следите за статусом в мини-приложении."

	// MessageVerificationApprovedButton is the inline-keyboard label paired
	// with MessageVerificationApproved.
	MessageVerificationApprovedButton = "Открыть мини-приложение"
)
