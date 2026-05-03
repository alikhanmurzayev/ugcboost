package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/AlekSi/pointer"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestCreatorApplicationStatusTransitionRepository_Insert(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO creator_application_status_transitions (actor_id,application_id,from_status,reason,to_status) VALUES ($1,$2,$3,$4,$5)"

	t.Run("system-driven transition (no actor) records null actor_id", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationStatusTransitionRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs(nil, "app-1", "verification", "instagram_auto", "moderation").
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		err := repo.Insert(context.Background(), CreatorApplicationStatusTransitionRow{
			ApplicationID: "app-1",
			FromStatus:    pointer.ToString("verification"),
			ToStatus:      "moderation",
			Reason:        pointer.ToString("instagram_auto"),
		})
		require.NoError(t, err)
	})

	t.Run("admin-driven transition stamps actor_id", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationStatusTransitionRepository{db: mock}
		actorID := "00000000-0000-0000-0000-000000000aaa"

		mock.ExpectExec(sqlStmt).
			WithArgs(actorID, "app-2", "moderation", nil, "rejected").
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		err := repo.Insert(context.Background(), CreatorApplicationStatusTransitionRow{
			ApplicationID: "app-2",
			FromStatus:    pointer.ToString("moderation"),
			ToStatus:      "rejected",
			ActorID:       &actorID,
		})
		require.NoError(t, err)
	})

	t.Run("propagates db errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationStatusTransitionRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs(nil, "app-1", "verification", "instagram_auto", "moderation").
			WillReturnError(errors.New("db down"))

		err := repo.Insert(context.Background(), CreatorApplicationStatusTransitionRow{
			ApplicationID: "app-1",
			FromStatus:    pointer.ToString("verification"),
			ToStatus:      "moderation",
			Reason:        pointer.ToString("instagram_auto"),
		})
		require.ErrorContains(t, err, "db down")
	})
}
