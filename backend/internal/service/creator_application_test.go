package service

import (
	"context"
	"database/sql"
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
		City:          "Алматы",
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
type creatorServiceRig struct {
	pool            *dbmocks.MockPool
	factory         *svcmocks.MockCreatorApplicationRepoFactory
	appRepo         *repomocks.MockCreatorApplicationRepo
	dictRepo        *repomocks.MockDictionaryRepo
	appCategoryRepo *repomocks.MockCreatorApplicationCategoryRepo
	appSocialRepo   *repomocks.MockCreatorApplicationSocialRepo
	appConsentRepo  *repomocks.MockCreatorApplicationConsentRepo
	auditRepo       *repomocks.MockAuditRepo
	logger          *logmocks.MockLogger
}

func newCreatorServiceRig(t *testing.T) creatorServiceRig {
	t.Helper()
	return creatorServiceRig{
		pool:            dbmocks.NewMockPool(t),
		factory:         svcmocks.NewMockCreatorApplicationRepoFactory(t),
		appRepo:         repomocks.NewMockCreatorApplicationRepo(t),
		dictRepo:        repomocks.NewMockDictionaryRepo(t),
		appCategoryRepo: repomocks.NewMockCreatorApplicationCategoryRepo(t),
		appSocialRepo:   repomocks.NewMockCreatorApplicationSocialRepo(t),
		appConsentRepo:  repomocks.NewMockCreatorApplicationConsentRepo(t),
		auditRepo:       repomocks.NewMockAuditRepo(t),
		logger:          logmocks.NewMockLogger(t),
	}
}

// expectTxBegin wires the mock pool so it returns testTx{} for the single
// WithTx call issued by a submission.
func expectTxBegin(rig creatorServiceRig) {
	rig.pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
}

// expectFactoryWiring configures the factory calls every TX performs eagerly
// at the top of the Submit transaction. The dictionary repo is constructed
// lazily inside resolveCategoryIDs and is wired separately by tests that
// reach that code path (see expectDictionaryWiring).
func expectFactoryWiring(rig creatorServiceRig) {
	rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
	rig.factory.EXPECT().NewCreatorApplicationCategoryRepo(mock.Anything).Return(rig.appCategoryRepo)
	rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo)
	rig.factory.EXPECT().NewCreatorApplicationConsentRepo(mock.Anything).Return(rig.appConsentRepo)
	rig.factory.EXPECT().NewAuditRepo(mock.Anything).Return(rig.auditRepo)
}

// expectDictionaryWiring registers the lazy NewDictionaryRepo call made by
// resolveCategoryIDs once the duplicate check has passed.
func expectDictionaryWiring(rig creatorServiceRig) {
	rig.factory.EXPECT().NewDictionaryRepo(mock.Anything).Return(rig.dictRepo)
}

func TestCreatorApplicationService_Submit(t *testing.T) {
	t.Parallel()

	t.Run("missing consent fails before tx", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		in.Consents.AcceptedAll = false

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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
				{ID: "c-1", Code: "beauty", Active: true},
				{ID: "c-2", Code: "fashion", Active: true},
			}, nil)
		rig.appRepo.EXPECT().Create(mock.Anything, mock.Anything).
			Return(&repository.CreatorApplicationRow{ID: "app-th"}, nil)
		rig.appCategoryRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.appSocialRepo.EXPECT().InsertMany(mock.Anything, []repository.CreatorApplicationSocialRow{
			{ApplicationID: "app-th", Platform: "threads", Handle: "aidana"},
		}).Return(nil)
		rig.appConsentRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		rig.logger.EXPECT().Info(mock.Anything, "creator application submitted", []any{"application_id", "app-th"}).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
		_, err := svc.Submit(context.Background(), in)
		require.NoError(t, err)
	})

	t.Run("too many categories rejected before tx", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		in.CategoryCodes = []string{"beauty", "fashion", "food", "fitness"} // 4 > MaxCategoriesPerApplication

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
		_, err := svc.Submit(context.Background(), in)

		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, domain.CodeValidation, ve.Code)
		require.Contains(t, ve.Message, "Максимум")
	})

	t.Run("other category without text rejected before tx", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		in.CategoryCodes = []string{"beauty", "other"}
		in.CategoryOtherText = nil

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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
			Return([]*repository.DictionaryEntryRow{{ID: "c-1", Code: "beauty", Active: true}}, nil)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
		_, err := svc.Submit(context.Background(), in)

		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, domain.CodeUnknownCategory, ve.Code)
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
				{ID: "c-1", Code: "beauty", Active: true},
				{ID: "c-2", Code: "fashion", Active: true},
			}, nil)
		rig.appRepo.EXPECT().Create(mock.Anything, mock.Anything).
			Return((*repository.CreatorApplicationRow)(nil), errors.New("insert failed"))

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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
				{ID: "c-1", Code: "beauty", Active: true},
				{ID: "c-2", Code: "fashion", Active: true},
			}, nil)
		rig.appRepo.EXPECT().Create(mock.Anything, repository.CreatorApplicationRow{
			LastName:   "Муратова",
			FirstName:  "Айдана",
			MiddleName: pointer.ToString("Ивановна"),
			IIN:        "950515312348",
			BirthDate:  birth,
			Phone:      "+77001234567",
			City:       "Алматы",
			Status:     domain.CreatorApplicationStatusPending,
		}).Return(&repository.CreatorApplicationRow{
			ID:         "app-1",
			LastName:   "Муратова",
			FirstName:  "Айдана",
			MiddleName: pointer.ToString("Ивановна"),
			IIN:        "950515312348",
			BirthDate:  birth,
			Phone:      "+77001234567",
			City:       "Алматы",
			Status:     "pending",
			CreatedAt:  created,
			UpdatedAt:  created,
		}, nil)
		rig.appCategoryRepo.EXPECT().InsertMany(mock.Anything, []repository.CreatorApplicationCategoryRow{
			{ApplicationID: "app-1", CategoryID: "c-1"},
			{ApplicationID: "app-1", CategoryID: "c-2"},
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
			return row.Action == AuditActionCreatorApplicationSubmit &&
				row.EntityType == AuditEntityTypeCreatorApplication &&
				row.EntityID != nil && *row.EntityID == "app-1" &&
				row.ActorID == nil
		})).Return(nil)
		rig.logger.EXPECT().Info(mock.Anything, "creator application submitted", []any{"application_id", "app-1"}).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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
				{ID: "c-1", Code: "beauty", Active: true},
				{ID: "c-2", Code: "fashion", Active: true},
			}, nil)
		rig.appRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row repository.CreatorApplicationRow) bool {
			return row.Address != nil && *row.Address == "ул. Абая 1"
		})).Return(&repository.CreatorApplicationRow{ID: "app-addr"}, nil)
		rig.appCategoryRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.appSocialRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.appConsentRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		rig.logger.EXPECT().Info(mock.Anything, "creator application submitted", []any{"application_id", "app-addr"}).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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
				{ID: "c-1", Code: "beauty", Active: true},
				{ID: "c-2", Code: "fashion", Active: true},
			}, nil)
		rig.appRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row repository.CreatorApplicationRow) bool {
			return row.Address == nil
		})).Return(&repository.CreatorApplicationRow{ID: "app-blank"}, nil)
		rig.appCategoryRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.appSocialRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.appConsentRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		rig.logger.EXPECT().Info(mock.Anything, "creator application submitted", []any{"application_id", "app-blank"}).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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
			Return([]*repository.DictionaryEntryRow{{ID: "c-other", Code: "other", Active: true}}, nil)
		rig.appRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row repository.CreatorApplicationRow) bool {
			return row.CategoryOtherText != nil && *row.CategoryOtherText == trimmed
		})).Return(&repository.CreatorApplicationRow{ID: "app-other"}, nil)
		rig.appCategoryRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.appSocialRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.appConsentRepo.EXPECT().InsertMany(mock.Anything, mock.Anything).Return(nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		rig.logger.EXPECT().Info(mock.Anything, "creator application submitted", []any{"application_id", "app-other"}).Once()

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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
			{"city", func(in *domain.CreatorApplicationInput) { in.City = "  " }, "city"},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				rig := newCreatorServiceRig(t)
				in := validCreatorInput(t)
				tc.mutate(&in)

				svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
		_, err := svc.Submit(context.Background(), in)

		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, domain.CodeValidation, ve.Code)
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
				{ID: "c-1", Code: "beauty", Active: true},
				{ID: "c-2", Code: "fashion", Active: true},
			}, nil)
		rig.appRepo.EXPECT().Create(mock.Anything, mock.Anything).
			Return((*repository.CreatorApplicationRow)(nil), domain.ErrCreatorApplicationDuplicate)

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
		_, err := svc.GetByID(context.Background(), appID)
		require.ErrorContains(t, err, "list consents")
		require.ErrorContains(t, err, "con boom")
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
		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).
			Return(&repository.CreatorApplicationRow{
				ID:                appID,
				LastName:          "Муратова",
				FirstName:         "Айдана",
				MiddleName:        pointer.ToString("Ивановна"),
				IIN:               "950515312348",
				BirthDate:         birth,
				Phone:             "+77001234567",
				City:              "Алматы",
				Address:           pointer.ToString("ул. Абая 1"),
				CategoryOtherText: pointer.ToString("Авторские ASMR"),
				Status:            domain.CreatorApplicationStatusPending,
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

		svc := NewCreatorApplicationService(rig.pool, rig.factory, rig.logger)
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
			City:              "Алматы",
			Address:           pointer.ToString("ул. Абая 1"),
			CategoryOtherText: pointer.ToString("Авторские ASMR"),
			Status:            domain.CreatorApplicationStatusPending,
			CreatedAt:         created,
			UpdatedAt:         updated,
			Categories: []string{"beauty", "fashion"},
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
}
