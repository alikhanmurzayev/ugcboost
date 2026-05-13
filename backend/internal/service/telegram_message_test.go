package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	dbmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	repomocks "github.com/alikhanmurzayev/ugcboost/backend/internal/repository/mocks"
	svcmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/service/mocks"
)

func TestTelegramMessageService_ListByChat(t *testing.T) {
	t.Parallel()

	t.Run("empty result returns nil rows and nil cursor", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockTelegramMessageRepoFactory(t)
		repo := repomocks.NewMockTelegramMessageRepo(t)

		factory.EXPECT().NewTelegramMessageRepo(mock.Anything).Return(repo)
		repo.EXPECT().ListByChat(mock.Anything, int64(42), (*domain.TelegramMessagesCursor)(nil), 6).
			Return(nil, nil)

		svc := NewTelegramMessageService(pool, factory)
		rows, next, err := svc.ListByChat(context.Background(), 42, nil, 5)
		require.NoError(t, err)
		require.Empty(t, rows)
		require.Nil(t, next)
	})

	t.Run("repo error propagates", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockTelegramMessageRepoFactory(t)
		repo := repomocks.NewMockTelegramMessageRepo(t)

		factory.EXPECT().NewTelegramMessageRepo(mock.Anything).Return(repo)
		repo.EXPECT().ListByChat(mock.Anything, int64(42), (*domain.TelegramMessagesCursor)(nil), 6).
			Return(nil, errors.New("db down"))

		svc := NewTelegramMessageService(pool, factory)
		_, _, err := svc.ListByChat(context.Background(), 42, nil, 5)
		require.ErrorContains(t, err, "db down")
	})

	t.Run("page is shorter than limit: nextCursor nil", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockTelegramMessageRepoFactory(t)
		repo := repomocks.NewMockTelegramMessageRepo(t)

		t1 := time.Date(2026, 5, 13, 11, 0, 0, 0, time.UTC)
		factory.EXPECT().NewTelegramMessageRepo(mock.Anything).Return(repo)
		repo.EXPECT().ListByChat(mock.Anything, int64(42), (*domain.TelegramMessagesCursor)(nil), 6).
			Return([]*repository.TelegramMessageRow{
				{ID: "row-1", ChatID: 42, CreatedAt: t1, Direction: domain.TelegramMessageDirectionInbound, Text: "a"},
			}, nil)

		svc := NewTelegramMessageService(pool, factory)
		rows, next, err := svc.ListByChat(context.Background(), 42, nil, 5)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, "row-1", rows[0].ID)
		require.Nil(t, next)
	})

	t.Run("repo returns limit+1: page trimmed, nextCursor from last kept row", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockTelegramMessageRepoFactory(t)
		repo := repomocks.NewMockTelegramMessageRepo(t)

		t1 := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
		t2 := time.Date(2026, 5, 13, 11, 0, 0, 0, time.UTC)
		t3 := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
		factory.EXPECT().NewTelegramMessageRepo(mock.Anything).Return(repo)
		repo.EXPECT().ListByChat(mock.Anything, int64(42), (*domain.TelegramMessagesCursor)(nil), 3).
			Return([]*repository.TelegramMessageRow{
				{ID: "row-3", ChatID: 42, CreatedAt: t1},
				{ID: "row-2", ChatID: 42, CreatedAt: t2},
				{ID: "row-1", ChatID: 42, CreatedAt: t3},
			}, nil)

		svc := NewTelegramMessageService(pool, factory)
		rows, next, err := svc.ListByChat(context.Background(), 42, nil, 2)
		require.NoError(t, err)
		require.Len(t, rows, 2)
		require.Equal(t, "row-3", rows[0].ID)
		require.Equal(t, "row-2", rows[1].ID)
		require.NotNil(t, next)
		require.Equal(t, "row-2", next.ID)
		require.Equal(t, t2, next.CreatedAt)
	})

	t.Run("cursor forwarded to repo", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockTelegramMessageRepoFactory(t)
		repo := repomocks.NewMockTelegramMessageRepo(t)

		incomingCursor := &domain.TelegramMessagesCursor{
			CreatedAt: time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC),
			ID:        "prev",
		}
		factory.EXPECT().NewTelegramMessageRepo(mock.Anything).Return(repo)
		repo.EXPECT().ListByChat(mock.Anything, int64(42), incomingCursor, 6).
			Return(nil, nil)

		svc := NewTelegramMessageService(pool, factory)
		rows, next, err := svc.ListByChat(context.Background(), 42, incomingCursor, 5)
		require.NoError(t, err)
		require.Empty(t, rows)
		require.Nil(t, next)
	})
}
