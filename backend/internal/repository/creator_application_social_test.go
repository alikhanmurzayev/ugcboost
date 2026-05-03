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
)

func TestCreatorApplicationSocialRepository_InsertMany(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO creator_application_socials (application_id,handle,platform) VALUES ($1,$2,$3),($4,$5,$6)"

	t.Run("empty input short-circuits", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationSocialRepository{db: mock}

		require.NoError(t, repo.InsertMany(context.Background(), nil))
	})

	t.Run("success batches two handles on different platforms", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationSocialRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("app-1", "aidana", "instagram", "app-1", "aidana_tt", "tiktok").
			WillReturnResult(pgconn.NewCommandTag("INSERT 0 2"))

		err := repo.InsertMany(context.Background(), []CreatorApplicationSocialRow{
			{ApplicationID: "app-1", Platform: "instagram", Handle: "aidana"},
			{ApplicationID: "app-1", Platform: "tiktok", Handle: "aidana_tt"},
		})
		require.NoError(t, err)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationSocialRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("app-1", "aidana", "instagram", "app-1", "aidana_tt", "tiktok").
			WillReturnError(errors.New("unique violation"))

		err := repo.InsertMany(context.Background(), []CreatorApplicationSocialRow{
			{ApplicationID: "app-1", Platform: "instagram", Handle: "aidana"},
			{ApplicationID: "app-1", Platform: "tiktok", Handle: "aidana_tt"},
		})
		require.ErrorContains(t, err, "unique violation")
	})
}

func TestCreatorApplicationSocialRepository_ListByApplicationID(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT application_id, created_at, handle, id, method, platform, verified, verified_at, verified_by_user_id FROM creator_application_socials WHERE application_id = $1 ORDER BY platform ASC, handle ASC"

	t.Run("success maps rows in DB order", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationSocialRepository{db: mock}
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1").
			WillReturnRows(pgxmock.NewRows([]string{"application_id", "created_at", "handle", "id", "method", "platform", "verified", "verified_at", "verified_by_user_id"}).
				AddRow("app-1", created, "aidana", "s-1", nil, "instagram", false, nil, nil).
				AddRow("app-1", created, "aidana_tt", "s-2", nil, "tiktok", false, nil, nil))

		got, err := repo.ListByApplicationID(context.Background(), "app-1")
		require.NoError(t, err)
		require.Equal(t, []*CreatorApplicationSocialRow{
			{ID: "s-1", ApplicationID: "app-1", Platform: "instagram", Handle: "aidana", CreatedAt: created},
			{ID: "s-2", ApplicationID: "app-1", Platform: "tiktok", Handle: "aidana_tt", CreatedAt: created},
		}, got)
	})

	t.Run("empty result", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationSocialRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-empty").
			WillReturnRows(pgxmock.NewRows([]string{"application_id", "created_at", "handle", "id", "method", "platform", "verified", "verified_at", "verified_by_user_id"}))

		got, err := repo.ListByApplicationID(context.Background(), "app-empty")
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationSocialRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1").
			WillReturnError(errors.New("db down"))

		_, err := repo.ListByApplicationID(context.Background(), "app-1")
		require.ErrorContains(t, err, "db down")
	})
}

func TestCreatorApplicationSocialRepository_ListByApplicationIDs(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT application_id, created_at, handle, id, method, platform, verified, verified_at, verified_by_user_id FROM creator_application_socials WHERE application_id IN ($1,$2) ORDER BY application_id ASC, platform ASC, handle ASC"

	t.Run("empty input short-circuits without query", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationSocialRepository{db: mock}

		got, err := repo.ListByApplicationIDs(context.Background(), nil)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("success groups handles by application id", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationSocialRepository{db: mock}
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1", "app-2").
			WillReturnRows(pgxmock.NewRows([]string{"application_id", "created_at", "handle", "id", "method", "platform", "verified", "verified_at", "verified_by_user_id"}).
				AddRow("app-1", created, "aidana", "s-1", nil, "instagram", false, nil, nil).
				AddRow("app-1", created, "aidana_tt", "s-2", nil, "tiktok", false, nil, nil).
				AddRow("app-2", created, "anotheruser", "s-3", nil, "instagram", false, nil, nil))

		got, err := repo.ListByApplicationIDs(context.Background(), []string{"app-1", "app-2"})
		require.NoError(t, err)
		require.Equal(t, map[string][]*CreatorApplicationSocialRow{
			"app-1": {
				{ID: "s-1", ApplicationID: "app-1", Platform: "instagram", Handle: "aidana", CreatedAt: created},
				{ID: "s-2", ApplicationID: "app-1", Platform: "tiktok", Handle: "aidana_tt", CreatedAt: created},
			},
			"app-2": {
				{ID: "s-3", ApplicationID: "app-2", Platform: "instagram", Handle: "anotheruser", CreatedAt: created},
			},
		}, got)
	})

	t.Run("propagates query error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationSocialRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1", "app-2").
			WillReturnError(errors.New("db down"))

		_, err := repo.ListByApplicationIDs(context.Background(), []string{"app-1", "app-2"})
		require.ErrorContains(t, err, "db down")
	})
}

func TestCreatorApplicationSocialRepository_UpdateVerification(t *testing.T) {
	t.Parallel()

	const sqlStmt = "UPDATE creator_application_socials SET handle = $1, verified = $2, method = $3, verified_by_user_id = $4, verified_at = $5 WHERE id = $6"

	verifiedAt := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)

	t.Run("auto-verification with self-fix overwrites handle", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationSocialRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("newhandle", true, "auto", (*string)(nil), verifiedAt, "social-1").
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		err := repo.UpdateVerification(context.Background(), UpdateSocialVerificationParams{
			ID:               "social-1",
			Handle:           "newhandle",
			Verified:         true,
			Method:           "auto",
			VerifiedByUserID: nil,
			VerifiedAt:       verifiedAt,
		})
		require.NoError(t, err)
	})

	t.Run("manual verification stamps actor id", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationSocialRepository{db: mock}
		actorID := "00000000-0000-0000-0000-000000000aaa"

		mock.ExpectExec(sqlStmt).
			WithArgs("aidana", true, "manual", &actorID, verifiedAt, "social-1").
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		err := repo.UpdateVerification(context.Background(), UpdateSocialVerificationParams{
			ID:               "social-1",
			Handle:           "aidana",
			Verified:         true,
			Method:           "manual",
			VerifiedByUserID: &actorID,
			VerifiedAt:       verifiedAt,
		})
		require.NoError(t, err)
	})

	t.Run("missing row returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationSocialRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("aidana", true, "auto", (*string)(nil), verifiedAt, "missing").
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))

		err := repo.UpdateVerification(context.Background(), UpdateSocialVerificationParams{
			ID:         "missing",
			Handle:     "aidana",
			Verified:   true,
			Method:     "auto",
			VerifiedAt: verifiedAt,
		})
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates db errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationSocialRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("aidana", true, "auto", (*string)(nil), verifiedAt, "social-1").
			WillReturnError(errors.New("db down"))

		err := repo.UpdateVerification(context.Background(), UpdateSocialVerificationParams{
			ID:         "social-1",
			Handle:     "aidana",
			Verified:   true,
			Method:     "auto",
			VerifiedAt: verifiedAt,
		})
		require.ErrorContains(t, err, "db down")
	})
}
