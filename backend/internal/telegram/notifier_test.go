package telegram_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/telegram"
	tgmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/telegram/mocks"
)

// captureSend installs a SendMessage expectation that records the params
// and closes the returned channel when SendMessage has fired. Use waitFor
// to block with a deadline — fail fast on missing/multiple calls instead
// of hanging until the test timeout.
func captureSend(t *testing.T, sender *tgmocks.MockSender, sendErr error) (params **bot.SendMessageParams, sendDone <-chan struct{}) {
	t.Helper()
	captured := new(*bot.SendMessageParams)
	done := make(chan struct{}, 1)
	sender.EXPECT().SendMessage(mock.Anything, mock.AnythingOfType("*bot.SendMessageParams")).
		Run(func(_ context.Context, p *bot.SendMessageParams) {
			*captured = p
			select {
			case done <- struct{}{}:
			default:
				// SendMessage fired more than once — second close would
				// otherwise panic on a closed channel; surface the regression
				// loudly via a test failure rather than swallowing it.
				t.Errorf("SendMessage was called more than once in this scenario")
			}
		}).
		Return(&models.Message{ID: 1}, sendErr)
	return captured, done
}

// waitFor blocks until done fires or the deadline elapses. Fails fast with
// a context-rich message instead of a generic test-suite timeout.
func waitFor(t *testing.T, label string, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: SendMessage was not invoked within deadline", label)
	}
}

func TestNotifier_NotifyApplicationLinked(t *testing.T) {
	t.Parallel()

	t.Run("with-IG variant carries UGC code, Direct URL, HTML parse mode", func(t *testing.T) {
		t.Parallel()
		sender := tgmocks.NewMockSender(t)
		log := logmocks.NewMockLogger(t)
		captured, sendDone := captureSend(t, sender, nil)

		n := telegram.NewNotifier(sender, log)
		n.NotifyApplicationLinked(context.Background(), 4242, telegram.ApplicationLinkedPayload{
			VerificationCode: "UGC-123456",
			HasInstagram:     true,
		})
		waitFor(t, t.Name(), sendDone)
		n.Wait()

		require.NotNil(t, *captured)
		chatID, ok := (*captured).ChatID.(int64)
		require.True(t, ok)
		require.Equal(t, int64(4242), chatID)
		require.Equal(t, models.ParseModeHTML, (*captured).ParseMode)
		require.Contains(t, (*captured).Text, "UGC-123456")
		require.Contains(t, (*captured).Text, "https://ig.me/m/ugc_boost")
		require.Contains(t, (*captured).Text, "<pre>UGC-123456</pre>")
	})

	t.Run("no-IG variant stays neutral, no code or Direct URL", func(t *testing.T) {
		t.Parallel()
		sender := tgmocks.NewMockSender(t)
		log := logmocks.NewMockLogger(t)
		captured, sendDone := captureSend(t, sender, nil)

		n := telegram.NewNotifier(sender, log)
		n.NotifyApplicationLinked(context.Background(), 7, telegram.ApplicationLinkedPayload{
			VerificationCode: "UGC-123456",
			HasInstagram:     false,
		})
		waitFor(t, t.Name(), sendDone)
		n.Wait()

		require.NotNil(t, *captured)
		require.Contains(t, (*captured).Text, "Спасибо за заявку")
		require.NotContains(t, (*captured).Text, "UGC-")
		require.NotContains(t, (*captured).Text, "ig.me")
	})

	t.Run("sender error logged, Wait still drains", func(t *testing.T) {
		t.Parallel()
		sender := tgmocks.NewMockSender(t)
		log := logmocks.NewMockLogger(t)
		_, sendDone := captureSend(t, sender, errors.New("upstream 5xx"))
		log.EXPECT().Error(mock.Anything, "telegram notify failed", mock.Anything).Once()

		n := telegram.NewNotifier(sender, log)
		require.NotPanics(t, func() {
			n.NotifyApplicationLinked(context.Background(), 1, telegram.ApplicationLinkedPayload{HasInstagram: true})
		})
		waitFor(t, t.Name(), sendDone)
		n.Wait()
	})
}

func TestNotifier_NotifyVerificationApproved(t *testing.T) {
	t.Parallel()

	t.Run("posts plain-text moderation message without inline keyboard", func(t *testing.T) {
		t.Parallel()
		sender := tgmocks.NewMockSender(t)
		log := logmocks.NewMockLogger(t)
		captured, sendDone := captureSend(t, sender, nil)

		n := telegram.NewNotifier(sender, log)
		n.NotifyVerificationApproved(context.Background(), 555)
		waitFor(t, t.Name(), sendDone)
		n.Wait()

		require.NotNil(t, *captured)
		chatID, ok := (*captured).ChatID.(int64)
		require.True(t, ok)
		require.Equal(t, int64(555), chatID)
		require.Contains(t, (*captured).Text, "модерацию")
		require.Empty(t, (*captured).ParseMode, "verification-approved is plain text — no parse mode")
		require.Nil(t, (*captured).ReplyMarkup, "no inline keyboard on chunk-9 verification-approved")
		// Belt-and-braces: substrings the e2e tests assert never appear.
		for _, banned := range []string{"tma", "mini", "webapp", "ig.me", "UGC-"} {
			require.NotContains(t, strings.ToLower((*captured).Text), strings.ToLower(banned))
		}
	})

	t.Run("sender error logged with chat id and op", func(t *testing.T) {
		t.Parallel()
		sender := tgmocks.NewMockSender(t)
		log := logmocks.NewMockLogger(t)
		_, sendDone := captureSend(t, sender, errors.New("network down"))
		log.EXPECT().Error(mock.Anything, "telegram notify failed",
			mock.MatchedBy(func(args []any) bool {
				// args alternate "key", value — ensure both op + chat_id are present.
				m := map[string]any{}
				for i := 0; i+1 < len(args); i += 2 {
					if k, ok := args[i].(string); ok {
						m[k] = args[i+1]
					}
				}
				return m["op"] == "verification_approved" && m["chat_id"] == int64(99)
			})).Once()

		n := telegram.NewNotifier(sender, log)
		require.NotPanics(t, func() {
			n.NotifyVerificationApproved(context.Background(), 99)
		})
		waitFor(t, t.Name(), sendDone)
		n.Wait()
	})
}

func TestNotifier_FireAndForget(t *testing.T) {
	t.Parallel()

	t.Run("Wait blocks until in-flight notify completes", func(t *testing.T) {
		t.Parallel()
		sender := tgmocks.NewMockSender(t)
		log := logmocks.NewMockLogger(t)

		release := make(chan struct{})
		sender.EXPECT().SendMessage(mock.Anything, mock.Anything).
			Run(func(_ context.Context, _ *bot.SendMessageParams) {
				<-release
			}).
			Return(&models.Message{ID: 1}, nil)

		n := telegram.NewNotifier(sender, log)
		n.NotifyVerificationApproved(context.Background(), 1)

		done := make(chan struct{})
		go func() {
			n.Wait()
			close(done)
		}()

		select {
		case <-done:
			t.Fatal("Wait returned before SendMessage finished")
		case <-time.After(50 * time.Millisecond):
			// expected — SendMessage is still parked
		}

		close(release)
		select {
		case <-done:
			// expected
		case <-time.After(time.Second):
			t.Fatal("Wait did not return after SendMessage completed")
		}
	})

	t.Run("panic in sender is recovered and logged, Wait still drains", func(t *testing.T) {
		t.Parallel()
		sender := tgmocks.NewMockSender(t)
		log := logmocks.NewMockLogger(t)

		fired := make(chan struct{}, 1)
		sender.EXPECT().SendMessage(mock.Anything, mock.Anything).
			Run(func(_ context.Context, _ *bot.SendMessageParams) {
				fired <- struct{}{}
				panic("synthetic SDK boom")
			}).
			Return(nil, nil)
		log.EXPECT().Error(mock.Anything, "telegram notify panic", mock.Anything).Once()

		n := telegram.NewNotifier(sender, log)
		require.NotPanics(t, func() {
			n.NotifyVerificationApproved(context.Background(), 42)
		})
		<-fired
		// Wait must return even though the goroutine recovered from a panic.
		done := make(chan struct{})
		go func() {
			n.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Wait did not return after panic-recovered notify")
		}
	})

	t.Run("caller cancellation does not abort the notify (WithoutCancel)", func(t *testing.T) {
		t.Parallel()
		sender := tgmocks.NewMockSender(t)
		log := logmocks.NewMockLogger(t)
		captured, sendDone := captureSend(t, sender, nil)

		callerCtx, cancel := context.WithCancel(context.Background())
		n := telegram.NewNotifier(sender, log)
		n.NotifyVerificationApproved(callerCtx, 7)
		// Cancel the caller context immediately — the notify must still flush.
		cancel()
		waitFor(t, t.Name(), sendDone)
		n.Wait()

		require.NotNil(t, *captured)
	})
}

func TestNewNotifier_Defensive(t *testing.T) {
	t.Parallel()

	t.Run("panics on nil sender", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		require.PanicsWithValue(t, "telegram: NewNotifier requires non-nil sender", func() {
			telegram.NewNotifier(nil, log)
		})
	})

	t.Run("panics on nil logger", func(t *testing.T) {
		t.Parallel()
		sender := tgmocks.NewMockSender(t)
		require.PanicsWithValue(t, "telegram: NewNotifier requires non-nil logger", func() {
			telegram.NewNotifier(sender, nil)
		})
	})
}
