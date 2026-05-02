package repository

import (
	"context"
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

	const sqlStmt = "SELECT application_id, created_at, handle, id, platform FROM creator_application_socials WHERE application_id = $1 ORDER BY platform ASC, handle ASC"

	t.Run("success maps rows in DB order", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationSocialRepository{db: mock}
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1").
			WillReturnRows(pgxmock.NewRows([]string{"application_id", "created_at", "handle", "id", "platform"}).
				AddRow("app-1", created, "aidana", "s-1", "instagram").
				AddRow("app-1", created, "aidana_tt", "s-2", "tiktok"))

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
			WillReturnRows(pgxmock.NewRows([]string{"application_id", "created_at", "handle", "id", "platform"}))

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

	const sqlStmt = "SELECT application_id, created_at, handle, id, platform FROM creator_application_socials WHERE application_id IN ($1,$2) ORDER BY application_id ASC, platform ASC, handle ASC"

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
			WillReturnRows(pgxmock.NewRows([]string{"application_id", "created_at", "handle", "id", "platform"}).
				AddRow("app-1", created, "aidana", "s-1", "instagram").
				AddRow("app-1", created, "aidana_tt", "s-2", "tiktok").
				AddRow("app-2", created, "anotheruser", "s-3", "instagram"))

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
