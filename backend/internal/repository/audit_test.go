package repository

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
)

func TestAuditCreate_SQL(t *testing.T) {
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

	assert.Equal(t,
		"INSERT INTO audit_logs (action,actor_id,actor_role,entity_id,entity_type,ip_address,new_value,old_value) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)",
		*gotSQL)
	assert.Equal(t, []any{"brand_create", "u-1", "admin", entityID, "brand", "127.0.0.1", newVal, json.RawMessage(nil)}, *gotArgs)
}
