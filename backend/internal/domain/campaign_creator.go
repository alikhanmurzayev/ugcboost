package domain

import (
	"errors"
	"time"
)

// CampaignCreator is the domain projection of one campaign_creators row. JSON
// tags are snake_case because the struct is serialized into audit_logs as-is
// — adding a field to the struct extends the audit payload automatically.
type CampaignCreator struct {
	ID            string     `json:"id"`
	CampaignID    string     `json:"campaign_id"`
	CreatorID     string     `json:"creator_id"`
	Status        string     `json:"status"`
	InvitedAt     *time.Time `json:"invited_at,omitempty"`
	InvitedCount  int        `json:"invited_count"`
	RemindedAt    *time.Time `json:"reminded_at,omitempty"`
	RemindedCount int        `json:"reminded_count"`
	DecidedAt     *time.Time `json:"decided_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// CampaignCreator status values. Stored as TEXT + CHECK in the schema; the
// service is the single writer, so these constants are the source of truth.
const (
	CampaignCreatorStatusPlanned  = "planned"
	CampaignCreatorStatusInvited  = "invited"
	CampaignCreatorStatusDeclined = "declined"
	CampaignCreatorStatusAgreed   = "agreed"
)

// ErrCreatorAlreadyInCampaign is raised when a 23505 fires on
// campaign_creators_campaign_creator_unique — i.e. the (campaign, creator)
// pair was already attached. respondError maps any *ValidationError to 422
// with the carried code.
var ErrCreatorAlreadyInCampaign = NewValidationError(
	CodeCreatorAlreadyInCampaign,
	"Креатор уже добавлен в эту кампанию.",
)

// ErrCampaignCreatorRemoveAfterAgreed blocks DELETE once the row has reached
// status=agreed — the row remains for the downstream TrustMe flow.
var ErrCampaignCreatorRemoveAfterAgreed = NewValidationError(
	CodeCampaignCreatorRemoveAfterAgreed,
	"Нельзя удалить креатора, который уже согласился на участие в кампании.",
)

// ErrCampaignCreatorCreatorNotFound is raised on POST /campaigns/{id}/creators
// when one of the requested creatorIds does not exist (23503 on the
// creator-FK constraint). The whole batch rolls back per strict-422 contract.
var ErrCampaignCreatorCreatorNotFound = NewValidationError(
	CodeCreatorNotFound,
	"Один из переданных креаторов не найден. Обновите список креаторов и повторите.",
)

// ErrCampaignCreatorNotFound is raised by the service when DELETE refers to a
// (campaign, creator) pair that is not in campaign_creators. respondError
// maps it to 404 CAMPAIGN_CREATOR_NOT_FOUND.
var ErrCampaignCreatorNotFound = errors.New("campaign creator not found")
