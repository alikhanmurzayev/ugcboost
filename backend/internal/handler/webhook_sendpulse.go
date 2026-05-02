package handler

import (
	"context"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// SendPulseInstagramWebhook is the SendPulse-driven Instagram DM webhook.
//
// The bearer token is validated by middleware.SendPulseAuth before this
// handler runs (any mismatch yields 401 with an empty body before the
// strict-server adapter takes over). Inside the handler the only job is to:
//
//  1. parse the verification code out of the message body — missing token
//     is a no-op debug-log, never a 4xx;
//  2. delegate to CreatorApplicationService.VerifyInstagramByCode for the
//     actual side effects (DB writes, audit, transition, post-commit
//     Telegram notification — all owned by the service).
//
// The response is always 200 with an empty JSON body, even on no-op outcomes
// — anti-fingerprinting per the operation description.
func (s *Server) SendPulseInstagramWebhook(ctx context.Context, request api.SendPulseInstagramWebhookRequestObject) (api.SendPulseInstagramWebhookResponseObject, error) {
	body := request.Body
	if body == nil {
		s.logger.Debug(ctx, "sendpulse webhook: empty body", "outcome", "noop_no_body")
		return sendPulseEmptyOK(), nil
	}

	code, ok := domain.ParseVerificationCode(body.LastMessage)
	if !ok {
		s.logger.Debug(ctx, "sendpulse webhook: code not found in message", "outcome", "noop_no_code")
		return sendPulseEmptyOK(), nil
	}

	status, err := s.creatorApplicationService.VerifyInstagramByCode(ctx, code, body.Username)
	if err != nil {
		// Infra failure: log and bubble. Strict-server maps to 500 via
		// respondError; the wrapping middleware in main.go forces a 200
		// `{}` response anyway so SendPulse can retry per its own policy
		// without the body leaking infra hints.
		return nil, err
	}

	// Outcome-only logs: verification_code is the secret SendPulse matches
	// against (per testapi.go contract), so it never lands in stdout. The
	// outcome label is enough for ops to read the funnel.
	switch status {
	case domain.VerifyInstagramStatusVerified:
		s.logger.Info(ctx, "sendpulse webhook: instagram verified", "outcome", string(status))
	case domain.VerifyInstagramStatusNoop:
		s.logger.Debug(ctx, "sendpulse webhook: already verified", "outcome", string(status))
	case domain.VerifyInstagramStatusNotFound:
		s.logger.Warn(ctx, "sendpulse webhook: no active application for code", "outcome", string(status))
	case domain.VerifyInstagramStatusNoIGSocial:
		s.logger.Warn(ctx, "sendpulse webhook: application has no instagram social", "outcome", string(status))
	}

	return sendPulseEmptyOK(), nil
}

// sendPulseEmptyOK returns the canonical "empty JSON object" 200 response
// shared by every webhook outcome — generated SendPulseInstagramWebhookResult
// is map[string]interface{}, so an empty (non-nil) map serialises to "{}".
func sendPulseEmptyOK() api.SendPulseInstagramWebhook200JSONResponse {
	return api.SendPulseInstagramWebhook200JSONResponse{}
}
