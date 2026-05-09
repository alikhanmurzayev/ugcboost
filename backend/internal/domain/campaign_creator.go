package domain

import (
	"errors"
	"fmt"
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
	CampaignCreatorStatusPlanned         = "planned"
	CampaignCreatorStatusInvited         = "invited"
	CampaignCreatorStatusDeclined        = "declined"
	CampaignCreatorStatusAgreed          = "agreed"
	CampaignCreatorStatusSigning         = "signing"
	CampaignCreatorStatusSigned          = "signed"
	CampaignCreatorStatusSigningDeclined = "signing_declined"
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

// CampaignCreator batch-invalid reason values — used in CampaignCreatorBatchInvalidError.
const (
	BatchInvalidReasonNotInCampaign = "not_in_campaign"
	BatchInvalidReasonWrongStatus   = "wrong_status"
)

// BatchValidationDetail is one entry in the strict-422 response of
// notify / remind-invitation: which creatorId was rejected, why, and (for
// wrong_status) what the current status is so the UI can phrase the error.
type BatchValidationDetail struct {
	CreatorID     string `json:"creator_id"`
	Reason        string `json:"reason"`
	CurrentStatus string `json:"current_status,omitempty"`
}

// CampaignCreatorBatchInvalidError carries the full set of batch validation
// failures for notify / remind-invitation. Validate-pass collects every
// offending creator before returning, so the admin UI gets the whole picture
// in one response. The handler renders this through a dedicated branch in
// respondError that emits the CAMPAIGN_CREATOR_BATCH_INVALID schema (rather
// than the generic ErrorResponse).
type CampaignCreatorBatchInvalidError struct {
	Details []BatchValidationDetail
}

func (e *CampaignCreatorBatchInvalidError) Error() string {
	return fmt.Sprintf("campaign creator batch invalid: %d details", len(e.Details))
}

// NotifyFailureReason values for the partial-success response of A4/A5.
const (
	NotifyFailureReasonBotBlocked = "bot_blocked"
	NotifyFailureReasonUnknown    = "unknown"
)

// NotifyFailure carries one creator that the bot could not reach during a
// notify / remind-invitation batch. Service returns these so the handler can
// surface them in the `undelivered` list of the 200 response.
type NotifyFailure struct {
	CreatorID string
	Reason    string
}

// CampaignCreatorDecision is the creator-side intent passed by the TMA
// agree/decline endpoints to TmaCampaignCreatorService.ApplyDecision.
type CampaignCreatorDecision string

const (
	CampaignCreatorDecisionAgree   CampaignCreatorDecision = "agree"
	CampaignCreatorDecisionDecline CampaignCreatorDecision = "decline"
)

// CampaignCreatorDecisionResult carries the post-decision row state plus an
// idempotency marker — `AlreadyDecided=true` means the row was already in
// the requested terminal state, so the service skipped both the UPDATE and
// the audit row. Handler renders both `Status` and `AlreadyDecided` to the
// client; UI uses `AlreadyDecided` to decide whether to show the "вы уже
// решили ранее" banner.
type CampaignCreatorDecisionResult struct {
	Status         string
	AlreadyDecided bool
}

// ErrCampaignCreatorNotInvited is raised by ApplyDecision when the row is
// in `planned` — invitation has not been delivered yet, the creator should
// wait for the admin notify to land.
var ErrCampaignCreatorNotInvited = NewValidationError(
	CodeCampaignCreatorNotInvited,
	"Приглашение ещё не отправлено. Дождитесь приглашения от менеджера.",
)

// ErrCampaignCreatorAlreadyAgreed is raised by ApplyDecision when an
// already-agreed row receives a decline — agreement is final from the
// creator side; reversal lives in admin flow.
var ErrCampaignCreatorAlreadyAgreed = NewValidationError(
	CodeCampaignCreatorAlreadyAgreed,
	"Вы уже согласились участвовать. Чтобы изменить решение, обратитесь к менеджеру.",
)

// ErrCampaignCreatorDeclinedNeedReinvite is raised by ApplyDecision when an
// already-declined row receives an agree — the admin must re-invite (status
// flips back to `invited`) before the creator can agree.
var ErrCampaignCreatorDeclinedNeedReinvite = NewValidationError(
	CodeCampaignCreatorDeclinedNeedReinvite,
	"Вы уже отказались. Чтобы согласиться, попросите менеджера прислать приглашение заново.",
)

// ErrTMAForbidden is raised by AuthzService.AuthorizeTMACampaignDecision
// when the resolved telegram_user_id is not a known creator OR is a creator
// but not attached to the requested campaign. Single sentinel for both
// branches by design — anti-fingerprint between "not registered" and
// "not in campaign". Mapped to 403 TMA_FORBIDDEN by respondError.
var ErrTMAForbidden = errors.New("tma forbidden")
