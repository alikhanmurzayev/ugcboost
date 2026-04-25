package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
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
