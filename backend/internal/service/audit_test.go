package service

import (
	"context"
	"encoding/json"
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

func TestAuditService_List(t *testing.T) {
	t.Parallel()

	t.Run("empty result returns empty slice and zero total", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuditRepoFactory(t)
		repo := repomocks.NewMockAuditRepo(t)

		factory.EXPECT().NewAuditRepo(mock.Anything).Return(repo)
		repo.EXPECT().List(mock.Anything, repository.AuditFilter{}, 1, 20).
			Return(nil, int64(0), nil)

		svc := NewAuditService(pool, factory)
		logs, total, err := svc.List(context.Background(), domain.AuditFilter{}, 1, 20)
		require.NoError(t, err)
		require.Zero(t, total)
		require.Empty(t, logs)
	})

	t.Run("repo error propagates", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuditRepoFactory(t)
		repo := repomocks.NewMockAuditRepo(t)

		factory.EXPECT().NewAuditRepo(mock.Anything).Return(repo)
		repo.EXPECT().List(mock.Anything, repository.AuditFilter{}, 1, 20).
			Return(nil, int64(0), errors.New("db error"))

		svc := NewAuditService(pool, factory)
		_, _, err := svc.List(context.Background(), domain.AuditFilter{}, 1, 20)
		require.ErrorContains(t, err, "db error")
	})

	t.Run("success maps rows to domain with filters and pagination", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuditRepoFactory(t)
		repo := repomocks.NewMockAuditRepo(t)

		dateFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		dateTo := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
		entityID := "e-1"
		created1 := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
		created2 := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
		newVal := json.RawMessage(`{"name":"test"}`)
		oldVal := json.RawMessage(`{"name":"old"}`)

		expectedFilter := repository.AuditFilter{
			ActorID:    "u-1",
			EntityType: "brand",
			EntityID:   "e-1",
			Action:     "brand_create",
			DateFrom:   &dateFrom,
			DateTo:     &dateTo,
		}

		factory.EXPECT().NewAuditRepo(mock.Anything).Return(repo)
		repo.EXPECT().List(mock.Anything, expectedFilter, 2, 50).
			Return([]*repository.AuditLogRow{
				{
					ID: "al-1", ActorID: "u-1", ActorRole: "admin", Action: "brand_create",
					EntityType: "brand", EntityID: &entityID, OldValue: oldVal, NewValue: newVal,
					IPAddress: "127.0.0.1", CreatedAt: created1,
				},
				{
					ID: "al-2", ActorID: "u-1", ActorRole: "admin", Action: "brand_update",
					EntityType: "brand", EntityID: &entityID, OldValue: oldVal, NewValue: newVal,
					IPAddress: "127.0.0.1", CreatedAt: created2,
				},
			}, int64(2), nil)

		svc := NewAuditService(pool, factory)
		logs, total, err := svc.List(context.Background(), domain.AuditFilter{
			ActorID:    "u-1",
			EntityType: "brand",
			EntityID:   "e-1",
			Action:     "brand_create",
			DateFrom:   &dateFrom,
			DateTo:     &dateTo,
		}, 2, 50)
		require.NoError(t, err)
		require.Equal(t, int64(2), total)
		require.Len(t, logs, 2)

		// JSON fields via JSONEq, then zero them out so require.Equal works on the struct.
		for _, l := range logs {
			require.JSONEq(t, string(newVal), string(l.NewValue))
			require.JSONEq(t, string(oldVal), string(l.OldValue))
			l.NewValue = nil
			l.OldValue = nil
		}
		require.Equal(t, []*domain.AuditLog{
			{ID: "al-1", ActorID: "u-1", ActorRole: "admin", Action: "brand_create", EntityType: "brand", EntityID: &entityID, IPAddress: "127.0.0.1", CreatedAt: created1},
			{ID: "al-2", ActorID: "u-1", ActorRole: "admin", Action: "brand_update", EntityType: "brand", EntityID: &entityID, IPAddress: "127.0.0.1", CreatedAt: created2},
		}, logs)
	})
}
