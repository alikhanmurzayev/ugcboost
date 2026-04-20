package service

import (
	"context"
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
	NewCategoryRepo(db dbutil.DB) repository.CategoryRepo
	NewCreatorApplicationCategoryRepo(db dbutil.DB) repository.CreatorApplicationCategoryRepo
	NewCreatorApplicationSocialRepo(db dbutil.DB) repository.CreatorApplicationSocialRepo
	NewCreatorApplicationConsentRepo(db dbutil.DB) repository.CreatorApplicationConsentRepo
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
// The IIN and the mandatory consents are checked before any write: duplicates
// yield CodeCreatorApplicationDuplicate (409) and missing consents or
// validation errors yield VALIDATION_ERROR (422). Nothing about the personal
// data lands in application logs — only the generated application id and
// request id are logged.
func (s *CreatorApplicationService) Submit(ctx context.Context, in domain.CreatorApplicationInput) (*domain.CreatorApplicationSubmission, error) {
	if err := requireAllConsents(in.Consents); err != nil {
		return nil, err
	}

	birth, err := domain.ValidateIIN(in.IIN)
	if err != nil {
		return nil, iinErrorToValidation(err)
	}
	if err := domain.EnsureAdult(birth, in.Now); err != nil {
		return nil, domain.NewValidationError(domain.CodeUnderAge, "Applicant must be at least 18 years old")
	}

	normalisedSocials, err := normaliseSocials(in.Socials)
	if err != nil {
		return nil, err
	}

	var submission *domain.CreatorApplicationSubmission

	err = dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		appRepo := s.repoFactory.NewCreatorApplicationRepo(tx)
		categoryRepo := s.repoFactory.NewCategoryRepo(tx)
		appCategoryRepo := s.repoFactory.NewCreatorApplicationCategoryRepo(tx)
		appSocialRepo := s.repoFactory.NewCreatorApplicationSocialRepo(tx)
		appConsentRepo := s.repoFactory.NewCreatorApplicationConsentRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)

		hasActive, err := appRepo.HasActiveByIIN(ctx, in.IIN)
		if err != nil {
			return fmt.Errorf("check duplicate iin: %w", err)
		}
		if hasActive {
			return domain.NewBusinessError(domain.CodeCreatorApplicationDuplicate,
				"Заявка по этому ИИН уже находится на рассмотрении или одобрена")
		}

		categoryIDs, err := resolveCategoryIDs(ctx, categoryRepo, in.CategoryCodes)
		if err != nil {
			return err
		}

		appRow, err := appRepo.Create(ctx, repository.CreatorApplicationRow{
			LastName:   strings.TrimSpace(in.LastName),
			FirstName:  strings.TrimSpace(in.FirstName),
			MiddleName: trimOptional(in.MiddleName),
			IIN:        in.IIN,
			BirthDate:  birth,
			Phone:      strings.TrimSpace(in.Phone),
			City:       strings.TrimSpace(in.City),
			Address:    strings.TrimSpace(in.Address),
		})
		if err != nil {
			return fmt.Errorf("create application: %w", err)
		}

		catRows := make([]repository.CreatorApplicationCategoryRow, len(categoryIDs))
		for i, id := range categoryIDs {
			catRows[i] = repository.CreatorApplicationCategoryRow{
				ApplicationID: appRow.ID,
				CategoryID:    id,
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

		consentRows := buildConsentRows(appRow.ID, in)
		if err := appConsentRepo.InsertMany(ctx, consentRows); err != nil {
			return fmt.Errorf("insert consents: %w", err)
		}

		if err := writeAudit(ctx, auditRepo,
			AuditActionCreatorApplicationSubmit, AuditEntityTypeCreatorApplication, appRow.ID,
			nil, auditNewValue(in, appRow.ID)); err != nil {
			return fmt.Errorf("write audit: %w", err)
		}

		submission = &domain.CreatorApplicationSubmission{
			ApplicationID: appRow.ID,
			BirthDate:     appRow.BirthDate,
		}
		s.logger.Info(ctx, "creator application submitted", "application_id", appRow.ID)
		return nil
	})

	if err != nil {
		return nil, err
	}
	return submission, nil
}

// requireAllConsents rejects the submission if any of the four mandatory
// consents is not explicitly true.
func requireAllConsents(c domain.ConsentsInput) error {
	for _, name := range domain.ConsentTypeValues {
		if !c.AsMap()[name] {
			return domain.NewValidationError(domain.CodeMissingConsent,
				fmt.Sprintf("Отсутствует обязательное согласие: %s", name))
		}
	}
	return nil
}

// iinErrorToValidation maps domain IIN sentinel errors onto user-facing
// validation errors. Everything that is not an IIN-format issue is treated as
// a generic 422 with a safe message — we never leak the internal reason.
func iinErrorToValidation(err error) error {
	switch {
	case errors.Is(err, domain.ErrIINUnderAge18):
		return domain.NewValidationError(domain.CodeUnderAge, "Applicant must be at least 18 years old")
	case errors.Is(err, domain.ErrIINFormat),
		errors.Is(err, domain.ErrIINChecksum),
		errors.Is(err, domain.ErrIINCentury),
		errors.Is(err, domain.ErrIINBirthDate):
		return domain.NewValidationError(domain.CodeInvalidIIN, "Некорректный ИИН")
	default:
		return err
	}
}

// normaliseSocials trims whitespace and strips a single leading @ from every
// handle. Platform values are checked against the domain whitelist — any
// unknown platform yields VALIDATION_ERROR so malformed OpenAPI bypasses (or
// accidents from trusted callers) cannot sneak garbage into the database.
func normaliseSocials(accounts []domain.SocialAccountInput) ([]domain.SocialAccountInput, error) {
	if len(accounts) == 0 {
		return nil, domain.NewValidationError(domain.CodeValidation, "At least one social account is required")
	}
	allowed := make(map[string]struct{}, len(domain.SocialPlatformValues))
	for _, v := range domain.SocialPlatformValues {
		allowed[v] = struct{}{}
	}
	out := make([]domain.SocialAccountInput, len(accounts))
	for i, a := range accounts {
		if _, ok := allowed[a.Platform]; !ok {
			return nil, domain.NewValidationError(domain.CodeValidation,
				fmt.Sprintf("Unsupported social platform: %s", a.Platform))
		}
		handle := strings.TrimSpace(a.Handle)
		handle = strings.TrimPrefix(handle, "@")
		if handle == "" {
			return nil, domain.NewValidationError(domain.CodeValidation,
				fmt.Sprintf("Empty handle for platform %s", a.Platform))
		}
		out[i] = domain.SocialAccountInput{Platform: a.Platform, Handle: handle}
	}
	return out, nil
}

// resolveCategoryIDs maps user-provided category codes to DB ids. Missing or
// inactive codes surface as UNKNOWN_CATEGORY (422) pointing at the first bad
// code — one error is enough, we don't need to enumerate every issue.
func resolveCategoryIDs(ctx context.Context, repo repository.CategoryRepo, codes []string) ([]string, error) {
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

	rows, err := repo.GetActiveByCodes(ctx, unique)
	if err != nil {
		return nil, fmt.Errorf("lookup categories: %w", err)
	}
	byCode := make(map[string]string, len(rows))
	for _, row := range rows {
		byCode[row.Code] = row.ID
	}
	ids := make([]string, 0, len(unique))
	for _, code := range unique {
		id, ok := byCode[code]
		if !ok {
			return nil, domain.NewValidationError(domain.CodeUnknownCategory,
				fmt.Sprintf("Неизвестная категория: %s", code))
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// buildConsentRows converts the request-level consent data into one repo row
// per canonical consent type. Document versions follow DocumentVersionFor.
func buildConsentRows(applicationID string, in domain.CreatorApplicationInput) []repository.CreatorApplicationConsentRow {
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
func auditNewValue(in domain.CreatorApplicationInput, applicationID string) map[string]any {
	platforms := make([]string, len(in.Socials))
	for i, a := range in.Socials {
		platforms[i] = a.Platform
	}
	return map[string]any{
		"application_id": applicationID,
		"city":           in.City,
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
