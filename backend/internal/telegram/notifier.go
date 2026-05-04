package telegram

import (
	"context"
	"fmt"
	"html"
	"runtime/debug"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

// telegramNotifyTimeout caps a single fire-and-forget notify against the
// Telegram API. Verification / link side effects have already committed by
// the time the goroutine runs, so caller-side cancellation must not silently
// drop the user-facing message; a stalled API call must not pin a goroutine.
const telegramNotifyTimeout = 10 * time.Second

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

// ApplicationLinkedPayload carries everything NotifyApplicationLinked needs
// to pick the right welcome variant and substitute the verification code.
type ApplicationLinkedPayload struct {
	VerificationCode string
	HasInstagram     bool
}

// Notifier owns every outbound bot notification that the service layer
// fires after a successful commit. It encapsulates the fire-and-forget
// goroutine, the per-call timeout, the WaitGroup the closer drains on
// shutdown, and the shared Sender — service constructors no longer need
// to thread these dependencies themselves.
type Notifier struct {
	sender  Sender
	wg      *sync.WaitGroup
	timeout time.Duration
	log     logger.Logger
}

// NewNotifier wires the notifier. The WaitGroup is owned internally so the
// closer talks to the Notifier (via Wait) rather than juggling a shared *sync.
// WaitGroup. log must be non-nil — every fire-and-forget logs Errors and any
// unexpected panic via the recovered goroutine path.
func NewNotifier(sender Sender, log logger.Logger) *Notifier {
	if sender == nil {
		panic("telegram: NewNotifier requires non-nil sender")
	}
	if log == nil {
		panic("telegram: NewNotifier requires non-nil logger")
	}
	return &Notifier{
		sender:  sender,
		wg:      &sync.WaitGroup{},
		timeout: telegramNotifyTimeout,
		log:     log,
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

// fire spawns the goroutine. ctx propagates trace metadata via
// context.WithoutCancel so request cancellation cannot silently drop
// the user-facing message; a hard timeout caps stalled API calls. A
// recover guards the process — Telegram SDK panics or unexpected nil-
// derefs in payload composition would otherwise kill every other
// goroutine via process exit.
func (n *Notifier) fire(ctx context.Context, op string, chatID int64, params *bot.SendMessageParams) {
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		notifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), n.timeout)
		defer cancel()
		defer func() {
			if rec := recover(); rec != nil {
				n.log.Error(notifyCtx, "telegram notify panic",
					"op", op,
					"chat_id", chatID,
					"panic", rec,
					"stack", string(debug.Stack()),
				)
			}
		}()
		if _, err := n.sender.SendMessage(notifyCtx, params); err != nil {
			n.log.Error(notifyCtx, "telegram notify failed",
				"op", op,
				"chat_id", chatID,
				"error", err,
			)
		}
	}()
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
