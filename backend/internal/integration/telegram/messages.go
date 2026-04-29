package telegram

// Reply texts the bot sends to creators. Russian, official tone, no emojis or
// jargon — copy is part of the user-visible legal-grade communication.
//
// Constants are pinned values: tests assert against them verbatim, so any
// copy change is a deliberate code change reviewed alongside the underlying
// flow. The Messages interface threads them through the bot package so
// nothing inside dispatcher / start-handler builds strings inline.
const (
	// MessageLinkSuccess — successful /start <uuid>: link created or
	// idempotent re-issue from the same Telegram account.
	MessageLinkSuccess = "Здравствуйте! Заявка успешно связана с вашим Telegram-аккаунтом. " +
		"В ближайшее время в этом чате откроется мини-приложение со статусом обработки заявки."

	// MessageStartNoPayload — /start without a payload (user typed it manually).
	MessageStartNoPayload = "Здравствуйте! Чтобы связать ваш Telegram-аккаунт с заявкой, " +
		"перейдите по ссылке со страницы успешной подачи заявки на ugcboost.kz."

	// MessageInvalidPayload — /start <something>, but <something> is not a UUID.
	MessageInvalidPayload = "Не удалось распознать ссылку. " +
		"Перейдите по ссылке со страницы успешной подачи заявки на ugcboost.kz."

	// MessageApplicationNotFound — UUID parses but no application exists.
	MessageApplicationNotFound = "Не удалось найти заявку по этой ссылке. " +
		"Возможно, заявка ещё не подана. Подайте заявку на ugcboost.kz."

	// MessageApplicationNotActive — application is rejected / blocked / closed.
	MessageApplicationNotActive = "Эта заявка неактивна. " +
		"Если хотите подать новую, перейдите на ugcboost.kz."

	// MessageApplicationAlreadyLinked — the application is bound to a
	// different Telegram account than the one issuing /start.
	MessageApplicationAlreadyLinked = "Эта заявка уже связана с другим Telegram-аккаунтом. " +
		"Если это ошибка, обратитесь в поддержку."

	// MessageAccountAlreadyLinked — this Telegram user is already bound to
	// a different application.
	MessageAccountAlreadyLinked = "У вас уже есть активная заявка, связанная с этим Telegram-аккаунтом. " +
		"Дождитесь решения по ней или обратитесь в поддержку."

	// MessageFallback — any other text or command we do not understand.
	MessageFallback = "Я понимаю только команду /start со специальной ссылкой. " +
		"Перейдите по ссылке со страницы успешной подачи заявки на ugcboost.kz."

	// MessageInternalError — a service-level error bubbled up; the user is
	// asked to retry. Tests cover the message but it should be rare in prod.
	MessageInternalError = "Произошла внутренняя ошибка. Попробуйте ещё раз через минуту. " +
		"Если ошибка повторится — обратитесь в поддержку."
)

// Messages is the abstraction the dispatcher / start-handler use to fetch
// reply texts. Production wires DefaultMessages(); unit tests can substitute
// a stub when the goal is to verify routing rather than copy.
type Messages interface {
	LinkSuccess() string
	StartNoPayload() string
	InvalidPayload() string
	ApplicationNotFound() string
	ApplicationNotActive() string
	ApplicationAlreadyLinked() string
	AccountAlreadyLinked() string
	Fallback() string
	InternalError() string
}

type defaultMessages struct{}

// DefaultMessages returns the production-canonical reply texts.
func DefaultMessages() Messages { return defaultMessages{} }

func (defaultMessages) LinkSuccess() string              { return MessageLinkSuccess }
func (defaultMessages) StartNoPayload() string           { return MessageStartNoPayload }
func (defaultMessages) InvalidPayload() string           { return MessageInvalidPayload }
func (defaultMessages) ApplicationNotFound() string      { return MessageApplicationNotFound }
func (defaultMessages) ApplicationNotActive() string     { return MessageApplicationNotActive }
func (defaultMessages) ApplicationAlreadyLinked() string { return MessageApplicationAlreadyLinked }
func (defaultMessages) AccountAlreadyLinked() string     { return MessageAccountAlreadyLinked }
func (defaultMessages) Fallback() string                 { return MessageFallback }
func (defaultMessages) InternalError() string            { return MessageInternalError }
