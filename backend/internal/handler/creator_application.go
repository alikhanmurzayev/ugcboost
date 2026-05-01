package handler

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
)

func (s *Server) SubmitCreatorApplication(ctx context.Context, request api.SubmitCreatorApplicationRequestObject) (api.SubmitCreatorApplicationResponseObject, error) {
	req := request.Body

	input := domain.CreatorApplicationInput{
		LastName:          req.LastName,
		FirstName:         req.FirstName,
		MiddleName:        req.MiddleName,
		IIN:               req.Iin,
		Phone:             req.Phone,
		CityCode:          req.City,
		Address:           req.Address,
		CategoryCodes:     req.Categories,
		CategoryOtherText: req.CategoryOtherText,
		Socials:           apiSocialsToDomain(req.Socials),
		Consents:          domain.ConsentsInput{AcceptedAll: req.AcceptedAll},
		IPAddress:         middleware.ClientIPFromContext(ctx),
		UserAgent:         middleware.UserAgentFromContext(ctx),
		AgreementVersion:  s.legalAgreementVersion,
		PrivacyVersion:    s.legalPrivacyVersion,
		Now:               time.Now().UTC(),
	}

	submission, err := s.creatorApplicationService.Submit(ctx, input)
	if err != nil {
		return nil, err
	}

	applicationID, err := uuid.Parse(submission.ApplicationID)
	if err != nil {
		// The service always returns a DB-generated UUID here, so a parse
		// failure indicates a real bug. The strict-server adapter forwards
		// the error to ResponseErrorHandlerFunc → respondError, which logs
		// it via the default branch as 500.
		return nil, err
	}

	return api.SubmitCreatorApplication201JSONResponse{
		Data: api.CreatorApplicationSubmitData{
			ApplicationId:  applicationID,
			TelegramBotUrl: s.buildTelegramBotURL(submission.ApplicationID),
		},
	}, nil
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
// service is mapped to 404 NOT_FOUND by respondError. After the service layer
// returns the raw aggregate (codes only), the handler hydrates categories and
// city against the active dictionary so the JSON payload carries human-readable
// names; deactivated codes degrade to a `{code, name: code, sortOrder: 0}`
// fallback so historical applications stay readable. The handler deliberately
// logs nothing about the response body — every persisted field is PII-bearing
// (iin, names, phone, address) and must not surface in stdout логах приложения
// per docs/standards/security.md.
func (s *Server) GetCreatorApplication(ctx context.Context, request api.GetCreatorApplicationRequestObject) (api.GetCreatorApplicationResponseObject, error) {
	if err := s.authzService.CanViewCreatorApplication(ctx); err != nil {
		return nil, err
	}

	detail, err := s.creatorApplicationService.GetByID(ctx, request.Id.String())
	if err != nil {
		return nil, err
	}

	categoryEntries, err := s.dictionaryService.List(ctx, domain.DictionaryTypeCategories)
	if err != nil {
		return nil, err
	}
	cityEntries, err := s.dictionaryService.List(ctx, domain.DictionaryTypeCities)
	if err != nil {
		return nil, err
	}

	return api.GetCreatorApplication200JSONResponse{
		Data: domainCreatorApplicationDetailToAPI(
			request.Id,
			detail,
			indexDictionaryByCode(categoryEntries),
			indexDictionaryByCode(cityEntries),
		),
	}, nil
}

// indexDictionaryByCode builds a code-keyed lookup over a freshly-fetched
// dictionary so the maptter can answer per-code reads in O(1). Each public
// dictionary is small (≲100 entries) and the GET aggregate is admin-only and
// low-traffic, so building this map per request is cheap — caching is
// deliberately out of scope.
func indexDictionaryByCode(entries []domain.DictionaryEntry) map[string]domain.DictionaryEntry {
	m := make(map[string]domain.DictionaryEntry, len(entries))
	for _, e := range entries {
		m[e.Code] = e
	}
	return m
}

// domainCreatorApplicationDetailToAPI maps the domain aggregate onto the
// generated response struct. Status, platform and consent_type cast directly —
// OpenAPI enums and domain string constants share the same canonical values by
// construction. Categories and city are resolved against the active
// dictionary; codes that no longer exist (deactivated entries, legacy data)
// degrade to `{code, name: code, sortOrder: 0}` so the response stays
// well-formed instead of failing the whole read. Categories are sorted
// in-memory by (sortOrder, code) to keep the response deterministic
// independent of repo-side ordering.
func domainCreatorApplicationDetailToAPI(
	id openapi_types.UUID,
	d *domain.CreatorApplicationDetail,
	categoriesByCode map[string]domain.DictionaryEntry,
	cityByCode map[string]domain.DictionaryEntry,
) api.CreatorApplicationDetailData {
	cats := make([]api.CreatorApplicationDetailCategory, len(d.Categories))
	for i, code := range d.Categories {
		cats[i] = resolveCategory(code, categoriesByCode)
	}
	slices.SortFunc(cats, func(a, b api.CreatorApplicationDetailCategory) int {
		if a.SortOrder != b.SortOrder {
			return a.SortOrder - b.SortOrder
		}
		return strings.Compare(a.Code, b.Code)
	})

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
	var tgLink *api.TelegramLink
	if d.TelegramLink != nil {
		tgLink = &api.TelegramLink{
			TelegramUserId:    d.TelegramLink.TelegramUserID,
			TelegramUsername:  d.TelegramLink.TelegramUsername,
			TelegramFirstName: d.TelegramLink.TelegramFirstName,
			TelegramLastName:  d.TelegramLink.TelegramLastName,
			LinkedAt:          d.TelegramLink.LinkedAt,
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
		City:              resolveCity(d.CityCode, cityByCode),
		Address:           d.Address,
		CategoryOtherText: d.CategoryOtherText,
		Status:            api.CreatorApplicationDetailDataStatus(d.Status),
		CreatedAt:         d.CreatedAt,
		UpdatedAt:         d.UpdatedAt,
		Categories:        cats,
		Socials:           socs,
		Consents:          cons,
		TelegramLink:      tgLink,
	}
}

// resolveCategory looks up a category code in the dictionary index and falls
// back to a code-only stub when the entry has been deactivated. The stub
// keeps the JSON shape identical so admins still see something meaningful
// instead of getting a 500 when a creator's historical category was retired.
func resolveCategory(code string, byCode map[string]domain.DictionaryEntry) api.CreatorApplicationDetailCategory {
	if entry, ok := byCode[code]; ok {
		return api.CreatorApplicationDetailCategory{
			Code:      entry.Code,
			Name:      entry.Name,
			SortOrder: entry.SortOrder,
		}
	}
	return api.CreatorApplicationDetailCategory{Code: code, Name: code, SortOrder: 0}
}

// resolveCity mirrors resolveCategory for the single city stored against the
// application. Same fallback rule: a city that has been removed from the
// dictionary still surfaces under its raw code so the read does not fail.
func resolveCity(code string, byCode map[string]domain.DictionaryEntry) api.CreatorApplicationDetailCity {
	if entry, ok := byCode[code]; ok {
		return api.CreatorApplicationDetailCity{
			Code:      entry.Code,
			Name:      entry.Name,
			SortOrder: entry.SortOrder,
		}
	}
	return api.CreatorApplicationDetailCity{Code: code, Name: code, SortOrder: 0}
}
