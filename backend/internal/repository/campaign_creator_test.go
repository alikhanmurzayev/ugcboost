package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

const (
	campaignCreatorAllCols   = "campaign_id, created_at, creator_id, decided_at, id, invited_at, invited_count, reminded_at, reminded_count, status, updated_at"
	campaignCreatorAddSQL    = "INSERT INTO campaign_creators (campaign_id,creator_id,status) VALUES ($1,$2,$3) RETURNING " + campaignCreatorAllCols
	campaignCreatorGetSQL    = "SELECT " + campaignCreatorAllCols + " FROM campaign_creators WHERE campaign_id = $1 AND creator_id = $2"
	campaignCreatorListSQL   = "SELECT " + campaignCreatorAllCols + " FROM campaign_creators WHERE campaign_id = $1 ORDER BY created_at ASC, id ASC"
	campaignCreatorDeleteSQL = "DELETE FROM campaign_creators WHERE id = $1"
)

var campaignCreatorRowCols = []string{
	"campaign_id", "created_at", "creator_id", "decided_at", "id",
	"invited_at", "invited_count", "reminded_at", "reminded_count",
	"status", "updated_at",
}

func TestCampaignCreatorRepository_Add(t *testing.T) {
	t.Parallel()

	t.Run("success maps row to struct", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}
		createdAt := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		mock.ExpectQuery(campaignCreatorAddSQL).
			WithArgs("camp-1", "cr-1", domain.CampaignCreatorStatusPlanned).
			WillReturnRows(pgxmock.NewRows(campaignCreatorRowCols).
				AddRow("camp-1", createdAt, "cr-1", (*time.Time)(nil), "cc-1",
					(*time.Time)(nil), 0, (*time.Time)(nil), 0,
					domain.CampaignCreatorStatusPlanned, createdAt))

		got, err := repo.Add(context.Background(), "camp-1", "cr-1", domain.CampaignCreatorStatusPlanned)
		require.NoError(t, err)
		require.Equal(t, &CampaignCreatorRow{
			ID:         "cc-1",
			CampaignID: "camp-1",
			CreatorID:  "cr-1",
			Status:     domain.CampaignCreatorStatusPlanned,
			CreatedAt:  createdAt,
			UpdatedAt:  createdAt,
		}, got)
	})

	t.Run("23505 unique constraint translates to ErrCreatorAlreadyInCampaign", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(campaignCreatorAddSQL).
			WithArgs("camp-1", "cr-1", domain.CampaignCreatorStatusPlanned).
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: CampaignCreatorsCampaignCreatorUnique})

		_, err := repo.Add(context.Background(), "camp-1", "cr-1", domain.CampaignCreatorStatusPlanned)
		require.ErrorIs(t, err, domain.ErrCreatorAlreadyInCampaign)
	})

	t.Run("23503 creator FK translates to ErrCampaignCreatorCreatorNotFound", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(campaignCreatorAddSQL).
			WithArgs("camp-1", "missing-creator", domain.CampaignCreatorStatusPlanned).
			WillReturnError(&pgconn.PgError{Code: "23503", ConstraintName: CampaignCreatorsCreatorIDFK})

		_, err := repo.Add(context.Background(), "camp-1", "missing-creator", domain.CampaignCreatorStatusPlanned)
		require.ErrorIs(t, err, domain.ErrCampaignCreatorCreatorNotFound)
	})

	t.Run("23503 campaign FK translates to ErrCampaignNotFound", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(campaignCreatorAddSQL).
			WithArgs("missing-campaign", "cr-1", domain.CampaignCreatorStatusPlanned).
			WillReturnError(&pgconn.PgError{Code: "23503", ConstraintName: CampaignCreatorsCampaignIDFK})

		_, err := repo.Add(context.Background(), "missing-campaign", "cr-1", domain.CampaignCreatorStatusPlanned)
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("unrelated 23505 propagates raw", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}
		pgErr := &pgconn.PgError{Code: "23505", ConstraintName: "campaign_creators_other_unique"}

		mock.ExpectQuery(campaignCreatorAddSQL).
			WithArgs("camp-1", "cr-1", domain.CampaignCreatorStatusPlanned).
			WillReturnError(pgErr)

		_, err := repo.Add(context.Background(), "camp-1", "cr-1", domain.CampaignCreatorStatusPlanned)
		require.NotErrorIs(t, err, domain.ErrCreatorAlreadyInCampaign)
		require.ErrorIs(t, err, pgErr)
	})

	t.Run("unrelated 23503 propagates raw", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}
		pgErr := &pgconn.PgError{Code: "23503", ConstraintName: "campaign_creators_other_fk"}

		mock.ExpectQuery(campaignCreatorAddSQL).
			WithArgs("camp-1", "cr-1", domain.CampaignCreatorStatusPlanned).
			WillReturnError(pgErr)

		_, err := repo.Add(context.Background(), "camp-1", "cr-1", domain.CampaignCreatorStatusPlanned)
		require.ErrorIs(t, err, pgErr)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(campaignCreatorAddSQL).
			WithArgs("camp-1", "cr-1", domain.CampaignCreatorStatusPlanned).
			WillReturnError(errors.New("db unavailable"))

		_, err := repo.Add(context.Background(), "camp-1", "cr-1", domain.CampaignCreatorStatusPlanned)
		require.ErrorContains(t, err, "db unavailable")
	})
}

func TestCampaignCreatorRepository_GetByCampaignAndCreator(t *testing.T) {
	t.Parallel()

	t.Run("success maps row to struct", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}
		createdAt := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
		invitedAt := createdAt.Add(time.Hour)

		mock.ExpectQuery(campaignCreatorGetSQL).
			WithArgs("camp-1", "cr-1").
			WillReturnRows(pgxmock.NewRows(campaignCreatorRowCols).
				AddRow("camp-1", createdAt, "cr-1", (*time.Time)(nil), "cc-1",
					&invitedAt, 1, (*time.Time)(nil), 0,
					domain.CampaignCreatorStatusInvited, createdAt))

		got, err := repo.GetByCampaignAndCreator(context.Background(), "camp-1", "cr-1")
		require.NoError(t, err)
		require.Equal(t, &CampaignCreatorRow{
			ID:           "cc-1",
			CampaignID:   "camp-1",
			CreatorID:    "cr-1",
			Status:       domain.CampaignCreatorStatusInvited,
			InvitedAt:    &invitedAt,
			InvitedCount: 1,
			CreatedAt:    createdAt,
			UpdatedAt:    createdAt,
		}, got)
	})

	t.Run("not found propagates sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(campaignCreatorGetSQL).
			WithArgs("camp-1", "missing").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetByCampaignAndCreator(context.Background(), "camp-1", "missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(campaignCreatorGetSQL).
			WithArgs("camp-1", "cr-1").
			WillReturnError(errors.New("db unavailable"))

		_, err := repo.GetByCampaignAndCreator(context.Background(), "camp-1", "cr-1")
		require.ErrorContains(t, err, "db unavailable")
	})
}

func TestCampaignCreatorRepository_ListByCampaign(t *testing.T) {
	t.Parallel()

	t.Run("success returns multiple rows in created_at then id order", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}
		t1 := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
		t2 := t1.Add(time.Hour)

		mock.ExpectQuery(campaignCreatorListSQL).
			WithArgs("camp-1").
			WillReturnRows(pgxmock.NewRows(campaignCreatorRowCols).
				AddRow("camp-1", t1, "cr-1", (*time.Time)(nil), "cc-1",
					(*time.Time)(nil), 0, (*time.Time)(nil), 0,
					domain.CampaignCreatorStatusPlanned, t1).
				AddRow("camp-1", t2, "cr-2", (*time.Time)(nil), "cc-2",
					(*time.Time)(nil), 0, (*time.Time)(nil), 0,
					domain.CampaignCreatorStatusPlanned, t2))

		got, err := repo.ListByCampaign(context.Background(), "camp-1")
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, "cc-1", got[0].ID)
		require.Equal(t, "cc-2", got[1].ID)
		require.Equal(t, t1, got[0].CreatedAt)
		require.Equal(t, t2, got[1].CreatedAt)
	})

	t.Run("empty result returns empty slice", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(campaignCreatorListSQL).
			WithArgs("camp-1").
			WillReturnRows(pgxmock.NewRows(campaignCreatorRowCols))

		got, err := repo.ListByCampaign(context.Background(), "camp-1")
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("propagates errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(campaignCreatorListSQL).
			WithArgs("camp-1").
			WillReturnError(errors.New("db unavailable"))

		_, err := repo.ListByCampaign(context.Background(), "camp-1")
		require.ErrorContains(t, err, "db unavailable")
	})
}

func TestCampaignCreatorRepository_DeleteByID(t *testing.T) {
	t.Parallel()

	t.Run("success returns nil", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectExec(campaignCreatorDeleteSQL).
			WithArgs("cc-1").
			WillReturnResult(pgxmock.NewResult("DELETE", 1))

		require.NoError(t, repo.DeleteByID(context.Background(), "cc-1"))
	})

	t.Run("zero rows returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectExec(campaignCreatorDeleteSQL).
			WithArgs("missing").
			WillReturnResult(pgxmock.NewResult("DELETE", 0))

		err := repo.DeleteByID(context.Background(), "missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectExec(campaignCreatorDeleteSQL).
			WithArgs("cc-1").
			WillReturnError(errors.New("db unavailable"))

		err := repo.DeleteByID(context.Background(), "cc-1")
		require.ErrorContains(t, err, "db unavailable")
	})
}
