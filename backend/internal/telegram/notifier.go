package telegram

import (
	"context"
	"errors"
	"fmt"
	"html"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

// telegramNotifyTimeout caps a single send attempt against the Telegram API.
// Each retry attempt gets its own sub-context with this deadline, so a stalled
// API call cannot pin a goroutine even when retry-loop budget allows further
// attempts.
const telegramNotifyTimeout = 10 * time.Second

// Production retry parameters. The retry loop adds at most ~7 seconds of
// sleep between attempts (1s + 2s + 4s with the randomization factor pinned
// to 0 for predictability), well under the 30s maxElapsed cap.
const (
	defaultBackoffInitial     = 1 * time.Second
	defaultBackoffMaxInterval = 8 * time.Second
	defaultBackoffMultiplier  = 2.0
	defaultBackoffMaxElapsed  = 30 * time.Second
	defaultBackoffMaxAttempts = 4
)

// igDirectURL is the Instagram Direct deep-link to the UGCBoost account.
// Used inside the welcome message when the application carries an IG handle.
const igDirectURL = "https://ig.me/m/ugc_boost"

// welcomeWithIGTemplate is the welcome text sent right after a successful
// /start link when the application has at least one Instagram social. The
// %s is the application's verification code (UGC-NNNNNN). HTML parse_mode
// is required for the <pre> tap-to-copy block.
const welcomeWithIGTemplate = "Здравствуйте! 👋\n\n" +
	"Мы получили вашу заявку.\n" +
	"Подтвердите, пожалуйста, что вы действительно владеете указанным аккаунтом Instagram:\n\n" +
	"1. Скопируйте код:\n" +
	"   <pre>%s</pre>\n\n" +
	"2. Отправьте код в Direct аккаунта ugc_boost (ссылка ниже):\n\n" +
	"   " + igDirectURL

// welcomeNoIGText is the welcome text sent right after a successful /start
// link when the application has no Instagram social. We do not surface the
// internal "manual verification" mechanic — the message stays generic.
const welcomeNoIGText = "Здравствуйте! 👋\n\n" +
	"Мы получили вашу заявку. Скоро сообщим здесь результаты отбора ✅"

// verificationApprovedText is the static post-verification message. No
// inline keyboard — TMA is out of the onboarding flow.
const verificationApprovedText = "Вы успешно подтвердили свой аккаунт ✅\n\n" +
	"Скоро сообщим здесь результаты отбора 🖤"

// applicationRejectedText is the static reject message. The wording is
// time-bound (mentions fashion-кампаний) and is rotated by replacing this
// constant in a separate PR — no Config switch, no template fan-out.
const applicationRejectedText = "Здравствуйте! Благодарим вас за интерес к платформе UGC boost.\n\n" +
	"Мы внимательно рассмотрели вашу заявку, профиль, контент и текущие показатели аккаунта. К сожалению, на данном этапе ваша заявка не прошла модерацию платформы.\n\n" +
	"Это не является оценкой вашего потенциала как креатора — просто сейчас ваш профиль не полностью совпадает с критериями отбора для текущих fashion-кампаний и запросов брендов на платформе 🙏\n\n" +
	"Желаем вам дальнейшего роста и удачи в ваших проектах 🤍"

// applicationApprovedText is the static congratulation sent after admin
// approve commits. Plain text, no parse_mode, no inline keyboard. Iterated
// by replacing this constant in a separate PR — no Config switch.
const applicationApprovedText = "Здравствуйте!\n\n" +
	"Рады сообщить, что ваша заявка прошла модерацию 😍 Ваш профиль, визуальный стиль и контент соответствуют критериям отбора для участия в fashion-кампаниях платформы UGC boost 💫\n\n" +
	"В ближайшее время мы отправим вам детали участия в EURASIAN FASHION WEEK и договор для подписания.\n\n" +
	"Добро пожаловать на платформу UGC boost 💫\n\n" +
	"После Недели моды мы планируем запустить приложение в App Store и добавить новые возможности для UGC-сотрудничества с брендами и партнерами EURASIAN FASHION WEEK.\n\n" +
	"Оставайтесь с нами — впереди много масштабных проектов!"

// campaignInviteText / campaignRemindInvitationText carry the universal copy
// for outbound invite / remind messages. Generic by design — the message
// body must not leak campaign details (name, brand, deadlines) because
// notify covers re-invites after declines as well, and stale text from a
// previous round would mislead. The accompanying inline web_app button
// drops the creator straight into the TMA where the creator reviews the
// brief and presses "Согласиться" inside the mini-app.
//
// These literals are mirrored in `backend/e2e/campaign_creator/
// campaign_notify_test.go` (the e2e module cannot import internal/telegram
// by design — see backend-testing-e2e.md). When changing the copy here,
// update the e2e mirror too or `waitInviteSent` will time out.
const (
	campaignInviteText = "Добрый день! EURASIAN FASHION WEEK уже скоро ✨\n\n" +
		"У нас есть для вас предложение по сотрудничеству в качестве UGC-креатора. Откройте ссылку, чтобы ознакомиться с датами, условиями, форматом участия и техническим заданием для контента.\n\n" +
		"Если вы согласны, нажмите кнопку \"Согласиться\" и мы отправим вам онлайн соглашение о сотрудничестве на подписание 💫"
	campaignRemindInvitationText = "Откройте ссылку, чтобы ознакомиться с датами, условиями, форматом участия и техническим заданием для контента.\n\n" +
		"Если вы согласны, нажмите кнопку \"Согласиться\" и мы отправим вам онлайн соглашение о сотрудничестве на подписание 💫"
	campaignInviteWebAppButtonText = "Посмотреть"
)

// campaignContractSignedText is the post-signing congrat message — sent
// from chunk 17 once TrustMe confirms the creator signed the contract.
// Defined here ahead of time so the copy lives in one place and reviewers
// can audit it before the chunk lands. No notifier method is wired up
// yet; chunk 17 introduces NotifyCampaignContractSigned and the e2e
// mirror at the same time.
const campaignContractSignedText = "Ура, мы подписали с вами соглашение ✅ Скоро отправим вам онлайн пригласительный на показы 😍"

// CampaignContractSignedText exposes the post-signing copy for tests
// and (later) for the chunk-17 notifier method.
func CampaignContractSignedText() string { return campaignContractSignedText }

// campaignContractSentText — креатору после Phase 3 outbox-worker'а: договор
// ушёл в TrustMe на подпись. TrustMe сам пришлёт SMS со ссылкой на свой
// сайт, где креатор подпишет — поэтому ссылку здесь не дублируем (TrustMe
// отдаёт нам не URL, а только short-code, и собирать `tct.kz/uploader/<code>`
// руками тут смысла нет).
const campaignContractSentText = "Мы отправили вам соглашение на подпись 📄"

// CampaignContractSentText экспортирует текст для тестов.
func CampaignContractSentText() string { return campaignContractSentText }

// NotifyContractSent отправляет креатору сообщение «договор отправлен на
// подпись». fire-and-forget — стандарт backend-transactions (бот ПОСЛЕ Tx),
// без блокирования outbox-worker'а.
func (n *Notifier) NotifyContractSent(ctx context.Context, chatID int64) {
	n.fire(ctx, "campaign_contract_sent", chatID, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   campaignContractSentText,
	})
}

// ApplicationLinkedPayload carries everything NotifyApplicationLinked needs
// to pick the right welcome variant and substitute the verification code.
type ApplicationLinkedPayload struct {
	VerificationCode string
	HasInstagram     bool
}

// Notifier owns every outbound bot notification that the service layer
// fires after a successful commit. It encapsulates the fire-and-forget
// goroutine, the retry loop with exponential backoff, the WaitGroup the
// closer drains on shutdown, and the shared Sender.
type Notifier struct {
	sender      Sender
	wg          *sync.WaitGroup
	log         logger.Logger
	timeout     time.Duration
	initial     time.Duration
	multiplier  float64
	maxInterval time.Duration
	maxElapsed  time.Duration
	maxAttempts int
}

// NewNotifier wires the notifier with production retry parameters
// (1s/2s/4s backoff over up to 4 attempts within 30s). Callers that need
// to tune retry timings (tests with millisecond budgets) should use
// NewNotifierWithBackoff.
func NewNotifier(sender Sender, log logger.Logger) *Notifier {
	return NewNotifierWithBackoff(
		sender, log,
		defaultBackoffInitial,
		defaultBackoffMaxInterval,
		defaultBackoffMultiplier,
		defaultBackoffMaxElapsed,
		defaultBackoffMaxAttempts,
	)
}

// NewNotifierWithBackoff wires the notifier with custom retry parameters.
// Tests use this constructor to compress the retry budget into milliseconds
// without affecting production timings. Set-methods are deliberately not
// exposed (per backend-design § Зависимости через конструктор) — the
// returned Notifier is immutable.
func NewNotifierWithBackoff(
	sender Sender,
	log logger.Logger,
	initial, maxInterval time.Duration,
	multiplier float64,
	maxElapsed time.Duration,
	maxAttempts int,
) *Notifier {
	if sender == nil {
		panic("telegram: NewNotifier requires non-nil sender")
	}
	if log == nil {
		panic("telegram: NewNotifier requires non-nil logger")
	}
	return &Notifier{
		sender:      sender,
		wg:          &sync.WaitGroup{},
		log:         log,
		timeout:     telegramNotifyTimeout,
		initial:     initial,
		multiplier:  multiplier,
		maxInterval: maxInterval,
		maxElapsed:  maxElapsed,
		maxAttempts: maxAttempts,
	}
}

// Wait blocks until every in-flight notify goroutine has finished. The
// closer registers this so SIGTERM does not race the goroutines past pool
// shutdown. Bounded externally by the closer's per-step context.
func (n *Notifier) Wait() {
	n.wg.Wait()
}

// NotifyApplicationLinked sends the welcome message right after /start
// successfully links a Telegram account to an application. With-IG variant
// embeds the verification code and Direct link; without-IG variant stays
// neutral and never reveals the manual-verification path.
//
// parse_mode is set to HTML only on the with-IG branch — that template
// carries a <pre> tap-to-copy block. The no-IG text is plain and skipping
// HTML mode means a stray '&' or '<' added later cannot crash the upstream
// parser silently.
func (n *Notifier) NotifyApplicationLinked(ctx context.Context, chatID int64, p ApplicationLinkedPayload) {
	params := &bot.SendMessageParams{
		ChatID: chatID,
		Text:   buildWelcomeText(p),
	}
	if p.HasInstagram {
		params.ParseMode = models.ParseModeHTML
	}
	n.fire(ctx, "application_linked", chatID, params)
}

// NotifyVerificationApproved sends the post-verification message after
// the SendPulse webhook flips an application from verification to
// moderation. Plain text — no inline keyboard.
func (n *Notifier) NotifyVerificationApproved(ctx context.Context, chatID int64) {
	n.fire(ctx, "verification_approved", chatID, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   verificationApprovedText,
	})
}

// NotifyApplicationRejected sends the static reject message after admin reject
// commits. Plain text — no inline keyboard, no parse mode.
func (n *Notifier) NotifyApplicationRejected(ctx context.Context, chatID int64) {
	n.fire(ctx, "application_rejected", chatID, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   applicationRejectedText,
	})
}

// NotifyApplicationApproved sends the static congratulation message after
// admin approve commits. Plain text — no inline keyboard, no parse mode.
func (n *Notifier) NotifyApplicationApproved(ctx context.Context, chatID int64) {
	n.fire(ctx, "application_approved", chatID, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   applicationApprovedText,
	})
}

// fire spawns the goroutine. The retry loop runs against an outer context
// derived via context.WithoutCancel — caller cancellation cannot silently
// drop the user-facing message — and capped by maxElapsed. Each individual
// SendMessage attempt gets its own sub-context with timeout so a single
// stalled call cannot eat the whole retry budget. A recover guards the
// process — Telegram SDK panics or unexpected nil-derefs in payload
// composition would otherwise kill every other goroutine via process exit.
func (n *Notifier) fire(ctx context.Context, op string, chatID int64, params *bot.SendMessageParams) {
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		outerCtx, cancelOuter := context.WithTimeout(context.WithoutCancel(ctx), n.maxElapsed)
		defer cancelOuter()
		defer func() {
			if rec := recover(); rec != nil {
				n.log.Error(outerCtx, "telegram notify panic",
					"op", op,
					"chat_id", chatID,
					"panic", rec,
					"stack", string(debug.Stack()),
				)
			}
		}()

		eb := backoff.NewExponentialBackOff()
		eb.InitialInterval = n.initial
		eb.Multiplier = n.multiplier
		eb.MaxInterval = n.maxInterval
		eb.RandomizationFactor = 0

		var attempts int
		_, err := backoff.Retry(outerCtx, func() (struct{}, error) {
			attempts++
			attemptCtx, cancelAttempt := context.WithTimeout(outerCtx, n.timeout)
			defer cancelAttempt()
			if _, sendErr := n.sender.SendMessage(attemptCtx, params); sendErr != nil {
				if !isRetryable(sendErr) {
					return struct{}{}, backoff.Permanent(sendErr)
				}
				return struct{}{}, sendErr
			}
			return struct{}{}, nil
		},
			backoff.WithBackOff(eb),
			backoff.WithMaxTries(uint(n.maxAttempts)),
			backoff.WithMaxElapsedTime(n.maxElapsed),
			backoff.WithNotify(func(retryErr error, next time.Duration) {
				n.log.Warn(outerCtx, "telegram notify retry",
					"op", op,
					"chat_id", chatID,
					"attempt", attempts,
					"next_backoff", next,
					"error", retryErr,
				)
			}),
		)
		if err != nil {
			n.log.Error(outerCtx, "telegram notify failed",
				"op", op,
				"chat_id", chatID,
				"attempts", attempts,
				"error", err,
			)
		}
	}()
}

// isRetryable classifies the Telegram SDK error to decide whether retry has
// any chance of succeeding. The Telegram-specific 4xx sentinels surface as
// wrapped errors so errors.Is matches them through fmt.Errorf("%w, ...").
//
// Terminal: 4xx (bad request, forbidden / bot blocked, unauthorized,
// not found, conflict, migrate). Retryable everything else: 429
// TooManyRequestsError, 5xx (default branch in raw_request.go, no typed
// surface), network errors (transport / decode failures), context
// deadline exceeded on a per-attempt sub-context. Outer context
// cancellation is not classified here — backoff.Retry observes
// outerCtx.Done() directly and aborts the loop without a Notify.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, bot.ErrorBadRequest),
		errors.Is(err, bot.ErrorForbidden),
		errors.Is(err, bot.ErrorUnauthorized),
		errors.Is(err, bot.ErrorNotFound),
		errors.Is(err, bot.ErrorConflict):
		return false
	}
	var migrateErr *bot.MigrateError
	return !errors.As(err, &migrateErr)
}

// buildWelcomeText picks the right welcome variant. With-IG substitutes
// the verification code into the <pre> block via fmt.Sprintf and runs the
// code through html.EscapeString defensively — even though the current
// generator emits only `UGC-NNNNNN`, future format changes should not be
// able to inject markup into the HTML-parsed message.
func buildWelcomeText(p ApplicationLinkedPayload) string {
	if !p.HasInstagram {
		return welcomeNoIGText
	}
	return fmt.Sprintf(welcomeWithIGTemplate, html.EscapeString(p.VerificationCode))
}

// SendCampaignInvite delivers an invite or remind-invitation message
// synchronously. Unlike the fire-and-forget Notify* family, this method
// returns the underlying SendMessage error so the campaign-creator service
// can map per-creator failures into the partial-success `undelivered` list.
// No retry — the admin re-invokes the endpoint with the unanswered subset.
//
// The message embeds an inline `web_app` button pointing at the campaign's
// TMA URL. The TMA agree/decline endpoints rely on this delivery surface:
// Telegram only attaches initData (HMAC-signed user payload) when the TMA
// is opened via a `web_app` button, so a plain-text URL would leave the
// creator unauthenticated.
func (n *Notifier) SendCampaignInvite(ctx context.Context, chatID int64, text, tmaURL string) error {
	callCtx, cancel := context.WithTimeout(ctx, n.timeout)
	defer cancel()
	_, err := n.sender.SendMessage(callCtx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{{{
				Text:   campaignInviteWebAppButtonText,
				WebApp: &models.WebAppInfo{URL: tmaURL},
			}}},
		},
	})
	return err
}

// CampaignInviteText returns the universal A4 invite copy. Exposed so the
// service layer can pass it back into SendCampaignInvite; keeping the
// constant private would force the service to hardcode the same string.
func CampaignInviteText() string { return campaignInviteText }

// CampaignRemindInvitationText returns the universal A5 reminder copy.
func CampaignRemindInvitationText() string { return campaignRemindInvitationText }

// MapTelegramErrorToReason classifies a SendMessage error into a
// domain.NotifyFailureReason* enum value. The sentinel branch
// (`bot.ErrorForbidden`) is the canonical signal that the creator
// blocked the bot. The string-substring fallback uses tight phrases
// (`bot was blocked by the user`, `user is deactivated`) tied to the
// current Telegram API surface so a future SDK rename falls into the
// safer `unknown` branch instead of misclassifying as `bot_blocked`.
// nil err returns "" — caller checks before invoking.
func MapTelegramErrorToReason(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, bot.ErrorForbidden) {
		return domain.NotifyFailureReasonBotBlocked
	}
	msg := err.Error()
	if strings.Contains(msg, "bot was blocked by the user") ||
		strings.Contains(msg, "user is deactivated") {
		return domain.NotifyFailureReasonBotBlocked
	}
	return domain.NotifyFailureReasonUnknown
}
