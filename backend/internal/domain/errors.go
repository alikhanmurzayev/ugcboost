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
	// 422 — tmaUrl is empty after trim.
	CodeCampaignTmaURLRequired = "CAMPAIGN_TMA_URL_REQUIRED"
	// 422 — tmaUrl exceeds the configured upper bound.
	CodeCampaignTmaURLTooLong = "CAMPAIGN_TMA_URL_TOO_LONG"
	// 409 — partial unique index `campaigns_name_active_unique` was violated:
	// another non-deleted campaign already uses this name.
	CodeCampaignNameTaken = "CAMPAIGN_NAME_TAKEN"
	// 404 — GET /campaigns/{id} could not find a campaign with this id.
	CodeCampaignNotFound = "CAMPAIGN_NOT_FOUND"
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
