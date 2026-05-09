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
	contractAllCols              = "created_at, declined_at, id, initiated_at, last_attempted_at, last_error_code, last_error_message, next_retry_at, serial_number, signed_at, subject_kind, trustme_document_id, trustme_short_url, trustme_status_code, updated_at, webhook_received_at"
	contractInsertSQL            = "INSERT INTO contracts (subject_kind,trustme_status_code) VALUES ($1,$2) RETURNING " + contractAllCols
	contractClaimSQL             = "SELECT cc.id, cc.campaign_id, cc.creator_id, cr.iin, cr.last_name, cr.first_name, cr.middle_name, cr.phone, c.contract_template_pdf FROM campaign_creators cc JOIN campaigns c ON c.id = cc.campaign_id JOIN creators cr ON cr.id = cc.creator_id WHERE cc.status = $1 AND cc.contract_id IS NULL AND c.is_deleted = $2 AND length(c.contract_template_pdf) > 0 ORDER BY cc.decided_at ASC LIMIT 4 FOR UPDATE OF cc SKIP LOCKED"
	contractOrphanSQL            = "SELECT id, unsigned_pdf_content FROM contracts WHERE subject_kind = $1 AND trustme_document_id IS NULL AND (next_retry_at IS NULL OR next_retry_at <= now()) ORDER BY next_retry_at ASC NULLS FIRST, initiated_at ASC LIMIT 8"
	contractUpdateUnsignedSQL    = "UPDATE contracts SET unsigned_pdf_content = $1, updated_at = now() WHERE id = $2"
	contractUpdateAfterSendSQL   = "UPDATE contracts SET trustme_document_id = $1, trustme_short_url = $2, trustme_status_code = $3, updated_at = now() WHERE id = $4 AND trustme_document_id IS NULL"
	contractRecordFailedSQL      = "UPDATE contracts SET last_error_code = $1, last_error_message = $2, last_attempted_at = now(), next_retry_at = $3, updated_at = now() WHERE id = $4"
	contractLockByDocSQL         = "SELECT " + contractAllCols + " FROM contracts WHERE trustme_document_id = $1 FOR UPDATE"
	contractWebhookUpdateBaseSQL = "UPDATE contracts SET trustme_status_code = $1, webhook_received_at = now(), updated_at = now()"
	contractWebhookGuardSQL      = " WHERE id = $2 AND trustme_status_code <> $3 AND trustme_status_code NOT IN ($4,$5)"
)

var contractRowCols = []string{
	"created_at", "declined_at", "id", "initiated_at",
	"last_attempted_at", "last_error_code", "last_error_message", "next_retry_at",
	"serial_number", "signed_at", "subject_kind", "trustme_document_id",
	"trustme_short_url", "trustme_status_code", "updated_at",
	"webhook_received_at",
}

func TestContractRepository_Insert(t *testing.T) {
	t.Parallel()

	t.Run("success projects metadata", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}
		now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

		mock.ExpectQuery(contractInsertSQL).
			WithArgs(ContractSubjectKindCampaignCreator, 0).
			WillReturnRows(pgxmock.NewRows(contractRowCols).
				AddRow(now, (*time.Time)(nil), "ct-1", now,
					(*time.Time)(nil), (*string)(nil), (*string)(nil), (*time.Time)(nil),
					int64(42), (*time.Time)(nil), ContractSubjectKindCampaignCreator,
					(*string)(nil), (*string)(nil), 0, now, (*time.Time)(nil)))

		got, err := repo.Insert(context.Background(), ContractRow{
			SubjectKind:       ContractSubjectKindCampaignCreator,
			TrustMeStatusCode: 0,
		})
		require.NoError(t, err)
		require.Equal(t, &ContractRow{
			ID:                "ct-1",
			SubjectKind:       ContractSubjectKindCampaignCreator,
			TrustMeStatusCode: 0,
			InitiatedAt:       now,
			SerialNumber:      42,
			CreatedAt:         now,
			UpdatedAt:         now,
		}, got)
	})

	t.Run("23505 on trustme_document_id translates to domain error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectQuery(contractInsertSQL).
			WithArgs(ContractSubjectKindCampaignCreator, 0).
			WillReturnError(&pgconn.PgError{
				Code:           "23505",
				ConstraintName: ContractsTrustMeDocumentIDUnique,
			})

		_, err := repo.Insert(context.Background(), ContractRow{
			SubjectKind:       ContractSubjectKindCampaignCreator,
			TrustMeStatusCode: 0,
		})
		require.ErrorIs(t, err, domain.ErrContractTrustMeDocumentIDTaken)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectQuery(contractInsertSQL).
			WithArgs(ContractSubjectKindCampaignCreator, 0).
			WillReturnError(errors.New("db down"))

		_, err := repo.Insert(context.Background(), ContractRow{
			SubjectKind:       ContractSubjectKindCampaignCreator,
			TrustMeStatusCode: 0,
		})
		require.ErrorContains(t, err, "db down")
	})
}

func TestContractRepository_SelectAgreedForClaim(t *testing.T) {
	t.Parallel()

	t.Run("zero limit returns nil without query", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}
		got, err := repo.SelectAgreedForClaim(context.Background(), 0)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("scans rows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectQuery(contractClaimSQL).
			WithArgs(domain.CampaignCreatorStatusAgreed, false).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "campaign_id", "creator_id",
				"iin", "last_name", "first_name", "middle_name", "phone",
				"contract_template_pdf",
			}).
				AddRow("cc-1", "camp-1", "cr-1",
					"880101300123", "Иванов", "Иван", (*string)(nil), "+77071234567",
					[]byte{0x25, 0x50, 0x44, 0x46}))

		got, err := repo.SelectAgreedForClaim(context.Background(), 4)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "cc-1", got[0].CampaignCreatorID)
		require.Equal(t, "camp-1", got[0].CampaignID)
		require.Equal(t, "cr-1", got[0].CreatorID)
		require.Equal(t, "880101300123", got[0].CreatorIIN)
		require.Equal(t, "Иванов", got[0].CreatorLastName)
		require.Equal(t, "Иван", got[0].CreatorFirstName)
		require.Nil(t, got[0].CreatorMiddleName)
		require.Equal(t, "+77071234567", got[0].CreatorPhone)
		require.Equal(t, []byte{0x25, 0x50, 0x44, 0x46}, got[0].ContractTemplatePDF)
	})
}

func TestContractRepository_SelectAgreedForClaim_DisjointBatches(t *testing.T) {
	t.Parallel()

	// Эмулирует поведение `FOR UPDATE SKIP LOCKED`: два параллельных
	// worker'а получают непересекающиеся подмножества `agreed`-рядов.
	// pgxmock не реализует настоящий row-lock, но мы фиксируем контракт:
	// SQL-запрос обоих worker'ов идентичен (это именно тот SELECT, который
	// получит SKIP LOCKED-семантику в Postgres) и repo возвращает
	// disjoint-данные, что отражает реальный план запроса.
	mock := newPgxmock(t)
	repo := &contractRepository{db: mock}

	pdfA := []byte{0x01}
	pdfB := []byte{0x02}

	mock.ExpectQuery(contractClaimSQL).
		WithArgs(domain.CampaignCreatorStatusAgreed, false).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "campaign_id", "creator_id",
			"iin", "last_name", "first_name", "middle_name", "phone",
			"contract_template_pdf",
		}).
			AddRow("cc-1", "camp-1", "cr-1", "880101300001", "A", "A", (*string)(nil), "+77000000001", pdfA))
	mock.ExpectQuery(contractClaimSQL).
		WithArgs(domain.CampaignCreatorStatusAgreed, false).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "campaign_id", "creator_id",
			"iin", "last_name", "first_name", "middle_name", "phone",
			"contract_template_pdf",
		}).
			AddRow("cc-2", "camp-1", "cr-2", "880101300002", "B", "B", (*string)(nil), "+77000000002", pdfB))

	a, err := repo.SelectAgreedForClaim(context.Background(), 4)
	require.NoError(t, err)
	require.Len(t, a, 1)
	require.Equal(t, "cc-1", a[0].CampaignCreatorID)

	b, err := repo.SelectAgreedForClaim(context.Background(), 4)
	require.NoError(t, err)
	require.Len(t, b, 1)
	require.Equal(t, "cc-2", b[0].CampaignCreatorID)

	// Никаких пересечений — каждый worker «увидел» свой ряд.
	require.NotEqual(t, a[0].CampaignCreatorID, b[0].CampaignCreatorID)
}

func TestContractRepository_SelectOrphansForRecovery(t *testing.T) {
	t.Parallel()

	t.Run("zero limit returns nil without query", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}
		got, err := repo.SelectOrphansForRecovery(context.Background(), 0)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("scans rows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectQuery(contractOrphanSQL).
			WithArgs(ContractSubjectKindCampaignCreator).
			WillReturnRows(pgxmock.NewRows([]string{"id", "unsigned_pdf_content"}).
				AddRow("ct-1", []byte{0xAA, 0xBB}).
				AddRow("ct-2", []byte(nil)))

		got, err := repo.SelectOrphansForRecovery(context.Background(), 8)
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, "ct-1", got[0].ContractID)
		require.Equal(t, []byte{0xAA, 0xBB}, got[0].UnsignedPDFContent)
		require.Equal(t, "ct-2", got[1].ContractID)
		require.Nil(t, got[1].UnsignedPDFContent)
	})

	t.Run("query error propagates", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectQuery(contractOrphanSQL).
			WithArgs(ContractSubjectKindCampaignCreator).
			WillReturnError(errors.New("db down"))

		_, err := repo.SelectOrphansForRecovery(context.Background(), 8)
		require.ErrorContains(t, err, "db down")
	})
}

func TestContractRepository_GetOrphanRequisites(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT cc.id, cc.creator_id, cr.iin, cr.last_name, cr.first_name, cr.middle_name, cr.phone, ct.serial_number FROM contracts ct JOIN campaign_creators cc ON cc.contract_id = ct.id JOIN creators cr ON cr.id = cc.creator_id WHERE ct.id = $1"

	t.Run("scans creator data", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		middle := "Иванович"
		mock.ExpectQuery(sqlStmt).
			WithArgs("ct-1").
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "creator_id", "iin", "last_name", "first_name", "middle_name", "phone", "serial_number",
			}).AddRow("cc-1", "cr-1", "880101300123", "Иванов", "Иван", &middle, "+77071234567", int64(77)))

		got, err := repo.GetOrphanRequisites(context.Background(), "ct-1")
		require.NoError(t, err)
		require.Equal(t, "cc-1", got.CampaignCreatorID)
		require.Equal(t, "cr-1", got.CreatorID)
		require.Equal(t, "880101300123", got.CreatorIIN)
		require.Equal(t, "Иванов", got.CreatorLastName)
		require.Equal(t, "Иван", got.CreatorFirstName)
		require.NotNil(t, got.CreatorMiddleName)
		require.Equal(t, "Иванович", *got.CreatorMiddleName)
		require.Equal(t, "+77071234567", got.CreatorPhone)
		require.Equal(t, int64(77), got.SerialNumber)
	})

	t.Run("not found returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("ct-missing").
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "creator_id", "iin", "last_name", "first_name", "middle_name", "phone", "serial_number",
			}))

		_, err := repo.GetOrphanRequisites(context.Background(), "ct-missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("query error propagates", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("ct-1").
			WillReturnError(errors.New("db down"))

		_, err := repo.GetOrphanRequisites(context.Background(), "ct-1")
		require.ErrorContains(t, err, "db down")
	})
}

func TestContractRepository_UpdateUnsignedPDF(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectExec(contractUpdateUnsignedSQL).
			WithArgs([]byte{0x01, 0x02}, "ct-1").
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		require.NoError(t, repo.UpdateUnsignedPDF(context.Background(), "ct-1", []byte{0x01, 0x02}))
	})

	t.Run("missing row returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectExec(contractUpdateUnsignedSQL).
			WithArgs([]byte(nil), "ct-missing").
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))

		err := repo.UpdateUnsignedPDF(context.Background(), "ct-missing", nil)
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectExec(contractUpdateUnsignedSQL).
			WithArgs([]byte{0x01}, "ct-1").
			WillReturnError(errors.New("db down"))

		err := repo.UpdateUnsignedPDF(context.Background(), "ct-1", []byte{0x01})
		require.ErrorContains(t, err, "db down")
	})
}

func TestContractRepository_RecordFailedAttempt(t *testing.T) {
	t.Parallel()

	t.Run("success persists code, message, next_retry_at", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}
		next := time.Date(2026, 5, 9, 12, 5, 0, 0, time.UTC)

		mock.ExpectExec(contractRecordFailedSQL).
			WithArgs("1219", "trustme: send-to-sign status=Error: 1219 (...)", next, "ct-1").
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		require.NoError(t, repo.RecordFailedAttempt(context.Background(),
			"ct-1", "1219", "trustme: send-to-sign status=Error: 1219 (...)", next))
	})

	t.Run("missing row returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}
		next := time.Date(2026, 5, 9, 12, 5, 0, 0, time.UTC)

		mock.ExpectExec(contractRecordFailedSQL).
			WithArgs("", "net err", next, "ct-missing").
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))

		err := repo.RecordFailedAttempt(context.Background(),
			"ct-missing", "", "net err", next)
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates other errors with context wrap", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}
		next := time.Date(2026, 5, 9, 12, 5, 0, 0, time.UTC)

		mock.ExpectExec(contractRecordFailedSQL).
			WithArgs("", "msg", next, "ct-1").
			WillReturnError(errors.New("db down"))

		err := repo.RecordFailedAttempt(context.Background(), "ct-1", "", "msg", next)
		require.Error(t, err)
		require.Contains(t, err.Error(), "db down")
		require.NotErrorIs(t, err, sql.ErrNoRows)
	})
}

func TestContractRepository_LockByTrustMeDocumentID(t *testing.T) {
	t.Parallel()

	t.Run("scans full row", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}
		now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
		docID := "doc-xyz"
		shortURL := "https://tct.kz/uploader/doc-xyz"

		mock.ExpectQuery(contractLockByDocSQL).
			WithArgs(docID).
			WillReturnRows(pgxmock.NewRows(contractRowCols).
				AddRow(now, (*time.Time)(nil), "ct-1", now,
					(*time.Time)(nil), (*string)(nil), (*string)(nil), (*time.Time)(nil),
					int64(7), (*time.Time)(nil), ContractSubjectKindCampaignCreator,
					&docID, &shortURL, 0, now, (*time.Time)(nil)))

		got, err := repo.LockByTrustMeDocumentID(context.Background(), docID)
		require.NoError(t, err)
		require.Equal(t, "ct-1", got.ID)
		require.NotNil(t, got.TrustMeDocumentID)
		require.Equal(t, docID, *got.TrustMeDocumentID)
	})

	t.Run("not found returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectQuery(contractLockByDocSQL).
			WithArgs("doc-missing").
			WillReturnRows(pgxmock.NewRows(contractRowCols))

		_, err := repo.LockByTrustMeDocumentID(context.Background(), "doc-missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectQuery(contractLockByDocSQL).
			WithArgs("doc-1").
			WillReturnError(errors.New("db down"))

		_, err := repo.LockByTrustMeDocumentID(context.Background(), "doc-1")
		require.ErrorContains(t, err, "db down")
	})
}

func TestContractRepository_UpdateAfterWebhook(t *testing.T) {
	t.Parallel()

	t.Run("status=3 stamps signed_at and matches one row", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectExec(contractWebhookUpdateBaseSQL+", signed_at = now()"+contractWebhookGuardSQL).
			WithArgs(3, "ct-1", 3, 3, 9).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		n, err := repo.UpdateAfterWebhook(context.Background(), "ct-1", 3)
		require.NoError(t, err)
		require.Equal(t, 1, n)
	})

	t.Run("status=9 stamps declined_at", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectExec(contractWebhookUpdateBaseSQL+", declined_at = now()"+contractWebhookGuardSQL).
			WithArgs(9, "ct-1", 9, 3, 9).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		n, err := repo.UpdateAfterWebhook(context.Background(), "ct-1", 9)
		require.NoError(t, err)
		require.Equal(t, 1, n)
	})

	t.Run("intermediate status omits signed_at/declined_at", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectExec(contractWebhookUpdateBaseSQL+contractWebhookGuardSQL).
			WithArgs(2, "ct-1", 2, 3, 9).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		n, err := repo.UpdateAfterWebhook(context.Background(), "ct-1", 2)
		require.NoError(t, err)
		require.Equal(t, 1, n)
	})

	t.Run("idempotent — same status returns 0 affected without error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectExec(contractWebhookUpdateBaseSQL+", signed_at = now()"+contractWebhookGuardSQL).
			WithArgs(3, "ct-1", 3, 3, 9).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))

		n, err := repo.UpdateAfterWebhook(context.Background(), "ct-1", 3)
		require.NoError(t, err)
		require.Equal(t, 0, n)
	})

	t.Run("propagates db errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectExec(contractWebhookUpdateBaseSQL+contractWebhookGuardSQL).
			WithArgs(1, "ct-1", 1, 3, 9).
			WillReturnError(errors.New("db down"))

		_, err := repo.UpdateAfterWebhook(context.Background(), "ct-1", 1)
		require.ErrorContains(t, err, "db down")
	})
}

func TestContractRepository_UpdateAfterSend(t *testing.T) {
	t.Parallel()

	t.Run("success persists document_id, short_url, status_code", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectExec(contractUpdateAfterSendSQL).
			WithArgs("doc-xyz", "https://tct.kz/uploader/doc-xyz", 0, "ct-1").
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		require.NoError(t, repo.UpdateAfterSend(context.Background(), "ct-1",
			"doc-xyz", "https://tct.kz/uploader/doc-xyz", 0))
	})

	t.Run("idempotent — already-finalized row is no-op (n=0, no error)", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		// Concurrent worker race: row already has trustme_document_id, our
		// UPDATE matches 0 rows because of WHERE trustme_document_id IS NULL.
		// We treat this as success — caller doesn't need to know.
		mock.ExpectExec(contractUpdateAfterSendSQL).
			WithArgs("doc-xyz", "url", 0, "ct-already-finalized").
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))

		require.NoError(t, repo.UpdateAfterSend(context.Background(),
			"ct-already-finalized", "doc-xyz", "url", 0))
	})

	t.Run("23505 translates to domain error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectExec(contractUpdateAfterSendSQL).
			WithArgs("doc-xyz", "url", 2, "ct-1").
			WillReturnError(&pgconn.PgError{
				Code:           "23505",
				ConstraintName: ContractsTrustMeDocumentIDUnique,
			})

		err := repo.UpdateAfterSend(context.Background(), "ct-1", "doc-xyz", "url", 2)
		require.ErrorIs(t, err, domain.ErrContractTrustMeDocumentIDTaken)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &contractRepository{db: mock}

		mock.ExpectExec(contractUpdateAfterSendSQL).
			WithArgs("doc-xyz", "url", 0, "ct-1").
			WillReturnError(errors.New("db down"))

		err := repo.UpdateAfterSend(context.Background(), "ct-1", "doc-xyz", "url", 0)
		require.ErrorContains(t, err, "db down")
	})
}
