package telegram_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
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

// retryTestParams keeps the retry budget tight so the test pack stays well
// under the unit-test deadline (race detector amplifies sleep waits). The 4
// attempts × 2× multiplier hit 1ms+2ms+4ms ≈ 7ms of cumulative sleep, so
// the maxElapsed cap of 200ms is a generous ceiling that never fires under
// happy/exhausted paths but stops a runaway test on a slow CI box.
const (
	retryInitial     = 1 * time.Millisecond
	retryMaxInterval = 4 * time.Millisecond
	retryMultiplier  = 2.0
	retryMaxElapsed  = 200 * time.Millisecond
	retryMaxAttempts = 4
)

func newRetryNotifier(sender telegram.Sender, log *logmocks.MockLogger) *telegram.Notifier {
	return telegram.NewNotifierWithBackoff(
		sender, log,
		retryInitial, retryMaxInterval, retryMultiplier, retryMaxElapsed, retryMaxAttempts,
	)
}

// newSingleShotNotifier preserves the pre-retry semantics for tests that
// expect exactly one SendMessage call on a sender error (the original
// "sender error logged" assertions).
func newSingleShotNotifier(sender telegram.Sender, log *logmocks.MockLogger) *telegram.Notifier {
	return telegram.NewNotifierWithBackoff(
		sender, log,
		retryInitial, retryMaxInterval, retryMultiplier, retryMaxElapsed, 1,
	)
}

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

// expectedWelcomeWithIG / expectedWelcomeNoIG / expectedVerificationApproved
// — точные строки, которые получит креатор в Telegram. Дублируют константы
// из notifier.go намеренно: assert-by-equality требует, чтобы любое изменение
// копирайта одновременно ломало тест и продакшен-код, иначе теряется аудит-
// сигнал «текст изменился, но тест зеленеет».
func expectedWelcomeWithIG(verificationCode string) string {
	return "Здравствуйте! 👋\n\n" +
		"Мы получили вашу заявку.\n" +
		"Подтвердите, пожалуйста, что вы действительно владеете указанным аккаунтом Instagram:\n\n" +
		"1. Скопируйте код:\n" +
		"   <pre>" + verificationCode + "</pre>\n\n" +
		"2. Откройте Direct и отправьте его нам:\n\n" +
		"   https://ig.me/m/ugc_boost"
}

const (
	expectedWelcomeNoIG = "Здравствуйте! 👋\n\n" +
		"Мы получили вашу заявку. Скоро сообщим здесь результаты отбора ✅"

	expectedVerificationApproved = "Вы успешно подтвердили свой аккаунт ✅\n\n" +
		"Скоро сообщим здесь результаты отбора 🖤"

	expectedApplicationRejected = "Здравствуйте! Благодарим вас за интерес к платформе UGC boost.\n\n" +
		"Мы внимательно рассмотрели вашу заявку, профиль, контент и текущие показатели аккаунта. К сожалению, на данном этапе ваша заявка не прошла модерацию платформы.\n\n" +
		"Это не является оценкой вашего потенциала как креатора — просто сейчас ваш профиль не полностью совпадает с критериями отбора для текущих fashion-кампаний и запросов брендов на платформе 🙏\n\n" +
		"Желаем вам дальнейшего роста и удачи в ваших проектах 🤍"
)

func TestNotifier_NotifyApplicationLinked(t *testing.T) {
	t.Parallel()

	t.Run("with-IG variant carries exact welcome text and HTML parse mode", func(t *testing.T) {
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
		require.Equal(t, expectedWelcomeWithIG("UGC-123456"), (*captured).Text)
	})

	t.Run("no-IG variant carries exact neutral welcome text without parse mode", func(t *testing.T) {
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
		chatID, ok := (*captured).ChatID.(int64)
		require.True(t, ok)
		require.Equal(t, int64(7), chatID)
		require.Equal(t, expectedWelcomeNoIG, (*captured).Text)
		require.Empty(t, (*captured).ParseMode, "no-IG variant has no HTML — parse mode must stay empty")
	})

	t.Run("sender error logged, Wait still drains", func(t *testing.T) {
		t.Parallel()
		sender := tgmocks.NewMockSender(t)
		log := logmocks.NewMockLogger(t)
		_, sendDone := captureSend(t, sender, errors.New("upstream 5xx"))
		log.EXPECT().Error(mock.Anything, "telegram notify failed", mock.Anything).Once()

		// maxAttempts=1 keeps single-shot semantics for the historical
		// assertion (one sender call → one Error log). The retry-on-error
		// behaviour is covered by the dedicated retry test pack below.
		n := newSingleShotNotifier(sender, log)
		require.NotPanics(t, func() {
			n.NotifyApplicationLinked(context.Background(), 1, telegram.ApplicationLinkedPayload{HasInstagram: true})
		})
		waitFor(t, t.Name(), sendDone)
		n.Wait()
	})
}

func TestNotifier_NotifyVerificationApproved(t *testing.T) {
	t.Parallel()

	t.Run("posts exact moderation message without inline keyboard", func(t *testing.T) {
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
		require.Equal(t, expectedVerificationApproved, (*captured).Text)
		require.Empty(t, (*captured).ParseMode, "verification-approved is plain text — no parse mode")
		require.Nil(t, (*captured).ReplyMarkup, "no inline keyboard on chunk-9 verification-approved")
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

		n := newSingleShotNotifier(sender, log)
		require.NotPanics(t, func() {
			n.NotifyVerificationApproved(context.Background(), 99)
		})
		waitFor(t, t.Name(), sendDone)
		n.Wait()
	})
}

func TestNotifier_NotifyApplicationRejected(t *testing.T) {
	t.Parallel()

	t.Run("posts exact reject message without parse mode or inline keyboard", func(t *testing.T) {
		t.Parallel()
		sender := tgmocks.NewMockSender(t)
		log := logmocks.NewMockLogger(t)
		captured, sendDone := captureSend(t, sender, nil)

		n := telegram.NewNotifier(sender, log)
		n.NotifyApplicationRejected(context.Background(), 12345)
		waitFor(t, t.Name(), sendDone)
		n.Wait()

		require.NotNil(t, *captured)
		chatID, ok := (*captured).ChatID.(int64)
		require.True(t, ok)
		require.Equal(t, int64(12345), chatID)
		require.Equal(t, expectedApplicationRejected, (*captured).Text)
		require.Empty(t, (*captured).ParseMode, "application-rejected is plain text — no parse mode")
		require.Nil(t, (*captured).ReplyMarkup, "no inline keyboard on chunk-13 reject message")
	})

	t.Run("sender error logged with chat id and op", func(t *testing.T) {
		t.Parallel()
		sender := tgmocks.NewMockSender(t)
		log := logmocks.NewMockLogger(t)
		_, sendDone := captureSend(t, sender, errors.New("network down"))
		log.EXPECT().Error(mock.Anything, "telegram notify failed",
			mock.MatchedBy(func(args []any) bool {
				m := map[string]any{}
				for i := 0; i+1 < len(args); i += 2 {
					if k, ok := args[i].(string); ok {
						m[k] = args[i+1]
					}
				}
				return m["op"] == "application_rejected" && m["chat_id"] == int64(77)
			})).Once()

		n := newSingleShotNotifier(sender, log)
		require.NotPanics(t, func() {
			n.NotifyApplicationRejected(context.Background(), 77)
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

// TestNotifier_Retry_TransientThenSuccess covers the recovery path: a 5xx
// on the first attempt, success on the second. backoff.Retry calls the
// Notify hook (Warn) between the two attempts, so the test pins exactly
// one Warn and zero Errors.
func TestNotifier_Retry_TransientThenSuccess(t *testing.T) {
	t.Parallel()
	sender := tgmocks.NewMockSender(t)
	log := logmocks.NewMockLogger(t)

	calls := atomic.Int32{}
	sender.EXPECT().SendMessage(mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ *bot.SendMessageParams) { calls.Add(1) }).
		Return(nil, errors.New("upstream 503")).Once()
	sender.EXPECT().SendMessage(mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ *bot.SendMessageParams) { calls.Add(1) }).
		Return(&models.Message{ID: 1}, nil).Once()
	log.EXPECT().Warn(mock.Anything, "telegram notify retry", mock.Anything).Once()

	n := newRetryNotifier(sender, log)
	n.NotifyVerificationApproved(context.Background(), 1)
	n.Wait()

	require.Equal(t, int32(2), calls.Load())
}

// TestNotifier_Retry_TransientThenSuccess_429 mirrors the 5xx recovery
// path with the 429 surface — a *bot.TooManyRequestsError on attempt #1
// is treated as transient and retried.
func TestNotifier_Retry_TransientThenSuccess_429(t *testing.T) {
	t.Parallel()
	sender := tgmocks.NewMockSender(t)
	log := logmocks.NewMockLogger(t)

	calls := atomic.Int32{}
	sender.EXPECT().SendMessage(mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ *bot.SendMessageParams) { calls.Add(1) }).
		Return(nil, &bot.TooManyRequestsError{Message: "too many", RetryAfter: 0}).Once()
	sender.EXPECT().SendMessage(mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ *bot.SendMessageParams) { calls.Add(1) }).
		Return(&models.Message{ID: 1}, nil).Once()
	log.EXPECT().Warn(mock.Anything, "telegram notify retry", mock.Anything).Once()

	n := newRetryNotifier(sender, log)
	n.NotifyVerificationApproved(context.Background(), 1)
	n.Wait()

	require.Equal(t, int32(2), calls.Load())
}

// TestNotifier_Retry_NetworkError_Success exercises the "transport-layer
// failure" branch — a generic plumbing error is unmatched by 4xx
// sentinels and qualifies as retryable.
func TestNotifier_Retry_NetworkError_Success(t *testing.T) {
	t.Parallel()
	sender := tgmocks.NewMockSender(t)
	log := logmocks.NewMockLogger(t)

	calls := atomic.Int32{}
	sender.EXPECT().SendMessage(mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ *bot.SendMessageParams) { calls.Add(1) }).
		Return(nil, errors.New("dial tcp 1.2.3.4:443: connect: connection refused")).Once()
	sender.EXPECT().SendMessage(mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ *bot.SendMessageParams) { calls.Add(1) }).
		Return(&models.Message{ID: 1}, nil).Once()
	log.EXPECT().Warn(mock.Anything, "telegram notify retry", mock.Anything).Once()

	n := newRetryNotifier(sender, log)
	n.NotifyVerificationApproved(context.Background(), 1)
	n.Wait()

	require.Equal(t, int32(2), calls.Load())
}

// TestNotifier_Retry_Terminal_400_NoRetry pins the 4xx-non-429 branch:
// bot.ErrorBadRequest is wrapped permanent and the loop stops immediately.
func TestNotifier_Retry_Terminal_400_NoRetry(t *testing.T) {
	t.Parallel()
	sender := tgmocks.NewMockSender(t)
	log := logmocks.NewMockLogger(t)

	calls := atomic.Int32{}
	sender.EXPECT().SendMessage(mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ *bot.SendMessageParams) { calls.Add(1) }).
		Return(nil, fmt.Errorf("%w, message text empty", bot.ErrorBadRequest)).Once()
	log.EXPECT().Error(mock.Anything, "telegram notify failed", mock.Anything).Once()

	n := newRetryNotifier(sender, log)
	n.NotifyVerificationApproved(context.Background(), 1)
	n.Wait()

	require.Equal(t, int32(1), calls.Load())
}

// TestNotifier_Retry_Terminal_403_NoRetry pins the second terminal branch:
// the bot has been blocked / kicked from the chat (Forbidden). Retrying
// would still bounce off the same permission error.
func TestNotifier_Retry_Terminal_403_NoRetry(t *testing.T) {
	t.Parallel()
	sender := tgmocks.NewMockSender(t)
	log := logmocks.NewMockLogger(t)

	calls := atomic.Int32{}
	sender.EXPECT().SendMessage(mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ *bot.SendMessageParams) { calls.Add(1) }).
		Return(nil, fmt.Errorf("%w, bot was blocked by the user", bot.ErrorForbidden)).Once()
	log.EXPECT().Error(mock.Anything, "telegram notify failed", mock.Anything).Once()

	n := newRetryNotifier(sender, log)
	n.NotifyVerificationApproved(context.Background(), 1)
	n.Wait()

	require.Equal(t, int32(1), calls.Load())
}

// TestNotifier_Retry_Exhausted covers the ceiling: every attempt returns
// the same retryable error. backoff.Retry caps at maxAttempts=4 so the
// sender is hit four times, the Notify hook runs three times (before
// each follow-up attempt), and exactly one final Error is logged.
func TestNotifier_Retry_Exhausted(t *testing.T) {
	t.Parallel()
	sender := tgmocks.NewMockSender(t)
	log := logmocks.NewMockLogger(t)

	calls := atomic.Int32{}
	sender.EXPECT().SendMessage(mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ *bot.SendMessageParams) { calls.Add(1) }).
		Return(nil, errors.New("upstream 503")).Times(4)
	log.EXPECT().Warn(mock.Anything, "telegram notify retry", mock.Anything).Times(3)
	log.EXPECT().Error(mock.Anything, "telegram notify failed", mock.Anything).Once()

	n := newRetryNotifier(sender, log)
	n.NotifyVerificationApproved(context.Background(), 1)
	n.Wait()

	require.Equal(t, int32(4), calls.Load())
}

// TestNotifier_Retry_OuterCtxBudgetExhausted pins the outer-context cap:
// when maxElapsed is exhausted (or the outer context is otherwise cancelled),
// backoff.Retry surfaces context.Cause and terminates the loop. The exact
// number of sender calls is a race-dependent 1–2 because the budget is
// crossed during the very first attempt; the test only asserts the
// terminal Error log fires.
func TestNotifier_Retry_OuterCtxBudgetExhausted(t *testing.T) {
	t.Parallel()
	sender := tgmocks.NewMockSender(t)
	log := logmocks.NewMockLogger(t)

	calls := atomic.Int32{}
	sender.EXPECT().SendMessage(mock.Anything, mock.Anything).
		Run(func(ctx context.Context, _ *bot.SendMessageParams) {
			calls.Add(1)
			<-ctx.Done()
		}).
		Return(nil, context.DeadlineExceeded).
		Maybe()
	log.EXPECT().Warn(mock.Anything, "telegram notify retry", mock.Anything).Maybe()
	log.EXPECT().Error(mock.Anything, "telegram notify failed", mock.Anything).Once()

	// 1ms outer budget — guarantees the loop exits after the first attempt.
	n := telegram.NewNotifierWithBackoff(
		sender, log,
		retryInitial, retryMaxInterval, retryMultiplier, 1*time.Millisecond, retryMaxAttempts,
	)
	n.NotifyVerificationApproved(context.Background(), 1)
	n.Wait()

	require.GreaterOrEqual(t, calls.Load(), int32(1))
}
