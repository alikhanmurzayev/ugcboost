package handler

import (
	"context"
	"fmt"
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

	data, err := domainCreatorApplicationDetailToAPI(
		request.Id,
		detail,
		indexDictionaryByCode(categoryEntries),
		indexDictionaryByCode(cityEntries),
	)
	if err != nil {
		return nil, err
	}
	return api.GetCreatorApplication200JSONResponse{Data: data}, nil
}

// indexDictionaryByCode builds a code-keyed lookup over a freshly-fetched
// dictionary so the mapper can answer per-code reads in O(1). Each public
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

func domainCreatorApplicationDetailToAPI(
	id openapi_types.UUID,
	d *domain.CreatorApplicationDetail,
	categoriesByCode map[string]domain.DictionaryEntry,
	cityByCode map[string]domain.DictionaryEntry,
) (api.CreatorApplicationDetailData, error) {
	status, err := mapCreatorApplicationStatusToAPI(d.Status)
	if err != nil {
		return api.CreatorApplicationDetailData{}, err
	}
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
		Status:            status,
		CreatedAt:         d.CreatedAt,
		UpdatedAt:         d.UpdatedAt,
		Categories:        cats,
		Socials:           socs,
		Consents:          cons,
		TelegramLink:      tgLink,
	}, nil
}

func mapCreatorApplicationStatusToAPI(s string) (api.CreatorApplicationDetailDataStatus, error) {
	switch s {
	case domain.CreatorApplicationStatusVerification:
		return api.CreatorApplicationDetailDataStatusVerification, nil
	case domain.CreatorApplicationStatusModeration:
		return api.CreatorApplicationDetailDataStatusModeration, nil
	case domain.CreatorApplicationStatusAwaitingContract:
		return api.CreatorApplicationDetailDataStatusAwaitingContract, nil
	case domain.CreatorApplicationStatusContractSent:
		return api.CreatorApplicationDetailDataStatusContractSent, nil
	case domain.CreatorApplicationStatusSigned:
		return api.CreatorApplicationDetailDataStatusSigned, nil
	case domain.CreatorApplicationStatusRejected:
		return api.CreatorApplicationDetailDataStatusRejected, nil
	case domain.CreatorApplicationStatusWithdrawn:
		return api.CreatorApplicationDetailDataStatusWithdrawn, nil
	default:
		return "", fmt.Errorf("unknown creator application status %q", s)
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

// ListCreatorApplications handles POST /creators/applications/list (admin-only).
//
// Authorisation runs first so non-admin callers receive 403 without leaking
// whether matching applications exist. The request body carries PII (IIN,
// names, social handles) in its `search` field — that is the whole reason the
// endpoint is POST and not GET. oapi-codegen does NOT enforce OpenAPI's
// minimum/maximum/maxLength/maxItems at runtime, so each numeric and string
// bound is checked explicitly here: an unchecked search would feed a
// megabyte-long ILIKE pattern straight to Postgres, an unchecked Page would
// overflow `(Page-1)*PerPage` past int64, and unchecked age values would feed
// `make_interval(years => N)` arbitrary integers. The service is trusted to
// ignore search after trim. After the service returns a code-only page, the
// handler hydrates the dictionary names for category and city codes; the
// telegramLinked flag is precomputed in the SQL query (LEFT JOIN). The
// response body is intentionally lean (no phone/address/consents) — anything
// else would expose extra PII to the moderation list view.
func (s *Server) ListCreatorApplications(ctx context.Context, request api.ListCreatorApplicationsRequestObject) (api.ListCreatorApplicationsResponseObject, error) {
	if err := s.authzService.CanListCreatorApplications(ctx); err != nil {
		return nil, err
	}
	body := request.Body

	if !body.Sort.Valid() {
		return nil, domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Неподдерживаемое значение sort. Допустимы: %s",
				strings.Join(domain.CreatorApplicationListSortFieldValues, ", ")))
	}
	if !body.Order.Valid() {
		return nil, domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Неподдерживаемое значение order. Допустимы: %s",
				strings.Join(domain.SortOrderValues, ", ")))
	}
	if body.Page < domain.CreatorApplicationListPageMin || body.Page > domain.CreatorApplicationListPageMax {
		return nil, domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Параметр page должен быть в диапазоне %d..%d",
				domain.CreatorApplicationListPageMin, domain.CreatorApplicationListPageMax))
	}
	if body.PerPage < domain.CreatorApplicationListPerPageMin || body.PerPage > domain.CreatorApplicationListPerPageMax {
		return nil, domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Параметр perPage должен быть в диапазоне %d..%d",
				domain.CreatorApplicationListPerPageMin, domain.CreatorApplicationListPerPageMax))
	}
	if err := validateAgeBound("ageFrom", body.AgeFrom); err != nil {
		return nil, err
	}
	if err := validateAgeBound("ageTo", body.AgeTo); err != nil {
		return nil, err
	}
	if body.AgeFrom != nil && body.AgeTo != nil && *body.AgeFrom > *body.AgeTo {
		return nil, domain.NewValidationError(domain.CodeValidation, "ageFrom не может быть больше ageTo")
	}
	if err := validateDateBound("dateFrom", body.DateFrom); err != nil {
		return nil, err
	}
	if err := validateDateBound("dateTo", body.DateTo); err != nil {
		return nil, err
	}
	if body.DateFrom != nil && body.DateTo != nil && body.DateFrom.After(*body.DateTo) {
		return nil, domain.NewValidationError(domain.CodeValidation, "dateFrom не может быть позже dateTo")
	}
	search, err := validateSearch(body.Search)
	if err != nil {
		return nil, err
	}
	cities, err := validateCodeArray("cities", body.Cities, domain.CreatorApplicationListCityCodeMaxLen)
	if err != nil {
		return nil, err
	}
	categories, err := validateCodeArray("categories", body.Categories, domain.CreatorApplicationListCategoryCodeMaxLen)
	if err != nil {
		return nil, err
	}
	statuses, err := apiListStatusesToDomain(body.Statuses)
	if err != nil {
		return nil, err
	}

	in := domain.CreatorApplicationListInput{
		Statuses:       statuses,
		Cities:         cities,
		Categories:     categories,
		DateFrom:       body.DateFrom,
		DateTo:         body.DateTo,
		AgeFrom:        body.AgeFrom,
		AgeTo:          body.AgeTo,
		TelegramLinked: body.TelegramLinked,
		Search:         search,
		Sort:           string(body.Sort),
		Order:          string(body.Order),
		Page:           body.Page,
		PerPage:        body.PerPage,
	}

	page, err := s.creatorApplicationService.List(ctx, in)
	if err != nil {
		return nil, err
	}

	// Empty page: skip the dictionary round-trips, the response carries no
	// items to hydrate. Total/page/perPage still surface so the client can
	// keep its pagination state in sync.
	if len(page.Items) == 0 {
		return api.ListCreatorApplications200JSONResponse{
			Data: api.CreatorApplicationsListData{
				Items:   []api.CreatorApplicationListItem{},
				Total:   page.Total,
				Page:    page.Page,
				PerPage: page.PerPage,
			},
		}, nil
	}

	categoryEntries, err := s.dictionaryService.List(ctx, domain.DictionaryTypeCategories)
	if err != nil {
		return nil, err
	}
	cityEntries, err := s.dictionaryService.List(ctx, domain.DictionaryTypeCities)
	if err != nil {
		return nil, err
	}

	data, err := domainCreatorApplicationListPageToAPI(
		page,
		indexDictionaryByCode(categoryEntries),
		indexDictionaryByCode(cityEntries),
	)
	if err != nil {
		return nil, err
	}
	return api.ListCreatorApplications200JSONResponse{Data: data}, nil
}

// validateAgeBound enforces the OpenAPI min/max range on an optional age
// field. Catches negative values (silent no-op filters) and runaway integers
// that would overflow `make_interval(years => N+1)` in the repo.
func validateAgeBound(field string, age *int) error {
	if age == nil {
		return nil
	}
	if *age < domain.CreatorApplicationListAgeMin || *age > domain.CreatorApplicationListAgeMax {
		return domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Параметр %s должен быть в диапазоне %d..%d",
				field, domain.CreatorApplicationListAgeMin, domain.CreatorApplicationListAgeMax))
	}
	return nil
}

// validateDateBound rejects the Go zero-time. The openapi date-time decoder
// happily accepts "0001-01-01T00:00:00Z", which silently produces an
// effectively no-op `created_at >= year 0001` filter.
func validateDateBound(field string, t *time.Time) error {
	if t == nil {
		return nil
	}
	if t.IsZero() {
		return domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Параметр %s не должен быть нулевой датой", field))
	}
	return nil
}

// validateSearch trims the optional search string and enforces the OpenAPI
// length cap. An empty search after trim is returned as "" so the service /
// repo can ignore it; a search longer than the limit is rejected so a client
// cannot push a 1MB ILIKE pattern through the body limit.
func validateSearch(p *string) (string, error) {
	if p == nil {
		return "", nil
	}
	trimmed := strings.TrimSpace(*p)
	if len([]rune(trimmed)) > domain.CreatorApplicationListSearchMaxLen {
		return "", domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Параметр search не должен превышать %d символов",
				domain.CreatorApplicationListSearchMaxLen))
	}
	return trimmed, nil
}

// validateCodeArray enforces the openapi maxLength: 64 + minLength: 1 on each
// element of cities/categories filter arrays, plus a soft array-length cap
// (CreatorApplicationListFilterArrayMax) so a client cannot flood IN-clauses.
// Empty/whitespace-only items are rejected up front instead of silently
// passed through — the latter would produce surprising "no results" pages
// when a client buggy enough to send `[""]` tried to filter.
func validateCodeArray(field string, p *[]string, maxLen int) ([]string, error) {
	if p == nil || len(*p) == 0 {
		return nil, nil
	}
	if len(*p) > domain.CreatorApplicationListFilterArrayMax {
		return nil, domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Параметр %s не должен содержать более %d элементов",
				field, domain.CreatorApplicationListFilterArrayMax))
	}
	out := make([]string, 0, len(*p))
	seen := make(map[string]struct{}, len(*p))
	for _, raw := range *p {
		code := strings.TrimSpace(raw)
		if code == "" {
			return nil, domain.NewValidationError(domain.CodeValidation,
				fmt.Sprintf("Параметр %s содержит пустое значение", field))
		}
		if len(code) > maxLen {
			return nil, domain.NewValidationError(domain.CodeValidation,
				fmt.Sprintf("Длина значения в %s не должна превышать %d символов", field, maxLen))
		}
		if _, dup := seen[code]; dup {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	return out, nil
}

// apiListStatusesToDomain validates the optional statuses array and returns
// the deduplicated set of canonical status strings. Unknown values surface as
// 422 — we don't silently drop them, otherwise a typo would degrade to "no
// filter" with no signal to the operator.
func apiListStatusesToDomain(in *[]api.CreatorApplicationsListRequestStatuses) ([]string, error) {
	if in == nil || len(*in) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(*in))
	seen := make(map[string]struct{}, len(*in))
	for _, item := range *in {
		if !item.Valid() {
			return nil, domain.NewValidationError(domain.CodeValidation,
				fmt.Sprintf("Неизвестный статус: %q", string(item)))
		}
		s := string(item)
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out, nil
}

// domainCreatorApplicationListPageToAPI builds the JSON-serialisable page
// payload from the service result and the two dictionary indexes. Categories
// for each item are sorted by (sortOrder, code) to match the detail
// endpoint's contract. Deactivated categories/cities surface as
// {code, name=code, sortOrder=0} so historical data stays readable.
func domainCreatorApplicationListPageToAPI(
	page *domain.CreatorApplicationListPage,
	categoriesByCode map[string]domain.DictionaryEntry,
	cityByCode map[string]domain.DictionaryEntry,
) (api.CreatorApplicationsListData, error) {
	items := make([]api.CreatorApplicationListItem, len(page.Items))
	for i, item := range page.Items {
		appID, err := uuid.Parse(item.ID)
		if err != nil {
			return api.CreatorApplicationsListData{}, fmt.Errorf("parse application id %q: %w", item.ID, err)
		}
		status, err := mapCreatorApplicationStatusToListItemAPI(item.Status)
		if err != nil {
			return api.CreatorApplicationsListData{}, err
		}

		cats := make([]api.CreatorApplicationDetailCategory, len(item.Categories))
		for j, code := range item.Categories {
			cats[j] = resolveCategory(code, categoriesByCode)
		}
		slices.SortFunc(cats, func(a, b api.CreatorApplicationDetailCategory) int {
			if a.SortOrder != b.SortOrder {
				return a.SortOrder - b.SortOrder
			}
			return strings.Compare(a.Code, b.Code)
		})

		socials := make([]api.CreatorApplicationDetailSocial, len(item.Socials))
		for j, sc := range item.Socials {
			socials[j] = api.CreatorApplicationDetailSocial{
				Platform: api.SocialPlatform(sc.Platform),
				Handle:   sc.Handle,
			}
		}

		items[i] = api.CreatorApplicationListItem{
			Id:             appID,
			Status:         status,
			LastName:       item.LastName,
			FirstName:      item.FirstName,
			MiddleName:     item.MiddleName,
			BirthDate:      openapi_types.Date{Time: item.BirthDate},
			City:           resolveCity(item.CityCode, cityByCode),
			Categories:     cats,
			Socials:        socials,
			TelegramLinked: item.TelegramLinked,
			CreatedAt:      item.CreatedAt,
			UpdatedAt:      item.UpdatedAt,
		}
	}
	return api.CreatorApplicationsListData{
		Items:   items,
		Total:   page.Total,
		Page:    page.Page,
		PerPage: page.PerPage,
	}, nil
}

// mapCreatorApplicationStatusToListItemAPI reuses the detail mapper and casts
// to the list-item enum type. Both API enums share the same string values, so
// the cast is safe; routing through the existing function keeps the
// "unknown status" branch in one place.
func mapCreatorApplicationStatusToListItemAPI(s string) (api.CreatorApplicationListItemStatus, error) {
	detail, err := mapCreatorApplicationStatusToAPI(s)
	if err != nil {
		return "", err
	}
	return api.CreatorApplicationListItemStatus(detail), nil
}
