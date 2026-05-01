package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// CreatorApplicationRepoFactory enumerates the repos the service needs.
// Each method matches a constructor on repository.RepoFactory.
type CreatorApplicationRepoFactory interface {
	NewCreatorApplicationRepo(db dbutil.DB) repository.CreatorApplicationRepo
	NewDictionaryRepo(db dbutil.DB) repository.DictionaryRepo
	NewCreatorApplicationCategoryRepo(db dbutil.DB) repository.CreatorApplicationCategoryRepo
	NewCreatorApplicationSocialRepo(db dbutil.DB) repository.CreatorApplicationSocialRepo
	NewCreatorApplicationConsentRepo(db dbutil.DB) repository.CreatorApplicationConsentRepo
	NewCreatorApplicationTelegramLinkRepo(db dbutil.DB) repository.CreatorApplicationTelegramLinkRepo
	NewAuditRepo(db dbutil.DB) repository.AuditRepo
}

// CreatorApplicationService owns the submission use case for creator
// applications coming from the public landing page.
type CreatorApplicationService struct {
	pool        dbutil.Pool
	repoFactory CreatorApplicationRepoFactory
	logger      logger.Logger
}

// NewCreatorApplicationService wires the service with its dependencies.
func NewCreatorApplicationService(pool dbutil.Pool, repoFactory CreatorApplicationRepoFactory, log logger.Logger) *CreatorApplicationService {
	return &CreatorApplicationService{pool: pool, repoFactory: repoFactory, logger: log}
}

// Submit persists one application together with its related rows atomically.
// The IIN, the consent flag and category limits are checked before any write:
// duplicates yield CodeCreatorApplicationDuplicate (409) and validation errors
// yield granular 422 codes (INVALID_IIN / UNDER_AGE / MISSING_CONSENT /
// UNKNOWN_CATEGORY / VALIDATION_ERROR). Nothing about the personal data lands
// in stdout-логах приложения — only the generated application id is logged on
// success. Audit_logs may carry PII per the spec (administrator-read only).
func (s *CreatorApplicationService) Submit(ctx context.Context, in domain.CreatorApplicationInput) (*domain.CreatorApplicationSubmission, error) {
	if !in.Consents.AcceptedAll {
		return nil, domain.NewValidationError(domain.CodeMissingConsent,
			"Требуется согласие со всеми условиями")
	}
	if len(in.CategoryCodes) > domain.MaxCategoriesPerApplication {
		return nil, domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Максимум %d категории", domain.MaxCategoriesPerApplication))
	}
	trimmed, err := s.trimAndValidateRequired(in)
	if err != nil {
		return nil, err
	}
	categoryOtherText, err := s.validateCategoryOtherText(in.CategoryCodes, in.CategoryOtherText)
	if err != nil {
		return nil, err
	}

	birth, err := domain.ValidateIIN(trimmed.IIN)
	if err != nil {
		return nil, s.iinErrorToValidation(err)
	}
	if err := domain.EnsureAdult(birth, in.Now); err != nil {
		return nil, domain.NewValidationError(domain.CodeUnderAge,
			fmt.Sprintf("Возраст менее %d лет", domain.MinCreatorAge))
	}

	normalisedSocials, err := s.normaliseSocials(in.Socials)
	if err != nil {
		return nil, err
	}

	var submission *domain.CreatorApplicationSubmission

	err = dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		appRepo := s.repoFactory.NewCreatorApplicationRepo(tx)
		appCategoryRepo := s.repoFactory.NewCreatorApplicationCategoryRepo(tx)
		appSocialRepo := s.repoFactory.NewCreatorApplicationSocialRepo(tx)
		appConsentRepo := s.repoFactory.NewCreatorApplicationConsentRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)

		hasActive, err := appRepo.HasActiveByIIN(ctx, trimmed.IIN)
		if err != nil {
			return fmt.Errorf("check duplicate iin: %w", err)
		}
		if hasActive {
			return s.duplicateError()
		}

		// One dictionary repo serves both lookups — categories and cities live
		// in different physical tables but share the same read interface.
		dictRepo := s.repoFactory.NewDictionaryRepo(tx)
		categoryCodes, err := s.resolveCategoryCodes(ctx, dictRepo, in.CategoryCodes)
		if err != nil {
			return err
		}
		if err := s.resolveCityCode(ctx, dictRepo, trimmed.CityCode); err != nil {
			return err
		}

		appRow, err := appRepo.Create(ctx, repository.CreatorApplicationRow{
			LastName:          trimmed.LastName,
			FirstName:         trimmed.FirstName,
			MiddleName:        trimOptional(in.MiddleName),
			IIN:               trimmed.IIN,
			BirthDate:         birth,
			Phone:             trimmed.Phone,
			CityCode:          trimmed.CityCode,
			Address:           trimmed.Address,
			CategoryOtherText: categoryOtherText,
			Status:            domain.CreatorApplicationStatusPending,
		})
		if err != nil {
			if errors.Is(err, domain.ErrCreatorApplicationDuplicate) {
				return s.duplicateError()
			}
			return fmt.Errorf("create application: %w", err)
		}

		catRows := make([]repository.CreatorApplicationCategoryRow, len(categoryCodes))
		for i, code := range categoryCodes {
			catRows[i] = repository.CreatorApplicationCategoryRow{
				ApplicationID: appRow.ID,
				CategoryCode:  code,
			}
		}
		if err := appCategoryRepo.InsertMany(ctx, catRows); err != nil {
			return fmt.Errorf("insert categories: %w", err)
		}

		socialRows := make([]repository.CreatorApplicationSocialRow, len(normalisedSocials))
		for i, acc := range normalisedSocials {
			socialRows[i] = repository.CreatorApplicationSocialRow{
				ApplicationID: appRow.ID,
				Platform:      acc.Platform,
				Handle:        acc.Handle,
			}
		}
		if err := appSocialRepo.InsertMany(ctx, socialRows); err != nil {
			return fmt.Errorf("insert socials: %w", err)
		}

		consentRows := s.buildConsentRows(appRow.ID, in)
		if err := appConsentRepo.InsertMany(ctx, consentRows); err != nil {
			return fmt.Errorf("insert consents: %w", err)
		}

		if err := writeAudit(ctx, auditRepo,
			AuditActionCreatorApplicationSubmit, AuditEntityTypeCreatorApplication, appRow.ID,
			nil, s.auditNewValue(in, appRow.ID)); err != nil {
			return fmt.Errorf("write audit: %w", err)
		}

		submission = &domain.CreatorApplicationSubmission{
			ApplicationID: appRow.ID,
			BirthDate:     appRow.BirthDate,
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	// Log after the transaction commits so the "submitted" signal can never
	// lie about a rolled-back request.
	s.logger.Info(ctx, "creator application submitted", "application_id", submission.ApplicationID)
	return submission, nil
}

// duplicateError is the single canonical 409 instance used both when the
// service spots an active IIN up front and when the partial unique index
// catches a concurrent race at INSERT time. The message gives the creator
// an actionable next step instead of leaving them stuck.
func (s *CreatorApplicationService) duplicateError() error {
	return domain.NewBusinessError(domain.CodeCreatorApplicationDuplicate,
		"Заявка по этому ИИН уже находится на рассмотрении или одобрена. Дождитесь решения модератора или, если заявка будет отклонена, подайте новую.")
}

// trimmedCreatorApplicationInput holds the post-trim required-field values so
// the service can both validate and reuse them without re-running TrimSpace.
// Address is optional: if the input pointer is nil or trims to empty, the
// trimmed value is nil and the column stays NULL — the bot/admin collects the
// real legal address later.
type trimmedCreatorApplicationInput struct {
	LastName  string
	FirstName string
	IIN       string
	Phone     string
	CityCode  string
	Address   *string
}

// trimAndValidateRequired trims whitespace from every mandatory string field
// and rejects the submission if any of them becomes empty. OpenAPI's
// minLength:1 lets a single space through, so the post-trim check is the real
// defence against whitespace-only PII landing in the DB.
func (s *CreatorApplicationService) trimAndValidateRequired(in domain.CreatorApplicationInput) (trimmedCreatorApplicationInput, error) {
	out := trimmedCreatorApplicationInput{
		LastName:  strings.TrimSpace(in.LastName),
		FirstName: strings.TrimSpace(in.FirstName),
		IIN:       strings.TrimSpace(in.IIN),
		Phone:     strings.TrimSpace(in.Phone),
		CityCode:  strings.TrimSpace(in.CityCode),
		Address:   trimOptional(in.Address),
	}
	missing := func(name string) error {
		return domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Обязательное поле не заполнено: %s", name))
	}
	switch {
	case out.LastName == "":
		return out, missing("last_name")
	case out.FirstName == "":
		return out, missing("first_name")
	case out.IIN == "":
		return out, missing("iin")
	case out.Phone == "":
		return out, missing("phone")
	case out.CityCode == "":
		return out, missing("city")
	}
	return out, nil
}

// validateCategoryOtherText enforces the contract for the free-text "other"
// category description: required and non-blank when the codes contain "other",
// trimmed, and capped at 200 runes. Returns the trimmed value (or nil when
// "other" is absent — the column stays NULL).
func (s *CreatorApplicationService) validateCategoryOtherText(codes []string, raw *string) (*string, error) {
	hasOther := false
	for _, c := range codes {
		if strings.TrimSpace(c) == domain.CategoryCodeOther {
			hasOther = true
			break
		}
	}
	if !hasOther {
		return nil, nil
	}
	missing := domain.NewValidationError(domain.CodeValidation,
		"Укажите название категории в поле «Другое»")
	if raw == nil {
		return nil, missing
	}
	txt := strings.TrimSpace(*raw)
	if txt == "" {
		return nil, missing
	}
	if len([]rune(txt)) > 200 {
		return nil, domain.NewValidationError(domain.CodeValidation,
			"Текст категории «Другое» слишком длинный (макс. 200 символов)")
	}
	return &txt, nil
}

// iinErrorToValidation always returns the safe INVALID_IIN message — we never
// leak the internal reason via a raw 500. The error parameter is preserved so
// callers can stay consistent if we later need to differentiate variants.
func (s *CreatorApplicationService) iinErrorToValidation(_ error) error {
	return domain.NewValidationError(domain.CodeInvalidIIN, "Невалидный ИИН")
}

// normaliseSocials enforces the whitelist of platforms and normalises each
// handle: trim whitespace → strip ALL leading '@' → lowercase → validate
// against domain.SocialHandleRegex. Duplicates on the same (platform, handle)
// pair inside one request are rejected up front so we never hit the DB
// UNIQUE constraint mid-TX. Validation messages reference only the platform
// (an enum value) — the user-controlled handle never makes it into an error
// response or, by extension, into stdout logs through respondError.
func (s *CreatorApplicationService) normaliseSocials(accounts []domain.SocialAccountInput) ([]domain.SocialAccountInput, error) {
	if len(accounts) == 0 {
		return nil, domain.NewValidationError(domain.CodeValidation, "Нужен хотя бы один аккаунт в соцсети")
	}
	allowed := make(map[string]struct{}, len(domain.SocialPlatformValues))
	for _, v := range domain.SocialPlatformValues {
		allowed[v] = struct{}{}
	}
	seen := make(map[string]struct{}, len(accounts))
	out := make([]domain.SocialAccountInput, len(accounts))
	for i, a := range accounts {
		if _, ok := allowed[a.Platform]; !ok {
			return nil, domain.NewValidationError(domain.CodeValidation,
				fmt.Sprintf("Неподдерживаемая соцсеть: %s", a.Platform))
		}
		handle := strings.ToLower(strings.TrimLeft(strings.TrimSpace(a.Handle), "@"))
		if handle == "" {
			return nil, domain.NewValidationError(domain.CodeValidation,
				fmt.Sprintf("Пустой handle для соцсети %s", a.Platform))
		}
		if !domain.SocialHandleRegex.MatchString(handle) {
			return nil, domain.NewValidationError(domain.CodeValidation,
				fmt.Sprintf("Некорректный handle для соцсети %s: допустимы буквы, цифры, точка и подчёркивание", a.Platform))
		}
		key := a.Platform + "|" + handle
		if _, dup := seen[key]; dup {
			return nil, domain.NewValidationError(domain.CodeValidation,
				fmt.Sprintf("Дубликат соцсети: %s", a.Platform))
		}
		seen[key] = struct{}{}
		out[i] = domain.SocialAccountInput{Platform: a.Platform, Handle: handle}
	}
	return out, nil
}

// resolveCategoryCodes validates user-provided category codes against the
// active dictionary and returns the canonical, deduplicated set ready for
// INSERT. Missing or inactive codes surface as UNKNOWN_CATEGORY (422)
// pointing at the first bad code — one error is enough, we don't need to
// enumerate every issue. The dictionary repo is bound to the caller's tx so
// the lookup runs inside the same transaction as the subsequent writes.
func (s *CreatorApplicationService) resolveCategoryCodes(ctx context.Context, dictRepo repository.DictionaryRepo, codes []string) ([]string, error) {
	if len(codes) == 0 {
		return nil, domain.NewValidationError(domain.CodeValidation, "At least one category is required")
	}
	unique := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, c := range codes {
		trimmed := strings.TrimSpace(c)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		unique = append(unique, trimmed)
	}
	if len(unique) == 0 {
		return nil, domain.NewValidationError(domain.CodeValidation, "At least one category is required")
	}

	rows, err := dictRepo.GetActiveByCodes(ctx, repository.TableCategories, unique)
	if err != nil {
		return nil, fmt.Errorf("lookup categories: %w", err)
	}
	known := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		known[row.Code] = struct{}{}
	}
	for _, code := range unique {
		if _, ok := known[code]; !ok {
			return nil, domain.NewValidationError(domain.CodeUnknownCategory,
				fmt.Sprintf("Неизвестная категория: %s", code))
		}
	}
	return unique, nil
}

// resolveCityCode confirms the city code from the request lives in the
// active cities dictionary. Mirrors resolveCategoryCodes so the FK on
// creator_applications.city_code never surfaces as a 500 — unknown or
// deactivated codes get translated to a 422 the client can reason about.
func (s *CreatorApplicationService) resolveCityCode(ctx context.Context, dictRepo repository.DictionaryRepo, code string) error {
	rows, err := dictRepo.GetActiveByCodes(ctx, repository.TableCities, []string{code})
	if err != nil {
		return fmt.Errorf("lookup city: %w", err)
	}
	if len(rows) == 0 {
		return domain.NewValidationError(domain.CodeUnknownCity,
			fmt.Sprintf("Неизвестный город: %s", code))
	}
	return nil
}

// buildConsentRows converts the request-level consent data into one repo row
// per canonical consent type. Document versions follow DocumentVersionFor.
func (s *CreatorApplicationService) buildConsentRows(applicationID string, in domain.CreatorApplicationInput) []repository.CreatorApplicationConsentRow {
	rows := make([]repository.CreatorApplicationConsentRow, 0, len(domain.ConsentTypeValues))
	for _, ct := range domain.ConsentTypeValues {
		rows = append(rows, repository.CreatorApplicationConsentRow{
			ApplicationID:   applicationID,
			ConsentType:     ct,
			AcceptedAt:      in.Now,
			DocumentVersion: domain.DocumentVersionFor(ct, in.AgreementVersion, in.PrivacyVersion),
			IPAddress:       in.IPAddress,
			UserAgent:       in.UserAgent,
		})
	}
	return rows
}

// auditNewValue assembles the sanitised payload that goes into audit_logs.
// Personal data (IIN, names, phone, address, handles) is deliberately absent —
// administrators reading audit_logs should only see non-PII context.
func (s *CreatorApplicationService) auditNewValue(in domain.CreatorApplicationInput, applicationID string) map[string]any {
	platforms := make([]string, len(in.Socials))
	for i, a := range in.Socials {
		platforms[i] = a.Platform
	}
	return map[string]any{
		"application_id": applicationID,
		"city":           in.CityCode,
		"categories":     in.CategoryCodes,
		"platforms":      platforms,
	}
}

// trimOptional returns a trimmed copy of the pointer's string, or nil if the
// pointer is nil or the trimmed result is empty.
func trimOptional(s *string) *string {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// GetByID assembles the full read aggregate for an application: main row +
// categories + socials + consents. All four queries run read-only against the
// pool — no transaction is needed because nothing changes here. sql.ErrNoRows
// from the main lookup is returned as-is (already wrapped by dbutil through
// %w) so the handler can map it to 404 via errors.Is.
func (s *CreatorApplicationService) GetByID(ctx context.Context, id string) (*domain.CreatorApplicationDetail, error) {
	appRow, err := s.repoFactory.NewCreatorApplicationRepo(s.pool).GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	categoryRows, err := s.repoFactory.NewCreatorApplicationCategoryRepo(s.pool).ListByApplicationID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}

	socialRows, err := s.repoFactory.NewCreatorApplicationSocialRepo(s.pool).ListByApplicationID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list socials: %w", err)
	}

	consentRows, err := s.repoFactory.NewCreatorApplicationConsentRepo(s.pool).ListByApplicationID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list consents: %w", err)
	}

	linkRow, err := s.repoFactory.NewCreatorApplicationTelegramLinkRepo(s.pool).GetByApplicationID(ctx, id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("get telegram link: %w", err)
	}

	return s.creatorApplicationDetailFromRows(appRow, categoryRows, socialRows, consentRows, linkRow), nil
}

// creatorApplicationDetailFromRows maps the four repo result sets onto the
// domain aggregate. Categories arrive as plain codes — name/sortOrder are
// resolved by the handler against DictionaryService at presentation time,
// so the service layer stays code-only. Consents are reordered in-memory by
// canonical ConsentTypeValues so the response is deterministic regardless of
// how Postgres returned them; missing types are skipped without error so the
// read side does not fail on legacy or partial data (though POST atomically
// creates all four).
func (s *CreatorApplicationService) creatorApplicationDetailFromRows(
	app *repository.CreatorApplicationRow,
	categories []string,
	socials []*repository.CreatorApplicationSocialRow,
	consents []*repository.CreatorApplicationConsentRow,
	link *repository.CreatorApplicationTelegramLinkRow,
) *domain.CreatorApplicationDetail {
	cats := append([]string(nil), categories...)

	socs := make([]domain.CreatorApplicationDetailSocial, len(socials))
	for i, s := range socials {
		socs[i] = domain.CreatorApplicationDetailSocial{
			Platform: s.Platform,
			Handle:   s.Handle,
		}
	}

	byType := make(map[string]*repository.CreatorApplicationConsentRow, len(consents))
	for _, c := range consents {
		byType[c.ConsentType] = c
	}
	cons := make([]domain.CreatorApplicationDetailConsent, 0, len(domain.ConsentTypeValues))
	for _, ct := range domain.ConsentTypeValues {
		c, ok := byType[ct]
		if !ok {
			continue
		}
		cons = append(cons, domain.CreatorApplicationDetailConsent{
			ConsentType:     c.ConsentType,
			AcceptedAt:      c.AcceptedAt,
			DocumentVersion: c.DocumentVersion,
			IPAddress:       c.IPAddress,
			UserAgent:       c.UserAgent,
		})
	}

	var tgLink *domain.CreatorApplicationTelegramLink
	if link != nil {
		tgLink = &domain.CreatorApplicationTelegramLink{
			ApplicationID:     link.ApplicationID,
			TelegramUserID:    link.TelegramUserID,
			TelegramUsername:  link.TelegramUsername,
			TelegramFirstName: link.TelegramFirstName,
			TelegramLastName:  link.TelegramLastName,
			LinkedAt:          link.LinkedAt,
		}
	}

	return &domain.CreatorApplicationDetail{
		ID:                app.ID,
		LastName:          app.LastName,
		FirstName:         app.FirstName,
		MiddleName:        app.MiddleName,
		IIN:               app.IIN,
		BirthDate:         app.BirthDate,
		Phone:             app.Phone,
		CityCode:          app.CityCode,
		Address:           app.Address,
		CategoryOtherText: app.CategoryOtherText,
		Status:            app.Status,
		CreatedAt:         app.CreatedAt,
		UpdatedAt:         app.UpdatedAt,
		Categories:        cats,
		Socials:           socs,
		Consents:          cons,
		TelegramLink:      tgLink,
	}
}
