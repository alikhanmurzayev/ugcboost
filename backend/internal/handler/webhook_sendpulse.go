package handler

import (
	"context"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// SendPulseInstagramWebhook handles SendPulse → IG verification calls.
// Bearer is validated upstream by middleware.SendPulseAuth (401 `{}` on
// failure). Always returns 200 `{}` past auth — anti-fingerprint.
func (s *Server) SendPulseInstagramWebhook(ctx context.Context, request api.SendPulseInstagramWebhookRequestObject) (api.SendPulseInstagramWebhookResponseObject, error) {
	body := request.Body
	if body == nil {
		s.logger.Warn(ctx, "sendpulse webhook: empty body", "outcome", "noop_no_body")
		return api.SendPulseInstagramWebhook200JSONResponse{}, nil
	}

	code, ok := domain.ParseVerificationCode(body.LastMessage)
	if !ok {
		s.logger.Debug(ctx, "sendpulse webhook: code not found in message", "outcome", "noop_no_code")
		return api.SendPulseInstagramWebhook200JSONResponse{}, nil
	}

	status, err := s.creatorApplicationService.VerifyInstagramByCode(ctx, code, body.Username)
	if err != nil {
		// Strict-server maps to 500 via respondError; the wrapping middleware
		// in main.go forces 200 `{}` so SendPulse retries per its own policy
		// without leaking infra hints.
		return nil, err
	}

	// verification_code is the secret SendPulse matches against — never log it.
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

	return api.SendPulseInstagramWebhook200JSONResponse{}, nil
}
