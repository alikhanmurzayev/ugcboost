package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	dbmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	repomocks "github.com/alikhanmurzayev/ugcboost/backend/internal/repository/mocks"
	svcmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/service/mocks"
)

// validCreatorInput builds an input that passes every precondition so scenarios
// can selectively invalidate one field to hit a specific branch.
// IIN 950515312348 encodes 1995-05-15. Against the fixed "now" of 2026-04-20
// the applicant is 30, which clears the MinCreatorAge floor with margin.
//
// Address is intentionally left nil — the landing flow does not collect a
// legal address (the bot/admin captures it after approval), so the canonical
// "valid input" reflects that. Tests that need a non-nil address set it
// explicitly.
func validCreatorInput(t *testing.T) domain.CreatorApplicationInput {
	t.Helper()
	return domain.CreatorApplicationInput{
		LastName:      "Муратова",
		FirstName:     "Айдана",
		MiddleName:    pointer.ToString("Ивановна"),
		IIN:           "950515312348",
		Phone:         "+77001234567",
		CityCode:      "almaty",
		CategoryCodes: []string{"beauty", "fashion"},
		Socials: []domain.SocialAccountInput{
			{Platform: domain.SocialPlatformInstagram, Handle: "@aidana"},
			{Platform: domain.SocialPlatformTikTok, Handle: "aidana_tt"},
		},
		Consents:         domain.ConsentsInput{AcceptedAll: true},
		IPAddress:        "127.0.0.1",
		UserAgent:        "ua/1",
		AgreementVersion: "2026-04-20",
		PrivacyVersion:   "2026-04-20",
		Now:              time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC),
	}
}

// creatorServiceRig assembles the common mock rig used by every test.
// dictRepo serves both the category lookup (dictionary GetActiveByCodes call)
// and stays around for any future dictionary-backed checks.
//
// notifier is the consumer-side mock injected into NewCreatorApplicationService
// for tests that exercise the SendPulse verification path; the Submit-flow
// tests leave it as a nil-tolerant mock because Submit never fires it.
type creatorServiceRig struct {
	pool                *dbmocks.MockPool
	factory             *svcmocks.MockCreatorApplicationRepoFactory
	appRepo             *repomocks.MockCreatorApplicationRepo
	dictRepo            *repomocks.MockDictionaryRepo
	appCategoryRepo     *repomocks.MockCreatorApplicationCategoryRepo
	appSocialRepo       *repomocks.MockCreatorApplicationSocialRepo
	appConsentRepo      *repomocks.MockCreatorApplicationConsentRepo
	appTelegramLinkRepo *repomocks.MockCreatorApplicationTelegramLinkRepo
	transitionRepo      *repomocks.MockCreatorApplicationStatusTransitionRepo
	auditRepo           *repomocks.MockAuditRepo
	notifier            *svcmocks.MockcreatorAppNotifier
	logger              *logmocks.MockLogger
}

func newCreatorServiceRig(t *testing.T) creatorServiceRig {
	t.Helper()
	return creatorServiceRig{
		pool:                dbmocks.NewMockPool(t),
		factory:             svcmocks.NewMockCreatorApplicationRepoFactory(t),
		appRepo:             repomocks.NewMockCreatorApplicationRepo(t),
		dictRepo:            repomocks.NewMockDictionaryRepo(t),
		appCategoryRepo:     repomocks.NewMockCreatorApplicationCategoryRepo(t),
		appSocialRepo:       repomocks.NewMockCreatorApplicationSocialRepo(t),
		appConsentRepo:      repomocks.NewMockCreatorApplicationConsentRepo(t),
		appTelegramLinkRepo: repomocks.NewMockCreatorApplicationTelegramLinkRepo(t),
		transitionRepo:      repomocks.NewMockCreatorApplicationStatusTransitionRepo(t),
		auditRepo:           repomocks.NewMockAuditRepo(t),
		notifier:            svcmocks.NewMockcreatorAppNotifier(t),
		logger:              logmocks.NewMockLogger(t),
	}
}

// expectTxBegin wires the mock pool so it returns testTx{} for the single
// WithTx call issued by a submission.
func expectTxBegin(rig creatorServiceRig) {
	rig.pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
}

// expectFactoryWiring configures the factory calls every TX performs eagerly
// at the top of the Submit transaction. The dictionary repo is constructed
// lazily inside resolveCategoryCodes and is wired separately by tests that
// reach that code path (see expectDictionaryWiring).
func expectFactoryWiring(rig creatorServiceRig) {
	rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
	rig.factory.EXPECT().NewCreatorApplicationCategoryRepo(mock.Anything).Return(rig.appCategoryRepo)
	rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo)
	rig.factory.EXPECT().NewCreatorApplicationConsentRepo(mock.Anything).Return(rig.appConsentRepo)
	rig.factory.EXPECT().NewAuditRepo(mock.Anything).Return(rig.auditRepo)
}

// expectDictionaryWiring registers the NewDictionaryRepo call Submit makes
// once the duplicate check has passed. The repo serves both the category
// and city lookups inside the same TX, so a single factory call is enough.
func expectDictionaryWiring(rig creatorServiceRig) {
	rig.factory.EXPECT().NewDictionaryRepo(mock.Anything).Return(rig.dictRepo)
}

// expectCityLookupSuccess wires the GetActiveByCodes call resolveCityCode
// makes against the cities table when the rest of the request is valid.
func expectCityLookupSuccess(rig creatorServiceRig, code string) {
	rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCities, []string{code}).
		Return([]*repository.DictionaryEntryRow{{Code: code, Active: true}}, nil)
}

// hasValidVerificationCodeFormat confirms a verification_code looks like
// "UGC-NNNNNN" — used by Submit-mock matchers so they can assert the rest of
// the row exactly without freezing the random digit sequence.
func hasValidVerificationCodeFormat(code string) bool {
	if !strings.HasPrefix(code, domain.VerificationCodePrefix) {
		return false
	}
	rest := code[len(domain.VerificationCodePrefix):]
	if len(rest) != domain.VerificationCodeDigits {
		return false
	}
	for _, c := range rest {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func TestCreatorApplicationService_Submit(t *testing.T) {
	t.Parallel()

	t.Run("missing consent fails before tx", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		in.Consents.AcceptedAll = false

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)

		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, domain.CodeMissingConsent, ve.Code)
	})

	t.Run("invalid iin fails before tx", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		in.IIN = "bad"

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)

		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, domain.CodeInvalidIIN, ve.Code)
	})

	t.Run("under MinCreatorAge fails before tx", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		// Birth-day - 1 of MinCreatorAge anniversary: still one day shy.
		in.Now = time.Date(1995+domain.MinCreatorAge, 5, 14, 0, 0, 0, 0, time.UTC)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)

		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, domain.CodeUnderAge, ve.Code)
	})

	t.Run("empty socials short-circuits before tx", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		in.Socials = nil

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)

		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, domain.CodeValidation, ve.Code)
	})

	t.Run("unsupported social platform rejected before tx", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		in.Socials[0].Platform = "facebook"

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)

		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, domain.CodeValidation, ve.Code)
	})

	t.Run("threads platform accepted", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		// Switch one entry to threads so we exercise the new platform end-to-end.
		in.Socials = []domain.SocialAccountInput{
			{Platform: domain.SocialPlatformThreads, Handle: "aidana"},
		}

		expectTxBegin(rig)
		expectFactoryWiring(rig)
		expectDictionaryWiring(rig)
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(false, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"beauty", "fashion"}).
			Return([]*repository.DictionaryEntryRow{
				{Code: "beauty", Active: true},
				{Code: "fashion", Active: true},
			}, nil)
		expectCityLookupSuccess(rig, "almaty")
		rig.appRepo.EXPECT().Create(mock.Anything, mock.Anything).
			Return(&repository.CreatorApplicationRow{ID: "app-th"}, nil)
		rig.appCategoryRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.appSocialRepo.EXPECT().InsertMany(mock.Anything, []repository.CreatorApplicationSocialRow{
			{ApplicationID: "app-th", Platform: "threads", Handle: "aidana"},
		}).Return(nil)
		rig.appConsentRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		rig.logger.EXPECT().Info(mock.Anything, "creator application submitted", []any{"application_id", "app-th"}).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)
		require.NoError(t, err)
	})

	t.Run("too many categories rejected before tx", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		in.CategoryCodes = []string{"beauty", "fashion", "food", "fitness"} // 4 > MaxCategoriesPerApplication

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)

		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, domain.CodeValidation, ve.Code)
		require.Contains(t, ve.Message, "Максимум")
	})

	t.Run("blank-only category codes rejected inside tx", func(t *testing.T) {
		t.Parallel()
		// Every entry trims to empty, so the dedup loop produces no codes
		// and resolveCategoryCodes refuses without ever hitting
		// GetActiveByCodes — but Submit has already constructed the shared
		// dictionary repo, so its factory call still fires.
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		in.CategoryCodes = []string{"   ", "\t"}

		expectTxBegin(rig)
		expectFactoryWiring(rig)
		expectDictionaryWiring(rig)
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(false, nil)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)
		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, domain.CodeValidation, ve.Code)
	})

	t.Run("duplicate category codes deduplicated before lookup", func(t *testing.T) {
		t.Parallel()
		// Same code repeated and one whitespace-only entry survive only one
		// dictionary lookup against ["beauty"]. Ensures the seen-map and the
		// trimmed=="" branches both run.
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		in.CategoryCodes = []string{"beauty", "beauty", " "}

		expectTxBegin(rig)
		expectFactoryWiring(rig)
		expectDictionaryWiring(rig)
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(false, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"beauty"}).
			Return([]*repository.DictionaryEntryRow{{Code: "beauty", Active: true}}, nil)
		expectCityLookupSuccess(rig, "almaty")
		rig.appRepo.EXPECT().Create(mock.Anything, mock.Anything).
			Return(&repository.CreatorApplicationRow{ID: "app-dedup"}, nil)
		rig.appCategoryRepo.EXPECT().InsertMany(mock.Anything, []repository.CreatorApplicationCategoryRow{
			{ApplicationID: "app-dedup", CategoryCode: "beauty"},
		}).Return(nil)
		rig.appSocialRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.appConsentRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		rig.logger.EXPECT().Info(mock.Anything, "creator application submitted", []any{"application_id", "app-dedup"}).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)
		require.NoError(t, err)
	})

	t.Run("dictionary repo error wraps as lookup categories", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)

		expectTxBegin(rig)
		expectFactoryWiring(rig)
		expectDictionaryWiring(rig)
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(false, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"beauty", "fashion"}).
			Return(nil, errors.New("db down"))

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)
		require.ErrorContains(t, err, "lookup categories")
		require.ErrorContains(t, err, "db down")
	})

	t.Run("other category without text rejected before tx", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		in.CategoryCodes = []string{"beauty", "other"}
		in.CategoryOtherText = nil

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)

		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, domain.CodeValidation, ve.Code)
		require.Contains(t, ve.Message, "«Другое»")
	})

	t.Run("other category with blank text rejected", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		blank := "   "
		in.CategoryCodes = []string{"beauty", "other"}
		in.CategoryOtherText = &blank

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)

		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, domain.CodeValidation, ve.Code)
	})

	t.Run("other category with too long text rejected", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		long := strings.Repeat("я", 201)
		in.CategoryCodes = []string{"beauty", "other"}
		in.CategoryOtherText = &long

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)

		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, domain.CodeValidation, ve.Code)
		require.Contains(t, ve.Message, "слишком длинный")
	})

	t.Run("duplicate iin returns 409 business error", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)

		expectTxBegin(rig)
		expectFactoryWiring(rig)
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(true, nil)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)

		var be *domain.BusinessError
		require.ErrorAs(t, err, &be)
		require.Equal(t, domain.CodeCreatorApplicationDuplicate, be.Code)
	})

	t.Run("duplicate check db error propagates", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)

		expectTxBegin(rig)
		expectFactoryWiring(rig)
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(false, errors.New("db down"))

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)
		require.ErrorContains(t, err, "db down")
	})

	t.Run("unknown category returns 422", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		in.CategoryCodes = []string{"beauty", "mystery"}

		expectTxBegin(rig)
		expectFactoryWiring(rig)
		expectDictionaryWiring(rig)
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(false, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"beauty", "mystery"}).
			Return([]*repository.DictionaryEntryRow{{Code: "beauty", Active: true}}, nil)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)

		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, domain.CodeUnknownCategory, ve.Code)
	})

	t.Run("unknown city returns 422 before create", func(t *testing.T) {
		t.Parallel()
		// City code that the cities dictionary does not know about: the FK on
		// creator_applications.city_code would surface as a 500 later, so the
		// service must catch it up front via resolveCityCode and answer 422.
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		in.CityCode = "neverland"

		expectTxBegin(rig)
		expectFactoryWiring(rig)
		expectDictionaryWiring(rig)
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(false, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"beauty", "fashion"}).
			Return([]*repository.DictionaryEntryRow{
				{Code: "beauty", Active: true},
				{Code: "fashion", Active: true},
			}, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCities, []string{"neverland"}).
			Return(nil, nil)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)
		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, domain.CodeUnknownCity, ve.Code)
	})

	t.Run("city dictionary error wraps as lookup city", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)

		expectTxBegin(rig)
		expectFactoryWiring(rig)
		expectDictionaryWiring(rig)
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(false, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"beauty", "fashion"}).
			Return([]*repository.DictionaryEntryRow{
				{Code: "beauty", Active: true},
				{Code: "fashion", Active: true},
			}, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCities, []string{"almaty"}).
			Return(nil, errors.New("db down"))

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)
		require.ErrorContains(t, err, "lookup city")
		require.ErrorContains(t, err, "db down")
	})

	t.Run("application insert error aborts tx", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)

		expectTxBegin(rig)
		expectFactoryWiring(rig)
		expectDictionaryWiring(rig)
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(false, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"beauty", "fashion"}).
			Return([]*repository.DictionaryEntryRow{
				{Code: "beauty", Active: true},
				{Code: "fashion", Active: true},
			}, nil)
		expectCityLookupSuccess(rig, "almaty")
		rig.appRepo.EXPECT().Create(mock.Anything, mock.Anything).
			Return((*repository.CreatorApplicationRow)(nil), errors.New("insert failed"))

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)
		require.ErrorContains(t, err, "insert failed")
	})

	t.Run("success writes full TX and returns submission", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)

		expectTxBegin(rig)
		expectFactoryWiring(rig)
		expectDictionaryWiring(rig)
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(false, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"beauty", "fashion"}).
			Return([]*repository.DictionaryEntryRow{
				{Code: "beauty", Active: true},
				{Code: "fashion", Active: true},
			}, nil)
		expectCityLookupSuccess(rig, "almaty")
		rig.appRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(got repository.CreatorApplicationRow) bool {
			if !hasValidVerificationCodeFormat(got.VerificationCode) {
				return false
			}
			return got.LastName == "Муратова" &&
				got.FirstName == "Айдана" &&
				got.MiddleName != nil && *got.MiddleName == "Ивановна" &&
				got.IIN == "950515312348" &&
				got.BirthDate.Equal(birth) &&
				got.Phone == "+77001234567" &&
				got.CityCode == "almaty" &&
				got.Address == nil &&
				got.CategoryOtherText == nil &&
				got.Status == domain.CreatorApplicationStatusVerification
		})).Return(&repository.CreatorApplicationRow{
			ID:         "app-1",
			LastName:   "Муратова",
			FirstName:  "Айдана",
			MiddleName: pointer.ToString("Ивановна"),
			IIN:        "950515312348",
			BirthDate:  birth,
			Phone:      "+77001234567",
			CityCode:   "almaty",
			Status:     "verification",
			CreatedAt:  created,
			UpdatedAt:  created,
		}, nil)
		rig.appCategoryRepo.EXPECT().InsertMany(mock.Anything, []repository.CreatorApplicationCategoryRow{
			{ApplicationID: "app-1", CategoryCode: "beauty"},
			{ApplicationID: "app-1", CategoryCode: "fashion"},
		}).Return(nil)
		rig.appSocialRepo.EXPECT().InsertMany(mock.Anything, []repository.CreatorApplicationSocialRow{
			{ApplicationID: "app-1", Platform: "instagram", Handle: "aidana"},
			{ApplicationID: "app-1", Platform: "tiktok", Handle: "aidana_tt"},
		}).Return(nil)
		rig.appConsentRepo.EXPECT().InsertMany(mock.Anything, []repository.CreatorApplicationConsentRow{
			{ApplicationID: "app-1", ConsentType: "processing", AcceptedAt: in.Now, DocumentVersion: "2026-04-20", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
			{ApplicationID: "app-1", ConsentType: "third_party", AcceptedAt: in.Now, DocumentVersion: "2026-04-20", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
			{ApplicationID: "app-1", ConsentType: "cross_border", AcceptedAt: in.Now, DocumentVersion: "2026-04-20", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
			{ApplicationID: "app-1", ConsentType: "terms", AcceptedAt: in.Now, DocumentVersion: "2026-04-20", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
		}).Return(nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row repository.AuditLogRow) bool {
			if row.Action != AuditActionCreatorApplicationSubmit ||
				row.EntityType != AuditEntityTypeCreatorApplication ||
				row.EntityID == nil || *row.EntityID != "app-1" ||
				row.ActorID != nil {
				return false
			}
			var raw map[string]any
			if err := json.Unmarshal(row.NewValue, &raw); err != nil {
				return false
			}
			// Pin the Always rule: verification_code is quasi-PII and must
			// never leak into the audit payload — chunk-7 spec carves it out
			// explicitly.
			if _, leaked := raw["verification_code"]; leaked {
				return false
			}
			status, _ := raw["status"].(string)
			return status == domain.CreatorApplicationStatusVerification
		})).Return(nil)
		rig.logger.EXPECT().Info(mock.Anything, "creator application submitted", []any{"application_id", "app-1"}).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		got, err := svc.Submit(context.Background(), in)
		require.NoError(t, err)
		require.Equal(t, &domain.CreatorApplicationSubmission{
			ApplicationID: "app-1",
			BirthDate:     birth,
		}, got)
	})

	t.Run("address provided — propagated to repo as pointer", func(t *testing.T) {
		t.Parallel()
		// Default validCreatorInput leaves Address nil (landing form does not
		// collect it). This scenario flips it on to lock the contract that a
		// non-nil pointer reaches the row verbatim — bot/admin will eventually
		// patch the same column once the legal address is captured.
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		in.Address = pointer.ToString("ул. Абая 1")

		expectTxBegin(rig)
		expectFactoryWiring(rig)
		expectDictionaryWiring(rig)
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(false, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"beauty", "fashion"}).
			Return([]*repository.DictionaryEntryRow{
				{Code: "beauty", Active: true},
				{Code: "fashion", Active: true},
			}, nil)
		expectCityLookupSuccess(rig, "almaty")
		rig.appRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row repository.CreatorApplicationRow) bool {
			return row.Address != nil && *row.Address == "ул. Абая 1"
		})).Return(&repository.CreatorApplicationRow{ID: "app-addr"}, nil)
		rig.appCategoryRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.appSocialRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.appConsentRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		rig.logger.EXPECT().Info(mock.Anything, "creator application submitted", []any{"application_id", "app-addr"}).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)
		require.NoError(t, err)
	})

	t.Run("address whitespace-only trims to nil", func(t *testing.T) {
		t.Parallel()
		// Whitespace input from the wire must collapse to NULL, not be persisted
		// as a blank string — the column is now nullable, and "  " in the DB
		// would lie about whether the address was actually captured.
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		in.Address = pointer.ToString("   ")

		expectTxBegin(rig)
		expectFactoryWiring(rig)
		expectDictionaryWiring(rig)
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(false, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"beauty", "fashion"}).
			Return([]*repository.DictionaryEntryRow{
				{Code: "beauty", Active: true},
				{Code: "fashion", Active: true},
			}, nil)
		expectCityLookupSuccess(rig, "almaty")
		rig.appRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row repository.CreatorApplicationRow) bool {
			return row.Address == nil
		})).Return(&repository.CreatorApplicationRow{ID: "app-blank"}, nil)
		rig.appCategoryRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.appSocialRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.appConsentRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		rig.logger.EXPECT().Info(mock.Anything, "creator application submitted", []any{"application_id", "app-blank"}).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)
		require.NoError(t, err)
	})

	t.Run("success with other category persists trimmed text", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		in.CategoryCodes = []string{"other"}
		raw := "  Авторские ASMR-видео про винтажные велосипеды  "
		in.CategoryOtherText = &raw
		trimmed := "Авторские ASMR-видео про винтажные велосипеды"

		expectTxBegin(rig)
		expectFactoryWiring(rig)
		expectDictionaryWiring(rig)
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(false, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"other"}).
			Return([]*repository.DictionaryEntryRow{{Code: "other", Active: true}}, nil)
		expectCityLookupSuccess(rig, "almaty")
		rig.appRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row repository.CreatorApplicationRow) bool {
			return row.CategoryOtherText != nil && *row.CategoryOtherText == trimmed
		})).Return(&repository.CreatorApplicationRow{ID: "app-other"}, nil)
		rig.appCategoryRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.appSocialRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.appConsentRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		rig.logger.EXPECT().Info(mock.Anything, "creator application submitted", []any{"application_id", "app-other"}).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)
		require.NoError(t, err)
	})

	t.Run("empty required field rejected before tx", func(t *testing.T) {
		t.Parallel()
		// Cover every required-field branch in trimAndValidateRequired so the
		// per-method coverage gate stays green when the awk filter no longer
		// excludes lowercase identifiers.
		cases := []struct {
			name    string
			mutate  func(*domain.CreatorApplicationInput)
			message string
		}{
			{"last_name", func(in *domain.CreatorApplicationInput) { in.LastName = "   " }, "last_name"},
			{"first_name", func(in *domain.CreatorApplicationInput) { in.FirstName = "" }, "first_name"},
			{"iin", func(in *domain.CreatorApplicationInput) { in.IIN = "  " }, "iin"},
			{"phone", func(in *domain.CreatorApplicationInput) { in.Phone = "" }, "phone"},
			{"city", func(in *domain.CreatorApplicationInput) { in.CityCode = "  " }, "city"},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				rig := newCreatorServiceRig(t)
				in := validCreatorInput(t)
				tc.mutate(&in)

				svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
				_, err := svc.Submit(context.Background(), in)

				var ve *domain.ValidationError
				require.ErrorAs(t, err, &ve)
				require.Equal(t, domain.CodeValidation, ve.Code)
				require.Contains(t, ve.Message, tc.message)
			})
		}
	})

	t.Run("duplicate social pair rejected before tx", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		in.Socials = []domain.SocialAccountInput{
			{Platform: domain.SocialPlatformInstagram, Handle: "@Aidana"},
			{Platform: domain.SocialPlatformInstagram, Handle: "aidana"},
		}

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)

		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, domain.CodeValidation, ve.Code)
		require.Contains(t, ve.Message, "Дубликат")
	})

	t.Run("invalid handle characters rejected before tx", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		in.Socials[0].Handle = "aidana user"

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)

		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, domain.CodeValidation, ve.Code)
	})

	t.Run("verification code conflict on first attempt — service retries with a fresh code", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)

		rig.pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil).Times(2)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo).Times(2)
		rig.factory.EXPECT().NewCreatorApplicationCategoryRepo(mock.Anything).Return(rig.appCategoryRepo).Times(2)
		rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo).Times(2)
		rig.factory.EXPECT().NewCreatorApplicationConsentRepo(mock.Anything).Return(rig.appConsentRepo).Times(2)
		rig.factory.EXPECT().NewAuditRepo(mock.Anything).Return(rig.auditRepo).Times(2)
		rig.factory.EXPECT().NewDictionaryRepo(mock.Anything).Return(rig.dictRepo).Times(2)
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(false, nil).Times(2)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"beauty", "fashion"}).
			Return([]*repository.DictionaryEntryRow{{Code: "beauty", Active: true}, {Code: "fashion", Active: true}}, nil).Times(2)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCities, []string{"almaty"}).
			Return([]*repository.DictionaryEntryRow{{Code: "almaty", Active: true}}, nil).Times(2)

		// First Create — verification_code collision sentinel; expected to retry.
		rig.appRepo.EXPECT().Create(mock.Anything, mock.Anything).
			Return((*repository.CreatorApplicationRow)(nil), domain.ErrCreatorApplicationVerificationCodeConflict).Once()
		// Second Create — success on the retry attempt.
		rig.appRepo.EXPECT().Create(mock.Anything, mock.Anything).
			Return(&repository.CreatorApplicationRow{ID: "app-retry", BirthDate: time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)}, nil).Once()

		rig.appCategoryRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil).Once()
		rig.appSocialRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil).Once()
		rig.appConsentRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil).Once()
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
		rig.logger.EXPECT().Info(mock.Anything, "creator application submitted", []any{"application_id", "app-retry"}).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		got, err := svc.Submit(context.Background(), in)
		require.NoError(t, err)
		require.Equal(t, "app-retry", got.ApplicationID)
	})

	t.Run("verification code conflicts exhaust max attempts — service returns 5xx-bound error", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		const tries = domain.VerificationCodeMaxGenerationAttempts

		rig.pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil).Times(tries)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo).Times(tries)
		rig.factory.EXPECT().NewCreatorApplicationCategoryRepo(mock.Anything).Return(rig.appCategoryRepo).Times(tries)
		rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo).Times(tries)
		rig.factory.EXPECT().NewCreatorApplicationConsentRepo(mock.Anything).Return(rig.appConsentRepo).Times(tries)
		rig.factory.EXPECT().NewAuditRepo(mock.Anything).Return(rig.auditRepo).Times(tries)
		rig.factory.EXPECT().NewDictionaryRepo(mock.Anything).Return(rig.dictRepo).Times(tries)
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(false, nil).Times(tries)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"beauty", "fashion"}).
			Return([]*repository.DictionaryEntryRow{{Code: "beauty", Active: true}, {Code: "fashion", Active: true}}, nil).Times(tries)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCities, []string{"almaty"}).
			Return([]*repository.DictionaryEntryRow{{Code: "almaty", Active: true}}, nil).Times(tries)
		rig.appRepo.EXPECT().Create(mock.Anything, mock.Anything).
			Return((*repository.CreatorApplicationRow)(nil), domain.ErrCreatorApplicationVerificationCodeConflict).Times(tries)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to generate unique verification code after")
	})

	t.Run("repo returns duplicate sentinel under race — service answers 409", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)

		expectTxBegin(rig)
		expectFactoryWiring(rig)
		expectDictionaryWiring(rig)
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(false, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"beauty", "fashion"}).
			Return([]*repository.DictionaryEntryRow{
				{Code: "beauty", Active: true},
				{Code: "fashion", Active: true},
			}, nil)
		expectCityLookupSuccess(rig, "almaty")
		rig.appRepo.EXPECT().Create(mock.Anything, mock.Anything).
			Return((*repository.CreatorApplicationRow)(nil), domain.ErrCreatorApplicationDuplicate)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.Submit(context.Background(), in)

		var be *domain.BusinessError
		require.ErrorAs(t, err, &be)
		require.Equal(t, domain.CodeCreatorApplicationDuplicate, be.Code)
	})
}

func TestCreatorApplicationService_GetByID(t *testing.T) {
	t.Parallel()

	const appID = "11111111-2222-3333-4444-555555555555"

	t.Run("not found surfaces sql.ErrNoRows untouched", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).Return(nil, sql.ErrNoRows)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.GetByID(context.Background(), appID)
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("categories list error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.factory.EXPECT().NewCreatorApplicationCategoryRepo(mock.Anything).Return(rig.appCategoryRepo)
		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).
			Return(&repository.CreatorApplicationRow{ID: appID}, nil)
		rig.appCategoryRepo.EXPECT().ListByApplicationID(mock.Anything, appID).
			Return(nil, errors.New("cat boom"))

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.GetByID(context.Background(), appID)
		require.ErrorContains(t, err, "list categories")
		require.ErrorContains(t, err, "cat boom")
	})

	t.Run("socials list error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.factory.EXPECT().NewCreatorApplicationCategoryRepo(mock.Anything).Return(rig.appCategoryRepo)
		rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo)
		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).
			Return(&repository.CreatorApplicationRow{ID: appID}, nil)
		rig.appCategoryRepo.EXPECT().ListByApplicationID(mock.Anything, appID).
			Return(nil, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationID(mock.Anything, appID).
			Return(nil, errors.New("soc boom"))

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.GetByID(context.Background(), appID)
		require.ErrorContains(t, err, "list socials")
		require.ErrorContains(t, err, "soc boom")
	})

	t.Run("consents list error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.factory.EXPECT().NewCreatorApplicationCategoryRepo(mock.Anything).Return(rig.appCategoryRepo)
		rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo)
		rig.factory.EXPECT().NewCreatorApplicationConsentRepo(mock.Anything).Return(rig.appConsentRepo)
		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).
			Return(&repository.CreatorApplicationRow{ID: appID}, nil)
		rig.appCategoryRepo.EXPECT().ListByApplicationID(mock.Anything, appID).
			Return(nil, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationID(mock.Anything, appID).
			Return(nil, nil)
		rig.appConsentRepo.EXPECT().ListByApplicationID(mock.Anything, appID).
			Return(nil, errors.New("con boom"))

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.GetByID(context.Background(), appID)
		require.ErrorContains(t, err, "list consents")
		require.ErrorContains(t, err, "con boom")
	})

	t.Run("telegram link error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.factory.EXPECT().NewCreatorApplicationCategoryRepo(mock.Anything).Return(rig.appCategoryRepo)
		rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo)
		rig.factory.EXPECT().NewCreatorApplicationConsentRepo(mock.Anything).Return(rig.appConsentRepo)
		rig.factory.EXPECT().NewCreatorApplicationTelegramLinkRepo(mock.Anything).Return(rig.appTelegramLinkRepo)
		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).
			Return(&repository.CreatorApplicationRow{ID: appID}, nil)
		rig.appCategoryRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(nil, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(nil, nil)
		rig.appConsentRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(nil, nil)
		rig.appTelegramLinkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).
			Return(nil, errors.New("link boom"))

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.GetByID(context.Background(), appID)
		require.ErrorContains(t, err, "get telegram link")
		require.ErrorContains(t, err, "link boom")
	})

	t.Run("success builds aggregate and reorders consents to canonical sequence", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)
		updated := time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC)
		acceptedAt := time.Date(2026, 4, 20, 18, 0, 1, 0, time.UTC)

		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.factory.EXPECT().NewCreatorApplicationCategoryRepo(mock.Anything).Return(rig.appCategoryRepo)
		rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo)
		rig.factory.EXPECT().NewCreatorApplicationConsentRepo(mock.Anything).Return(rig.appConsentRepo)
		rig.factory.EXPECT().NewCreatorApplicationTelegramLinkRepo(mock.Anything).Return(rig.appTelegramLinkRepo)
		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).
			Return(&repository.CreatorApplicationRow{
				ID:                appID,
				LastName:          "Муратова",
				FirstName:         "Айдана",
				MiddleName:        pointer.ToString("Ивановна"),
				IIN:               "950515312348",
				BirthDate:         birth,
				Phone:             "+77001234567",
				CityCode:          "almaty",
				Address:           pointer.ToString("ул. Абая 1"),
				CategoryOtherText: pointer.ToString("Авторские ASMR"),
				Status:            domain.CreatorApplicationStatusVerification,
				CreatedAt:         created,
				UpdatedAt:         updated,
			}, nil)
		rig.appCategoryRepo.EXPECT().ListByApplicationID(mock.Anything, appID).
			Return([]string{"beauty", "fashion"}, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationID(mock.Anything, appID).
			Return([]*repository.CreatorApplicationSocialRow{
				{ApplicationID: appID, Platform: domain.SocialPlatformInstagram, Handle: "aidana"},
				{ApplicationID: appID, Platform: domain.SocialPlatformTikTok, Handle: "aidana_tt"},
			}, nil)
		// Repo returns consents in REVERSE canonical order — service must
		// re-sort them to (processing → third_party → cross_border → terms).
		rig.appConsentRepo.EXPECT().ListByApplicationID(mock.Anything, appID).
			Return([]*repository.CreatorApplicationConsentRow{
				{ApplicationID: appID, ConsentType: domain.ConsentTypeTerms, AcceptedAt: acceptedAt, DocumentVersion: "agr-1", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
				{ApplicationID: appID, ConsentType: domain.ConsentTypeCrossBorder, AcceptedAt: acceptedAt, DocumentVersion: "pp-1", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
				{ApplicationID: appID, ConsentType: domain.ConsentTypeThirdParty, AcceptedAt: acceptedAt, DocumentVersion: "pp-1", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
				{ApplicationID: appID, ConsentType: domain.ConsentTypeProcessing, AcceptedAt: acceptedAt, DocumentVersion: "pp-1", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
			}, nil)
		rig.appTelegramLinkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).
			Return(nil, sql.ErrNoRows)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		got, err := svc.GetByID(context.Background(), appID)
		require.NoError(t, err)
		require.Equal(t, &domain.CreatorApplicationDetail{
			ID:                appID,
			LastName:          "Муратова",
			FirstName:         "Айдана",
			MiddleName:        pointer.ToString("Ивановна"),
			IIN:               "950515312348",
			BirthDate:         birth,
			Phone:             "+77001234567",
			CityCode:          "almaty",
			Address:           pointer.ToString("ул. Абая 1"),
			CategoryOtherText: pointer.ToString("Авторские ASMR"),
			Status:            domain.CreatorApplicationStatusVerification,
			CreatedAt:         created,
			UpdatedAt:         updated,
			Categories:        []string{"beauty", "fashion"},
			Socials: []domain.CreatorApplicationDetailSocial{
				{Platform: domain.SocialPlatformInstagram, Handle: "aidana"},
				{Platform: domain.SocialPlatformTikTok, Handle: "aidana_tt"},
			},
			Consents: []domain.CreatorApplicationDetailConsent{
				{ConsentType: domain.ConsentTypeProcessing, AcceptedAt: acceptedAt, DocumentVersion: "pp-1", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
				{ConsentType: domain.ConsentTypeThirdParty, AcceptedAt: acceptedAt, DocumentVersion: "pp-1", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
				{ConsentType: domain.ConsentTypeCrossBorder, AcceptedAt: acceptedAt, DocumentVersion: "pp-1", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
				{ConsentType: domain.ConsentTypeTerms, AcceptedAt: acceptedAt, DocumentVersion: "agr-1", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
			},
		}, got)
	})

	t.Run("rejected app populates rejection block from latest transition", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		actor := "cccc3333-3333-3333-3333-333333333333"
		fromStatus := domain.CreatorApplicationStatusModeration
		reason := domain.TransitionReasonReject
		rejectedAt := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)

		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.factory.EXPECT().NewCreatorApplicationCategoryRepo(mock.Anything).Return(rig.appCategoryRepo)
		rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo)
		rig.factory.EXPECT().NewCreatorApplicationConsentRepo(mock.Anything).Return(rig.appConsentRepo)
		rig.factory.EXPECT().NewCreatorApplicationTelegramLinkRepo(mock.Anything).Return(rig.appTelegramLinkRepo)
		rig.factory.EXPECT().NewCreatorApplicationStatusTransitionRepo(mock.Anything).Return(rig.transitionRepo)

		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).
			Return(&repository.CreatorApplicationRow{ID: appID, Status: domain.CreatorApplicationStatusRejected}, nil)
		rig.appCategoryRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(nil, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(nil, nil)
		rig.appConsentRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(nil, nil)
		rig.appTelegramLinkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).Return(nil, sql.ErrNoRows)
		rig.transitionRepo.EXPECT().GetLatestByApplicationAndToStatus(mock.Anything, appID, domain.CreatorApplicationStatusRejected).
			Return(&repository.CreatorApplicationStatusTransitionRow{
				ID:            "tx-1",
				ApplicationID: appID,
				FromStatus:    pointer.ToString(fromStatus),
				ToStatus:      domain.CreatorApplicationStatusRejected,
				ActorID:       pointer.ToString(actor),
				Reason:        pointer.ToString(reason),
				CreatedAt:     rejectedAt,
			}, nil)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		got, err := svc.GetByID(context.Background(), appID)
		require.NoError(t, err)
		require.NotNil(t, got.Rejection)
		require.Equal(t, &domain.CreatorApplicationRejection{
			FromStatus:       fromStatus,
			RejectedAt:       rejectedAt,
			RejectedByUserID: actor,
		}, got.Rejection)
	})

	t.Run("rejected app without transition row degrades to nil rejection + warn", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.factory.EXPECT().NewCreatorApplicationCategoryRepo(mock.Anything).Return(rig.appCategoryRepo)
		rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo)
		rig.factory.EXPECT().NewCreatorApplicationConsentRepo(mock.Anything).Return(rig.appConsentRepo)
		rig.factory.EXPECT().NewCreatorApplicationTelegramLinkRepo(mock.Anything).Return(rig.appTelegramLinkRepo)
		rig.factory.EXPECT().NewCreatorApplicationStatusTransitionRepo(mock.Anything).Return(rig.transitionRepo)

		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).
			Return(&repository.CreatorApplicationRow{ID: appID, Status: domain.CreatorApplicationStatusRejected}, nil)
		rig.appCategoryRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(nil, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(nil, nil)
		rig.appConsentRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(nil, nil)
		rig.appTelegramLinkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).Return(nil, sql.ErrNoRows)
		rig.transitionRepo.EXPECT().GetLatestByApplicationAndToStatus(mock.Anything, appID, domain.CreatorApplicationStatusRejected).
			Return(nil, sql.ErrNoRows)
		rig.logger.EXPECT().Warn(mock.Anything,
			"creator application detail: rejected without transition row",
			mock.MatchedBy(func(args []any) bool {
				return len(args) == 2 && args[0] == "application_id" && args[1] == appID
			})).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		got, err := svc.GetByID(context.Background(), appID)
		require.NoError(t, err)
		require.Nil(t, got.Rejection)
	})

	t.Run("rejected app with nil from_status degrades to nil rejection + warn", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.factory.EXPECT().NewCreatorApplicationCategoryRepo(mock.Anything).Return(rig.appCategoryRepo)
		rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo)
		rig.factory.EXPECT().NewCreatorApplicationConsentRepo(mock.Anything).Return(rig.appConsentRepo)
		rig.factory.EXPECT().NewCreatorApplicationTelegramLinkRepo(mock.Anything).Return(rig.appTelegramLinkRepo)
		rig.factory.EXPECT().NewCreatorApplicationStatusTransitionRepo(mock.Anything).Return(rig.transitionRepo)

		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).
			Return(&repository.CreatorApplicationRow{ID: appID, Status: domain.CreatorApplicationStatusRejected}, nil)
		rig.appCategoryRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(nil, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(nil, nil)
		rig.appConsentRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(nil, nil)
		rig.appTelegramLinkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).Return(nil, sql.ErrNoRows)
		rig.transitionRepo.EXPECT().GetLatestByApplicationAndToStatus(mock.Anything, appID, domain.CreatorApplicationStatusRejected).
			Return(&repository.CreatorApplicationStatusTransitionRow{
				ID:            "tx-1",
				ApplicationID: appID,
				FromStatus:    nil,
				ToStatus:      domain.CreatorApplicationStatusRejected,
				ActorID:       pointer.ToString("cccc3333-3333-3333-3333-333333333333"),
				CreatedAt:     time.Now().UTC(),
			}, nil)
		rig.logger.EXPECT().Warn(mock.Anything,
			"creator application detail: rejected transition row has nil actor or from_status",
			mock.MatchedBy(func(args []any) bool {
				return len(args) == 4 &&
					args[0] == "application_id" && args[1] == appID &&
					args[2] == "transition_id" && args[3] == "tx-1"
			})).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		got, err := svc.GetByID(context.Background(), appID)
		require.NoError(t, err)
		require.Nil(t, got.Rejection)
	})

	t.Run("rejected app with transition repo error wraps", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.factory.EXPECT().NewCreatorApplicationCategoryRepo(mock.Anything).Return(rig.appCategoryRepo)
		rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo)
		rig.factory.EXPECT().NewCreatorApplicationConsentRepo(mock.Anything).Return(rig.appConsentRepo)
		rig.factory.EXPECT().NewCreatorApplicationTelegramLinkRepo(mock.Anything).Return(rig.appTelegramLinkRepo)
		rig.factory.EXPECT().NewCreatorApplicationStatusTransitionRepo(mock.Anything).Return(rig.transitionRepo)

		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).
			Return(&repository.CreatorApplicationRow{ID: appID, Status: domain.CreatorApplicationStatusRejected}, nil)
		rig.appCategoryRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(nil, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(nil, nil)
		rig.appConsentRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(nil, nil)
		rig.appTelegramLinkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).Return(nil, sql.ErrNoRows)
		rig.transitionRepo.EXPECT().GetLatestByApplicationAndToStatus(mock.Anything, appID, domain.CreatorApplicationStatusRejected).
			Return(nil, errors.New("tx db down"))

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.GetByID(context.Background(), appID)
		require.ErrorContains(t, err, "get rejection transition")
		require.ErrorContains(t, err, "tx db down")
	})
}

func TestCreatorApplicationService_List(t *testing.T) {
	t.Parallel()

	baseInput := func() domain.CreatorApplicationListInput {
		return domain.CreatorApplicationListInput{
			Sort:    domain.CreatorApplicationSortCreatedAt,
			Order:   domain.SortOrderDesc,
			Page:    1,
			PerPage: 20,
		}
	}

	t.Run("trims search before delegating to repo", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.appRepo.EXPECT().List(mock.Anything, mock.MatchedBy(func(p repository.CreatorApplicationListParams) bool {
			return p.Search == "aidana" && p.Sort == domain.CreatorApplicationSortCreatedAt && p.Order == domain.SortOrderDesc && p.Page == 1 && p.PerPage == 20
		})).Return(nil, int64(0), nil)

		in := baseInput()
		in.Search = "   aidana   "
		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		page, err := svc.List(context.Background(), in)
		require.NoError(t, err)
		require.Equal(t, &domain.CreatorApplicationListPage{
			Items:   nil,
			Total:   0,
			Page:    1,
			PerPage: 20,
		}, page)
	})

	t.Run("repo error wrapped with context", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.appRepo.EXPECT().List(mock.Anything, mock.Anything).Return(nil, int64(0), errors.New("db down"))

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.List(context.Background(), baseInput())
		require.ErrorContains(t, err, "list applications")
		require.ErrorContains(t, err, "db down")
	})

	t.Run("category hydration error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.factory.EXPECT().NewCreatorApplicationCategoryRepo(mock.Anything).Return(rig.appCategoryRepo)
		rig.appRepo.EXPECT().List(mock.Anything, mock.Anything).Return(
			[]*repository.CreatorApplicationListRow{{ID: "app-1"}}, int64(1), nil)
		rig.appCategoryRepo.EXPECT().ListByApplicationIDs(mock.Anything, []string{"app-1"}).
			Return(nil, errors.New("cat boom"))

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.List(context.Background(), baseInput())
		require.ErrorContains(t, err, "hydrate categories")
		require.ErrorContains(t, err, "cat boom")
	})

	t.Run("social hydration error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.factory.EXPECT().NewCreatorApplicationCategoryRepo(mock.Anything).Return(rig.appCategoryRepo)
		rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo)
		rig.appRepo.EXPECT().List(mock.Anything, mock.Anything).Return(
			[]*repository.CreatorApplicationListRow{{ID: "app-1"}}, int64(1), nil)
		rig.appCategoryRepo.EXPECT().ListByApplicationIDs(mock.Anything, []string{"app-1"}).
			Return(map[string][]string{"app-1": {"beauty"}}, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationIDs(mock.Anything, []string{"app-1"}).
			Return(nil, errors.New("soc boom"))

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		_, err := svc.List(context.Background(), baseInput())
		require.ErrorContains(t, err, "hydrate socials")
		require.ErrorContains(t, err, "soc boom")
	})

	t.Run("zero results — short-circuits without hydration", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.appRepo.EXPECT().List(mock.Anything, mock.Anything).Return(nil, int64(0), nil)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		page, err := svc.List(context.Background(), baseInput())
		require.NoError(t, err)
		require.Equal(t, &domain.CreatorApplicationListPage{
			Items:   nil,
			Total:   0,
			Page:    1,
			PerPage: 20,
		}, page)
	})

	t.Run("success — assembles page with batch-hydrated categories and socials", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)
		updated := time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.factory.EXPECT().NewCreatorApplicationCategoryRepo(mock.Anything).Return(rig.appCategoryRepo)
		rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo)

		rig.appRepo.EXPECT().List(mock.Anything, mock.Anything).Return(
			[]*repository.CreatorApplicationListRow{
				{
					ID: "app-1", LastName: "Муратова", FirstName: "Айдана",
					MiddleName: pointer.ToString("Ивановна"),
					BirthDate:  birth, CityCode: "almaty",
					Status:    domain.CreatorApplicationStatusVerification,
					CreatedAt: created, UpdatedAt: updated,
					TelegramLinked: true,
				},
				{
					ID: "app-2", LastName: "Иванова", FirstName: "Анна",
					BirthDate: birth, CityCode: "astana",
					Status:    domain.CreatorApplicationStatusModeration,
					CreatedAt: created, UpdatedAt: updated,
				},
			}, int64(2), nil)
		rig.appCategoryRepo.EXPECT().ListByApplicationIDs(mock.Anything, []string{"app-1", "app-2"}).
			Return(map[string][]string{
				"app-1": {"beauty", "fashion"},
				"app-2": {"food"},
			}, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationIDs(mock.Anything, []string{"app-1", "app-2"}).
			Return(map[string][]*repository.CreatorApplicationSocialRow{
				"app-1": {
					{Platform: domain.SocialPlatformInstagram, Handle: "aidana"},
				},
			}, nil)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		page, err := svc.List(context.Background(), baseInput())
		require.NoError(t, err)
		require.Equal(t, &domain.CreatorApplicationListPage{
			Items: []*domain.CreatorApplicationListItem{
				{
					ID: "app-1", Status: domain.CreatorApplicationStatusVerification,
					LastName: "Муратова", FirstName: "Айдана",
					MiddleName: pointer.ToString("Ивановна"),
					BirthDate:  birth, CityCode: "almaty",
					Categories: []string{"beauty", "fashion"},
					Socials: []domain.CreatorApplicationDetailSocial{
						{Platform: domain.SocialPlatformInstagram, Handle: "aidana"},
					},
					TelegramLinked: true,
					CreatedAt:      created, UpdatedAt: updated,
				},
				{
					ID: "app-2", Status: domain.CreatorApplicationStatusModeration,
					LastName: "Иванова", FirstName: "Анна",
					BirthDate: birth, CityCode: "astana",
					Categories:     []string{"food"},
					Socials:        []domain.CreatorApplicationDetailSocial{},
					TelegramLinked: false,
					CreatedAt:      created, UpdatedAt: updated,
				},
			},
			Total:   2,
			Page:    1,
			PerPage: 20,
		}, page)
	})
}

func TestCreatorApplicationService_Counts(t *testing.T) {
	t.Parallel()

	t.Run("repo error wrapped with context", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.appRepo.EXPECT().Counts(mock.Anything).Return(nil, errors.New("db down"))

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		got, err := svc.Counts(context.Background())
		require.ErrorContains(t, err, "counts applications")
		require.ErrorContains(t, err, "db down")
		require.Nil(t, got)
	})

	t.Run("empty repo result returns empty map", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.appRepo.EXPECT().Counts(mock.Anything).Return(map[string]int64{}, nil)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		got, err := svc.Counts(context.Background())
		require.NoError(t, err)
		require.Equal(t, map[string]int64{}, got)
	})

	t.Run("known statuses pass through verbatim", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.appRepo.EXPECT().Counts(mock.Anything).Return(map[string]int64{
			domain.CreatorApplicationStatusVerification: 3,
			domain.CreatorApplicationStatusModeration:   1,
			domain.CreatorApplicationStatusRejected:     2,
		}, nil)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		got, err := svc.Counts(context.Background())
		require.NoError(t, err)
		require.Equal(t, map[string]int64{
			domain.CreatorApplicationStatusVerification: 3,
			domain.CreatorApplicationStatusModeration:   1,
			domain.CreatorApplicationStatusRejected:     2,
		}, got)
	})

	t.Run("unknown status from rolling deploy is dropped with warn log", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.appRepo.EXPECT().Counts(mock.Anything).Return(map[string]int64{
			domain.CreatorApplicationStatusVerification: 5,
			"future_status": 9,
		}, nil)
		rig.logger.EXPECT().Warn(mock.Anything,
			"creator application counts: dropping unknown status",
			mock.MatchedBy(func(args []any) bool {
				return len(args) == 4 &&
					args[0] == "status" && args[1] == "future_status" &&
					args[2] == "count" && args[3] == int64(9)
			})).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		got, err := svc.Counts(context.Background())
		require.NoError(t, err)
		require.Equal(t, map[string]int64{
			domain.CreatorApplicationStatusVerification: 5,
		}, got)
	})
}

// expectVerifyTxBegin wires the mock pool for the single WithTx call inside
// VerifyInstagramByCode.
func expectVerifyTxBegin(rig creatorServiceRig) {
	rig.pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
}

// expectVerifyFactoryWiring registers every repo constructor the
// VerifyInstagramByCode TX issues eagerly. Variants for the early-exit
// branches strip the tail of the wiring as appropriate.
func expectVerifyFactoryWiring(rig creatorServiceRig) {
	rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
	rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo)
	rig.factory.EXPECT().NewCreatorApplicationTelegramLinkRepo(mock.Anything).Return(rig.appTelegramLinkRepo)
	rig.factory.EXPECT().NewAuditRepo(mock.Anything).Return(rig.auditRepo)
}

// applicationRow returns a fresh copy of the canonical "in verification"
// application row used across the verify tests.
func applicationRow(id string) *repository.CreatorApplicationRow {
	return &repository.CreatorApplicationRow{
		ID:               id,
		Status:           domain.CreatorApplicationStatusVerification,
		VerificationCode: "UGC-123456",
	}
}

func TestCreatorApplicationService_VerifyInstagramByCode(t *testing.T) {
	t.Parallel()

	t.Run("happy path verifies, transitions, audits and notifies", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectVerifyTxBegin(rig)
		expectVerifyFactoryWiring(rig)
		rig.factory.EXPECT().NewCreatorApplicationStatusTransitionRepo(mock.Anything).Return(rig.transitionRepo)

		appRow := applicationRow("app-1")
		rig.appRepo.EXPECT().GetByVerificationCodeAndStatus(mock.Anything, "UGC-123456", domain.CreatorApplicationStatusVerification).
			Return(appRow, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationID(mock.Anything, "app-1").
			Return([]*repository.CreatorApplicationSocialRow{
				{ID: "social-1", ApplicationID: "app-1", Platform: domain.SocialPlatformInstagram, Handle: "aidana"},
				{ID: "social-2", ApplicationID: "app-1", Platform: domain.SocialPlatformTikTok, Handle: "aidana_tt"},
			}, nil)

		var capturedSocial repository.UpdateSocialVerificationParams
		rig.appSocialRepo.EXPECT().UpdateVerification(mock.Anything, mock.AnythingOfType("repository.UpdateSocialVerificationParams")).
			Run(func(_ context.Context, params repository.UpdateSocialVerificationParams) {
				capturedSocial = params
			}).
			Return(nil)
		rig.appRepo.EXPECT().UpdateStatus(mock.Anything, "app-1", domain.CreatorApplicationStatusModeration).Return(nil)

		var capturedTransition repository.CreatorApplicationStatusTransitionRow
		rig.transitionRepo.EXPECT().Insert(mock.Anything, mock.AnythingOfType("repository.CreatorApplicationStatusTransitionRow")).
			Run(func(_ context.Context, row repository.CreatorApplicationStatusTransitionRow) {
				capturedTransition = row
			}).
			Return(nil)

		var capturedAudit repository.AuditLogRow
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.AnythingOfType("repository.AuditLogRow")).
			Run(func(_ context.Context, row repository.AuditLogRow) {
				capturedAudit = row
			}).
			Return(nil)

		rig.appTelegramLinkRepo.EXPECT().GetByApplicationID(mock.Anything, "app-1").
			Return(&repository.CreatorApplicationTelegramLinkRow{
				ApplicationID:  "app-1",
				TelegramUserID: 555,
			}, nil)

		rig.notifier.EXPECT().NotifyVerificationApproved(mock.Anything, int64(555)).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		rig.logger.EXPECT().Info(mock.Anything, "sendpulse webhook: instagram verified", mock.Anything).Maybe()

		status, err := svc.VerifyInstagramByCode(context.Background(), "UGC-123456", "AIDANA")
		require.NoError(t, err)
		require.Equal(t, domain.VerifyInstagramStatusVerified, status)

		require.Equal(t, "social-1", capturedSocial.ID)
		require.Equal(t, "aidana", capturedSocial.Handle)
		require.True(t, capturedSocial.Verified)
		require.Equal(t, domain.SocialVerificationMethodAuto, capturedSocial.Method)
		require.Nil(t, capturedSocial.VerifiedByUserID)
		require.WithinDuration(t, time.Now().UTC(), capturedSocial.VerifiedAt, time.Minute)

		require.Equal(t, "app-1", capturedTransition.ApplicationID)
		require.NotNil(t, capturedTransition.FromStatus)
		require.Equal(t, domain.CreatorApplicationStatusVerification, *capturedTransition.FromStatus)
		require.Equal(t, domain.CreatorApplicationStatusModeration, capturedTransition.ToStatus)
		require.Nil(t, capturedTransition.ActorID)
		require.NotNil(t, capturedTransition.Reason)
		require.Equal(t, domain.TransitionReasonInstagramAuto, *capturedTransition.Reason)

		require.Equal(t, AuditActionCreatorApplicationVerificationAuto, capturedAudit.Action)
		require.Equal(t, AuditEntityTypeCreatorApplication, capturedAudit.EntityType)
		require.NotNil(t, capturedAudit.EntityID)
		require.Equal(t, "app-1", *capturedAudit.EntityID)
		var auditPayload map[string]any
		require.NoError(t, json.Unmarshal(capturedAudit.NewValue, &auditPayload))
		require.Equal(t, "app-1", auditPayload["application_id"])
		require.Equal(t, "social-1", auditPayload["social_id"])
		require.Equal(t, domain.CreatorApplicationStatusVerification, auditPayload["from_status"])
		require.Equal(t, domain.CreatorApplicationStatusModeration, auditPayload["to_status"])
		require.Equal(t, false, auditPayload["handle_changed"])
	})

	t.Run("self-fix mismatch overwrites handle and stamps audit flag", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectVerifyTxBegin(rig)
		expectVerifyFactoryWiring(rig)
		rig.factory.EXPECT().NewCreatorApplicationStatusTransitionRepo(mock.Anything).Return(rig.transitionRepo)

		appRow := applicationRow("app-1")
		rig.appRepo.EXPECT().GetByVerificationCodeAndStatus(mock.Anything, "UGC-123456", domain.CreatorApplicationStatusVerification).
			Return(appRow, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationID(mock.Anything, "app-1").
			Return([]*repository.CreatorApplicationSocialRow{
				{ID: "social-1", ApplicationID: "app-1", Platform: domain.SocialPlatformInstagram, Handle: "old"},
			}, nil)

		var capturedSocial repository.UpdateSocialVerificationParams
		rig.appSocialRepo.EXPECT().UpdateVerification(mock.Anything, mock.AnythingOfType("repository.UpdateSocialVerificationParams")).
			Run(func(_ context.Context, params repository.UpdateSocialVerificationParams) {
				capturedSocial = params
			}).
			Return(nil)
		rig.appRepo.EXPECT().UpdateStatus(mock.Anything, "app-1", domain.CreatorApplicationStatusModeration).Return(nil)
		rig.transitionRepo.EXPECT().Insert(mock.Anything, mock.AnythingOfType("repository.CreatorApplicationStatusTransitionRow")).
			Return(nil)

		var capturedAudit repository.AuditLogRow
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.AnythingOfType("repository.AuditLogRow")).
			Run(func(_ context.Context, row repository.AuditLogRow) {
				capturedAudit = row
			}).
			Return(nil)
		rig.appTelegramLinkRepo.EXPECT().GetByApplicationID(mock.Anything, "app-1").
			Return(&repository.CreatorApplicationTelegramLinkRow{
				ApplicationID:  "app-1",
				TelegramUserID: 777,
			}, nil)
		rig.notifier.EXPECT().NotifyVerificationApproved(mock.Anything, int64(777)).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		rig.logger.EXPECT().Info(mock.Anything, "sendpulse webhook: instagram verified", mock.Anything).Maybe()

		status, err := svc.VerifyInstagramByCode(context.Background(), "UGC-123456", "@New")
		require.NoError(t, err)
		require.Equal(t, domain.VerifyInstagramStatusVerified, status)

		require.Equal(t, "new", capturedSocial.Handle, "handle should be the normalised webhook value")

		var auditPayload map[string]any
		require.NoError(t, json.Unmarshal(capturedAudit.NewValue, &auditPayload))
		require.Equal(t, true, auditPayload["handle_changed"])
	})

	t.Run("verified path skips notify and warns when application not linked", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectVerifyTxBegin(rig)
		expectVerifyFactoryWiring(rig)
		rig.factory.EXPECT().NewCreatorApplicationStatusTransitionRepo(mock.Anything).Return(rig.transitionRepo)

		appRow := applicationRow("app-1")
		rig.appRepo.EXPECT().GetByVerificationCodeAndStatus(mock.Anything, "UGC-123456", domain.CreatorApplicationStatusVerification).
			Return(appRow, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationID(mock.Anything, "app-1").
			Return([]*repository.CreatorApplicationSocialRow{
				{ID: "social-1", ApplicationID: "app-1", Platform: domain.SocialPlatformInstagram, Handle: "aidana"},
			}, nil)
		rig.appSocialRepo.EXPECT().UpdateVerification(mock.Anything, mock.Anything).Return(nil)
		rig.appRepo.EXPECT().UpdateStatus(mock.Anything, "app-1", domain.CreatorApplicationStatusModeration).Return(nil)
		rig.transitionRepo.EXPECT().Insert(mock.Anything, mock.Anything).Return(nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		rig.appTelegramLinkRepo.EXPECT().GetByApplicationID(mock.Anything, "app-1").
			Return(nil, sql.ErrNoRows)

		// No notifier expectation — the nil-link branch must NOT fire a
		// notification. Mockery cleanup will fail the test if it does.
		rig.logger.EXPECT().Warn(mock.Anything, "creator verification: skipping telegram notify, application not linked").Once()
		rig.logger.EXPECT().Info(mock.Anything, "sendpulse webhook: instagram verified", mock.Anything).Maybe()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		status, err := svc.VerifyInstagramByCode(context.Background(), "UGC-123456", "aidana")
		require.NoError(t, err)
		require.Equal(t, domain.VerifyInstagramStatusVerified, status)
	})

	t.Run("already verified social returns noop without writes", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectVerifyTxBegin(rig)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo)
		rig.factory.EXPECT().NewCreatorApplicationTelegramLinkRepo(mock.Anything).Return(rig.appTelegramLinkRepo)
		rig.factory.EXPECT().NewAuditRepo(mock.Anything).Return(rig.auditRepo)

		appRow := applicationRow("app-1")
		rig.appRepo.EXPECT().GetByVerificationCodeAndStatus(mock.Anything, "UGC-123456", domain.CreatorApplicationStatusVerification).
			Return(appRow, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationID(mock.Anything, "app-1").
			Return([]*repository.CreatorApplicationSocialRow{
				{ID: "social-1", ApplicationID: "app-1", Platform: domain.SocialPlatformInstagram, Handle: "aidana", Verified: true},
			}, nil)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)

		status, err := svc.VerifyInstagramByCode(context.Background(), "UGC-123456", "aidana")
		require.NoError(t, err)
		require.Equal(t, domain.VerifyInstagramStatusNoop, status)
	})

	t.Run("application not found returns not_found", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectVerifyTxBegin(rig)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo)
		rig.factory.EXPECT().NewCreatorApplicationTelegramLinkRepo(mock.Anything).Return(rig.appTelegramLinkRepo)
		rig.factory.EXPECT().NewAuditRepo(mock.Anything).Return(rig.auditRepo)

		rig.appRepo.EXPECT().GetByVerificationCodeAndStatus(mock.Anything, "UGC-999999", domain.CreatorApplicationStatusVerification).
			Return(nil, sql.ErrNoRows)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)

		status, err := svc.VerifyInstagramByCode(context.Background(), "UGC-999999", "aidana")
		require.NoError(t, err)
		require.Equal(t, domain.VerifyInstagramStatusNotFound, status)
	})

	t.Run("empty normalised handle short-circuits as not_found before tx", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		// No tx, no factory wiring expected — handler must reject before opening
		// the WithTx scope. Logger captures the warn line.
		rig.logger.EXPECT().Warn(mock.Anything, "sendpulse webhook: empty username after normalisation", mock.Anything).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		status, err := svc.VerifyInstagramByCode(context.Background(), "UGC-123456", "@@@   ")
		require.NoError(t, err)
		require.Equal(t, domain.VerifyInstagramStatusNotFound, status)
	})

	t.Run("application without IG social returns no_ig_social", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectVerifyTxBegin(rig)
		rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
		rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo)
		rig.factory.EXPECT().NewCreatorApplicationTelegramLinkRepo(mock.Anything).Return(rig.appTelegramLinkRepo)
		rig.factory.EXPECT().NewAuditRepo(mock.Anything).Return(rig.auditRepo)

		appRow := applicationRow("app-1")
		rig.appRepo.EXPECT().GetByVerificationCodeAndStatus(mock.Anything, "UGC-123456", domain.CreatorApplicationStatusVerification).
			Return(appRow, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationID(mock.Anything, "app-1").
			Return([]*repository.CreatorApplicationSocialRow{
				{ID: "social-2", ApplicationID: "app-1", Platform: domain.SocialPlatformTikTok, Handle: "aidana_tt"},
			}, nil)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)

		status, err := svc.VerifyInstagramByCode(context.Background(), "UGC-123456", "aidana")
		require.NoError(t, err)
		require.Equal(t, domain.VerifyInstagramStatusNoIGSocial, status)
	})

	t.Run("update social db error rolls back tx and bubbles", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectVerifyTxBegin(rig)
		expectVerifyFactoryWiring(rig)

		appRow := applicationRow("app-1")
		rig.appRepo.EXPECT().GetByVerificationCodeAndStatus(mock.Anything, "UGC-123456", domain.CreatorApplicationStatusVerification).
			Return(appRow, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationID(mock.Anything, "app-1").
			Return([]*repository.CreatorApplicationSocialRow{
				{ID: "social-1", ApplicationID: "app-1", Platform: domain.SocialPlatformInstagram, Handle: "aidana"},
			}, nil)
		rig.appSocialRepo.EXPECT().UpdateVerification(mock.Anything, mock.Anything).
			Return(errors.New("update failed"))

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)

		_, err := svc.VerifyInstagramByCode(context.Background(), "UGC-123456", "aidana")
		require.ErrorContains(t, err, "update failed")
	})
}

// expectManualVerifyTxBegin wires the mock pool for the single WithTx call
// inside VerifyApplicationSocialManually.
func expectManualVerifyTxBegin(rig creatorServiceRig) {
	rig.pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
}

// expectManualVerifyFactoryWiring registers every repo constructor the
// VerifyApplicationSocialManually TX issues eagerly. Strip the tail in
// early-exit tests where some constructors are unreachable.
func expectManualVerifyFactoryWiring(rig creatorServiceRig) {
	rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
	rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo)
	rig.factory.EXPECT().NewCreatorApplicationTelegramLinkRepo(mock.Anything).Return(rig.appTelegramLinkRepo)
	rig.factory.EXPECT().NewAuditRepo(mock.Anything).Return(rig.auditRepo)
}

const (
	manualVerifyAdminID  = "11111111-1111-1111-1111-111111111111"
	manualVerifyAppID    = "22222222-2222-2222-2222-222222222222"
	manualVerifySocialID = "33333333-3333-3333-3333-333333333333"
)

func TestCreatorApplicationService_VerifyApplicationSocialManually(t *testing.T) {
	t.Parallel()

	t.Run("happy path verifies, transitions, audits and never notifies", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectManualVerifyTxBegin(rig)
		expectManualVerifyFactoryWiring(rig)
		rig.factory.EXPECT().NewCreatorApplicationStatusTransitionRepo(mock.Anything).Return(rig.transitionRepo)

		appRow := applicationRow(manualVerifyAppID)
		rig.appRepo.EXPECT().GetByID(mock.Anything, manualVerifyAppID).Return(appRow, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationID(mock.Anything, manualVerifyAppID).
			Return([]*repository.CreatorApplicationSocialRow{
				{ID: manualVerifySocialID, ApplicationID: manualVerifyAppID, Platform: domain.SocialPlatformTikTok, Handle: "aidana_tt"},
				{ID: "44444444-4444-4444-4444-444444444444", ApplicationID: manualVerifyAppID, Platform: domain.SocialPlatformInstagram, Handle: "aidana"},
			}, nil)
		rig.appTelegramLinkRepo.EXPECT().GetByApplicationID(mock.Anything, manualVerifyAppID).
			Return(&repository.CreatorApplicationTelegramLinkRow{ApplicationID: manualVerifyAppID, TelegramUserID: 555}, nil)

		var capturedSocial repository.UpdateSocialVerificationParams
		rig.appSocialRepo.EXPECT().UpdateVerification(mock.Anything, mock.AnythingOfType("repository.UpdateSocialVerificationParams")).
			Run(func(_ context.Context, params repository.UpdateSocialVerificationParams) {
				capturedSocial = params
			}).
			Return(nil)
		rig.appRepo.EXPECT().UpdateStatus(mock.Anything, manualVerifyAppID, domain.CreatorApplicationStatusModeration).Return(nil)

		var capturedTransition repository.CreatorApplicationStatusTransitionRow
		rig.transitionRepo.EXPECT().Insert(mock.Anything, mock.AnythingOfType("repository.CreatorApplicationStatusTransitionRow")).
			Run(func(_ context.Context, row repository.CreatorApplicationStatusTransitionRow) {
				capturedTransition = row
			}).
			Return(nil)

		var capturedAudit repository.AuditLogRow
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.AnythingOfType("repository.AuditLogRow")).
			Run(func(_ context.Context, row repository.AuditLogRow) {
				capturedAudit = row
			}).
			Return(nil)

		// Notifier is wired but configured WITHOUT EXPECT — mockery will fail
		// the test at cleanup if the service touches it. This is the explicit
		// "creator did not self-verify, so no push" assertion.
		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)

		err := svc.VerifyApplicationSocialManually(context.Background(), manualVerifyAppID, manualVerifySocialID, manualVerifyAdminID)
		require.NoError(t, err)

		require.Equal(t, manualVerifySocialID, capturedSocial.ID)
		require.Equal(t, "aidana_tt", capturedSocial.Handle, "handle stays untouched on manual verify")
		require.True(t, capturedSocial.Verified)
		require.Equal(t, domain.SocialVerificationMethodManual, capturedSocial.Method)
		require.NotNil(t, capturedSocial.VerifiedByUserID)
		require.Equal(t, manualVerifyAdminID, *capturedSocial.VerifiedByUserID)
		require.WithinDuration(t, time.Now().UTC(), capturedSocial.VerifiedAt, time.Minute)

		require.Equal(t, manualVerifyAppID, capturedTransition.ApplicationID)
		require.NotNil(t, capturedTransition.FromStatus)
		require.Equal(t, domain.CreatorApplicationStatusVerification, *capturedTransition.FromStatus)
		require.Equal(t, domain.CreatorApplicationStatusModeration, capturedTransition.ToStatus)
		require.NotNil(t, capturedTransition.ActorID)
		require.Equal(t, manualVerifyAdminID, *capturedTransition.ActorID)
		require.NotNil(t, capturedTransition.Reason)
		require.Equal(t, domain.TransitionReasonManualVerify, *capturedTransition.Reason)

		require.Equal(t, AuditActionCreatorApplicationVerificationManual, capturedAudit.Action)
		require.Equal(t, AuditEntityTypeCreatorApplication, capturedAudit.EntityType)
		require.NotNil(t, capturedAudit.EntityID)
		require.Equal(t, manualVerifyAppID, *capturedAudit.EntityID)
		require.NotNil(t, capturedAudit.ActorID)
		require.Equal(t, manualVerifyAdminID, *capturedAudit.ActorID)
		var auditPayload map[string]any
		require.NoError(t, json.Unmarshal(capturedAudit.NewValue, &auditPayload))
		require.Equal(t, manualVerifyAppID, auditPayload["application_id"])
		require.Equal(t, manualVerifySocialID, auditPayload["social_id"])
		require.Equal(t, domain.SocialPlatformTikTok, auditPayload["social_platform"])
		require.Equal(t, domain.CreatorApplicationStatusVerification, auditPayload["from_status"])
		require.Equal(t, domain.CreatorApplicationStatusModeration, auditPayload["to_status"])
	})

	t.Run("application not found returns ErrCreatorApplicationNotFound and writes nothing", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectManualVerifyTxBegin(rig)
		expectManualVerifyFactoryWiring(rig)

		rig.appRepo.EXPECT().GetByID(mock.Anything, manualVerifyAppID).Return(nil, sql.ErrNoRows)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.VerifyApplicationSocialManually(context.Background(), manualVerifyAppID, manualVerifySocialID, manualVerifyAdminID)
		require.ErrorIs(t, err, domain.ErrCreatorApplicationNotFound)
	})

	t.Run("wrong status returns ErrCreatorApplicationNotInVerification and writes nothing", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectManualVerifyTxBegin(rig)
		expectManualVerifyFactoryWiring(rig)

		appRow := applicationRow(manualVerifyAppID)
		appRow.Status = domain.CreatorApplicationStatusModeration
		rig.appRepo.EXPECT().GetByID(mock.Anything, manualVerifyAppID).Return(appRow, nil)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.VerifyApplicationSocialManually(context.Background(), manualVerifyAppID, manualVerifySocialID, manualVerifyAdminID)
		require.ErrorIs(t, err, domain.ErrCreatorApplicationNotInVerification)
	})

	t.Run("social not in this application returns ErrCreatorApplicationSocialNotFound", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectManualVerifyTxBegin(rig)
		expectManualVerifyFactoryWiring(rig)

		appRow := applicationRow(manualVerifyAppID)
		rig.appRepo.EXPECT().GetByID(mock.Anything, manualVerifyAppID).Return(appRow, nil)
		// Two unrelated socials — none with manualVerifySocialID.
		rig.appSocialRepo.EXPECT().ListByApplicationID(mock.Anything, manualVerifyAppID).
			Return([]*repository.CreatorApplicationSocialRow{
				{ID: "55555555-5555-5555-5555-555555555555", ApplicationID: manualVerifyAppID, Platform: domain.SocialPlatformTikTok, Handle: "x"},
			}, nil)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.VerifyApplicationSocialManually(context.Background(), manualVerifyAppID, manualVerifySocialID, manualVerifyAdminID)
		require.ErrorIs(t, err, domain.ErrCreatorApplicationSocialNotFound)
	})

	t.Run("already verified social returns ErrCreatorApplicationSocialAlreadyVerified and writes nothing", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectManualVerifyTxBegin(rig)
		expectManualVerifyFactoryWiring(rig)

		appRow := applicationRow(manualVerifyAppID)
		rig.appRepo.EXPECT().GetByID(mock.Anything, manualVerifyAppID).Return(appRow, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationID(mock.Anything, manualVerifyAppID).
			Return([]*repository.CreatorApplicationSocialRow{
				{ID: manualVerifySocialID, ApplicationID: manualVerifyAppID, Platform: domain.SocialPlatformTikTok, Handle: "aidana_tt", Verified: true, Method: pointer.ToString(domain.SocialVerificationMethodAuto)},
			}, nil)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.VerifyApplicationSocialManually(context.Background(), manualVerifyAppID, manualVerifySocialID, manualVerifyAdminID)
		require.ErrorIs(t, err, domain.ErrCreatorApplicationSocialAlreadyVerified)
	})

	t.Run("missing telegram link returns ErrCreatorApplicationTelegramNotLinked and writes nothing", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectManualVerifyTxBegin(rig)
		expectManualVerifyFactoryWiring(rig)

		appRow := applicationRow(manualVerifyAppID)
		rig.appRepo.EXPECT().GetByID(mock.Anything, manualVerifyAppID).Return(appRow, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationID(mock.Anything, manualVerifyAppID).
			Return([]*repository.CreatorApplicationSocialRow{
				{ID: manualVerifySocialID, ApplicationID: manualVerifyAppID, Platform: domain.SocialPlatformTikTok, Handle: "aidana_tt"},
			}, nil)
		rig.appTelegramLinkRepo.EXPECT().GetByApplicationID(mock.Anything, manualVerifyAppID).
			Return(nil, sql.ErrNoRows)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.VerifyApplicationSocialManually(context.Background(), manualVerifyAppID, manualVerifySocialID, manualVerifyAdminID)
		require.ErrorIs(t, err, domain.ErrCreatorApplicationTelegramNotLinked)
	})

	t.Run("update social db error rolls back tx and bubbles", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectManualVerifyTxBegin(rig)
		expectManualVerifyFactoryWiring(rig)

		appRow := applicationRow(manualVerifyAppID)
		rig.appRepo.EXPECT().GetByID(mock.Anything, manualVerifyAppID).Return(appRow, nil)
		rig.appSocialRepo.EXPECT().ListByApplicationID(mock.Anything, manualVerifyAppID).
			Return([]*repository.CreatorApplicationSocialRow{
				{ID: manualVerifySocialID, ApplicationID: manualVerifyAppID, Platform: domain.SocialPlatformTikTok, Handle: "aidana_tt"},
			}, nil)
		rig.appTelegramLinkRepo.EXPECT().GetByApplicationID(mock.Anything, manualVerifyAppID).
			Return(&repository.CreatorApplicationTelegramLinkRow{ApplicationID: manualVerifyAppID, TelegramUserID: 555}, nil)
		rig.appSocialRepo.EXPECT().UpdateVerification(mock.Anything, mock.Anything).
			Return(errors.New("update failed"))

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.VerifyApplicationSocialManually(context.Background(), manualVerifyAppID, manualVerifySocialID, manualVerifyAdminID)
		require.ErrorContains(t, err, "update failed")
	})
}

// expectRejectTxBegin wires the mock pool for the single WithTx call inside
// RejectApplication.
func expectRejectTxBegin(rig creatorServiceRig) {
	rig.pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
}

// expectRejectFactoryWiring registers every repo constructor the
// RejectApplication TX issues. NewCreatorApplicationRepo also covers the
// inner applyTransition lookup; NewCreatorApplicationStatusTransitionRepo
// is set even on early-exit tests so partial-wiring noise stays out of test
// failures (mockery only fails on unmet expectations, not on unused ones).
func expectRejectFactoryWiring(rig creatorServiceRig) {
	rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
	rig.factory.EXPECT().NewAuditRepo(mock.Anything).Return(rig.auditRepo)
}

const (
	rejectAdminID = "aaaa1111-1111-1111-1111-111111111111"
	rejectAppID   = "bbbb2222-2222-2222-2222-222222222222"
)

func TestCreatorApplicationService_RejectApplication(t *testing.T) {
	t.Parallel()

	t.Run("application not found returns ErrCreatorApplicationNotFound and writes nothing", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectRejectTxBegin(rig)
		expectRejectFactoryWiring(rig)

		rig.appRepo.EXPECT().GetByID(mock.Anything, rejectAppID).Return(nil, sql.ErrNoRows)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.RejectApplication(context.Background(), rejectAppID, rejectAdminID)
		require.ErrorIs(t, err, domain.ErrCreatorApplicationNotFound)
	})

	t.Run("get application repo error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectRejectTxBegin(rig)
		expectRejectFactoryWiring(rig)

		rig.appRepo.EXPECT().GetByID(mock.Anything, rejectAppID).Return(nil, errors.New("db down"))

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.RejectApplication(context.Background(), rejectAppID, rejectAdminID)
		require.ErrorContains(t, err, "reject application: lookup application")
		require.ErrorContains(t, err, "db down")
	})

	notRejectableStatuses := []string{
		domain.CreatorApplicationStatusRejected,
		domain.CreatorApplicationStatusWithdrawn,
		domain.CreatorApplicationStatusApproved,
	}
	for _, status := range notRejectableStatuses {
		status := status
		t.Run("not rejectable from "+status+" returns ErrCreatorApplicationNotRejectable and writes nothing", func(t *testing.T) {
			t.Parallel()
			rig := newCreatorServiceRig(t)
			expectRejectTxBegin(rig)
			expectRejectFactoryWiring(rig)

			appRow := applicationRow(rejectAppID)
			appRow.Status = status
			rig.appRepo.EXPECT().GetByID(mock.Anything, rejectAppID).Return(appRow, nil)

			svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
			err := svc.RejectApplication(context.Background(), rejectAppID, rejectAdminID)
			require.ErrorIs(t, err, domain.ErrCreatorApplicationNotRejectable)
		})
	}

	t.Run("update status error rolls back tx and bubbles", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectRejectTxBegin(rig)
		expectRejectFactoryWiring(rig)
		rig.factory.EXPECT().NewCreatorApplicationStatusTransitionRepo(mock.Anything).Return(rig.transitionRepo)

		appRow := applicationRow(rejectAppID)
		rig.appRepo.EXPECT().GetByID(mock.Anything, rejectAppID).Return(appRow, nil)
		rig.appRepo.EXPECT().UpdateStatus(mock.Anything, rejectAppID, domain.CreatorApplicationStatusRejected).
			Return(errors.New("update boom"))

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.RejectApplication(context.Background(), rejectAppID, rejectAdminID)
		require.ErrorContains(t, err, "apply transition")
		require.ErrorContains(t, err, "update boom")
	})

	t.Run("audit write error wrapped — caller sees reject application context", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectRejectTxBegin(rig)
		expectRejectFactoryWiring(rig)
		rig.factory.EXPECT().NewCreatorApplicationStatusTransitionRepo(mock.Anything).Return(rig.transitionRepo)

		appRow := applicationRow(rejectAppID)
		rig.appRepo.EXPECT().GetByID(mock.Anything, rejectAppID).Return(appRow, nil)
		rig.appRepo.EXPECT().UpdateStatus(mock.Anything, rejectAppID, domain.CreatorApplicationStatusRejected).Return(nil)
		rig.transitionRepo.EXPECT().Insert(mock.Anything, mock.Anything).Return(nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("audit boom"))

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.RejectApplication(context.Background(), rejectAppID, rejectAdminID)
		require.ErrorContains(t, err, "reject application: write audit")
		require.ErrorContains(t, err, "audit boom")
	})

	for _, fromStatus := range []string{
		domain.CreatorApplicationStatusVerification,
		domain.CreatorApplicationStatusModeration,
	} {
		fromStatus := fromStatus
		t.Run("happy path from "+fromStatus+" — transitions, audits, notifies linked telegram", func(t *testing.T) {
			t.Parallel()
			rig := newCreatorServiceRig(t)
			expectRejectTxBegin(rig)
			expectRejectFactoryWiring(rig)
			rig.factory.EXPECT().NewCreatorApplicationStatusTransitionRepo(mock.Anything).Return(rig.transitionRepo)

			appRow := applicationRow(rejectAppID)
			appRow.Status = fromStatus
			rig.appRepo.EXPECT().GetByID(mock.Anything, rejectAppID).Return(appRow, nil)
			rig.appRepo.EXPECT().UpdateStatus(mock.Anything, rejectAppID, domain.CreatorApplicationStatusRejected).Return(nil)

			var capturedTransition repository.CreatorApplicationStatusTransitionRow
			rig.transitionRepo.EXPECT().Insert(mock.Anything, mock.AnythingOfType("repository.CreatorApplicationStatusTransitionRow")).
				Run(func(_ context.Context, row repository.CreatorApplicationStatusTransitionRow) {
					capturedTransition = row
				}).
				Return(nil)

			var capturedAudit repository.AuditLogRow
			rig.auditRepo.EXPECT().Create(mock.Anything, mock.AnythingOfType("repository.AuditLogRow")).
				Run(func(_ context.Context, row repository.AuditLogRow) {
					capturedAudit = row
				}).
				Return(nil)

			rig.factory.EXPECT().NewCreatorApplicationTelegramLinkRepo(mock.Anything).Return(rig.appTelegramLinkRepo)
			rig.appTelegramLinkRepo.EXPECT().GetByApplicationID(mock.Anything, rejectAppID).
				Return(&repository.CreatorApplicationTelegramLinkRow{
					ApplicationID:  rejectAppID,
					TelegramUserID: 12345,
				}, nil)

			var capturedChat int64
			rig.notifier.EXPECT().NotifyApplicationRejected(mock.Anything, int64(12345)).
				Run(func(_ context.Context, chatID int64) {
					capturedChat = chatID
				}).
				Once()

			svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
			err := svc.RejectApplication(context.Background(), rejectAppID, rejectAdminID)
			require.NoError(t, err)

			require.Equal(t, int64(12345), capturedChat)

			require.Equal(t, rejectAppID, capturedTransition.ApplicationID)
			require.NotNil(t, capturedTransition.FromStatus)
			require.Equal(t, fromStatus, *capturedTransition.FromStatus)
			require.Equal(t, domain.CreatorApplicationStatusRejected, capturedTransition.ToStatus)
			require.NotNil(t, capturedTransition.ActorID)
			require.Equal(t, rejectAdminID, *capturedTransition.ActorID)
			require.NotNil(t, capturedTransition.Reason)
			require.Equal(t, domain.TransitionReasonReject, *capturedTransition.Reason)

			require.Equal(t, AuditActionCreatorApplicationReject, capturedAudit.Action)
			require.Equal(t, AuditEntityTypeCreatorApplication, capturedAudit.EntityType)
			require.NotNil(t, capturedAudit.EntityID)
			require.Equal(t, rejectAppID, *capturedAudit.EntityID)
			require.NotNil(t, capturedAudit.ActorID)
			require.Equal(t, rejectAdminID, *capturedAudit.ActorID)
			require.JSONEq(t,
				`{"application_id":"`+rejectAppID+`","from_status":"`+fromStatus+`","to_status":"rejected"}`,
				string(capturedAudit.NewValue))
		})
	}

	t.Run("happy path without telegram link — warns, never notifies", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectRejectTxBegin(rig)
		expectRejectFactoryWiring(rig)
		rig.factory.EXPECT().NewCreatorApplicationStatusTransitionRepo(mock.Anything).Return(rig.transitionRepo)

		appRow := applicationRow(rejectAppID)
		rig.appRepo.EXPECT().GetByID(mock.Anything, rejectAppID).Return(appRow, nil)
		rig.appRepo.EXPECT().UpdateStatus(mock.Anything, rejectAppID, domain.CreatorApplicationStatusRejected).Return(nil)
		rig.transitionRepo.EXPECT().Insert(mock.Anything, mock.Anything).Return(nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)

		rig.factory.EXPECT().NewCreatorApplicationTelegramLinkRepo(mock.Anything).Return(rig.appTelegramLinkRepo)
		rig.appTelegramLinkRepo.EXPECT().GetByApplicationID(mock.Anything, rejectAppID).
			Return(nil, sql.ErrNoRows)

		rig.logger.EXPECT().Warn(mock.Anything,
			"creator application rejected without telegram link",
			mock.MatchedBy(func(args []any) bool {
				return len(args) == 2 && args[0] == "application_id" && args[1] == rejectAppID
			})).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.RejectApplication(context.Background(), rejectAppID, rejectAdminID)
		require.NoError(t, err)
	})

	t.Run("link lookup error logged, never notifies, reject still succeeds", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		expectRejectTxBegin(rig)
		expectRejectFactoryWiring(rig)
		rig.factory.EXPECT().NewCreatorApplicationStatusTransitionRepo(mock.Anything).Return(rig.transitionRepo)

		appRow := applicationRow(rejectAppID)
		rig.appRepo.EXPECT().GetByID(mock.Anything, rejectAppID).Return(appRow, nil)
		rig.appRepo.EXPECT().UpdateStatus(mock.Anything, rejectAppID, domain.CreatorApplicationStatusRejected).Return(nil)
		rig.transitionRepo.EXPECT().Insert(mock.Anything, mock.Anything).Return(nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)

		lookupErr := errors.New("db down")
		rig.factory.EXPECT().NewCreatorApplicationTelegramLinkRepo(mock.Anything).Return(rig.appTelegramLinkRepo)
		rig.appTelegramLinkRepo.EXPECT().GetByApplicationID(mock.Anything, rejectAppID).
			Return(nil, lookupErr)

		rig.logger.EXPECT().Error(mock.Anything,
			"creator application reject notify lookup failed",
			mock.MatchedBy(func(args []any) bool {
				if len(args) != 4 {
					return false
				}
				if args[0] != "application_id" || args[1] != rejectAppID || args[2] != "error" {
					return false
				}
				gotErr, ok := args[3].(error)
				return ok && errors.Is(gotErr, lookupErr)
			})).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.RejectApplication(context.Background(), rejectAppID, rejectAdminID)
		require.NoError(t, err)
	})
}

func TestCreatorApplicationService_applyTransition(t *testing.T) {
	t.Parallel()

	t.Run("disallowed transition surfaces ErrInvalidStatusTransition with from→to context", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.notifier, rig.logger)

		// approved → moderation is not declared in the state machine; the
		// helper must reject it before any repo wiring.
		err := svc.applyTransition(
			context.Background(),
			testTx{},
			&repository.CreatorApplicationRow{ID: "app-1", Status: domain.CreatorApplicationStatusApproved},
			domain.CreatorApplicationStatusModeration,
			nil,
			"",
		)
		require.ErrorIs(t, err, domain.ErrInvalidStatusTransition)
		require.ErrorContains(t, err, domain.CreatorApplicationStatusApproved)
		require.ErrorContains(t, err, domain.CreatorApplicationStatusModeration)
	})
}
