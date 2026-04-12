package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
)

func TestAuditRepository_Create(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := NewAuditRepository(db)
		entityID := "e-1"
		newVal := json.RawMessage(`{"name":"test"}`)
		gotSQL, gotArgs := captureExec(t, db, 8)

		_ = repo.Create(context.Background(), AuditLogRow{
			ActorID:    "u-1",
			ActorRole:  "admin",
			Action:     "brand_create",
			EntityType: "brand",
			EntityID:   &entityID,
			NewValue:   newVal,
			IPAddress:  "127.0.0.1",
		})

		require.Equal(t,
			"INSERT INTO audit_logs (action,actor_id,actor_role,entity_id,entity_type,ip_address,new_value,old_value) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)",
			*gotSQL)
		require.Equal(t, []any{"brand_create", "u-1", "admin", entityID, "brand", "127.0.0.1", newVal, json.RawMessage(nil)}, *gotArgs)
	})
}

func TestAuditRepository_List(t *testing.T) {
	t.Parallel()

	t.Run("count SQL no filters", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := NewAuditRepository(db)
		gotSQL, _ := captureQuery(t, db, 0)

		_, _, _ = repo.List(context.Background(), AuditFilter{}, 1, 20)

		require.Equal(t,
			"SELECT COUNT(*) FROM audit_logs",
			*gotSQL)
	})

	t.Run("count SQL with all filters", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := NewAuditRepository(db)
		dateFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		dateTo := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
		gotSQL, gotArgs := captureQuery(t, db, 6)

		_, _, _ = repo.List(context.Background(), AuditFilter{
			ActorID:    "u-1",
			EntityType: "brand",
			EntityID:   "e-1",
			Action:     "brand_create",
			DateFrom:   &dateFrom,
			DateTo:     &dateTo,
		}, 1, 20)

		require.Equal(t,
			"SELECT COUNT(*) FROM audit_logs WHERE actor_id = $1 AND entity_type = $2 AND entity_id = $3 AND action = $4 AND created_at >= $5 AND created_at <= $6",
			*gotSQL)
		require.Equal(t, []any{"u-1", "brand", "e-1", "brand_create", dateFrom, dateTo}, *gotArgs)
	})

	t.Run("data SQL with pagination", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := NewAuditRepository(db)

		var capturedSQL string
		var capturedArgs []any

		// First Query call: count — return scalarRows with total=1.
		db.On("Query", mock.Anything, mock.Anything, mock.Anything).
			Return(&scalarRows{val: 1}, nil).
			Once()

		// Second Query call: data — capture SQL and args.
		db.On("Query", mock.Anything, mock.Anything, mock.Anything).
			Run(func(callArgs mock.Arguments) {
				capturedSQL = callArgs.String(1)
				if len(callArgs) > 2 {
					capturedArgs = callArgs[2].([]any)
				}
			}).
			Return(nil, errors.New("mock: query intercepted")).
			Once()

		_, _, _ = repo.List(context.Background(), AuditFilter{ActorID: "u-1"}, 2, 20)

		require.Equal(t,
			"SELECT action, actor_id, actor_role, created_at, entity_id, entity_type, id, ip_address, new_value, old_value FROM audit_logs WHERE actor_id = $1 ORDER BY created_at DESC LIMIT 20 OFFSET 20",
			capturedSQL)
		require.Equal(t, []any{"u-1"}, capturedArgs)
	})
}
