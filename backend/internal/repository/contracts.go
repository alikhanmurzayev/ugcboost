package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/elgris/stom"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// Contract subject_kind values.
const (
	ContractSubjectKindCampaignCreator = "campaign_creator"
)

// Contract unique-constraint names — matched against pgErr.ConstraintName.
const (
	ContractsTrustMeDocumentIDUnique = "contracts_trustme_document_id_unique"
)

// Contracts table and column names.
const (
	TableContracts                   = "contracts"
	ContractColumnID                 = "id"
	ContractColumnSubjectKind        = "subject_kind"
	ContractColumnTrustMeDocumentID  = "trustme_document_id"
	ContractColumnTrustMeShortURL    = "trustme_short_url"
	ContractColumnTrustMeStatusCode  = "trustme_status_code"
	ContractColumnUnsignedPDFContent = "unsigned_pdf_content"
	ContractColumnSignedPDFContent   = "signed_pdf_content"
	ContractColumnInitiatedAt        = "initiated_at"
	ContractColumnSignedAt           = "signed_at"
	ContractColumnDeclinedAt         = "declined_at"
	ContractColumnWebhookReceivedAt  = "webhook_received_at"
	ContractColumnNextRetryAt        = "next_retry_at"
	ContractColumnLastErrorCode      = "last_error_code"
	ContractColumnLastErrorMessage   = "last_error_message"
	ContractColumnLastAttemptedAt    = "last_attempted_at"
	ContractColumnSerialNumber       = "serial_number"
	ContractColumnCreatedAt          = "created_at"
	ContractColumnUpdatedAt          = "updated_at"
)

// ContractRow maps to the contracts table. Big bytea blobs (unsigned/signed
// PDFs) live alongside metadata. Default SELECT does NOT pull PDFs — repos
// project them only on dedicated reads (orphan recovery).
type ContractRow struct {
	ID                string     `db:"id"`
	SubjectKind       string     `db:"subject_kind"          insert:"subject_kind"`
	TrustMeDocumentID *string    `db:"trustme_document_id"`
	TrustMeShortURL   *string    `db:"trustme_short_url"`
	TrustMeStatusCode int        `db:"trustme_status_code"   insert:"trustme_status_code"`
	InitiatedAt       time.Time  `db:"initiated_at"`
	SignedAt          *time.Time `db:"signed_at"`
	DeclinedAt        *time.Time `db:"declined_at"`
	WebhookReceivedAt *time.Time `db:"webhook_received_at"`
	NextRetryAt       *time.Time `db:"next_retry_at"`
	LastErrorCode     *string    `db:"last_error_code"`
	LastErrorMessage  *string    `db:"last_error_message"`
	LastAttemptedAt   *time.Time `db:"last_attempted_at"`
	SerialNumber      int64      `db:"serial_number"`
	CreatedAt         time.Time  `db:"created_at"`
	UpdatedAt         time.Time  `db:"updated_at"`
}

var (
	contractSelectColumns = sortColumns(stom.MustNewStom(ContractRow{}).SetTag(string(tagSelect)).TagValues())
	contractInsertMapper  = stom.MustNewStom(ContractRow{}).SetTag(string(tagInsert))
)

// AgreedClaimRow — projected join between campaign_creators / campaigns /
// creators that the outbox-worker reads in Phase 1. Carries everything the
// later phases need so the worker keeps working from in-memory state and the
// transaction stays at milliseconds.
type AgreedClaimRow struct {
	CampaignCreatorID   string
	CampaignID          string
	CreatorID           string
	CreatorIIN          string
	CreatorLastName     string
	CreatorFirstName    string
	CreatorMiddleName   *string
	CreatorPhone        string
	ContractTemplatePDF []byte
}

// OrphanRow — Phase 0 recovery snapshot for one orphan'd contracts row. Carries
// the persisted unsigned PDF (may be nil for Phase-2b-failed orphans) so the
// worker can re-send without re-rendering.
type OrphanRow struct {
	ContractID         string
	UnsignedPDFContent []byte
}

// OrphanRequisites — данные креатора + serial_number по contract_id для
// Phase 0 resend.
type OrphanRequisites struct {
	CampaignCreatorID string
	CreatorID         string
	CreatorIIN        string
	CreatorLastName   string
	CreatorFirstName  string
	CreatorMiddleName *string
	CreatorPhone      string
	SerialNumber      int64
}

// ContractRepo lists every public method of the contracts repository.
type ContractRepo interface {
	Insert(ctx context.Context, row ContractRow) (*ContractRow, error)
	SelectAgreedForClaim(ctx context.Context, limit int) ([]*AgreedClaimRow, error)
	SelectOrphansForRecovery(ctx context.Context, limit int) ([]*OrphanRow, error)
	UpdateUnsignedPDF(ctx context.Context, id string, pdf []byte) error
	UpdateAfterSend(ctx context.Context, id, trustMeDocumentID, trustMeShortURL string, trustMeStatusCode int) error
	GetOrphanRequisites(ctx context.Context, contractID string) (*OrphanRequisites, error)
	RecordFailedAttempt(ctx context.Context, contractID, code, message string, nextRetryAt time.Time) error
}

type contractRepository struct {
	db dbutil.DB
}

// Insert persists a fresh contracts row. `INSERT ... RETURNING` projects the
// metadata columns (PDFs are NULL on insert anyway). Race-translation:
// 23505 on `contracts_trustme_document_id_unique` is impossible on Phase 1
// insert (trustme_document_id is set later in Phase 3) but covered for
// future-proofing.
func (r *contractRepository) Insert(ctx context.Context, row ContractRow) (*ContractRow, error) {
	q := sq.Insert(TableContracts).
		SetMap(toMap(row, contractInsertMapper)).
		Suffix(returningClause(contractSelectColumns))
	res, err := dbutil.One[ContractRow](ctx, r.db, q)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) &&
			pgErr.Code == "23505" &&
			pgErr.ConstraintName == ContractsTrustMeDocumentIDUnique {
			return nil, domain.ErrContractTrustMeDocumentIDTaken
		}
		return nil, err
	}
	return res, nil
}

// SelectAgreedForClaim picks up to `limit` campaign_creators rows in
// `agreed`, joined with campaigns + creators for in-memory overlay later.
// `FOR UPDATE SKIP LOCKED` makes concurrent workers split the batch instead
// of fighting over the same rows; campaigns.is_deleted=false and the
// non-empty template guard cover soft-deletes and legacy/unflagged campaigns.
//
// Caller MUST be inside a transaction — outside one, FOR UPDATE is a no-op
// (auto-commit). The columns projected here drive the manual scan loop —
// dbutil scanners cannot see across multi-table aliases without a single
// row-struct-with-all-fields, so we hand-roll the Query/Scan pair.
func (r *contractRepository) SelectAgreedForClaim(ctx context.Context, limit int) ([]*AgreedClaimRow, error) {
	if limit <= 0 {
		return nil, nil
	}
	const ccAlias = "cc"
	const cAlias = "c"
	const crAlias = "cr"
	q := sq.Select(
		ccAlias+"."+CampaignCreatorColumnID,
		ccAlias+"."+CampaignCreatorColumnCampaignID,
		ccAlias+"."+CampaignCreatorColumnCreatorID,
		crAlias+"."+CreatorColumnIIN,
		crAlias+"."+CreatorColumnLastName,
		crAlias+"."+CreatorColumnFirstName,
		crAlias+"."+CreatorColumnMiddleName,
		crAlias+"."+CreatorColumnPhone,
		cAlias+"."+CampaignColumnContractTemplatePDF,
	).
		From(TableCampaignCreators + " " + ccAlias).
		Join(TableCampaigns + " " + cAlias + " ON " + cAlias + "." + CampaignColumnID + " = " + ccAlias + "." + CampaignCreatorColumnCampaignID).
		Join(TableCreators + " " + crAlias + " ON " + crAlias + "." + CreatorColumnID + " = " + ccAlias + "." + CampaignCreatorColumnCreatorID).
		Where(sq.Eq{ccAlias + "." + CampaignCreatorColumnStatus: domain.CampaignCreatorStatusAgreed}).
		Where(sq.Expr(ccAlias + "." + CampaignCreatorColumnContractID + " IS NULL")).
		Where(sq.Eq{cAlias + "." + CampaignColumnIsDeleted: false}).
		Where(sq.Expr("length(" + cAlias + "." + CampaignColumnContractTemplatePDF + ") > 0")).
		OrderBy(ccAlias + "." + CampaignCreatorColumnDecidedAt + " ASC").
		Limit(uint64(limit)).
		Suffix("FOR UPDATE OF " + ccAlias + " SKIP LOCKED")

	sqlStr, args, err := q.ToSql()
	if err != nil {
		return nil, err
	}
	sqlStr, err = sq.Dollar.ReplacePlaceholders(sqlStr)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*AgreedClaimRow
	for rows.Next() {
		var (
			row        AgreedClaimRow
			middleName *string
		)
		if err := rows.Scan(
			&row.CampaignCreatorID,
			&row.CampaignID,
			&row.CreatorID,
			&row.CreatorIIN,
			&row.CreatorLastName,
			&row.CreatorFirstName,
			&middleName,
			&row.CreatorPhone,
			&row.ContractTemplatePDF,
		); err != nil {
			return nil, err
		}
		row.CreatorMiddleName = middleName
		out = append(out, &row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// SelectOrphansForRecovery returns contracts rows where the previous tick
// claimed the row but failed to land a TrustMe document ID. Phase 0 picks
// these up out of band and reconciles via TrustMe search. Per-row backoff
// (next_retry_at) keeps a poison row from blocking fresh slots: failed
// attempts push next_retry_at forward by cfg.TrustMeRetryBackoffSeconds,
// rows whose next_retry_at is still in the future are skipped this tick.
// NULLS FIRST keeps brand-new orphans (never attempted) ahead of retries.
func (r *contractRepository) SelectOrphansForRecovery(ctx context.Context, limit int) ([]*OrphanRow, error) {
	if limit <= 0 {
		return nil, nil
	}
	q := sq.Select(ContractColumnID, ContractColumnUnsignedPDFContent).
		From(TableContracts).
		Where(sq.Eq{ContractColumnSubjectKind: ContractSubjectKindCampaignCreator}).
		Where(sq.Eq{ContractColumnTrustMeDocumentID: nil}).
		Where(sq.Or{
			sq.Eq{ContractColumnNextRetryAt: nil},
			sq.Expr(ContractColumnNextRetryAt + " <= now()"),
		}).
		OrderBy(ContractColumnNextRetryAt + " ASC NULLS FIRST").
		OrderBy(ContractColumnInitiatedAt + " ASC").
		Limit(uint64(limit))

	sqlStr, args, err := q.ToSql()
	if err != nil {
		return nil, err
	}
	sqlStr, err = sq.Dollar.ReplacePlaceholders(sqlStr)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*OrphanRow
	for rows.Next() {
		var (
			row OrphanRow
			pdf []byte
		)
		if err := rows.Scan(&row.ContractID, &pdf); err != nil {
			return nil, err
		}
		row.UnsignedPDFContent = pdf
		out = append(out, &row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateUnsignedPDF persists Phase 2b — the rendered PDF lands in the row
// before the network call so Phase 0 recovery can re-send without re-render.
// Single mutating statement, no transaction wrapper needed (Postgres-atomic).
func (r *contractRepository) UpdateUnsignedPDF(ctx context.Context, id string, pdf []byte) error {
	q := sq.Update(TableContracts).
		Set(ContractColumnUnsignedPDFContent, pdf).
		Set(ContractColumnUpdatedAt, sq.Expr("now()")).
		Where(sq.Eq{ContractColumnID: id})
	n, err := dbutil.Exec(ctx, r.db, q)
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateAfterSend lands Phase 3 — TrustMe-issued document_id + short_url +
// observed status_code. Idempotency guard: WHERE trustme_document_id IS NULL
// — повторный finalize (Phase 0 race / overlapping ticks) не перезаписывает
// уже-finalized ряд. n=0 без error означает что ряд уже finalize'нут другим
// worker'ом — caller трактует это как success (no-op).
//
// Race-translation: 23505 on contracts_trustme_document_id_unique would mean
// TrustMe issued the same document_id for two of our rows (impossible per
// their semantics, but guarded for clarity).
func (r *contractRepository) UpdateAfterSend(ctx context.Context, id, trustMeDocumentID, trustMeShortURL string, trustMeStatusCode int) error {
	q := sq.Update(TableContracts).
		Set(ContractColumnTrustMeDocumentID, trustMeDocumentID).
		Set(ContractColumnTrustMeShortURL, trustMeShortURL).
		Set(ContractColumnTrustMeStatusCode, trustMeStatusCode).
		Set(ContractColumnUpdatedAt, sq.Expr("now()")).
		Where(sq.Eq{ContractColumnID: id}).
		Where(sq.Expr(ContractColumnTrustMeDocumentID + " IS NULL"))
	_, err := dbutil.Exec(ctx, r.db, q)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) &&
			pgErr.Code == "23505" &&
			pgErr.ConstraintName == ContractsTrustMeDocumentIDUnique {
			return domain.ErrContractTrustMeDocumentIDTaken
		}
		return err
	}
	return nil
}

// RecordFailedAttempt фиксирует неудачный TrustMe-вызов на orphan'е: код от
// API (или "" для сетевых сбоев), formatted error message и следующий
// next_retry_at. Phase 0 SelectOrphansForRecovery уважает next_retry_at,
// чтобы кривой orphan уходил в backoff и не блокировал свежие. Single
// UPDATE, без транзакционной обёртки.
func (r *contractRepository) RecordFailedAttempt(ctx context.Context, contractID, code, message string, nextRetryAt time.Time) error {
	q := sq.Update(TableContracts).
		Set(ContractColumnLastErrorCode, code).
		Set(ContractColumnLastErrorMessage, message).
		Set(ContractColumnLastAttemptedAt, sq.Expr("now()")).
		Set(ContractColumnNextRetryAt, nextRetryAt).
		Set(ContractColumnUpdatedAt, sq.Expr("now()")).
		Where(sq.Eq{ContractColumnID: contractID})
	n, err := dbutil.Exec(ctx, r.db, q)
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetOrphanRequisites JOIN'ит contracts → campaign_creators → creators по
// contract_id. Phase 0 resend подкладывает FIO/IIN/Phone и serial_number в
// trustme.SendToSignInput.
func (r *contractRepository) GetOrphanRequisites(ctx context.Context, contractID string) (*OrphanRequisites, error) {
	const ctAlias = "ct"
	const ccAlias = "cc"
	const crAlias = "cr"
	q := sq.Select(
		ccAlias+"."+CampaignCreatorColumnID,
		ccAlias+"."+CampaignCreatorColumnCreatorID,
		crAlias+"."+CreatorColumnIIN,
		crAlias+"."+CreatorColumnLastName,
		crAlias+"."+CreatorColumnFirstName,
		crAlias+"."+CreatorColumnMiddleName,
		crAlias+"."+CreatorColumnPhone,
		ctAlias+"."+ContractColumnSerialNumber,
	).
		From(TableContracts + " " + ctAlias).
		Join(TableCampaignCreators + " " + ccAlias + " ON " + ccAlias + "." + CampaignCreatorColumnContractID + " = " + ctAlias + "." + ContractColumnID).
		Join(TableCreators + " " + crAlias + " ON " + crAlias + "." + CreatorColumnID + " = " + ccAlias + "." + CampaignCreatorColumnCreatorID).
		Where(sq.Eq{ctAlias + "." + ContractColumnID: contractID})

	sqlStr, args, err := q.ToSql()
	if err != nil {
		return nil, err
	}
	sqlStr, err = sq.Dollar.ReplacePlaceholders(sqlStr)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}
	var (
		out        OrphanRequisites
		middleName *string
	)
	if err := rows.Scan(
		&out.CampaignCreatorID,
		&out.CreatorID,
		&out.CreatorIIN,
		&out.CreatorLastName,
		&out.CreatorFirstName,
		&middleName,
		&out.CreatorPhone,
		&out.SerialNumber,
	); err != nil {
		return nil, err
	}
	out.CreatorMiddleName = middleName
	return &out, nil
}
