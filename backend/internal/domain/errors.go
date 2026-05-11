package domain

import (
	"errors"
	"fmt"
)

// Error response codes — machine-readable codes in API responses.
const (
	CodeValidation   = "VALIDATION_ERROR"
	CodeNotFound     = "NOT_FOUND"
	CodeForbidden    = "FORBIDDEN"
	CodeUnauthorized = "UNAUTHORIZED"
	CodeConflict     = "CONFLICT"
	CodeInternal     = "INTERNAL_ERROR"
)

// Campaign user-facing codes carried in 4xx responses by POST /campaigns.
const (
	// 422 — name is empty after trim.
	CodeCampaignNameRequired = "CAMPAIGN_NAME_REQUIRED"
	// 422 — name exceeds the configured upper bound.
	CodeCampaignNameTooLong = "CAMPAIGN_NAME_TOO_LONG"
	// 422 — tmaUrl exceeds the configured upper bound.
	CodeCampaignTmaURLTooLong = "CAMPAIGN_TMA_URL_TOO_LONG"
	// 409 — partial unique index `campaigns_name_active_unique` was violated:
	// another non-deleted campaign already uses this name.
	CodeCampaignNameTaken = "CAMPAIGN_NAME_TAKEN"
	// 404 — GET /campaigns/{id} could not find a campaign with this id.
	CodeCampaignNotFound = "CAMPAIGN_NOT_FOUND"
	// 422 — POST /creators/applications/{id}/approve received `campaignIds`
	// with more entries than the per-call cap (matches OpenAPI maxItems=20;
	// oapi-codegen does not enforce schema limits at runtime).
	CodeCampaignIdsTooMany = "CAMPAIGN_IDS_TOO_MANY"
	// 422 — POST /creators/applications/{id}/approve received `campaignIds`
	// with duplicate UUIDs.
	CodeCampaignIdsDuplicates = "CAMPAIGN_IDS_DUPLICATES"
	// 422 — POST /creators/applications/{id}/approve received `campaignIds`
	// where at least one campaign is missing or soft-deleted at pre-validation
	// time. Single code for both cases — admin re-fetches the campaign list and
	// retries.
	CodeCampaignNotAvailableForAdd = "CAMPAIGN_NOT_AVAILABLE_FOR_ADD"
	// 422 — POST /creators/applications/{id}/approve transactional approve
	// committed and Telegram-notify already fired, but the per-campaign add
	// loop hit a failure on the N-th campaign (mid-cycle race, e.g. campaign
	// soft-deleted between pre-validation and the add). Earlier campaigns
	// remain attached; admin must finish the rest manually.
	CodeCampaignAddAfterApproveFailed = "CAMPAIGN_ADD_AFTER_APPROVE_FAILED"
)

// Campaign-creator user-facing codes carried in 4xx responses by the
// admin-only batch add / single remove / list / notify / remind endpoints.
const (
	// 422 — POST /campaigns/{id}/creators with empty creatorIds.
	CodeCampaignCreatorIdsRequired = "CAMPAIGN_CREATOR_IDS_REQUIRED"
	// 422 — POST /campaigns/{id}/creators with more creatorIds than the
	// per-batch cap (matches OpenAPI maxItems=200; oapi-codegen does not
	// enforce schema limits at runtime, so the handler guards explicitly).
	CodeCampaignCreatorIdsTooMany = "CAMPAIGN_CREATOR_IDS_TOO_MANY"
	// 422 — POST /campaigns/{id}/creators with duplicate creatorIds.
	CodeCampaignCreatorIdsDuplicates = "CAMPAIGN_CREATOR_IDS_DUPLICATES"
	// 404 — DELETE /campaigns/{id}/creators/{creatorId} when the (campaign,
	// creator) pair is not in campaign_creators.
	CodeCampaignCreatorNotFound = "CAMPAIGN_CREATOR_NOT_FOUND"
	// 422 — POST /campaigns/{id}/creators when the (campaign, creator) pair
	// already exists; the unique-index race translates here.
	CodeCreatorAlreadyInCampaign = "CREATOR_ALREADY_IN_CAMPAIGN"
	// 422 — DELETE /campaigns/{id}/creators/{creatorId} once the row has
	// reached status=agreed; the row stays for the downstream TrustMe flow.
	CodeCampaignCreatorRemoveAfterAgreed = "CAMPAIGN_CREATOR_REMOVE_AFTER_AGREED"
	// 422 — POST /campaigns/{id}/notify or remind-invitation when at least one
	// creatorId in the batch is not attached to this campaign or sits in a
	// status incompatible with the action. Returned with a custom response
	// schema carrying `details` (one entry per offending creator).
	CodeCampaignCreatorBatchInvalid = "CAMPAIGN_CREATOR_BATCH_INVALID"
)

// Campaign tma_url lock code — PATCH guard.
const (
	// 422 — PATCH /campaigns/{id} that flips tma_url while at least one
	// creator in this campaign has invited_count > 0. Outbound bot messages
	// embed the previous tma_url via inline web_app button, so changing it
	// would silently strand creators on a dead link.
	CodeCampaignTmaURLLocked = "CAMPAIGN_TMA_URL_LOCKED"
)

const (
	CodeContractRequired           = "CONTRACT_REQUIRED"
	CodeContractInvalidPDF         = "CONTRACT_INVALID_PDF"
	CodeContractMissingPlaceholder = "CONTRACT_MISSING_PLACEHOLDER"
	CodeContractUnknownPlaceholder = "CONTRACT_UNKNOWN_PLACEHOLDER"
	CodeContractTemplateNotFound   = "CONTRACT_TEMPLATE_NOT_FOUND"
	// CodeContractTemplateRequired — POST /campaigns/{id}/notify upholds it
	// when the campaign has no contract_template_pdf uploaded yet.
	CodeContractTemplateRequired = "CONTRACT_TEMPLATE_REQUIRED"
)

// ErrContractTrustMeDocumentIDTaken — defensive translation of
// `contracts_trustme_document_id_unique` (23505). Should never fire under
// normal flow (TrustMe issues distinct ids) but kept so a Postgres-level
// race is observable as a domain sentinel rather than a raw pg error.
var ErrContractTrustMeDocumentIDTaken = errors.New("contract trustme_document_id already taken")

// TMA decision + secret_token codes — surfaced by /tma/* endpoints and by
// POST/PATCH /campaigns when the admin-provided tma_url fails the
// secret-token-format check or collides with another live campaign.
const (
	// 422 — POST/PATCH /campaigns supplied a tma_url whose last path segment
	// is not the secret_token format (`^[A-Za-z0-9_-]{16,}$`) or the URL
	// itself does not parse / lacks a scheme/host.
	CodeInvalidTmaURL = "INVALID_TMA_URL"
	// 422 — POST/PATCH /campaigns hit the partial UNIQUE on
	// `campaigns_secret_token_uniq`: another live campaign already uses this
	// secret_token (last path segment of tma_url).
	CodeTmaURLConflict = "TMA_URL_CONFLICT"
	// 422 — TMA decision (agree/decline) when the row is `planned` —
	// invitation has not been delivered yet, the creator should wait.
	CodeCampaignCreatorNotInvited = "CAMPAIGN_CREATOR_NOT_INVITED"
	// 422 — TMA decline when the row is already `agreed` — agreement is
	// final from the creator-side; reversal lives in admin flow.
	CodeCampaignCreatorAlreadyAgreed = "CAMPAIGN_CREATOR_ALREADY_AGREED"
	// 422 — TMA agree when the row is already `declined` — the admin must
	// re-invite (status flips back to `invited`) before the creator can agree.
	CodeCampaignCreatorDeclinedNeedReinvite = "CAMPAIGN_CREATOR_DECLINED_NEED_REINVITE"
	// 403 — TMA decision when the resolved telegram_user_id is not a known
	// creator OR is a creator but not attached to the requested campaign.
	// Same code + message for both branches: anti-fingerprint between
	// "not registered" and "not in campaign".
	CodeTMAForbidden = "TMA_FORBIDDEN"
	// 422 — PATCH /campaigns/{id}/creators/{creatorId} on a row that is not in
	// status=signed when a ticketSent toggle is requested.
	CodeCampaignCreatorTicketSentBadStatus = "CAMPAIGN_CREATOR_TICKET_SENT_BAD_STATUS"
	// 422 — PATCH /campaigns/{id}/creators/{creatorId} with no toggleable
	// fields supplied — defensive guard against accidental no-op calls.
	CodeCampaignCreatorPatchEmpty = "CAMPAIGN_CREATOR_PATCH_EMPTY"
)

// Sentinel domain errors — handlers map these to HTTP status codes.
var (
	ErrNotFound      = errors.New("not found")
	ErrForbidden     = errors.New("forbidden")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrConflict      = errors.New("conflict")
	ErrAlreadyExists = errors.New("already exists")
)

// ValidationError carries a machine-readable code and a human-readable fallback message.
type ValidationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewValidationError creates a domain validation error with a string code.
func NewValidationError(code, message string) *ValidationError {
	return &ValidationError{Code: code, Message: message}
}

// BusinessError wraps a domain-specific error with a machine-readable code.
type BusinessError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

func (e *BusinessError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *BusinessError) Unwrap() error {
	return e.Err
}

// NewBusinessError creates a domain business error (e.g. CAMPAIGN_FULL).
func NewBusinessError(code, message string) *BusinessError {
	return &BusinessError{Code: code, Message: message}
}

// ErrorCode extracts the user-facing code from a wrapped error chain. Returns
// empty string when the error does not carry a domain code — caller picks a
// sentinel like "non_domain". Used by structured logging where leaking the raw
// error string risks PII contamination (security.md § PII в логах) but the
// code itself is enum-safe.
func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve.Code
	}
	var be *BusinessError
	if errors.As(err, &be) {
		return be.Code
	}
	return ""
}
