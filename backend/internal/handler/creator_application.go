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
		s.buildTelegramBotURL(request.Id.String()),
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
	telegramBotURL string,
) (api.CreatorApplicationDetailData, error) {
	status, err := mapCreatorApplicationStatusToAPI(d.Status)
	if err != nil {
		return api.CreatorApplicationDetailData{}, err
	}
	cats := make([]api.DictionaryItem, len(d.Categories))
	for i, code := range d.Categories {
		cats[i] = resolveDictionaryItem(code, categoriesByCode)
	}
	slices.SortFunc(cats, sortDictionaryItem)

	socs := make([]api.CreatorApplicationDetailSocial, len(d.Socials))
	for i, sc := range d.Socials {
		mapped, err := domainCreatorApplicationDetailSocialToAPI(sc)
		if err != nil {
			return api.CreatorApplicationDetailData{}, err
		}
		socs[i] = mapped
	}
	cons := make([]api.CreatorApplicationDetailConsent, len(d.Consents))
	for i, c := range d.Consents {
		cons[i] = api.CreatorApplicationDetailConsent{
			ConsentType:     api.ConsentType(c.ConsentType),
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
	var rejection *api.CreatorApplicationRejection
	if d.Rejection != nil {
		fromStatus, err := mapCreatorApplicationStatusToAPI(d.Rejection.FromStatus)
		if err != nil {
			return api.CreatorApplicationDetailData{}, err
		}
		rejectedBy, err := uuid.Parse(d.Rejection.RejectedByUserID)
		if err != nil {
			return api.CreatorApplicationDetailData{}, fmt.Errorf("parse rejected_by_user_id %q: %w", d.Rejection.RejectedByUserID, err)
		}
		rejection = &api.CreatorApplicationRejection{
			FromStatus:       fromStatus,
			RejectedAt:       d.Rejection.RejectedAt,
			RejectedByUserId: rejectedBy,
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
		City:              resolveDictionaryItem(d.CityCode, cityByCode),
		Address:           d.Address,
		CategoryOtherText: d.CategoryOtherText,
		Status:            status,
		VerificationCode:  d.VerificationCode,
		CreatedAt:         d.CreatedAt,
		UpdatedAt:         d.UpdatedAt,
		Categories:        cats,
		Socials:           socs,
		Consents:          cons,
		TelegramLink:      tgLink,
		TelegramBotUrl:    telegramBotURL,
		Rejection:         rejection,
	}, nil
}

func mapCreatorApplicationStatusToAPI(s string) (api.CreatorApplicationStatus, error) {
	status := api.CreatorApplicationStatus(s)
	if !status.Valid() {
		return "", fmt.Errorf("unknown creator application status %q", s)
	}
	return status, nil
}

// resolveDictionaryItem looks up a dictionary code (category or city) in the
// indexed dictionary and falls back to a code-only stub when the entry has
// been deactivated. The stub keeps the JSON shape identical so admins still
// see something meaningful instead of getting a 500 when a creator's
// historical reference was retired.
func resolveDictionaryItem(code string, byCode map[string]domain.DictionaryEntry) api.DictionaryItem {
	if entry, ok := byCode[code]; ok {
		return api.DictionaryItem{
			Code:      entry.Code,
			Name:      entry.Name,
			SortOrder: entry.SortOrder,
		}
	}
	return api.DictionaryItem{Code: code, Name: code, SortOrder: 0}
}

// domainCreatorApplicationDetailSocialToAPI maps a domain social row onto its
// API DTO, including the social uuid and the four verification fields. UUID
// parsing failure on either ID or VerifiedByUserID surfaces as a wrapped
// error so the strict-server adapter converts it into 500 — both fields come
// from DB UUID columns, so a parse failure is genuine corruption rather than
// user input.
func domainCreatorApplicationDetailSocialToAPI(s domain.CreatorApplicationDetailSocial) (api.CreatorApplicationDetailSocial, error) {
	socialID, err := uuid.Parse(s.ID)
	if err != nil {
		return api.CreatorApplicationDetailSocial{}, fmt.Errorf("parse social id %q: %w", s.ID, err)
	}
	out := api.CreatorApplicationDetailSocial{
		Id:       socialID,
		Platform: api.SocialPlatform(s.Platform),
		Handle:   s.Handle,
		Verified: s.Verified,
	}
	if s.Method != nil {
		m := api.SocialVerificationMethod(*s.Method)
		out.Method = &m
	}
	if s.VerifiedByUserID != nil {
		u, err := uuid.Parse(*s.VerifiedByUserID)
		if err != nil {
			return api.CreatorApplicationDetailSocial{}, fmt.Errorf("parse verified_by_user_id %q: %w", *s.VerifiedByUserID, err)
		}
		out.VerifiedByUserId = &u
	}
	if s.VerifiedAt != nil {
		t := *s.VerifiedAt
		out.VerifiedAt = &t
	}
	return out, nil
}

// sortDictionaryItem sorts dictionary items by (sortOrder, code) so the
// detail and list responses share a deterministic order.
func sortDictionaryItem(a, b api.DictionaryItem) int {
	if a.SortOrder != b.SortOrder {
		return a.SortOrder - b.SortOrder
	}
	return strings.Compare(a.Code, b.Code)
}

// VerifyCreatorApplicationSocial handles
// POST /creators/applications/{id}/socials/{socialId}/verify (admin-only).
//
// The action marks one social account as `manual`-verified under the admin's
// responsibility and transitions the application from `verification` to
// `moderation`. Authorisation runs first — non-admin callers see 403 without
// the service ever being asked. The actor uuid comes from the bearer token
// via middleware.UserIDFromContext; no body field is accepted to keep the
// surface area minimal. Sentinel errors from the service map to user-facing
// codes through respondError; success returns an empty object so the caller
// refetches the application aggregate to observe state changes.
func (s *Server) VerifyCreatorApplicationSocial(ctx context.Context, request api.VerifyCreatorApplicationSocialRequestObject) (api.VerifyCreatorApplicationSocialResponseObject, error) {
	if err := s.authzService.CanVerifyCreatorApplicationSocialManually(ctx); err != nil {
		return nil, err
	}
	actorUserID := middleware.UserIDFromContext(ctx)
	if err := s.creatorApplicationService.VerifyApplicationSocialManually(
		ctx, request.Id.String(), request.SocialId.String(), actorUserID,
	); err != nil {
		return nil, err
	}
	return api.VerifyCreatorApplicationSocial200JSONResponse{}, nil
}

// RejectCreatorApplication handles POST /creators/applications/{id}/reject
// (admin-only). It moves an application to the terminal `rejected` status
// from either `verification` or `moderation`. Authorisation runs first so
// non-admin callers see 403 without the service ever being asked. The actor
// uuid comes from the bearer token via middleware.UserIDFromContext; no body
// is accepted (no internal note, no category, no creator-facing message —
// the Telegram template is static and ships separately in chunk 14). Sentinel
// errors from the service map to user-facing codes through respondError;
// success returns an empty object so the caller refetches the aggregate to
// observe the new state and the populated rejection block.
func (s *Server) RejectCreatorApplication(ctx context.Context, request api.RejectCreatorApplicationRequestObject) (api.RejectCreatorApplicationResponseObject, error) {
	if err := s.authzService.CanRejectCreatorApplication(ctx); err != nil {
		return nil, err
	}
	actorUserID := middleware.UserIDFromContext(ctx)
	if err := s.creatorApplicationService.RejectApplication(ctx, request.Id.String(), actorUserID); err != nil {
		return nil, err
	}
	return api.RejectCreatorApplication200JSONResponse{}, nil
}

// GetCreatorApplicationsCounts handles GET /creators/applications/counts
// (admin-only). It returns one (status, count) pair per status that currently
// has at least one application — the response is **sparse** by design, see the
// operation description in openapi.yaml. Sorting items alphabetically by
// `status` keeps the wire output deterministic so frontends and tests can
// reason about the ordering without extra plumbing. Authorization is checked
// before any DB call: a non-admin caller gets 403 regardless of the underlying
// counts.
func (s *Server) GetCreatorApplicationsCounts(ctx context.Context, _ api.GetCreatorApplicationsCountsRequestObject) (api.GetCreatorApplicationsCountsResponseObject, error) {
	if err := s.authzService.CanGetCreatorApplicationsCounts(ctx); err != nil {
		return nil, err
	}
	counts, err := s.creatorApplicationService.Counts(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]api.CreatorApplicationStatusCount, 0, len(counts))
	for status, count := range counts {
		items = append(items, api.CreatorApplicationStatusCount{
			Status: api.CreatorApplicationStatus(status),
			Count:  count,
		})
	}
	slices.SortFunc(items, func(a, b api.CreatorApplicationStatusCount) int {
		return strings.Compare(string(a.Status), string(b.Status))
	})
	return api.GetCreatorApplicationsCounts200JSONResponse{
		Data: api.CreatorApplicationsCountsData{Items: items},
	}, nil
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
func apiListStatusesToDomain(in *[]api.CreatorApplicationStatus) ([]string, error) {
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
		status, err := mapCreatorApplicationStatusToAPI(item.Status)
		if err != nil {
			return api.CreatorApplicationsListData{}, err
		}

		cats := make([]api.DictionaryItem, len(item.Categories))
		for j, code := range item.Categories {
			cats[j] = resolveDictionaryItem(code, categoriesByCode)
		}
		slices.SortFunc(cats, sortDictionaryItem)

		socials := make([]api.CreatorApplicationDetailSocial, len(item.Socials))
		for j, sc := range item.Socials {
			mapped, err := domainCreatorApplicationDetailSocialToAPI(sc)
			if err != nil {
				return api.CreatorApplicationsListData{}, err
			}
			socials[j] = mapped
		}

		items[i] = api.CreatorApplicationListItem{
			Id:             appID,
			Status:         status,
			LastName:       item.LastName,
			FirstName:      item.FirstName,
			MiddleName:     item.MiddleName,
			BirthDate:      openapi_types.Date{Time: item.BirthDate},
			City:           resolveDictionaryItem(item.CityCode, cityByCode),
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
