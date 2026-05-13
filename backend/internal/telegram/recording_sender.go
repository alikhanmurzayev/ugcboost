package telegram

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// RecordingSender wraps any Sender and persists every send attempt through
// the MessageRecorder. The wrapper sits BETWEEN the Notifier and the upstream
// bot client (or TeeSender in test wiring) so a single decorator covers every
// outbound notify path (Notifier.fire retries, SendCampaignInvite, manual
// service-layer SendMessage calls).
type RecordingSender struct {
	inner    Sender
	recorder MessageRecorder
}

// NewRecordingSender wires the wrapper. Both inner and recorder must be
// non-nil — production wiring never passes nil and a silent no-op would mask
// the recording feature in prod.
func NewRecordingSender(inner Sender, recorder MessageRecorder) *RecordingSender {
	if inner == nil {
		panic("telegram: NewRecordingSender requires non-nil inner sender")
	}
	if recorder == nil {
		panic("telegram: NewRecordingSender requires non-nil recorder")
	}
	return &RecordingSender{inner: inner, recorder: recorder}
}

// SendMessage delegates to the wrapped sender, then records the outcome.
// The wrapper returns the inner result verbatim — the recorder is observation,
// not transformation.
func (s *RecordingSender) SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	msg, err := s.inner.SendMessage(ctx, params)
	s.recorder.RecordOutbound(ctx, params, msg, err)
	return msg, err
}
