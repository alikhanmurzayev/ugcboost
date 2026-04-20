package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
)

// SubmitCreatorApplication handles POST /creators/applications.
//
// The endpoint is public (no auth, no RBAC check). The handler parses the
// request body into the generated API type, hydrates domain.CreatorApplicationInput
// with request metadata (client IP, User-Agent, legal document versions,
// current time) and delegates to CreatorApplicationService. On success the
// response includes the application id and the Telegram bot deep-link.
func (s *Server) SubmitCreatorApplication(w http.ResponseWriter, r *http.Request) {
	var req api.CreatorApplicationSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"), s.logger)
		return
	}

	input := domain.CreatorApplicationInput{
		LastName:         req.LastName,
		FirstName:        req.FirstName,
		MiddleName:       req.MiddleName,
		IIN:              req.Iin,
		Phone:            req.Phone,
		City:             req.City,
		Address:          req.Address,
		CategoryCodes:    req.Categories,
		Socials:          apiSocialsToDomain(req.Socials),
		Consents:         apiConsentsToDomain(req.Consents),
		IPAddress:        middleware.ClientIPFromContext(r.Context()),
		UserAgent:        r.UserAgent(),
		AgreementVersion: s.legalAgreementVersion,
		PrivacyVersion:   s.legalPrivacyVersion,
		Now:              time.Now().UTC(),
	}

	submission, err := s.creatorApplicationService.Submit(r.Context(), input)
	if err != nil {
		respondError(w, r, err, s.logger)
		return
	}

	applicationID, err := uuid.Parse(submission.ApplicationID)
	if err != nil {
		s.logger.Error(r.Context(), "invalid application id returned from service", "error", err, "application_id", submission.ApplicationID)
		respondError(w, r, err, s.logger)
		return
	}

	respondJSON(w, r, http.StatusCreated, api.CreatorApplicationSubmitResult{
		Data: api.CreatorApplicationSubmitData{
			ApplicationId:  applicationID,
			TelegramBotUrl: s.buildTelegramBotURL(submission.ApplicationID),
		},
	}, s.logger)
}

// buildTelegramBotURL assembles the deep-link returned to the creator. The
// bot username is environment-specific (dev/staging share a test bot, prod has
// its own). ApplicationID is used directly as the start parameter so the bot
// can link the Telegram user back to their submission.
func (s *Server) buildTelegramBotURL(applicationID string) string {
	return "https://t.me/" + s.telegramBotUsername + "?start=" + applicationID
}

func apiSocialsToDomain(in []api.SocialAccountInput) []domain.SocialAccountInput {
	out := make([]domain.SocialAccountInput, len(in))
	for i, acc := range in {
		out[i] = domain.SocialAccountInput{
			Platform: string(acc.Platform),
			Handle:   acc.Handle,
		}
	}
	return out
}

func apiConsentsToDomain(in api.ConsentsInput) domain.ConsentsInput {
	return domain.ConsentsInput{
		Processing:  in.Processing,
		ThirdParty:  in.ThirdParty,
		CrossBorder: in.CrossBorder,
		Terms:       in.Terms,
	}
}
