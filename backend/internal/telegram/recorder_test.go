package telegram

import (
	"context"
	"errors"
	"testing"

	"github.com/AlekSi/pointer"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	dbmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	repomocks "github.com/alikhanmurzayev/ugcboost/backend/internal/repository/mocks"
	tgmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/telegram/mocks"
)

func TestMessageRecorderService_RecordInbound(t *testing.T) {
	t.Parallel()

	t.Run("happy text message: row built from update, no log call", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := tgmocks.NewMockRecorderRepoFactory(t)
		repo := repomocks.NewMockTelegramMessageRepo(t)
		log := logmocks.NewMockLogger(t)
		factory.EXPECT().NewTelegramMessageRepo(mock.Anything).Return(repo)
		repo.EXPECT().Insert(mock.Anything, mock.AnythingOfType("*repository.TelegramMessageRow")).
			Run(func(_ context.Context, row *repository.TelegramMessageRow) {
				require.Equal(t, int64(42), row.ChatID)
				require.Equal(t, domain.TelegramMessageDirectionInbound, row.Direction)
				require.Equal(t, "hello", row.Text)
				require.Equal(t, pointer.ToInt64(7), row.TelegramMessageID)
				require.Equal(t, pointer.ToString("aidana"), row.TelegramUsername)
				require.Nil(t, row.Status)
				require.Nil(t, row.Error)
			}).Return(nil)

		svc := NewMessageRecorderService(pool, factory, log)
		svc.RecordInbound(context.Background(), &models.Update{
			Message: &models.Message{
				ID:   7,
				Chat: models.Chat{ID: 42, Type: "private"},
				From: &models.User{ID: 42, Username: "aidana"},
				Text: "hello",
			},
		})
	})

	t.Run("non-text message: row carries empty text, no log call", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := tgmocks.NewMockRecorderRepoFactory(t)
		repo := repomocks.NewMockTelegramMessageRepo(t)
		log := logmocks.NewMockLogger(t)
		factory.EXPECT().NewTelegramMessageRepo(mock.Anything).Return(repo)
		repo.EXPECT().Insert(mock.Anything, mock.MatchedBy(func(row *repository.TelegramMessageRow) bool {
			return row.Text == ""
		})).Return(nil)

		svc := NewMessageRecorderService(pool, factory, log)
		svc.RecordInbound(context.Background(), &models.Update{
			Message: &models.Message{
				ID:   8,
				Chat: models.Chat{ID: 42, Type: "private"},
				From: &models.User{ID: 42},
				Text: "",
			},
		})
	})

	t.Run("nil update: no-op", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := tgmocks.NewMockRecorderRepoFactory(t)
		log := logmocks.NewMockLogger(t)
		svc := NewMessageRecorderService(pool, factory, log)
		svc.RecordInbound(context.Background(), nil)
	})

	t.Run("dedup sentinel: Debug log, no Error", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := tgmocks.NewMockRecorderRepoFactory(t)
		repo := repomocks.NewMockTelegramMessageRepo(t)
		log := logmocks.NewMockLogger(t)
		factory.EXPECT().NewTelegramMessageRepo(mock.Anything).Return(repo)
		repo.EXPECT().Insert(mock.Anything, mock.Anything).
			Return(domain.ErrTelegramMessageAlreadyRecorded)
		log.EXPECT().Debug(mock.Anything, "telegram inbound already recorded",
			mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Run(func(_ context.Context, _ string, args ...any) {
			require.Equal(t, []any{"chat_id", int64(42), "telegram_message_id", 7}, args)
		}).Return()

		svc := NewMessageRecorderService(pool, factory, log)
		svc.RecordInbound(context.Background(), &models.Update{
			Message: &models.Message{
				ID:   7,
				Chat: models.Chat{ID: 42, Type: "private"},
				From: &models.User{ID: 42},
				Text: "hello",
			},
		})
	})

	t.Run("generic db error: Error log WITHOUT text in fields", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := tgmocks.NewMockRecorderRepoFactory(t)
		repo := repomocks.NewMockTelegramMessageRepo(t)
		log := logmocks.NewMockLogger(t)
		factory.EXPECT().NewTelegramMessageRepo(mock.Anything).Return(repo)
		insertErr := errors.New("conn refused")
		repo.EXPECT().Insert(mock.Anything, mock.Anything).Return(insertErr)
		log.EXPECT().Error(mock.Anything, "telegram inbound record failed",
			mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Run(func(_ context.Context, _ string, args ...any) {
			require.Equal(t, []any{"chat_id", int64(42), "direction", domain.TelegramMessageDirectionInbound, "error", insertErr}, args)
			for _, a := range args {
				if s, ok := a.(string); ok {
					require.NotEqual(t, "secret text", s, "PII text must NOT appear in log fields")
				}
			}
		}).Return()

		svc := NewMessageRecorderService(pool, factory, log)
		svc.RecordInbound(context.Background(), &models.Update{
			Message: &models.Message{
				ID:   7,
				Chat: models.Chat{ID: 42, Type: "private"},
				From: &models.User{ID: 42},
				Text: "secret text",
			},
		})
	})
}

func TestMessageRecorderService_RecordOutbound(t *testing.T) {
	t.Parallel()

	t.Run("sent: row carries telegram_message_id from returned msg, status=sent", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := tgmocks.NewMockRecorderRepoFactory(t)
		repo := repomocks.NewMockTelegramMessageRepo(t)
		log := logmocks.NewMockLogger(t)
		factory.EXPECT().NewTelegramMessageRepo(mock.Anything).Return(repo)
		repo.EXPECT().Insert(mock.Anything, mock.MatchedBy(func(row *repository.TelegramMessageRow) bool {
			return row.ChatID == 42 &&
				row.Direction == domain.TelegramMessageDirectionOutbound &&
				row.Text == "hi" &&
				row.Status != nil && *row.Status == domain.TelegramMessageStatusSent &&
				row.Error == nil &&
				row.TelegramMessageID != nil && *row.TelegramMessageID == 99
		})).Return(nil)

		svc := NewMessageRecorderService(pool, factory, log)
		svc.RecordOutbound(context.Background(),
			&bot.SendMessageParams{ChatID: int64(42), Text: "hi"},
			&models.Message{ID: 99},
			nil,
		)
	})

	t.Run("failed: row carries error.Error(), telegram_message_id nil", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := tgmocks.NewMockRecorderRepoFactory(t)
		repo := repomocks.NewMockTelegramMessageRepo(t)
		log := logmocks.NewMockLogger(t)
		factory.EXPECT().NewTelegramMessageRepo(mock.Anything).Return(repo)
		sendErr := errors.New("Forbidden: bot was blocked by the user")
		repo.EXPECT().Insert(mock.Anything, mock.MatchedBy(func(row *repository.TelegramMessageRow) bool {
			return row.Status != nil && *row.Status == domain.TelegramMessageStatusFailed &&
				row.Error != nil && *row.Error == sendErr.Error() &&
				row.TelegramMessageID == nil
		})).Return(nil)

		svc := NewMessageRecorderService(pool, factory, log)
		svc.RecordOutbound(context.Background(),
			&bot.SendMessageParams{ChatID: int64(42), Text: "hi"},
			nil,
			sendErr,
		)
	})

	t.Run("nil params: no-op", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := tgmocks.NewMockRecorderRepoFactory(t)
		log := logmocks.NewMockLogger(t)
		svc := NewMessageRecorderService(pool, factory, log)
		svc.RecordOutbound(context.Background(), nil, nil, nil)
	})

	t.Run("non-int64 ChatID: no-op (string @username chat-id is not supported by the table)", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := tgmocks.NewMockRecorderRepoFactory(t)
		log := logmocks.NewMockLogger(t)
		svc := NewMessageRecorderService(pool, factory, log)
		svc.RecordOutbound(context.Background(),
			&bot.SendMessageParams{ChatID: "@username", Text: "hi"},
			nil,
			nil,
		)
	})

	t.Run("insert error: Error log without text", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := tgmocks.NewMockRecorderRepoFactory(t)
		repo := repomocks.NewMockTelegramMessageRepo(t)
		log := logmocks.NewMockLogger(t)
		factory.EXPECT().NewTelegramMessageRepo(mock.Anything).Return(repo)
		insertErr := errors.New("db down")
		repo.EXPECT().Insert(mock.Anything, mock.Anything).Return(insertErr)
		log.EXPECT().Error(mock.Anything, "telegram outbound record failed",
			mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Run(func(_ context.Context, _ string, args ...any) {
			require.Equal(t, []any{"chat_id", int64(42), "direction", domain.TelegramMessageDirectionOutbound, "error", insertErr}, args)
		}).Return()

		svc := NewMessageRecorderService(pool, factory, log)
		svc.RecordOutbound(context.Background(),
			&bot.SendMessageParams{ChatID: int64(42), Text: "secret"},
			&models.Message{ID: 99},
			nil,
		)
	})
}
