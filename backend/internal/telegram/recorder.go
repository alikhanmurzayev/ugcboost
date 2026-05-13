package telegram

import (
	"context"
	"errors"

	"github.com/AlekSi/pointer"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// MessageRecorder captures inbound updates and outbound send attempts into
// telegram_messages. Both methods are synchronous — recorder shares the
// caller's goroutine (long-poll handler for inbound, Notifier.fire goroutine
// for outbound). Failures never surface to the caller: a stuck recorder must
// not block dispatch, and a flaky DB must not lose the user-facing reply.
type MessageRecorder interface {
	RecordInbound(ctx context.Context, update *models.Update)
	RecordOutbound(ctx context.Context, params *bot.SendMessageParams, msg *models.Message, sendErr error)
}

// noopRecorder discards both directions. Used by test wiring (testapi) and as
// a placeholder when the caller intentionally bypasses persistence — keeps
// `recorder` non-nil so Handler.Handle does not have to nil-check.
type noopRecorder struct{}

// NoopRecorder returns a MessageRecorder that does nothing. Test wiring uses
// it so production Handler still gets a non-nil dependency.
func NoopRecorder() MessageRecorder { return noopRecorder{} }

func (noopRecorder) RecordInbound(context.Context, *models.Update) {}
func (noopRecorder) RecordOutbound(context.Context, *bot.SendMessageParams, *models.Message, error) {
}

// RecorderRepoFactory exposes only the constructor the recorder needs.
type RecorderRepoFactory interface {
	NewTelegramMessageRepo(db dbutil.DB) repository.TelegramMessageRepo
}

// MessageRecorderService is the production MessageRecorder. It writes one row
// per inbound update (after the private + non-nil filters in Handler.Handle)
// and one row per outbound SendMessage call (wired via RecordingSender).
type MessageRecorderService struct {
	pool        dbutil.Pool
	repoFactory RecorderRepoFactory
	log         logger.Logger
}

// NewMessageRecorderService wires the recorder.
func NewMessageRecorderService(pool dbutil.Pool, repoFactory RecorderRepoFactory, log logger.Logger) *MessageRecorderService {
	if pool == nil {
		panic("telegram: NewMessageRecorderService requires non-nil pool")
	}
	if repoFactory == nil {
		panic("telegram: NewMessageRecorderService requires non-nil repoFactory")
	}
	if log == nil {
		panic("telegram: NewMessageRecorderService requires non-nil logger")
	}
	return &MessageRecorderService{pool: pool, repoFactory: repoFactory, log: log}
}

// RecordInbound persists the inbound row. Handler.Handle has already filtered
// out non-private chats and from=nil updates, so this method assumes a valid
// payload. The caller's ctx is reused — propagation into the repo keeps the
// long-poll timeout/deadline intact. Duplicate updates (Telegram redelivery)
// are logged at Debug; other failures land in Error WITHOUT the message text
// (PII guard, see security.md § PII).
func (r *MessageRecorderService) RecordInbound(ctx context.Context, update *models.Update) {
	if update == nil || update.Message == nil {
		return
	}
	msg := update.Message
	row := &repository.TelegramMessageRow{
		ChatID:            msg.Chat.ID,
		Direction:         domain.TelegramMessageDirectionInbound,
		Text:              msg.Text,
		TelegramMessageID: pointer.ToInt64(int64(msg.ID)),
	}
	if from := msg.From; from != nil && from.Username != "" {
		row.TelegramUsername = pointer.ToString(from.Username)
	}
	repo := r.repoFactory.NewTelegramMessageRepo(r.pool)
	if err := repo.Insert(ctx, row); err != nil {
		if errors.Is(err, domain.ErrTelegramMessageAlreadyRecorded) {
			r.log.Debug(ctx, "telegram inbound already recorded",
				"chat_id", msg.Chat.ID,
				"telegram_message_id", msg.ID,
			)
			return
		}
		r.log.Error(ctx, "telegram inbound record failed",
			"chat_id", msg.Chat.ID,
			"direction", domain.TelegramMessageDirectionInbound,
			"error", err,
		)
	}
}

// RecordOutbound persists one row per SendMessage call regardless of
// ParseMode / ReplyMarkup. status=sent populates telegram_message_id from the
// returned message; status=failed populates error from sendErr.Error().
// Recorder failures are logged at Error WITHOUT the message text (PII guard).
// The caller is unaffected — RecordingSender returns (msg, sendErr) verbatim.
func (r *MessageRecorderService) RecordOutbound(ctx context.Context, params *bot.SendMessageParams, msg *models.Message, sendErr error) {
	if params == nil {
		return
	}
	chatID, ok := params.ChatID.(int64)
	if !ok {
		// SendMessageParams.ChatID is a generic any (Telegram allows ints and
		// "@username" strings); telegram_messages.chat_id is BIGINT NOT NULL
		// so a non-int64 chat id cannot be stored. Drop silently — every
		// production call-site passes int64 (audited by `params.ChatID.(int64)`
		// in tests).
		return
	}
	row := &repository.TelegramMessageRow{
		ChatID:    chatID,
		Direction: domain.TelegramMessageDirectionOutbound,
		Text:      params.Text,
	}
	if sendErr != nil {
		row.Status = pointer.ToString(domain.TelegramMessageStatusFailed)
		row.Error = pointer.ToString(sendErr.Error())
	} else {
		row.Status = pointer.ToString(domain.TelegramMessageStatusSent)
		if msg != nil {
			row.TelegramMessageID = pointer.ToInt64(int64(msg.ID))
		}
	}
	repo := r.repoFactory.NewTelegramMessageRepo(r.pool)
	if err := repo.Insert(ctx, row); err != nil {
		r.log.Error(ctx, "telegram outbound record failed",
			"chat_id", chatID,
			"direction", domain.TelegramMessageDirectionOutbound,
			"error", err,
		)
	}
}
