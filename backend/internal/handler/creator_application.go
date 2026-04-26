package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

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
		LastName:          req.LastName,
		FirstName:         req.FirstName,
		MiddleName:        req.MiddleName,
		IIN:               req.Iin,
		Phone:             req.Phone,
		City:              req.City,
		Address:           req.Address,
		CategoryCodes:     req.Categories,
		CategoryOtherText: req.CategoryOtherText,
		Socials:           apiSocialsToDomain(req.Socials),
		Consents:          domain.ConsentsInput{AcceptedAll: req.AcceptedAll},
		IPAddress:         middleware.ClientIPFromContext(r.Context()),
		UserAgent:         ua,
		AgreementVersion:  s.legalAgreementVersion,
		PrivacyVersion:    s.legalPrivacyVersion,
		Now:               time.Now().UTC(),
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

// GetCreatorApplication handles GET /creators/applications/{id} (admin-only).
//
// Authorisation runs first — non-admin callers see 403 without learning whether
// the application exists. The bearer-token check itself happens upstream in the
// AuthFromScopes middleware (driven by the OpenAPI security clause), so a
// missing/invalid token never reaches this handler. sql.ErrNoRows from the
// service is mapped to 404 NOT_FOUND by respondError. The handler deliberately
// logs nothing about the response body — every persisted field is PII-bearing
// (iin, names, phone, address) and must not surface in stdout логах приложения
// per docs/standards/security.md.
func (s *Server) GetCreatorApplication(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if err := s.authzService.CanViewCreatorApplication(r.Context()); err != nil {
		respondError(w, r, err, s.logger)
		return
	}

	detail, err := s.creatorApplicationService.GetByID(r.Context(), id.String())
	if err != nil {
		respondError(w, r, err, s.logger)
		return
	}

	respondJSON(w, r, http.StatusOK, api.GetCreatorApplicationResult{
		Data: domainCreatorApplicationDetailToAPI(id, detail),
	}, s.logger)
}

// domainCreatorApplicationDetailToAPI maps the domain aggregate onto the
// generated response struct. Status, platform and consent_type cast directly —
// OpenAPI enums and domain string constants share the same canonical values by
// construction. The path-level UUID is reused as the response id; the DB-level
// id string is the same value but only available as plain string in domain.
func domainCreatorApplicationDetailToAPI(id openapi_types.UUID, d *domain.CreatorApplicationDetail) api.CreatorApplicationDetailData {
	cats := make([]api.CreatorApplicationDetailCategory, len(d.Categories))
	for i, c := range d.Categories {
		cats[i] = api.CreatorApplicationDetailCategory{
			Code:      c.Code,
			Name:      c.Name,
			SortOrder: c.SortOrder,
		}
	}
	socs := make([]api.CreatorApplicationDetailSocial, len(d.Socials))
	for i, sc := range d.Socials {
		socs[i] = api.CreatorApplicationDetailSocial{
			Platform: api.SocialPlatform(sc.Platform),
			Handle:   sc.Handle,
		}
	}
	cons := make([]api.CreatorApplicationDetailConsent, len(d.Consents))
	for i, c := range d.Consents {
		cons[i] = api.CreatorApplicationDetailConsent{
			ConsentType:     api.CreatorApplicationDetailConsentConsentType(c.ConsentType),
			AcceptedAt:      c.AcceptedAt,
			DocumentVersion: c.DocumentVersion,
			IpAddress:       c.IPAddress,
			UserAgent:       c.UserAgent,
		}
	}
	return api.CreatorApplicationDetailData{
		Id:                id,
		LastName:          d.LastName,
		FirstName:         d.FirstName,
		MiddleName:        d.MiddleName,
		Iin:               d.IIN,
		BirthDate:         openapi_types.Date{Time: d.BirthDate},
		Phone:             d.Phone,
		City:              d.City,
		Address:           d.Address,
		CategoryOtherText: d.CategoryOtherText,
		Status:            api.CreatorApplicationDetailDataStatus(d.Status),
		CreatedAt:         d.CreatedAt,
		UpdatedAt:         d.UpdatedAt,
		Categories:        cats,
		Socials:           socs,
		Consents:          cons,
	}
}

