package service

import (
	"context"
	"database/sql"
	"errors"
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

const (
	getCreatorID         = "aaaa1111-1111-1111-1111-111111111111"
	getCreatorAppID      = "bbbb2222-2222-2222-2222-222222222222"
	getCreatorVerifierID = "cccc3333-3333-3333-3333-333333333333"
)

// creatorReadRig keeps the read-side test rig small — CreatorService needs
// just six repos (no transitions, no audit) and a pool placeholder, so we do
// not reuse the writer-side creatorServiceRig from creator_application_test.
type creatorReadRig struct {
	pool                *dbmocks.MockPool
	factory             *svcmocks.MockCreatorRepoFactory
	creatorRepo         *repomocks.MockCreatorRepo
	creatorSocialRepo   *repomocks.MockCreatorSocialRepo
	creatorCategoryRepo *repomocks.MockCreatorCategoryRepo
	campaignCreatorRepo *repomocks.MockCampaignCreatorRepo
	campaignRepo        *repomocks.MockCampaignRepo
	dictRepo            *repomocks.MockDictionaryRepo
	logger              *logmocks.MockLogger
}

func newCreatorReadRig(t *testing.T) creatorReadRig {
	t.Helper()
	return creatorReadRig{
		pool:                dbmocks.NewMockPool(t),
		factory:             svcmocks.NewMockCreatorRepoFactory(t),
		creatorRepo:         repomocks.NewMockCreatorRepo(t),
		creatorSocialRepo:   repomocks.NewMockCreatorSocialRepo(t),
		creatorCategoryRepo: repomocks.NewMockCreatorCategoryRepo(t),
		campaignCreatorRepo: repomocks.NewMockCampaignCreatorRepo(t),
		campaignRepo:        repomocks.NewMockCampaignRepo(t),
		dictRepo:            repomocks.NewMockDictionaryRepo(t),
		logger:              logmocks.NewMockLogger(t),
	}
}

func fullCreatorRow() *repository.CreatorRow {
	createdAt := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	return &repository.CreatorRow{
		ID:                  getCreatorID,
		IIN:                 "950515312348",
		LastName:            "Муратова",
		FirstName:           "Айдана",
		MiddleName:          pointer.ToString("Ивановна"),
		BirthDate:           time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC),
		Phone:               "+77001234567",
		CityCode:            "almaty",
		Address:             pointer.ToString("ул. Абая 10"),
		CategoryOtherText:   pointer.ToString("стримы"),
		TelegramUserID:      424242,
		TelegramUsername:    pointer.ToString("aidana_tg"),
		TelegramFirstName:   pointer.ToString("Айдана"),
		TelegramLastName:    pointer.ToString("Муратова"),
		SourceApplicationID: getCreatorAppID,
		CreatedAt:           createdAt,
		UpdatedAt:           updatedAt,
	}
}

func fullCreatorSocialRows() []*repository.CreatorSocialRow {
	verifiedAt := time.Date(2026, 5, 5, 11, 30, 0, 0, time.UTC)
	createdAt := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	return []*repository.CreatorSocialRow{
		{
			ID:               "social-ig",
			CreatorID:        getCreatorID,
			Platform:         domain.SocialPlatformInstagram,
			Handle:           "aidana",
			Verified:         true,
			Method:           pointer.ToString(domain.SocialVerificationMethodAuto),
			VerifiedByUserID: nil,
			VerifiedAt:       &verifiedAt,
			CreatedAt:        createdAt,
		},
		{
			ID:        "social-th",
			CreatorID: getCreatorID,
			Platform:  domain.SocialPlatformThreads,
			Handle:    "aidana_th",
			Verified:  false,
			CreatedAt: createdAt,
		},
		{
			ID:               "social-tt",
			CreatorID:        getCreatorID,
			Platform:         domain.SocialPlatformTikTok,
			Handle:           "aidana_tt",
			Verified:         true,
			Method:           pointer.ToString(domain.SocialVerificationMethodManual),
			VerifiedByUserID: pointer.ToString(getCreatorVerifierID),
			VerifiedAt:       &verifiedAt,
			CreatedAt:        createdAt,
		},
	}
}

// expectFactoryCalls wires the NewXRepo calls the service issues at the top
// of GetByID. Tests that exit early stop after the calls their path reaches
// — mockery only fails on unmet expectations.
func expectCreatorReadFactoryWiring(rig creatorReadRig, includeSocial, includeCategory, includeParticipation, includeDict bool) {
	rig.factory.EXPECT().NewCreatorRepo(rig.pool).Return(rig.creatorRepo)
	if includeSocial {
		rig.factory.EXPECT().NewCreatorSocialRepo(rig.pool).Return(rig.creatorSocialRepo)
	}
	if includeCategory {
		rig.factory.EXPECT().NewCreatorCategoryRepo(rig.pool).Return(rig.creatorCategoryRepo)
	}
	if includeParticipation {
		rig.factory.EXPECT().NewCampaignCreatorRepo(rig.pool).Return(rig.campaignCreatorRepo)
	}
	if includeDict {
		rig.factory.EXPECT().NewDictionaryRepo(rig.pool).Return(rig.dictRepo)
	}
}

// expectEmptyParticipations expects a single ListByCreatorIDs call that
// returns no rows for the given creator ids — the service then short-circuits
// without touching CampaignRepo. Helper avoids repeating the boilerplate in
// every test where the participation branch is wired but irrelevant.
func expectEmptyParticipations(rig creatorReadRig, creatorIDs []string) {
	rig.campaignCreatorRepo.EXPECT().ListByCreatorIDs(mock.Anything, creatorIDs).
		Return(nil, nil)
}

func TestCreatorService_GetByID(t *testing.T) {
	t.Parallel()

	t.Run("creator not found returns ErrCreatorNotFound", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		expectCreatorReadFactoryWiring(rig, false, false, false, false)
		rig.creatorRepo.EXPECT().GetByID(mock.Anything, getCreatorID).Return(nil, sql.ErrNoRows)

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		_, err := svc.GetByID(context.Background(), getCreatorID)
		require.ErrorIs(t, err, domain.ErrCreatorNotFound)
	})

	t.Run("get creator repo error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		expectCreatorReadFactoryWiring(rig, false, false, false, false)
		rig.creatorRepo.EXPECT().GetByID(mock.Anything, getCreatorID).Return(nil, errors.New("db down"))

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		_, err := svc.GetByID(context.Background(), getCreatorID)
		require.ErrorContains(t, err, "get creator")
		require.ErrorContains(t, err, "db down")
	})

	t.Run("socials list error propagated", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		expectCreatorReadFactoryWiring(rig, true, false, false, false)
		rig.creatorRepo.EXPECT().GetByID(mock.Anything, getCreatorID).Return(fullCreatorRow(), nil)
		rig.creatorSocialRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).Return(nil, errors.New("socials boom"))

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		_, err := svc.GetByID(context.Background(), getCreatorID)
		require.ErrorContains(t, err, "list socials")
		require.ErrorContains(t, err, "socials boom")
	})

	t.Run("categories list error propagated", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		expectCreatorReadFactoryWiring(rig, true, true, false, false)
		rig.creatorRepo.EXPECT().GetByID(mock.Anything, getCreatorID).Return(fullCreatorRow(), nil)
		rig.creatorSocialRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(map[string][]*repository.CreatorSocialRow{getCreatorID: fullCreatorSocialRows()}, nil)
		rig.creatorCategoryRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).Return(nil, errors.New("categories boom"))

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		_, err := svc.GetByID(context.Background(), getCreatorID)
		require.ErrorContains(t, err, "list categories")
		require.ErrorContains(t, err, "categories boom")
	})

	t.Run("participation list error propagated", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		expectCreatorReadFactoryWiring(rig, true, true, true, false)
		rig.creatorRepo.EXPECT().GetByID(mock.Anything, getCreatorID).Return(fullCreatorRow(), nil)
		rig.creatorSocialRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(map[string][]*repository.CreatorSocialRow{getCreatorID: fullCreatorSocialRows()}, nil)
		rig.creatorCategoryRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(map[string][]string{getCreatorID: {"beauty"}}, nil)
		rig.campaignCreatorRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(nil, errors.New("participations boom"))

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		_, err := svc.GetByID(context.Background(), getCreatorID)
		require.ErrorContains(t, err, "list campaign participations")
		require.ErrorContains(t, err, "participations boom")
	})

	t.Run("campaign list-by-ids error propagated", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		expectCreatorReadFactoryWiring(rig, true, true, true, false)
		rig.factory.EXPECT().NewCampaignRepo(rig.pool).Return(rig.campaignRepo)
		participationCreated := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
		rig.creatorRepo.EXPECT().GetByID(mock.Anything, getCreatorID).Return(fullCreatorRow(), nil)
		rig.creatorSocialRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(map[string][]*repository.CreatorSocialRow{getCreatorID: fullCreatorSocialRows()}, nil)
		rig.creatorCategoryRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(map[string][]string{getCreatorID: {"beauty"}}, nil)
		rig.campaignCreatorRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return([]*repository.CampaignCreatorRow{
				{ID: "cc-1", CampaignID: "camp-1", CreatorID: getCreatorID, Status: domain.CampaignCreatorStatusInvited, CreatedAt: participationCreated},
			}, nil)
		rig.campaignRepo.EXPECT().ListByIDs(mock.Anything, []string{"camp-1"}).
			Return(nil, errors.New("campaigns boom"))

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		_, err := svc.GetByID(context.Background(), getCreatorID)
		require.ErrorContains(t, err, "list campaigns by ids")
		require.ErrorContains(t, err, "campaigns boom")
	})

	t.Run("dictionary city lookup error propagated", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		expectCreatorReadFactoryWiring(rig, true, true, true, true)
		rig.creatorRepo.EXPECT().GetByID(mock.Anything, getCreatorID).Return(fullCreatorRow(), nil)
		rig.creatorSocialRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(map[string][]*repository.CreatorSocialRow{getCreatorID: fullCreatorSocialRows()}, nil)
		rig.creatorCategoryRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(map[string][]string{getCreatorID: {"beauty", "fashion"}}, nil)
		expectEmptyParticipations(rig, []string{getCreatorID})
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCities, []string{"almaty"}).
			Return(nil, errors.New("city lookup down"))

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		_, err := svc.GetByID(context.Background(), getCreatorID)
		require.ErrorContains(t, err, "lookup city")
		require.ErrorContains(t, err, "city lookup down")
	})

	t.Run("dictionary category lookup error propagated", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		expectCreatorReadFactoryWiring(rig, true, true, true, true)
		rig.creatorRepo.EXPECT().GetByID(mock.Anything, getCreatorID).Return(fullCreatorRow(), nil)
		rig.creatorSocialRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(map[string][]*repository.CreatorSocialRow{getCreatorID: fullCreatorSocialRows()}, nil)
		rig.creatorCategoryRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(map[string][]string{getCreatorID: {"beauty", "fashion"}}, nil)
		expectEmptyParticipations(rig, []string{getCreatorID})
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCities, []string{"almaty"}).
			Return([]*repository.DictionaryEntryRow{{Code: "almaty", Name: "Алматы", Active: true}}, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"beauty", "fashion"}).
			Return(nil, errors.New("category lookup down"))

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		_, err := svc.GetByID(context.Background(), getCreatorID)
		require.ErrorContains(t, err, "lookup categories")
		require.ErrorContains(t, err, "category lookup down")
	})

	t.Run("happy full aggregate matches expected", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		expectCreatorReadFactoryWiring(rig, true, true, true, true)
		rig.factory.EXPECT().NewCampaignRepo(rig.pool).Return(rig.campaignRepo)

		creatorRow := fullCreatorRow()
		socialRows := fullCreatorSocialRows()
		participationCreated1 := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
		participationCreated2 := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
		participationCreated3 := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
		rig.creatorRepo.EXPECT().GetByID(mock.Anything, getCreatorID).Return(creatorRow, nil)
		rig.creatorSocialRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(map[string][]*repository.CreatorSocialRow{getCreatorID: socialRows}, nil)
		rig.creatorCategoryRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(map[string][]string{getCreatorID: {"beauty", "fashion", "lifestyle"}}, nil)
		rig.campaignCreatorRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return([]*repository.CampaignCreatorRow{
				{ID: "cc-1", CampaignID: "camp-active-1", CreatorID: getCreatorID, Status: domain.CampaignCreatorStatusSigned, CreatedAt: participationCreated1},
				{ID: "cc-2", CampaignID: "camp-deleted", CreatorID: getCreatorID, Status: domain.CampaignCreatorStatusInvited, CreatedAt: participationCreated2},
				{ID: "cc-3", CampaignID: "camp-active-2", CreatorID: getCreatorID, Status: domain.CampaignCreatorStatusPlanned, CreatedAt: participationCreated3},
			}, nil)
		rig.campaignRepo.EXPECT().ListByIDs(mock.Anything, mock.MatchedBy(func(ids []string) bool {
			want := map[string]struct{}{"camp-active-1": {}, "camp-deleted": {}, "camp-active-2": {}}
			if len(ids) != len(want) {
				return false
			}
			for _, id := range ids {
				if _, ok := want[id]; !ok {
					return false
				}
			}
			return true
		})).Return([]*repository.CampaignRow{
			{ID: "camp-active-1", Name: "Spring Drop", IsDeleted: false},
			{ID: "camp-deleted", Name: "Retired", IsDeleted: true},
			{ID: "camp-active-2", Name: "Holiday Push", IsDeleted: false},
		}, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCities, []string{"almaty"}).
			Return([]*repository.DictionaryEntryRow{{Code: "almaty", Name: "Алматы", Active: true}}, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"beauty", "fashion", "lifestyle"}).
			Return([]*repository.DictionaryEntryRow{
				{Code: "beauty", Name: "Красота", Active: true},
				{Code: "fashion", Name: "Мода", Active: true},
				{Code: "lifestyle", Name: "Лайфстайл", Active: true},
			}, nil)

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		got, err := svc.GetByID(context.Background(), getCreatorID)
		require.NoError(t, err)

		expected := &domain.CreatorAggregate{
			ID:                  getCreatorID,
			IIN:                 creatorRow.IIN,
			SourceApplicationID: getCreatorAppID,
			LastName:            creatorRow.LastName,
			FirstName:           creatorRow.FirstName,
			MiddleName:          creatorRow.MiddleName,
			BirthDate:           creatorRow.BirthDate,
			Phone:               creatorRow.Phone,
			CityCode:            "almaty",
			CityName:            "Алматы",
			Address:             creatorRow.Address,
			CategoryOtherText:   creatorRow.CategoryOtherText,
			TelegramUserID:      creatorRow.TelegramUserID,
			TelegramUsername:    creatorRow.TelegramUsername,
			TelegramFirstName:   creatorRow.TelegramFirstName,
			TelegramLastName:    creatorRow.TelegramLastName,
			Socials: []domain.CreatorAggregateSocial{
				{
					ID:               socialRows[0].ID,
					Platform:         socialRows[0].Platform,
					Handle:           socialRows[0].Handle,
					Verified:         socialRows[0].Verified,
					Method:           socialRows[0].Method,
					VerifiedByUserID: socialRows[0].VerifiedByUserID,
					VerifiedAt:       socialRows[0].VerifiedAt,
					CreatedAt:        socialRows[0].CreatedAt,
				},
				{
					ID:        socialRows[1].ID,
					Platform:  socialRows[1].Platform,
					Handle:    socialRows[1].Handle,
					Verified:  socialRows[1].Verified,
					CreatedAt: socialRows[1].CreatedAt,
				},
				{
					ID:               socialRows[2].ID,
					Platform:         socialRows[2].Platform,
					Handle:           socialRows[2].Handle,
					Verified:         socialRows[2].Verified,
					Method:           socialRows[2].Method,
					VerifiedByUserID: socialRows[2].VerifiedByUserID,
					VerifiedAt:       socialRows[2].VerifiedAt,
					CreatedAt:        socialRows[2].CreatedAt,
				},
			},
			Categories: []domain.CreatorAggregateCategory{
				{Code: "beauty", Name: "Красота"},
				{Code: "fashion", Name: "Мода"},
				{Code: "lifestyle", Name: "Лайфстайл"},
			},
			Campaigns: []domain.CreatorCampaignBrief{
				{ID: "camp-active-1", Name: "Spring Drop", Status: domain.CampaignCreatorStatusSigned},
				{ID: "camp-active-2", Name: "Holiday Push", Status: domain.CampaignCreatorStatusPlanned},
			},
			CreatedAt: creatorRow.CreatedAt,
			UpdatedAt: creatorRow.UpdatedAt,
		}
		require.Equal(t, expected, got)
	})

	t.Run("happy sparse — nullables stay nil, no participations", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		expectCreatorReadFactoryWiring(rig, true, true, true, true)

		creatorRow := &repository.CreatorRow{
			ID:                  getCreatorID,
			IIN:                 "950515312348",
			LastName:            "Иванов",
			FirstName:           "Алексей",
			MiddleName:          nil,
			BirthDate:           time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC),
			Phone:               "+77001234567",
			CityCode:            "astana",
			Address:             nil,
			CategoryOtherText:   nil,
			TelegramUserID:      525252,
			TelegramUsername:    nil,
			TelegramFirstName:   nil,
			TelegramLastName:    nil,
			SourceApplicationID: getCreatorAppID,
			CreatedAt:           time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC),
			UpdatedAt:           time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC),
		}
		rig.creatorRepo.EXPECT().GetByID(mock.Anything, getCreatorID).Return(creatorRow, nil)
		rig.creatorSocialRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(map[string][]*repository.CreatorSocialRow{}, nil)
		rig.creatorCategoryRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(map[string][]string{getCreatorID: {"beauty"}}, nil)
		expectEmptyParticipations(rig, []string{getCreatorID})
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCities, []string{"astana"}).
			Return([]*repository.DictionaryEntryRow{{Code: "astana", Name: "Астана", Active: true}}, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"beauty"}).
			Return([]*repository.DictionaryEntryRow{{Code: "beauty", Name: "Красота", Active: true}}, nil)

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		got, err := svc.GetByID(context.Background(), getCreatorID)
		require.NoError(t, err)

		expected := &domain.CreatorAggregate{
			ID:                  getCreatorID,
			IIN:                 creatorRow.IIN,
			SourceApplicationID: getCreatorAppID,
			LastName:            creatorRow.LastName,
			FirstName:           creatorRow.FirstName,
			MiddleName:          nil,
			BirthDate:           creatorRow.BirthDate,
			Phone:               creatorRow.Phone,
			CityCode:            "astana",
			CityName:            "Астана",
			Address:             nil,
			CategoryOtherText:   nil,
			TelegramUserID:      creatorRow.TelegramUserID,
			TelegramUsername:    nil,
			TelegramFirstName:   nil,
			TelegramLastName:    nil,
			Socials:             []domain.CreatorAggregateSocial{},
			Categories: []domain.CreatorAggregateCategory{
				{Code: "beauty", Name: "Красота"},
			},
			Campaigns: []domain.CreatorCampaignBrief{},
			CreatedAt: creatorRow.CreatedAt,
			UpdatedAt: creatorRow.UpdatedAt,
		}
		require.Equal(t, expected, got)
	})

	t.Run("all participations point to deleted campaigns → campaigns empty", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		expectCreatorReadFactoryWiring(rig, true, true, true, true)
		rig.factory.EXPECT().NewCampaignRepo(rig.pool).Return(rig.campaignRepo)

		creatorRow := fullCreatorRow()
		participationCreated := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
		rig.creatorRepo.EXPECT().GetByID(mock.Anything, getCreatorID).Return(creatorRow, nil)
		rig.creatorSocialRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(map[string][]*repository.CreatorSocialRow{}, nil)
		rig.creatorCategoryRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(map[string][]string{getCreatorID: {"beauty"}}, nil)
		rig.campaignCreatorRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return([]*repository.CampaignCreatorRow{
				{ID: "cc-1", CampaignID: "camp-x", CreatorID: getCreatorID, Status: domain.CampaignCreatorStatusInvited, CreatedAt: participationCreated},
			}, nil)
		rig.campaignRepo.EXPECT().ListByIDs(mock.Anything, []string{"camp-x"}).
			Return([]*repository.CampaignRow{
				{ID: "camp-x", Name: "Retired", IsDeleted: true},
			}, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCities, []string{"almaty"}).
			Return([]*repository.DictionaryEntryRow{{Code: "almaty", Name: "Алматы", Active: true}}, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"beauty"}).
			Return([]*repository.DictionaryEntryRow{{Code: "beauty", Name: "Красота", Active: true}}, nil)

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		got, err := svc.GetByID(context.Background(), getCreatorID)
		require.NoError(t, err)
		require.Equal(t, []domain.CreatorCampaignBrief{}, got.Campaigns)
	})

	t.Run("deactivated city falls back to code", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		expectCreatorReadFactoryWiring(rig, true, true, true, true)

		creatorRow := fullCreatorRow()
		rig.creatorRepo.EXPECT().GetByID(mock.Anything, getCreatorID).Return(creatorRow, nil)
		rig.creatorSocialRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(map[string][]*repository.CreatorSocialRow{}, nil)
		rig.creatorCategoryRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(map[string][]string{getCreatorID: {"beauty"}}, nil)
		expectEmptyParticipations(rig, []string{getCreatorID})
		// City row deactivated → empty result.
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCities, []string{"almaty"}).
			Return(nil, nil)
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"beauty"}).
			Return([]*repository.DictionaryEntryRow{{Code: "beauty", Name: "Красота", Active: true}}, nil)

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		got, err := svc.GetByID(context.Background(), getCreatorID)
		require.NoError(t, err)
		require.Equal(t, "almaty", got.CityCode)
		require.Equal(t, "almaty", got.CityName, "deactivated city must fall back to code")
	})

	t.Run("deactivated category falls back to code", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		expectCreatorReadFactoryWiring(rig, true, true, true, true)

		creatorRow := fullCreatorRow()
		rig.creatorRepo.EXPECT().GetByID(mock.Anything, getCreatorID).Return(creatorRow, nil)
		rig.creatorSocialRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(map[string][]*repository.CreatorSocialRow{}, nil)
		rig.creatorCategoryRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{getCreatorID}).
			Return(map[string][]string{getCreatorID: {"beauty", "retired_niche"}}, nil)
		expectEmptyParticipations(rig, []string{getCreatorID})
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCities, []string{"almaty"}).
			Return([]*repository.DictionaryEntryRow{{Code: "almaty", Name: "Алматы", Active: true}}, nil)
		// Only the active category row comes back; "retired_niche" is missing.
		rig.dictRepo.EXPECT().GetActiveByCodes(mock.Anything, repository.TableCategories, []string{"beauty", "retired_niche"}).
			Return([]*repository.DictionaryEntryRow{{Code: "beauty", Name: "Красота", Active: true}}, nil)

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		got, err := svc.GetByID(context.Background(), getCreatorID)
		require.NoError(t, err)
		require.Equal(t, []domain.CreatorAggregateCategory{
			{Code: "beauty", Name: "Красота"},
			{Code: "retired_niche", Name: "retired_niche"},
		}, got.Categories)
	})
}

func TestCreatorService_List(t *testing.T) {
	t.Parallel()

	const creatorA = "aaaa1111-1111-1111-1111-111111111111"
	const creatorB = "bbbb2222-2222-2222-2222-222222222222"

	birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
	created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC)

	listRows := func() []*repository.CreatorListRow {
		return []*repository.CreatorListRow{
			{
				ID:               creatorA,
				LastName:         "Муратова",
				FirstName:        "Айдана",
				MiddleName:       pointer.ToString("Ивановна"),
				IIN:              "950515312348",
				BirthDate:        birth,
				Phone:            "+77001234567",
				CityCode:         "almaty",
				TelegramUsername: pointer.ToString("aidana_tg"),
				CreatedAt:        created,
				UpdatedAt:        updated,
			},
			{
				ID:        creatorB,
				LastName:  "Иванов",
				FirstName: "Алексей",
				IIN:       "950515312349",
				BirthDate: birth,
				Phone:     "+77001234568",
				CityCode:  "astana",
				CreatedAt: created,
				UpdatedAt: updated,
			},
		}
	}

	t.Run("repo error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		rig.factory.EXPECT().NewCreatorRepo(rig.pool).Return(rig.creatorRepo)
		rig.creatorRepo.EXPECT().List(mock.Anything, mock.Anything).Return(nil, int64(0), errors.New("db down"))

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		_, err := svc.List(context.Background(), domain.CreatorListInput{Sort: domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 20})
		require.ErrorContains(t, err, "list creators")
		require.ErrorContains(t, err, "db down")
	})

	t.Run("empty page short-circuits hydration", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		rig.factory.EXPECT().NewCreatorRepo(rig.pool).Return(rig.creatorRepo)
		rig.creatorRepo.EXPECT().List(mock.Anything, mock.Anything).Return(nil, int64(0), nil)

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		got, err := svc.List(context.Background(), domain.CreatorListInput{Sort: domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 20})
		require.NoError(t, err)
		require.Equal(t, &domain.CreatorListPage{Items: nil, Total: 0, Page: 1, PerPage: 20}, got)
	})

	t.Run("socials hydrate error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		rig.factory.EXPECT().NewCreatorRepo(rig.pool).Return(rig.creatorRepo)
		rig.factory.EXPECT().NewCreatorSocialRepo(rig.pool).Return(rig.creatorSocialRepo)
		rig.creatorRepo.EXPECT().List(mock.Anything, mock.Anything).Return(listRows(), int64(2), nil)
		rig.creatorSocialRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{creatorA, creatorB}).
			Return(nil, errors.New("socials boom"))

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		_, err := svc.List(context.Background(), domain.CreatorListInput{Sort: domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 20})
		require.ErrorContains(t, err, "hydrate socials")
	})

	t.Run("categories hydrate error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		rig.factory.EXPECT().NewCreatorRepo(rig.pool).Return(rig.creatorRepo)
		rig.factory.EXPECT().NewCreatorSocialRepo(rig.pool).Return(rig.creatorSocialRepo)
		rig.factory.EXPECT().NewCreatorCategoryRepo(rig.pool).Return(rig.creatorCategoryRepo)
		rig.creatorRepo.EXPECT().List(mock.Anything, mock.Anything).Return(listRows(), int64(2), nil)
		rig.creatorSocialRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{creatorA, creatorB}).
			Return(map[string][]*repository.CreatorSocialRow{}, nil)
		rig.creatorCategoryRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{creatorA, creatorB}).
			Return(nil, errors.New("cats boom"))

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		_, err := svc.List(context.Background(), domain.CreatorListInput{Sort: domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 20})
		require.ErrorContains(t, err, "hydrate categories")
	})

	t.Run("participation list error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		rig.factory.EXPECT().NewCreatorRepo(rig.pool).Return(rig.creatorRepo)
		rig.factory.EXPECT().NewCreatorSocialRepo(rig.pool).Return(rig.creatorSocialRepo)
		rig.factory.EXPECT().NewCreatorCategoryRepo(rig.pool).Return(rig.creatorCategoryRepo)
		rig.factory.EXPECT().NewCampaignCreatorRepo(rig.pool).Return(rig.campaignCreatorRepo)
		rig.creatorRepo.EXPECT().List(mock.Anything, mock.Anything).Return(listRows(), int64(2), nil)
		rig.creatorSocialRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{creatorA, creatorB}).
			Return(map[string][]*repository.CreatorSocialRow{}, nil)
		rig.creatorCategoryRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{creatorA, creatorB}).
			Return(map[string][]string{}, nil)
		rig.campaignCreatorRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{creatorA, creatorB}).
			Return(nil, errors.New("participations boom"))

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		_, err := svc.List(context.Background(), domain.CreatorListInput{Sort: domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 20})
		require.ErrorContains(t, err, "list campaign participations")
		require.ErrorContains(t, err, "participations boom")
	})

	t.Run("campaign list-by-ids error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		rig.factory.EXPECT().NewCreatorRepo(rig.pool).Return(rig.creatorRepo)
		rig.factory.EXPECT().NewCreatorSocialRepo(rig.pool).Return(rig.creatorSocialRepo)
		rig.factory.EXPECT().NewCreatorCategoryRepo(rig.pool).Return(rig.creatorCategoryRepo)
		rig.factory.EXPECT().NewCampaignCreatorRepo(rig.pool).Return(rig.campaignCreatorRepo)
		rig.factory.EXPECT().NewCampaignRepo(rig.pool).Return(rig.campaignRepo)
		participationCreated := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
		rig.creatorRepo.EXPECT().List(mock.Anything, mock.Anything).Return(listRows(), int64(2), nil)
		rig.creatorSocialRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{creatorA, creatorB}).
			Return(map[string][]*repository.CreatorSocialRow{}, nil)
		rig.creatorCategoryRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{creatorA, creatorB}).
			Return(map[string][]string{}, nil)
		rig.campaignCreatorRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{creatorA, creatorB}).
			Return([]*repository.CampaignCreatorRow{
				{ID: "cc-1", CampaignID: "camp-1", CreatorID: creatorA, Status: domain.CampaignCreatorStatusInvited, CreatedAt: participationCreated},
			}, nil)
		rig.campaignRepo.EXPECT().ListByIDs(mock.Anything, []string{"camp-1"}).
			Return(nil, errors.New("campaigns boom"))

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		_, err := svc.List(context.Background(), domain.CreatorListInput{Sort: domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 20})
		require.ErrorContains(t, err, "list campaigns by ids")
		require.ErrorContains(t, err, "campaigns boom")
	})

	t.Run("happy two creators with hydration and active counts", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		rig.factory.EXPECT().NewCreatorRepo(rig.pool).Return(rig.creatorRepo)
		rig.factory.EXPECT().NewCreatorSocialRepo(rig.pool).Return(rig.creatorSocialRepo)
		rig.factory.EXPECT().NewCreatorCategoryRepo(rig.pool).Return(rig.creatorCategoryRepo)
		rig.factory.EXPECT().NewCampaignCreatorRepo(rig.pool).Return(rig.campaignCreatorRepo)
		rig.factory.EXPECT().NewCampaignRepo(rig.pool).Return(rig.campaignRepo)
		participationCreated := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
		rig.creatorRepo.EXPECT().List(mock.Anything, mock.Anything).Return(listRows(), int64(2), nil)
		rig.creatorSocialRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{creatorA, creatorB}).
			Return(map[string][]*repository.CreatorSocialRow{
				creatorA: {
					{ID: "s-a-1", CreatorID: creatorA, Platform: domain.SocialPlatformInstagram, Handle: "aidana", CreatedAt: created},
					{ID: "s-a-2", CreatorID: creatorA, Platform: domain.SocialPlatformTikTok, Handle: "aidana_tt", CreatedAt: created},
				},
				creatorB: {
					{ID: "s-b-1", CreatorID: creatorB, Platform: domain.SocialPlatformInstagram, Handle: "ivanov", CreatedAt: created},
				},
			}, nil)
		rig.creatorCategoryRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{creatorA, creatorB}).
			Return(map[string][]string{
				creatorA: {"beauty", "fashion"},
				creatorB: {"sport"},
			}, nil)
		// creatorA: 2 active + 1 deleted; creatorB: 1 active.
		rig.campaignCreatorRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{creatorA, creatorB}).
			Return([]*repository.CampaignCreatorRow{
				{ID: "cc-1", CampaignID: "camp-a1", CreatorID: creatorA, Status: domain.CampaignCreatorStatusInvited, CreatedAt: participationCreated},
				{ID: "cc-2", CampaignID: "camp-a2", CreatorID: creatorA, Status: domain.CampaignCreatorStatusPlanned, CreatedAt: participationCreated},
				{ID: "cc-3", CampaignID: "camp-del", CreatorID: creatorA, Status: domain.CampaignCreatorStatusSigned, CreatedAt: participationCreated},
				{ID: "cc-4", CampaignID: "camp-b1", CreatorID: creatorB, Status: domain.CampaignCreatorStatusInvited, CreatedAt: participationCreated},
			}, nil)
		rig.campaignRepo.EXPECT().ListByIDs(mock.Anything, mock.MatchedBy(func(ids []string) bool {
			want := map[string]struct{}{"camp-a1": {}, "camp-a2": {}, "camp-del": {}, "camp-b1": {}}
			if len(ids) != len(want) {
				return false
			}
			for _, id := range ids {
				if _, ok := want[id]; !ok {
					return false
				}
			}
			return true
		})).Return([]*repository.CampaignRow{
			{ID: "camp-a1", Name: "A1", IsDeleted: false},
			{ID: "camp-a2", Name: "A2", IsDeleted: false},
			{ID: "camp-del", Name: "Retired", IsDeleted: true},
			{ID: "camp-b1", Name: "B1", IsDeleted: false},
		}, nil)

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		got, err := svc.List(context.Background(), domain.CreatorListInput{Sort: domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 20})
		require.NoError(t, err)

		require.Equal(t, &domain.CreatorListPage{
			Items: []*domain.CreatorListItem{
				{
					ID:                   creatorA,
					LastName:             "Муратова",
					FirstName:            "Айдана",
					MiddleName:           pointer.ToString("Ивановна"),
					IIN:                  "950515312348",
					BirthDate:            birth,
					Phone:                "+77001234567",
					CityCode:             "almaty",
					Categories:           []string{"beauty", "fashion"},
					Socials:              []domain.CreatorListSocial{{Platform: "instagram", Handle: "aidana"}, {Platform: "tiktok", Handle: "aidana_tt"}},
					ActiveCampaignsCount: 2,
					TelegramUsername:     pointer.ToString("aidana_tg"),
					CreatedAt:            created,
					UpdatedAt:            updated,
				},
				{
					ID:                   creatorB,
					LastName:             "Иванов",
					FirstName:            "Алексей",
					IIN:                  "950515312349",
					BirthDate:            birth,
					Phone:                "+77001234568",
					CityCode:             "astana",
					Categories:           []string{"sport"},
					Socials:              []domain.CreatorListSocial{{Platform: "instagram", Handle: "ivanov"}},
					ActiveCampaignsCount: 1,
					CreatedAt:            created,
					UpdatedAt:            updated,
				},
			},
			Total:   2,
			Page:    1,
			PerPage: 20,
		}, got)
	})

	t.Run("creator with zero participations gets count 0", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		rig.factory.EXPECT().NewCreatorRepo(rig.pool).Return(rig.creatorRepo)
		rig.factory.EXPECT().NewCreatorSocialRepo(rig.pool).Return(rig.creatorSocialRepo)
		rig.factory.EXPECT().NewCreatorCategoryRepo(rig.pool).Return(rig.creatorCategoryRepo)
		rig.factory.EXPECT().NewCampaignCreatorRepo(rig.pool).Return(rig.campaignCreatorRepo)
		rig.creatorRepo.EXPECT().List(mock.Anything, mock.Anything).Return(listRows(), int64(2), nil)
		rig.creatorSocialRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{creatorA, creatorB}).
			Return(map[string][]*repository.CreatorSocialRow{}, nil)
		rig.creatorCategoryRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{creatorA, creatorB}).
			Return(map[string][]string{}, nil)
		rig.campaignCreatorRepo.EXPECT().ListByCreatorIDs(mock.Anything, []string{creatorA, creatorB}).
			Return(nil, nil)

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		got, err := svc.List(context.Background(), domain.CreatorListInput{Sort: domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 20})
		require.NoError(t, err)
		require.Len(t, got.Items, 2)
		require.Equal(t, 0, got.Items[0].ActiveCampaignsCount)
		require.Equal(t, 0, got.Items[1].ActiveCampaignsCount)
	})

	t.Run("forwards search verbatim (handler owns the trim)", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		rig.factory.EXPECT().NewCreatorRepo(rig.pool).Return(rig.creatorRepo)
		var captured repository.CreatorListParams
		rig.creatorRepo.EXPECT().List(mock.Anything, mock.Anything).
			Run(func(_ context.Context, p repository.CreatorListParams) {
				captured = p
			}).
			Return(nil, int64(0), nil)

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		_, err := svc.List(context.Background(), domain.CreatorListInput{
			Search: "aidana",
			Sort:   domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 20,
		})
		require.NoError(t, err)
		require.Equal(t, "aidana", captured.Search)
	})

	t.Run("forwards ids verbatim to repo", func(t *testing.T) {
		t.Parallel()
		rig := newCreatorReadRig(t)
		rig.factory.EXPECT().NewCreatorRepo(rig.pool).Return(rig.creatorRepo)
		var captured repository.CreatorListParams
		rig.creatorRepo.EXPECT().List(mock.Anything, mock.Anything).
			Run(func(_ context.Context, p repository.CreatorListParams) {
				captured = p
			}).
			Return(nil, int64(0), nil)

		svc := NewCreatorService(rig.pool, rig.factory, rig.logger)
		ids := []string{creatorA, creatorB}
		_, err := svc.List(context.Background(), domain.CreatorListInput{
			IDs:  ids,
			Sort: domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 20,
		})
		require.NoError(t, err)
		require.Equal(t, ids, captured.IDs)
	})
}
