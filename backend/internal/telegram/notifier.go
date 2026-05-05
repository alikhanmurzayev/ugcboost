package telegram

import (
	"context"
	"errors"
	"fmt"
	"html"
	"runtime/debug"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

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
	"2. Откройте Direct и отправьте его нам:\n\n" +
	"   " + igDirectURL

// welcomeNoIGText is the welcome text sent right after a successful /start
// link when the application has no Instagram social. We do not surface the
// internal "manual verification" mechanic — the message stays generic.
const welcomeNoIGText = "Здравствуйте! 👋\n\n" +
	"Мы получили вашу заявку. Скоро сообщим здесь результаты отбора ✅"

// verificationApprovedText replaces the chunk-8 placeholder. No inline
// keyboard — TMA is out of the onboarding flow per roadmap v2.
const verificationApprovedText = "Вы успешно подтвердили свой аккаунт ✅\n\n" +
	"Скоро сообщим здесь результаты отбора 🖤"

// applicationRejectedText is the static reject message (chunk 13). The wording
// is time-bound (mentions fashion-кампаний) and is rotated by replacing this
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
