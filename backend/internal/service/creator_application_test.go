package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	dbmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	repomocks "github.com/alikhanmurzayev/ugcboost/backend/internal/repository/mocks"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	svcmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/service/mocks"
)

// validCreatorInput builds an input that passes every precondition so scenarios
// can selectively invalidate one field to hit a specific branch.
// IIN 950515312348 encodes 1995-05-15, which is safely over 18 against the
// fixed "now" used by every test.
func validCreatorInput(t *testing.T) domain.CreatorApplicationInput {
	t.Helper()
	middle := "Ивановна"
	return domain.CreatorApplicationInput{
		LastName:   "Муратова",
		FirstName:  "Айдана",
		MiddleName: &middle,
		IIN:        "950515312348",
		Phone:      "+77001234567",
		City:       "Алматы",
		Address:    "ул. Абая 1",
		CategoryCodes: []string{"beauty", "fashion"},
		Socials: []domain.SocialAccountInput{
			{Platform: domain.SocialPlatformInstagram, Handle: "@aidana"},
			{Platform: domain.SocialPlatformTikTok, Handle: "aidana_tt"},
		},
		Consents: domain.ConsentsInput{
			Processing:  true,
			ThirdParty:  true,
			CrossBorder: true,
			Terms:       true,
		},
		IPAddress:        "127.0.0.1",
		UserAgent:        "ua/1",
		AgreementVersion: "2026-04-20",
		PrivacyVersion:   "2026-04-20",
		Now:              time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC),
	}
}

// newCreatorServiceMocks assembles the common mock rig used by every test.
// Returning them explicitly makes it easy to set expectations per scenario.
type creatorServiceRig struct {
	pool            *dbmocks.MockPool
	factory         *svcmocks.MockCreatorApplicationRepoFactory
	appRepo         *repomocks.MockCreatorApplicationRepo
	categoryRepo    *repomocks.MockCategoryRepo
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
		categoryRepo:    repomocks.NewMockCategoryRepo(t),
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

// expectFactoryWiring configures the factory calls every TX performs.
// A test can simply disable the ones it does not need by calling
// .Return(nil).Maybe() — but since we always exercise every step on happy
// paths, we expect them in order here.
func expectFactoryWiring(rig creatorServiceRig) {
	rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
	rig.factory.EXPECT().NewCategoryRepo(mock.Anything).Return(rig.categoryRepo)
	rig.factory.EXPECT().NewCreatorApplicationCategoryRepo(mock.Anything).Return(rig.appCategoryRepo)
	rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.appSocialRepo)
	rig.factory.EXPECT().NewCreatorApplicationConsentRepo(mock.Anything).Return(rig.appConsentRepo)
	rig.factory.EXPECT().NewAuditRepo(mock.Anything).Return(rig.auditRepo)
}

func TestCreatorApplicationService_Submit(t *testing.T) {
	t.Parallel()

	t.Run("missing consent fails before tx", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		in.Consents.CrossBorder = false

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

	t.Run("under 18 fails before tx", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorServiceRig(t)
		in := validCreatorInput(t)
		// Shift "now" to just after IIN's birth + 17 years: still 17.
		in.Now = time.Date(2012, 5, 14, 0, 0, 0, 0, time.UTC)

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
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(false, nil)
		rig.categoryRepo.EXPECT().GetActiveByCodes(mock.Anything, []string{"beauty", "mystery"}).
			Return([]*repository.CategoryRow{{ID: "c-1", Code: "beauty", Active: true}}, nil)

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
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(false, nil)
		rig.categoryRepo.EXPECT().GetActiveByCodes(mock.Anything, []string{"beauty", "fashion"}).
			Return([]*repository.CategoryRow{
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
		middle := "Ивановна"

		expectTxBegin(rig)
		expectFactoryWiring(rig)
		rig.appRepo.EXPECT().HasActiveByIIN(mock.Anything, in.IIN).Return(false, nil)
		rig.categoryRepo.EXPECT().GetActiveByCodes(mock.Anything, []string{"beauty", "fashion"}).
			Return([]*repository.CategoryRow{
				{ID: "c-1", Code: "beauty", Active: true},
				{ID: "c-2", Code: "fashion", Active: true},
			}, nil)
		rig.appRepo.EXPECT().Create(mock.Anything, repository.CreatorApplicationRow{
			LastName:   "Муратова",
			FirstName:  "Айдана",
			MiddleName: &middle,
			IIN:        "950515312348",
			BirthDate:  birth,
			Phone:      "+77001234567",
			City:       "Алматы",
			Address:    "ул. Абая 1",
		}).Return(&repository.CreatorApplicationRow{
			ID:         "app-1",
			LastName:   "Муратова",
			FirstName:  "Айдана",
			MiddleName: &middle,
			IIN:        "950515312348",
			BirthDate:  birth,
			Phone:      "+77001234567",
			City:       "Алматы",
			Address:    "ул. Абая 1",
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
}
