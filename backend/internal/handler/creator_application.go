package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
)

// maxUserAgentLength caps the User-Agent string persisted with consent rows.
// Anything longer is truncated before the service layer touches it — the
// attacker-controlled header should not balloon DB rows or stdout logs.
const maxUserAgentLength = 1024

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

	ua := r.UserAgent()
	if len(ua) > maxUserAgentLength {
		ua = ua[:maxUserAgentLength]
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
		UserAgent:        ua,
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
		// The service always returns a DB-generated UUID here, so a parse
		// failure indicates a real bug. respondError already logs the wrapped
		// internal error via its default branch — no need for a second log.
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
// can link the Telegram user back to their submission. URL-escape guards
// against a malformed env var smuggling characters into the path or query.
func (s *Server) buildTelegramBotURL(applicationID string) string {
	return "https://t.me/" + url.PathEscape(s.telegramBotUsername) + "?start=" + url.QueryEscape(applicationID)
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
