package handler

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// GetCreator handles GET /creators/{id} (admin-only).
//
// The authorisation check runs first so non-admin callers see 403 before any
// DB read; that keeps the response timing identical regardless of whether the
// creator exists. ErrCreatorNotFound from the service is mapped to 404
// CREATOR_NOT_FOUND by respondError. Like GetCreatorApplication, this handler
// deliberately logs nothing about the response body — every persisted field is
// PII-bearing (iin, names, phone, address, handles, telegram metadata) and
// must not surface in stdout-логах приложения per docs/standards/security.md.
func (s *Server) GetCreator(ctx context.Context, request api.GetCreatorRequestObject) (api.GetCreatorResponseObject, error) {
	if err := s.authzService.CanViewCreator(ctx); err != nil {
		return nil, err
	}

	aggregate, err := s.creatorService.GetByID(ctx, request.Id.String())
	if err != nil {
		return nil, err
	}

	data, err := domainCreatorAggregateToAPI(request.Id, aggregate)
	if err != nil {
		return nil, err
	}
	return api.GetCreator200JSONResponse{Data: data}, nil
}

// domainCreatorAggregateToAPI maps the service aggregate onto its strict-server
// counterpart. UUID parse failures on stored IDs (creator id is supplied via
// path and re-used as-is, but sourceApplicationId, social ids and
// verifiedByUserId come from DB columns) surface as wrapped errors so the
// strict-server adapter renders 500 — the rows are populated by trusted
// services so a parse failure means real corruption, not user input.
func domainCreatorAggregateToAPI(id openapi_types.UUID, a *domain.CreatorAggregate) (api.CreatorAggregate, error) {
	sourceApplicationID, err := uuid.Parse(a.SourceApplicationID)
	if err != nil {
		return api.CreatorAggregate{}, fmt.Errorf("parse source_application_id %q: %w", a.SourceApplicationID, err)
	}

	socials := make([]api.CreatorAggregateSocial, len(a.Socials))
	for i, social := range a.Socials {
		mapped, err := domainCreatorAggregateSocialToAPI(social)
		if err != nil {
			return api.CreatorAggregate{}, err
		}
		socials[i] = mapped
	}

	categories := make([]api.CreatorAggregateCategory, len(a.Categories))
	for i, c := range a.Categories {
		categories[i] = api.CreatorAggregateCategory{Code: c.Code, Name: c.Name}
	}

	campaigns := make([]api.CreatorCampaignBrief, len(a.Campaigns))
	for i, c := range a.Campaigns {
		campaignID, err := uuid.Parse(c.ID)
		if err != nil {
			return api.CreatorAggregate{}, fmt.Errorf("parse campaign id %q: %w", c.ID, err)
		}
		campaigns[i] = api.CreatorCampaignBrief{
			Id:     campaignID,
			Name:   c.Name,
			Status: api.CampaignCreatorStatus(c.Status),
		}
	}

	return api.CreatorAggregate{
		Id:                  id,
		Iin:                 a.IIN,
		SourceApplicationId: sourceApplicationID,
		LastName:            a.LastName,
		FirstName:           a.FirstName,
		MiddleName:          a.MiddleName,
		BirthDate:           openapi_types.Date{Time: a.BirthDate},
		Phone:               a.Phone,
		CityCode:            a.CityCode,
		CityName:            a.CityName,
		Address:             a.Address,
		CategoryOtherText:   a.CategoryOtherText,
		TelegramUserId:      a.TelegramUserID,
		TelegramUsername:    a.TelegramUsername,
		TelegramFirstName:   a.TelegramFirstName,
		TelegramLastName:    a.TelegramLastName,
		Socials:             socials,
		Categories:          categories,
		Campaigns:           campaigns,
		CreatedAt:           a.CreatedAt,
		UpdatedAt:           a.UpdatedAt,
	}, nil
}

// ListCreators handles POST /creators/list (admin-only).
//
// Authorisation runs first so non-admin callers receive 403 without leaking
// whether matching creators exist. The request body carries PII (IIN, names,
// social handles, phone, telegram_username) in its `search` field — that is
// the whole reason the endpoint is POST and not GET. oapi-codegen does NOT
// enforce OpenAPI's minimum/maximum/maxLength/maxItems at runtime, so each
// numeric and string bound is checked explicitly here: an unchecked search
// would feed a megabyte-long ILIKE pattern straight to Postgres, an unchecked
// Page would overflow `(Page-1)*PerPage` past int64, and unchecked age values
// would feed `make_interval(years => N)` arbitrary integers. The service is
// trusted to ignore search after trim. After the service returns a code-only
// page, the handler hydrates the dictionary names for category and city
// codes. The response body is intentionally lean — phone and telegram_username
// surface (admins copy these from the table); address, category_other_text
// and the full Telegram block stay in `GET /creators/{id}`.
func (s *Server) ListCreators(ctx context.Context, request api.ListCreatorsRequestObject) (api.ListCreatorsResponseObject, error) {
	if err := s.authzService.CanViewCreators(ctx); err != nil {
		return nil, err
	}
	body := request.Body

	if !body.Sort.Valid() {
		return nil, domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Неподдерживаемое значение sort. Допустимы: %s",
				strings.Join(domain.CreatorListSortFieldValues, ", ")))
	}
	if !body.Order.Valid() {
		return nil, domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Неподдерживаемое значение order. Допустимы: %s",
				strings.Join(domain.SortOrderValues, ", ")))
	}
	if body.Page < domain.CreatorListPageMin || body.Page > domain.CreatorListPageMax {
		return nil, domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Параметр page должен быть в диапазоне %d..%d",
				domain.CreatorListPageMin, domain.CreatorListPageMax))
	}
	if body.PerPage < domain.CreatorListPerPageMin || body.PerPage > domain.CreatorListPerPageMax {
		return nil, domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Параметр perPage должен быть в диапазоне %d..%d",
				domain.CreatorListPerPageMin, domain.CreatorListPerPageMax))
	}
	if err := validateCreatorAgeBound("ageFrom", body.AgeFrom); err != nil {
		return nil, err
	}
	if err := validateCreatorAgeBound("ageTo", body.AgeTo); err != nil {
		return nil, err
	}
	if body.AgeFrom != nil && body.AgeTo != nil && *body.AgeFrom > *body.AgeTo {
		return nil, domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("ageFrom (%d) не может быть больше ageTo (%d)", *body.AgeFrom, *body.AgeTo))
	}
	if err := validateDateBound("dateFrom", body.DateFrom); err != nil {
		return nil, err
	}
	if err := validateDateBound("dateTo", body.DateTo); err != nil {
		return nil, err
	}
	// Strict After: equality (DateFrom == DateTo) passes — narrows the filter
	// to a single moment, which is a legitimate use case.
	if body.DateFrom != nil && body.DateTo != nil && body.DateFrom.After(*body.DateTo) {
		return nil, domain.NewValidationError(domain.CodeValidation, "dateFrom не может быть позже dateTo")
	}
	search, err := validateCreatorSearch(body.Search)
	if err != nil {
		return nil, err
	}
	cities, err := validateCreatorCodeArray("cities", body.Cities, domain.CreatorListCityCodeMaxLen)
	if err != nil {
		return nil, err
	}
	categories, err := validateCreatorCodeArray("categories", body.Categories, domain.CreatorListCategoryCodeMaxLen)
	if err != nil {
		return nil, err
	}
	ids, err := validateCreatorIDs(body.Ids)
	if err != nil {
		return nil, err
	}

	in := domain.CreatorListInput{
		IDs:        ids,
		Cities:     cities,
		Categories: categories,
		DateFrom:   body.DateFrom,
		DateTo:     body.DateTo,
		AgeFrom:    body.AgeFrom,
		AgeTo:      body.AgeTo,
		Search:     search,
		Sort:       string(body.Sort),
		Order:      string(body.Order),
		Page:       body.Page,
		PerPage:    body.PerPage,
	}

	page, err := s.creatorService.List(ctx, in)
	if err != nil {
		return nil, err
	}

	if len(page.Items) == 0 {
		return api.ListCreators200JSONResponse{
			Data: api.CreatorsListData{
				Items:   []api.CreatorListItem{},
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

	data, err := domainCreatorListPageToAPI(
		page,
		indexDictionaryByCode(categoryEntries),
		indexDictionaryByCode(cityEntries),
	)
	if err != nil {
		return nil, err
	}
	return api.ListCreators200JSONResponse{Data: data}, nil
}

// validateCreatorAgeBound enforces the OpenAPI min/max range on an optional
// age field. Catches negative values (silent no-op filters) and runaway
// integers that would overflow `make_interval(years => N+1)` in the repo.
func validateCreatorAgeBound(field string, age *int) error {
	if age == nil {
		return nil
	}
	if *age < domain.CreatorListAgeMin || *age > domain.CreatorListAgeMax {
		return domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Параметр %s должен быть в диапазоне %d..%d",
				field, domain.CreatorListAgeMin, domain.CreatorListAgeMax))
	}
	return nil
}

// validateCreatorSearch trims the optional search string and enforces the
// OpenAPI length cap. An empty search after trim is returned as "" so the
// service / repo can ignore it; a search longer than the limit is rejected
// so a client cannot push a 1MB ILIKE pattern through the body limit. NUL
// bytes are rejected up-front because Postgres' text/varchar columns reject
// them at the driver level — letting one through would surface as a generic
// 500 instead of a clean 422.
func validateCreatorSearch(p *string) (string, error) {
	if p == nil {
		return "", nil
	}
	trimmed := strings.TrimSpace(*p)
	if strings.ContainsRune(trimmed, 0) {
		return "", domain.NewValidationError(domain.CodeValidation,
			"Параметр search содержит недопустимый символ")
	}
	if len([]rune(trimmed)) > domain.CreatorListSearchMaxLen {
		return "", domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Параметр search не должен превышать %d символов",
				domain.CreatorListSearchMaxLen))
	}
	return trimmed, nil
}

// validateCreatorIDs enforces the OpenAPI maxItems on the optional ids filter
// and normalises the input before it leaves the handler. oapi-codegen does
// not surface maxItems as a runtime check, so an unenforced array would let
// a caller balloon the IN-clause arbitrarily. Three further failure modes
// matter beyond the cap: JSON `null` decodes into a zero UUID (not an error
// from the json package), uppercase and lowercase spellings of the same UUID
// would silently bloat the SQL placeholder list, and bare duplicates do the
// same. Reject zero, dedupe canonical (lowercase) strings, return nil for
// empty/nil — same shape as validateCreatorCodeArray.
func validateCreatorIDs(p *[]openapi_types.UUID) ([]string, error) {
	if p == nil || len(*p) == 0 {
		return nil, nil
	}
	if len(*p) > domain.CreatorListIDsMax {
		return nil, domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Параметр ids не должен содержать более %d элементов. Сократите запрос.",
				domain.CreatorListIDsMax))
	}
	out := make([]string, 0, len(*p))
	seen := make(map[uuid.UUID]struct{}, len(*p))
	for _, id := range *p {
		if id == uuid.Nil {
			return nil, domain.NewValidationError(domain.CodeValidation,
				"Параметр ids содержит нулевой UUID")
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id.String())
	}
	return out, nil
}

// validateCreatorCodeArray enforces the openapi maxLength + minLength on each
// element of the cities/categories filter arrays, plus a soft array-length
// cap so a client cannot flood IN-clauses. Empty/whitespace-only items are
// rejected up front instead of silently passed through.
func validateCreatorCodeArray(field string, p *[]string, maxLen int) ([]string, error) {
	if p == nil || len(*p) == 0 {
		return nil, nil
	}
	if len(*p) > domain.CreatorListFilterArrayMax {
		return nil, domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Параметр %s не должен содержать более %d элементов",
				field, domain.CreatorListFilterArrayMax))
	}
	out := make([]string, 0, len(*p))
	seen := make(map[string]struct{}, len(*p))
	for _, raw := range *p {
		code := strings.TrimSpace(raw)
		if code == "" {
			return nil, domain.NewValidationError(domain.CodeValidation,
				fmt.Sprintf("Параметр %s содержит пустое значение", field))
		}
		if len([]rune(code)) > maxLen {
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

// domainCreatorListPageToAPI builds the JSON-serialisable page payload from
// the service result and the two dictionary indexes. Categories for each
// item are sorted by (sortOrder, code) to match the GET aggregate's contract.
// Deactivated categories/cities surface as {code, name=code, sortOrder=0} so
// historical data stays readable.
func domainCreatorListPageToAPI(
	page *domain.CreatorListPage,
	categoriesByCode map[string]domain.DictionaryEntry,
	cityByCode map[string]domain.DictionaryEntry,
) (api.CreatorsListData, error) {
	items := make([]api.CreatorListItem, len(page.Items))
	for i, item := range page.Items {
		creatorID, err := uuid.Parse(item.ID)
		if err != nil {
			return api.CreatorsListData{}, fmt.Errorf("parse creator id %q: %w", item.ID, err)
		}

		cats := make([]api.DictionaryItem, len(item.Categories))
		for j, code := range item.Categories {
			cats[j] = resolveDictionaryItem(code, categoriesByCode)
		}
		slices.SortFunc(cats, sortDictionaryItem)

		socials := make([]api.CreatorListSocial, len(item.Socials))
		for j, sc := range item.Socials {
			socials[j] = api.CreatorListSocial{
				Platform: api.SocialPlatform(sc.Platform),
				Handle:   sc.Handle,
			}
		}

		items[i] = api.CreatorListItem{
			Id:                   creatorID,
			LastName:             item.LastName,
			FirstName:            item.FirstName,
			MiddleName:           item.MiddleName,
			Iin:                  item.IIN,
			BirthDate:            openapi_types.Date{Time: item.BirthDate},
			Phone:                item.Phone,
			City:                 resolveDictionaryItem(item.CityCode, cityByCode),
			Categories:           cats,
			Socials:              socials,
			ActiveCampaignsCount: item.ActiveCampaignsCount,
			TelegramUsername:     item.TelegramUsername,
			CreatedAt:            item.CreatedAt,
			UpdatedAt:            item.UpdatedAt,
		}
	}
	return api.CreatorsListData{
		Items:   items,
		Total:   page.Total,
		Page:    page.Page,
		PerPage: page.PerPage,
	}, nil
}

func domainCreatorAggregateSocialToAPI(s domain.CreatorAggregateSocial) (api.CreatorAggregateSocial, error) {
	socialID, err := uuid.Parse(s.ID)
	if err != nil {
		return api.CreatorAggregateSocial{}, fmt.Errorf("parse social id %q: %w", s.ID, err)
	}
	out := api.CreatorAggregateSocial{
		Id:        socialID,
		Platform:  api.SocialPlatform(s.Platform),
		Handle:    s.Handle,
		Verified:  s.Verified,
		CreatedAt: s.CreatedAt,
	}
	if s.Method != nil {
		m := api.SocialVerificationMethod(*s.Method)
		out.Method = &m
	}
	if s.VerifiedByUserID != nil {
		u, err := uuid.Parse(*s.VerifiedByUserID)
		if err != nil {
			return api.CreatorAggregateSocial{}, fmt.Errorf("parse verified_by_user_id %q: %w", *s.VerifiedByUserID, err)
		}
		out.VerifiedByUserId = &u
	}
	if s.VerifiedAt != nil {
		out.VerifiedAt = s.VerifiedAt
	}
	return out, nil
}
