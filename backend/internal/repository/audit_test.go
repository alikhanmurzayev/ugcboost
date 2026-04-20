package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestAuditRepository_Create(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO audit_logs (action,actor_id,actor_role,entity_id,entity_type,ip_address,new_value,old_value) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)"

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &auditRepository{db: mock}
		entityID := "e-1"
		newVal := json.RawMessage(`{"name":"test"}`)

		mock.ExpectExec(sqlStmt).
			WithArgs("brand_create", "u-1", "admin", entityID, "brand", "127.0.0.1", newVal, json.RawMessage(nil)).
			WillReturnResult(pgconn.NewCommandTag("INSERT 0 1"))

		err := repo.Create(context.Background(), AuditLogRow{
			ActorID:    "u-1",
			ActorRole:  "admin",
			Action:     "brand_create",
			EntityType: "brand",
			EntityID:   &entityID,
			NewValue:   newVal,
			IPAddress:  "127.0.0.1",
		})
		require.NoError(t, err)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &auditRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("brand_delete", "u-1", "admin", nil, "brand", "127.0.0.1", json.RawMessage(nil), json.RawMessage(nil)).
			WillReturnError(errors.New("fk violation"))

		err := repo.Create(context.Background(), AuditLogRow{
			ActorID:    "u-1",
			ActorRole:  "admin",
			Action:     "brand_delete",
			EntityType: "brand",
			IPAddress:  "127.0.0.1",
		})
		require.ErrorContains(t, err, "fk violation")
	})
}

func TestAuditRepository_List(t *testing.T) {
	t.Parallel()

	const countSQLNoFilters = "SELECT COUNT(*) FROM audit_logs"
	const countSQLAllFilters = "SELECT COUNT(*) FROM audit_logs WHERE actor_id = $1 AND entity_type = $2 AND entity_id = $3 AND action = $4 AND created_at >= $5 AND created_at <= $6"
	const dataSQLActorFilter = "SELECT action, actor_id, actor_role, created_at, entity_id, entity_type, id, ip_address, new_value, old_value FROM audit_logs WHERE actor_id = $1 ORDER BY created_at DESC LIMIT 20 OFFSET 20"

	t.Run("empty result returns nil 0 nil without data query", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &auditRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		rows, total, err := repo.List(context.Background(), AuditFilter{}, 1, 20)
		require.NoError(t, err)
		require.Nil(t, rows)
		require.Zero(t, total)
	})

	t.Run("count error propagates", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &auditRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnError(errors.New("count failed"))

		_, _, err := repo.List(context.Background(), AuditFilter{}, 1, 20)
		require.ErrorContains(t, err, "count failed")
	})

	t.Run("data query error propagates", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &auditRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(5)))
		mock.ExpectQuery("SELECT action, actor_id, actor_role, created_at, entity_id, entity_type, id, ip_address, new_value, old_value FROM audit_logs ORDER BY created_at DESC LIMIT 20 OFFSET 0").
			WillReturnError(errors.New("data failed"))

		_, _, err := repo.List(context.Background(), AuditFilter{}, 1, 20)
		require.ErrorContains(t, err, "data failed")
	})

	t.Run("success maps rows with filters and pagination", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &auditRepository{db: mock}
		entityID := "e-1"
		newVal := json.RawMessage(`{"name":"test"}`)
		oldVal := json.RawMessage(`{"name":"old"}`)
		dateFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		dateTo := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
		created1 := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
		created2 := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)

		mock.ExpectQuery(countSQLAllFilters).
			WithArgs("u-1", "brand", entityID, "brand_create", dateFrom, dateTo).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(2)))
		mock.ExpectQuery("SELECT action, actor_id, actor_role, created_at, entity_id, entity_type, id, ip_address, new_value, old_value FROM audit_logs WHERE actor_id = $1 AND entity_type = $2 AND entity_id = $3 AND action = $4 AND created_at >= $5 AND created_at <= $6 ORDER BY created_at DESC LIMIT 20 OFFSET 0").
			WithArgs("u-1", "brand", entityID, "brand_create", dateFrom, dateTo).
			WillReturnRows(pgxmock.NewRows([]string{"action", "actor_id", "actor_role", "created_at", "entity_id", "entity_type", "id", "ip_address", "new_value", "old_value"}).
				AddRow("brand_create", "u-1", "admin", created1, &entityID, "brand", "al-1", "127.0.0.1", newVal, oldVal).
				AddRow("brand_update", "u-1", "admin", created2, &entityID, "brand", "al-2", "127.0.0.1", newVal, oldVal))

		rows, total, err := repo.List(context.Background(), AuditFilter{
			ActorID:    "u-1",
			EntityType: "brand",
			EntityID:   entityID,
			Action:     "brand_create",
			DateFrom:   &dateFrom,
			DateTo:     &dateTo,
		}, 1, 20)
		require.NoError(t, err)
		require.Equal(t, int64(2), total)
		require.Len(t, rows, 2)

		// Compare JSON payloads via JSONEq, then zero them for whole-struct equality.
		require.JSONEq(t, string(newVal), string(rows[0].NewValue))
		require.JSONEq(t, string(oldVal), string(rows[0].OldValue))
		require.JSONEq(t, string(newVal), string(rows[1].NewValue))
		require.JSONEq(t, string(oldVal), string(rows[1].OldValue))

		for _, r := range rows {
			r.NewValue = nil
			r.OldValue = nil
		}
		require.Equal(t, []*AuditLogRow{
			{ID: "al-1", ActorID: "u-1", ActorRole: "admin", Action: "brand_create", EntityType: "brand", EntityID: &entityID, IPAddress: "127.0.0.1", CreatedAt: created1},
			{ID: "al-2", ActorID: "u-1", ActorRole: "admin", Action: "brand_update", EntityType: "brand", EntityID: &entityID, IPAddress: "127.0.0.1", CreatedAt: created2},
		}, rows)
	})

	t.Run("data query uses actor filter and offset for page 2", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &auditRepository{db: mock}

		mock.ExpectQuery("SELECT COUNT(*) FROM audit_logs WHERE actor_id = $1").
			WithArgs("u-1").
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(25)))
		mock.ExpectQuery(dataSQLActorFilter).
			WithArgs("u-1").
			WillReturnRows(pgxmock.NewRows([]string{"action", "actor_id", "actor_role", "created_at", "entity_id", "entity_type", "id", "ip_address", "new_value", "old_value"}))

		rows, total, err := repo.List(context.Background(), AuditFilter{ActorID: "u-1"}, 2, 20)
		require.NoError(t, err)
		require.Equal(t, int64(25), total)
		require.Empty(t, rows)
	})
}
