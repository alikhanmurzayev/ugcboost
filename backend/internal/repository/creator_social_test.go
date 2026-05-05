package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestCreatorSocialRepository_InsertMany(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO creator_socials (creator_id,handle,method,platform,verified,verified_at,verified_by_user_id) VALUES ($1,$2,$3,$4,$5,$6,$7),($8,$9,$10,$11,$12,$13,$14)"

	t.Run("empty input is a no-op", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorSocialRepository{db: mock}
		require.NoError(t, repo.InsertMany(context.Background(), nil))
		require.NoError(t, repo.InsertMany(context.Background(), []CreatorSocialRow{}))
	})

	t.Run("happy: writes rows in a single INSERT", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorSocialRepository{db: mock}
		verifiedAt := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)

		mock.ExpectExec(sqlStmt).
			WithArgs(
				"creator-1", "aidana", "auto", "instagram", true, verifiedAt, "admin-1",
				"creator-1", "aidana_tt", nil, "tiktok", false, nil, nil,
			).
			WillReturnResult(pgconn.NewCommandTag("INSERT 0 2"))

		rows := []CreatorSocialRow{
			{
				CreatorID:        "creator-1",
				Platform:         "instagram",
				Handle:           "aidana",
				Verified:         true,
				Method:           pointer.ToString("auto"),
				VerifiedByUserID: pointer.ToString("admin-1"),
				VerifiedAt:       pointer.To(verifiedAt),
			},
			{
				CreatorID: "creator-1",
				Platform:  "tiktok",
				Handle:    "aidana_tt",
				Verified:  false,
			},
		}
		require.NoError(t, repo.InsertMany(context.Background(), rows))
	})

	t.Run("propagates db error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorSocialRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs(
				"creator-1", "aidana", nil, "instagram", false, nil, nil,
				"creator-1", "aidana_tt", nil, "tiktok", false, nil, nil,
			).
			WillReturnError(errors.New("db down"))

		err := repo.InsertMany(context.Background(), []CreatorSocialRow{
			{CreatorID: "creator-1", Platform: "instagram", Handle: "aidana"},
			{CreatorID: "creator-1", Platform: "tiktok", Handle: "aidana_tt"},
		})
		require.ErrorContains(t, err, "db down")
	})
}

func TestCreatorSocialRepository_ListByCreatorID(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT created_at, creator_id, handle, id, method, platform, verified, verified_at, verified_by_user_id FROM creator_socials WHERE creator_id = $1 ORDER BY platform ASC, handle ASC"

	created := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	verifiedAt := time.Date(2026, 5, 5, 13, 0, 0, 0, time.UTC)

	t.Run("happy: maps rows ordered by platform/handle", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorSocialRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("creator-1").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "creator_id", "handle", "id", "method", "platform", "verified", "verified_at", "verified_by_user_id"}).
				AddRow(created, "creator-1", "aidana", "social-1", pointer.ToString("auto"), "instagram", true, pointer.To(verifiedAt), pointer.ToString("admin-1")).
				AddRow(created, "creator-1", "aidana_tt", "social-2", nil, "tiktok", false, nil, nil))

		got, err := repo.ListByCreatorID(context.Background(), "creator-1")
		require.NoError(t, err)
		require.Equal(t, []*CreatorSocialRow{
			{
				ID:               "social-1",
				CreatorID:        "creator-1",
				Platform:         "instagram",
				Handle:           "aidana",
				Verified:         true,
				Method:           pointer.ToString("auto"),
				VerifiedByUserID: pointer.ToString("admin-1"),
				VerifiedAt:       pointer.To(verifiedAt),
				CreatedAt:        created,
			},
			{
				ID:        "social-2",
				CreatorID: "creator-1",
				Platform:  "tiktok",
				Handle:    "aidana_tt",
				Verified:  false,
				CreatedAt: created,
			},
		}, got)
	})

	t.Run("empty result returns nil slice", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorSocialRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("creator-1").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "creator_id", "handle", "id", "method", "platform", "verified", "verified_at", "verified_by_user_id"}))

		got, err := repo.ListByCreatorID(context.Background(), "creator-1")
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("propagates db error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorSocialRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("creator-1").
			WillReturnError(errors.New("db down"))

		_, err := repo.ListByCreatorID(context.Background(), "creator-1")
		require.ErrorContains(t, err, "db down")
	})
}
