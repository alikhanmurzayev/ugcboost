package domain

import (
	"encoding/json"
	"time"
)

// AuditLog is the domain representation of an audit log entry.
type AuditLog struct {
	ID         string          `json:"id"`
	ActorID    string          `json:"actor_id"`
	ActorRole  string          `json:"actor_role"`
	Action     string          `json:"action"`
	EntityType string          `json:"entity_type"`
	EntityID   *string         `json:"entity_id"`
	OldValue   json.RawMessage `json:"old_value"`
	NewValue   json.RawMessage `json:"new_value"`
	IPAddress  string          `json:"ip_address"`
	CreatedAt  time.Time       `json:"created_at"`
}

// AuditFilter defines filter parameters for listing audit logs.
type AuditFilter struct {
	ActorID    string
	EntityType string
	EntityID   string
	Action     string
	DateFrom   *time.Time
	DateTo     *time.Time
}
