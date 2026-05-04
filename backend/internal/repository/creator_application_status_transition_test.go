package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

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

func TestCreatorApplicationStatusTransitionRepository_GetLatestByApplicationAndToStatus(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT actor_id, application_id, created_at, from_status, id, reason, to_status " +
		"FROM creator_application_status_transitions " +
		"WHERE application_id = $1 AND to_status = $2 " +
		"ORDER BY created_at DESC LIMIT 1"

	cols := []string{"actor_id", "application_id", "created_at", "from_status", "id", "reason", "to_status"}

	t.Run("success returns row", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationStatusTransitionRepository{db: mock}

		actor := "00000000-0000-0000-0000-000000000aaa"
		from := "verification"
		reason := "reject_admin"
		createdAt := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1", "rejected").
			WillReturnRows(pgxmock.NewRows(cols).
				AddRow(&actor, "app-1", createdAt, &from, "tx-1", &reason, "rejected"))

		got, err := repo.GetLatestByApplicationAndToStatus(context.Background(), "app-1", "rejected")
		require.NoError(t, err)
		require.Equal(t, &CreatorApplicationStatusTransitionRow{
			ID:            "tx-1",
			ApplicationID: "app-1",
			FromStatus:    &from,
			ToStatus:      "rejected",
			ActorID:       &actor,
			Reason:        &reason,
			CreatedAt:     createdAt,
		}, got)
	})

	t.Run("not found returns wrapped sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationStatusTransitionRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("missing", "rejected").
			WillReturnRows(pgxmock.NewRows(cols))

		_, err := repo.GetLatestByApplicationAndToStatus(context.Background(), "missing", "rejected")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationStatusTransitionRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1", "rejected").
			WillReturnError(errors.New("db down"))

		_, err := repo.GetLatestByApplicationAndToStatus(context.Background(), "app-1", "rejected")
		require.ErrorContains(t, err, "db down")
	})
}
