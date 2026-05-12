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
	campaignCreatorAllCols                      = "campaign_id, contract_id, created_at, creator_id, decided_at, id, invited_at, invited_count, reminded_at, reminded_count, status, ticket_sent_at, updated_at"
	campaignCreatorAddSQL                       = "INSERT INTO campaign_creators (campaign_id,creator_id,status) VALUES ($1,$2,$3) RETURNING " + campaignCreatorAllCols
	campaignCreatorGetSQL                       = "SELECT " + campaignCreatorAllCols + " FROM campaign_creators WHERE campaign_id = $1 AND creator_id = $2"
	campaignCreatorListSQL                      = "SELECT " + campaignCreatorAllCols + " FROM campaign_creators WHERE campaign_id = $1 ORDER BY created_at ASC, id ASC"
	campaignCreatorDeleteSQL                    = "DELETE FROM campaign_creators WHERE id = $1"
	campaignCreatorListByCampaignAndCreatorsSQL = "SELECT " + campaignCreatorAllCols + " FROM campaign_creators WHERE campaign_id = $1 AND creator_id IN ($2,$3)"
	campaignCreatorListByCreatorIDsSQL          = "SELECT " + campaignCreatorAllCols + " FROM campaign_creators WHERE creator_id IN ($1,$2) ORDER BY created_at DESC, id DESC"
	campaignCreatorForceCleanupSQL              = "DELETE FROM campaign_creators WHERE campaign_id = $1 AND creator_id = $2"
	campaignCreatorApplyInviteSQL               = "UPDATE campaign_creators SET status = $1, invited_count = invited_count + 1, invited_at = now(), reminded_count = CASE WHEN status = $2 THEN $3 ELSE reminded_count END, reminded_at = CASE WHEN status = $4 THEN $5 ELSE reminded_at END, decided_at = CASE WHEN status = $6 THEN $7 ELSE decided_at END, updated_at = now() WHERE id = $8 RETURNING " + campaignCreatorAllCols
	campaignCreatorApplyRemindSQL               = "UPDATE campaign_creators SET reminded_count = reminded_count + 1, reminded_at = now(), updated_at = now() WHERE id = $1 RETURNING " + campaignCreatorAllCols
	campaignCreatorExistsInvitedSQL             = "SELECT EXISTS (SELECT 1 FROM campaign_creators WHERE campaign_id = $1 AND invited_count > $2)"
)

var campaignCreatorRowCols = []string{
	"campaign_id", "contract_id", "created_at", "creator_id", "decided_at", "id",
	"invited_at", "invited_count", "reminded_at", "reminded_count",
	"status", "ticket_sent_at", "updated_at",
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
				AddRow("camp-1", (*string)(nil), createdAt, "cr-1", (*time.Time)(nil), "cc-1",
					(*time.Time)(nil), 0, (*time.Time)(nil), 0,
					domain.CampaignCreatorStatusPlanned, (*time.Time)(nil), createdAt))

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
				AddRow("camp-1", (*string)(nil), createdAt, "cr-1", (*time.Time)(nil), "cc-1",
					&invitedAt, 1, (*time.Time)(nil), 0,
					domain.CampaignCreatorStatusInvited, (*time.Time)(nil), createdAt))

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
				AddRow("camp-1", (*string)(nil), t1, "cr-1", (*time.Time)(nil), "cc-1",
					(*time.Time)(nil), 0, (*time.Time)(nil), 0,
					domain.CampaignCreatorStatusPlanned, (*time.Time)(nil), t1).
				AddRow("camp-1", (*string)(nil), t2, "cr-2", (*time.Time)(nil), "cc-2",
					(*time.Time)(nil), 0, (*time.Time)(nil), 0,
					domain.CampaignCreatorStatusPlanned, (*time.Time)(nil), t2))

		got, err := repo.ListByCampaign(context.Background(), "camp-1")
		require.NoError(t, err)
		// Full struct comparison catches column-mapping regressions
		// (e.g. invited_count <-> reminded_count swap) that per-field
		// asserts would silently miss.
		require.Equal(t, []*CampaignCreatorRow{
			{
				ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
				Status:    domain.CampaignCreatorStatusPlanned,
				CreatedAt: t1, UpdatedAt: t1,
			},
			{
				ID: "cc-2", CampaignID: "camp-1", CreatorID: "cr-2",
				Status:    domain.CampaignCreatorStatusPlanned,
				CreatedAt: t2, UpdatedAt: t2,
			},
		}, got)
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

func TestCampaignCreatorRepository_ListByCampaignAndCreators(t *testing.T) {
	t.Parallel()

	t.Run("empty creator list short-circuits without DB call", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}
		got, err := repo.ListByCampaignAndCreators(context.Background(), "camp-1", nil)
		require.NoError(t, err)
		require.Empty(t, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns matching rows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}
		t1 := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		mock.ExpectQuery(campaignCreatorListByCampaignAndCreatorsSQL).
			WithArgs("camp-1", "cr-1", "cr-2").
			WillReturnRows(pgxmock.NewRows(campaignCreatorRowCols).
				AddRow("camp-1", (*string)(nil), t1, "cr-1", (*time.Time)(nil), "cc-1",
					(*time.Time)(nil), 0, (*time.Time)(nil), 0,
					domain.CampaignCreatorStatusPlanned, (*time.Time)(nil), t1).
				AddRow("camp-1", (*string)(nil), t1, "cr-2", (*time.Time)(nil), "cc-2",
					(*time.Time)(nil), 0, (*time.Time)(nil), 0,
					domain.CampaignCreatorStatusInvited, (*time.Time)(nil), t1))

		got, err := repo.ListByCampaignAndCreators(context.Background(), "camp-1", []string{"cr-1", "cr-2"})
		require.NoError(t, err)
		require.Equal(t, []*CampaignCreatorRow{
			{
				ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
				Status:    domain.CampaignCreatorStatusPlanned,
				CreatedAt: t1, UpdatedAt: t1,
			},
			{
				ID: "cc-2", CampaignID: "camp-1", CreatorID: "cr-2",
				Status:    domain.CampaignCreatorStatusInvited,
				CreatedAt: t1, UpdatedAt: t1,
			},
		}, got)
	})

	t.Run("propagates errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(campaignCreatorListByCampaignAndCreatorsSQL).
			WithArgs("camp-1", "cr-1", "cr-2").
			WillReturnError(errors.New("db unavailable"))

		_, err := repo.ListByCampaignAndCreators(context.Background(), "camp-1", []string{"cr-1", "cr-2"})
		require.ErrorContains(t, err, "db unavailable")
	})
}

func TestCampaignCreatorRepository_DeleteByCampaignAndCreatorForTests(t *testing.T) {
	t.Parallel()

	const auditCleanupSQL = "DELETE FROM audit_logs WHERE entity_type = $1 AND entity_id IN (SELECT id FROM campaign_creators WHERE campaign_id = $2 AND creator_id = $3)"

	t.Run("success drops audit_logs then the row", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectExec(auditCleanupSQL).
			WithArgs("campaign_creator", "camp-1", "cr-1").
			WillReturnResult(pgconn.NewCommandTag("DELETE 3"))
		mock.ExpectExec(campaignCreatorForceCleanupSQL).
			WithArgs("camp-1", "cr-1").
			WillReturnResult(pgconn.NewCommandTag("DELETE 1"))

		require.NoError(t, repo.DeleteByCampaignAndCreatorForTests(context.Background(), "camp-1", "cr-1"))
	})

	t.Run("zero rows returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectExec(auditCleanupSQL).
			WithArgs("campaign_creator", "camp-1", "cr-1").
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))
		mock.ExpectExec(campaignCreatorForceCleanupSQL).
			WithArgs("camp-1", "cr-1").
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))

		require.ErrorIs(t, repo.DeleteByCampaignAndCreatorForTests(context.Background(), "camp-1", "cr-1"), sql.ErrNoRows)
	})

	t.Run("audit cleanup failure short-circuits before the row delete", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectExec(auditCleanupSQL).
			WithArgs("campaign_creator", "camp-1", "cr-1").
			WillReturnError(errors.New("audit boom"))

		err := repo.DeleteByCampaignAndCreatorForTests(context.Background(), "camp-1", "cr-1")
		require.ErrorContains(t, err, "audit boom")
	})

	t.Run("propagates row delete errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectExec(auditCleanupSQL).
			WithArgs("campaign_creator", "camp-1", "cr-1").
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))
		mock.ExpectExec(campaignCreatorForceCleanupSQL).
			WithArgs("camp-1", "cr-1").
			WillReturnError(errors.New("db unavailable"))

		err := repo.DeleteByCampaignAndCreatorForTests(context.Background(), "camp-1", "cr-1")
		require.ErrorContains(t, err, "db unavailable")
	})
}

func TestCampaignCreatorRepository_ListByCreatorIDs(t *testing.T) {
	t.Parallel()

	t.Run("empty creator list short-circuits without DB call", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}
		got, err := repo.ListByCreatorIDs(context.Background(), nil)
		require.NoError(t, err)
		require.Nil(t, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns rows ordered by created_at DESC, id DESC", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}
		t1 := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
		t2 := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)

		mock.ExpectQuery(campaignCreatorListByCreatorIDsSQL).
			WithArgs("cr-1", "cr-2").
			WillReturnRows(pgxmock.NewRows(campaignCreatorRowCols).
				AddRow("camp-1", (*string)(nil), t1, "cr-1", (*time.Time)(nil), "cc-1",
					(*time.Time)(nil), 0, (*time.Time)(nil), 0,
					domain.CampaignCreatorStatusInvited, (*time.Time)(nil), t1).
				AddRow("camp-2", (*string)(nil), t2, "cr-2", (*time.Time)(nil), "cc-2",
					(*time.Time)(nil), 0, (*time.Time)(nil), 0,
					domain.CampaignCreatorStatusPlanned, (*time.Time)(nil), t2))

		got, err := repo.ListByCreatorIDs(context.Background(), []string{"cr-1", "cr-2"})
		require.NoError(t, err)
		require.Equal(t, []*CampaignCreatorRow{
			{
				ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
				Status:    domain.CampaignCreatorStatusInvited,
				CreatedAt: t1, UpdatedAt: t1,
			},
			{
				ID: "cc-2", CampaignID: "camp-2", CreatorID: "cr-2",
				Status:    domain.CampaignCreatorStatusPlanned,
				CreatedAt: t2, UpdatedAt: t2,
			},
		}, got)
	})

	t.Run("propagates errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(campaignCreatorListByCreatorIDsSQL).
			WithArgs("cr-1", "cr-2").
			WillReturnError(errors.New("db unavailable"))

		_, err := repo.ListByCreatorIDs(context.Background(), []string{"cr-1", "cr-2"})
		require.ErrorContains(t, err, "db unavailable")
	})
}

func TestCampaignCreatorRepository_ApplyInvite(t *testing.T) {
	t.Parallel()

	t.Run("from planned: increments invited_count, sets invited_at, status=invited (CASE keeps reminded fields)", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}
		now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)

		mock.ExpectQuery(campaignCreatorApplyInviteSQL).
			WithArgs(
				domain.CampaignCreatorStatusInvited,
				domain.CampaignCreatorStatusDeclined, 0,
				domain.CampaignCreatorStatusDeclined, nil,
				domain.CampaignCreatorStatusDeclined, nil,
				"cc-1",
			).
			WillReturnRows(pgxmock.NewRows(campaignCreatorRowCols).
				AddRow("camp-1", (*string)(nil), now, "cr-1", (*time.Time)(nil), "cc-1",
					&now, 1, (*time.Time)(nil), 0,
					domain.CampaignCreatorStatusInvited, (*time.Time)(nil), now))

		got, err := repo.ApplyInvite(context.Background(), "cc-1")
		require.NoError(t, err)
		require.Equal(t, &CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status:       domain.CampaignCreatorStatusInvited,
			InvitedAt:    &now,
			InvitedCount: 1,
			CreatedAt:    now, UpdatedAt: now,
		}, got)
	})

	t.Run("propagates errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(campaignCreatorApplyInviteSQL).
			WithArgs(
				domain.CampaignCreatorStatusInvited,
				domain.CampaignCreatorStatusDeclined, 0,
				domain.CampaignCreatorStatusDeclined, nil,
				domain.CampaignCreatorStatusDeclined, nil,
				"cc-1",
			).
			WillReturnError(errors.New("db down"))

		_, err := repo.ApplyInvite(context.Background(), "cc-1")
		require.ErrorContains(t, err, "db down")
	})
}

func TestCampaignCreatorRepository_ApplyRemind(t *testing.T) {
	t.Parallel()

	t.Run("increments reminded_count and sets reminded_at, leaves status alone", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}
		t1 := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)
		t2 := t1.Add(time.Hour)

		mock.ExpectQuery(campaignCreatorApplyRemindSQL).
			WithArgs("cc-1").
			WillReturnRows(pgxmock.NewRows(campaignCreatorRowCols).
				AddRow("camp-1", (*string)(nil), t1, "cr-1", (*time.Time)(nil), "cc-1",
					&t1, 1, &t2, 1,
					domain.CampaignCreatorStatusInvited, (*time.Time)(nil), t2))

		got, err := repo.ApplyRemind(context.Background(), "cc-1")
		require.NoError(t, err)
		require.Equal(t, &CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status:        domain.CampaignCreatorStatusInvited,
			InvitedAt:     &t1,
			InvitedCount:  1,
			RemindedAt:    &t2,
			RemindedCount: 1,
			CreatedAt:     t1, UpdatedAt: t2,
		}, got)
	})

	t.Run("propagates errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(campaignCreatorApplyRemindSQL).
			WithArgs("cc-1").
			WillReturnError(errors.New("db down"))

		_, err := repo.ApplyRemind(context.Background(), "cc-1")
		require.ErrorContains(t, err, "db down")
	})
}

func TestCampaignCreatorRepository_ExistsInvitedInCampaign(t *testing.T) {
	t.Parallel()

	t.Run("returns true when EXISTS matches", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(campaignCreatorExistsInvitedSQL).
			WithArgs("camp-1", 0).
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))

		got, err := repo.ExistsInvitedInCampaign(context.Background(), "camp-1")
		require.NoError(t, err)
		require.True(t, got)
	})

	t.Run("returns false when nothing invited", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(campaignCreatorExistsInvitedSQL).
			WithArgs("camp-1", 0).
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))

		got, err := repo.ExistsInvitedInCampaign(context.Background(), "camp-1")
		require.NoError(t, err)
		require.False(t, got)
	})

	t.Run("propagates errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(campaignCreatorExistsInvitedSQL).
			WithArgs("camp-1", 0).
			WillReturnError(errors.New("db down"))

		_, err := repo.ExistsInvitedInCampaign(context.Background(), "camp-1")
		require.ErrorContains(t, err, "db down")
	})
}

func TestCampaignCreatorRepository_GetByIDForUpdate(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT " + campaignCreatorAllCols + " FROM campaign_creators WHERE id = $1 FOR UPDATE"

	t.Run("success returns row", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}
		createdAt := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
		invitedAt := time.Date(2026, 5, 7, 13, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("cc-1").
			WillReturnRows(pgxmock.NewRows(campaignCreatorRowCols).
				AddRow("camp-1", (*string)(nil), createdAt, "cr-1", (*time.Time)(nil), "cc-1",
					&invitedAt, 1, (*time.Time)(nil), 0,
					domain.CampaignCreatorStatusInvited, (*time.Time)(nil), createdAt))

		got, err := repo.GetByIDForUpdate(context.Background(), "cc-1")
		require.NoError(t, err)
		require.Equal(t, &CampaignCreatorRow{
			ID:            "cc-1",
			CampaignID:    "camp-1",
			CreatorID:     "cr-1",
			Status:        domain.CampaignCreatorStatusInvited,
			InvitedAt:     &invitedAt,
			InvitedCount:  1,
			RemindedAt:    nil,
			RemindedCount: 0,
			DecidedAt:     nil,
			CreatedAt:     createdAt,
			UpdatedAt:     createdAt,
		}, got)
	})

	t.Run("not found propagates sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("missing").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetByIDForUpdate(context.Background(), "missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("cc-1").
			WillReturnError(errors.New("db down"))

		_, err := repo.GetByIDForUpdate(context.Background(), "cc-1")
		require.ErrorContains(t, err, "db down")
	})
}

func TestCampaignCreatorRepository_ApplyDecision(t *testing.T) {
	t.Parallel()

	const sqlStmt = "UPDATE campaign_creators SET status = $1, decided_at = now(), updated_at = now() WHERE id = $2 RETURNING " + campaignCreatorAllCols

	t.Run("success flips status to agreed", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}
		createdAt := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
		invitedAt := time.Date(2026, 5, 7, 12, 30, 0, 0, time.UTC)
		decidedAt := time.Date(2026, 5, 7, 13, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs(domain.CampaignCreatorStatusAgreed, "cc-1").
			WillReturnRows(pgxmock.NewRows(campaignCreatorRowCols).
				AddRow("camp-1", (*string)(nil), createdAt, "cr-1", &decidedAt, "cc-1",
					&invitedAt, 1, (*time.Time)(nil), 0,
					domain.CampaignCreatorStatusAgreed, (*time.Time)(nil), decidedAt))

		got, err := repo.ApplyDecision(context.Background(), "cc-1", domain.CampaignCreatorStatusAgreed)
		require.NoError(t, err)
		require.Equal(t, &CampaignCreatorRow{
			ID:            "cc-1",
			CampaignID:    "camp-1",
			CreatorID:     "cr-1",
			Status:        domain.CampaignCreatorStatusAgreed,
			InvitedAt:     &invitedAt,
			InvitedCount:  1,
			RemindedAt:    nil,
			RemindedCount: 0,
			DecidedAt:     &decidedAt,
			CreatedAt:     createdAt,
			UpdatedAt:     decidedAt,
		}, got)
	})

	t.Run("propagates errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs(domain.CampaignCreatorStatusAgreed, "cc-1").
			WillReturnError(errors.New("db down"))

		_, err := repo.ApplyDecision(context.Background(), "cc-1", domain.CampaignCreatorStatusAgreed)
		require.ErrorContains(t, err, "db down")
	})
}

func TestCampaignCreatorRepository_UpdateContractIDAndStatus(t *testing.T) {
	t.Parallel()

	const sqlStmt = "UPDATE campaign_creators SET contract_id = $1, status = $2, updated_at = now() WHERE id = $3"

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("ct-1", domain.CampaignCreatorStatusSigning, "cc-1").
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		require.NoError(t, repo.UpdateContractIDAndStatus(
			context.Background(), "cc-1", "ct-1", domain.CampaignCreatorStatusSigning))
	})

	t.Run("missing row returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("ct-1", domain.CampaignCreatorStatusSigning, "cc-missing").
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))

		err := repo.UpdateContractIDAndStatus(
			context.Background(), "cc-missing", "ct-1", domain.CampaignCreatorStatusSigning)
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("ct-1", domain.CampaignCreatorStatusSigning, "cc-1").
			WillReturnError(errors.New("db down"))

		err := repo.UpdateContractIDAndStatus(
			context.Background(), "cc-1", "ct-1", domain.CampaignCreatorStatusSigning)
		require.ErrorContains(t, err, "db down")
	})
}

func TestCampaignCreatorRepository_GetByContractID(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT " + campaignCreatorAllCols + " FROM campaign_creators WHERE contract_id = $1"

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}
		now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
		ctID := "ct-1"

		mock.ExpectQuery(sqlStmt).
			WithArgs("ct-1").
			WillReturnRows(pgxmock.NewRows(campaignCreatorRowCols).
				AddRow("camp-1", &ctID, now, "cr-1", &now, "cc-1",
					&now, 1, (*time.Time)(nil), 0,
					domain.CampaignCreatorStatusSigning, (*time.Time)(nil), now))

		got, err := repo.GetByContractID(context.Background(), "ct-1")
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, "cc-1", got.ID)
		require.NotNil(t, got.ContractID)
		require.Equal(t, "ct-1", *got.ContractID)
	})

	t.Run("not found propagates sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("missing").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetByContractID(context.Background(), "missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})
}

func TestCampaignCreatorRepository_GetWithCampaignAndCreatorByContractID(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT cc.id, cc.status, c.is_deleted, c.tma_url, cr.telegram_user_id FROM campaign_creators cc JOIN campaigns c ON c.id = cc.campaign_id JOIN creators cr ON cr.id = cc.creator_id WHERE cc.contract_id = $1"

	t.Run("scans projected columns", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("ct-1").
			WillReturnRows(pgxmock.NewRows([]string{"id", "status", "is_deleted", "tma_url", "telegram_user_id"}).
				AddRow("cc-1", domain.CampaignCreatorStatusSigning, false, "https://tma.example/tz/abc", int64(123456789)))

		got, err := repo.GetWithCampaignAndCreatorByContractID(context.Background(), "ct-1")
		require.NoError(t, err)
		require.Equal(t, &CampaignCreatorWebhookView{
			CampaignCreatorID:     "cc-1",
			CampaignCreatorStatus: domain.CampaignCreatorStatusSigning,
			CampaignIsDeleted:     false,
			CampaignTmaURL:        "https://tma.example/tz/abc",
			CreatorTelegramUserID: 123456789,
		}, got)
	})

	t.Run("soft-deleted campaign returns is_deleted=true", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("ct-2").
			WillReturnRows(pgxmock.NewRows([]string{"id", "status", "is_deleted", "tma_url", "telegram_user_id"}).
				AddRow("cc-2", domain.CampaignCreatorStatusSigning, true, "https://tma.example/tz/xyz", int64(42)))

		got, err := repo.GetWithCampaignAndCreatorByContractID(context.Background(), "ct-2")
		require.NoError(t, err)
		require.True(t, got.CampaignIsDeleted)
		require.Equal(t, "https://tma.example/tz/xyz", got.CampaignTmaURL)
	})

	t.Run("not found returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("missing").
			WillReturnRows(pgxmock.NewRows([]string{"id", "status", "is_deleted", "tma_url", "telegram_user_id"}))

		_, err := repo.GetWithCampaignAndCreatorByContractID(context.Background(), "missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates db errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("ct-1").
			WillReturnError(errors.New("db down"))

		_, err := repo.GetWithCampaignAndCreatorByContractID(context.Background(), "ct-1")
		require.ErrorContains(t, err, "db down")
	})
}

func TestCampaignCreatorRepository_UpdateStatus(t *testing.T) {
	t.Parallel()

	const sqlStmt = "UPDATE campaign_creators SET status = $1, updated_at = now() WHERE id = $2"

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs(domain.CampaignCreatorStatusSigned, "cc-1").
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		require.NoError(t, repo.UpdateStatus(context.Background(), "cc-1", domain.CampaignCreatorStatusSigned))
	})

	t.Run("missing row returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs(domain.CampaignCreatorStatusSigningDeclined, "cc-missing").
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))

		err := repo.UpdateStatus(context.Background(), "cc-missing", domain.CampaignCreatorStatusSigningDeclined)
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates db errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs(domain.CampaignCreatorStatusSigned, "cc-1").
			WillReturnError(errors.New("db down"))

		err := repo.UpdateStatus(context.Background(), "cc-1", domain.CampaignCreatorStatusSigned)
		require.ErrorContains(t, err, "db down")
	})
}

func TestCampaignCreatorRepository_UpdateTicketSentAt(t *testing.T) {
	t.Parallel()

	const sqlStmt = "UPDATE campaign_creators SET ticket_sent_at = CASE WHEN $1 THEN now() ELSE NULL END, updated_at = now() WHERE id = $2 RETURNING " + campaignCreatorAllCols

	t.Run("set stamps now() and returns row", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}
		now := time.Date(2026, 5, 11, 18, 0, 0, 0, time.UTC)
		invitedAt := now.Add(-2 * time.Hour)
		decidedAt := now.Add(-1 * time.Hour)
		ctID := "ct-1"

		mock.ExpectQuery(sqlStmt).
			WithArgs(true, "cc-1").
			WillReturnRows(pgxmock.NewRows(campaignCreatorRowCols).
				AddRow("camp-1", &ctID, now, "cr-1", &decidedAt, "cc-1",
					&invitedAt, 1, (*time.Time)(nil), 0,
					domain.CampaignCreatorStatusSigned, &now, now))

		got, err := repo.UpdateTicketSentAt(context.Background(), "cc-1", true)
		require.NoError(t, err)
		require.NotNil(t, got.TicketSentAt)
		require.Equal(t, now, *got.TicketSentAt)
		require.Equal(t, domain.CampaignCreatorStatusSigned, got.Status)
	})

	t.Run("unset clears timestamp to NULL", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}
		now := time.Date(2026, 5, 11, 18, 0, 0, 0, time.UTC)
		invitedAt := now.Add(-2 * time.Hour)
		decidedAt := now.Add(-1 * time.Hour)
		ctID := "ct-1"

		mock.ExpectQuery(sqlStmt).
			WithArgs(false, "cc-1").
			WillReturnRows(pgxmock.NewRows(campaignCreatorRowCols).
				AddRow("camp-1", &ctID, now, "cr-1", &decidedAt, "cc-1",
					&invitedAt, 1, (*time.Time)(nil), 0,
					domain.CampaignCreatorStatusSigned, (*time.Time)(nil), now))

		got, err := repo.UpdateTicketSentAt(context.Background(), "cc-1", false)
		require.NoError(t, err)
		require.Nil(t, got.TicketSentAt)
	})

	t.Run("missing row returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs(true, "cc-missing").
			WillReturnRows(pgxmock.NewRows(campaignCreatorRowCols))

		_, err := repo.UpdateTicketSentAt(context.Background(), "cc-missing", true)
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates db errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignCreatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs(true, "cc-1").
			WillReturnError(errors.New("db down"))

		_, err := repo.UpdateTicketSentAt(context.Background(), "cc-1", true)
		require.ErrorContains(t, err, "db down")
	})
}
