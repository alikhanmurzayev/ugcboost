package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestCreatorApplicationConsentRepository_InsertMany(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO creator_application_consents (accepted_at,application_id,consent_type,document_version,ip_address,user_agent) VALUES ($1,$2,$3,$4,$5,$6),($7,$8,$9,$10,$11,$12)"

	t.Run("empty input short-circuits", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationConsentRepository{db: mock}

		require.NoError(t, repo.InsertMany(context.Background(), nil))
	})

	t.Run("success batches two consents in declared order", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationConsentRepository{db: mock}
		acceptedAt := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)

		mock.ExpectExec(sqlStmt).
			WithArgs(
				acceptedAt, "app-1", "processing", "2026-04-20", "127.0.0.1", "ua/1",
				acceptedAt, "app-1", "terms", "2026-04-20", "127.0.0.1", "ua/1",
			).
			WillReturnResult(pgconn.NewCommandTag("INSERT 0 2"))

		err := repo.InsertMany(context.Background(), []CreatorApplicationConsentRow{
			{ApplicationID: "app-1", ConsentType: "processing", AcceptedAt: acceptedAt, DocumentVersion: "2026-04-20", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
			{ApplicationID: "app-1", ConsentType: "terms", AcceptedAt: acceptedAt, DocumentVersion: "2026-04-20", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
		})
		require.NoError(t, err)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationConsentRepository{db: mock}
		acceptedAt := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)

		mock.ExpectExec(sqlStmt).
			WithArgs(
				acceptedAt, "app-1", "processing", "2026-04-20", "127.0.0.1", "ua/1",
				acceptedAt, "app-1", "terms", "2026-04-20", "127.0.0.1", "ua/1",
			).
			WillReturnError(errors.New("constraint failed"))

		err := repo.InsertMany(context.Background(), []CreatorApplicationConsentRow{
			{ApplicationID: "app-1", ConsentType: "processing", AcceptedAt: acceptedAt, DocumentVersion: "2026-04-20", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
			{ApplicationID: "app-1", ConsentType: "terms", AcceptedAt: acceptedAt, DocumentVersion: "2026-04-20", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
		})
		require.ErrorContains(t, err, "constraint failed")
	})
}
