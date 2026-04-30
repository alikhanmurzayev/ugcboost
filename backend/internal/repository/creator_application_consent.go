package repository

import (
	"context"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/elgris/stom"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
)

// Creator application consents table and column names.
const (
	TableCreatorApplicationConsents                = "creator_application_consents"
	CreatorApplicationConsentColumnID              = "id"
	CreatorApplicationConsentColumnApplicationID   = "application_id"
	CreatorApplicationConsentColumnConsentType     = "consent_type"
	CreatorApplicationConsentColumnAcceptedAt      = "accepted_at"
	CreatorApplicationConsentColumnDocumentVersion = "document_version"
	CreatorApplicationConsentColumnIPAddress       = "ip_address"
	CreatorApplicationConsentColumnUserAgent       = "user_agent"
)

// CreatorApplicationConsentRow maps to the creator_application_consents table.
// AcceptedAt is insert-tagged so the service can stamp a single "now" for the
// whole submission batch rather than letting each row drift by milliseconds.
type CreatorApplicationConsentRow struct {
	ID              string    `db:"id"`
	ApplicationID   string    `db:"application_id"  insert:"application_id"`
	ConsentType     string    `db:"consent_type"    insert:"consent_type"`
	AcceptedAt      time.Time `db:"accepted_at"     insert:"accepted_at"`
	DocumentVersion string    `db:"document_version" insert:"document_version"`
	IPAddress       string    `db:"ip_address"      insert:"ip_address"`
	UserAgent       string    `db:"user_agent"      insert:"user_agent"`
}

var (
	creatorApplicationConsentSelectColumns = sortColumns(stom.MustNewStom(CreatorApplicationConsentRow{}).SetTag(string(tagSelect)).TagValues())
	creatorApplicationConsentInsertMapper  = stom.MustNewStom(CreatorApplicationConsentRow{}).SetTag(string(tagInsert))
	creatorApplicationConsentInsertColumns = sortColumns(creatorApplicationConsentInsertMapper.TagValues())
)

// CreatorApplicationConsentRepo batches the consent records (exactly four per
// application) captured at submission time.
type CreatorApplicationConsentRepo interface {
	InsertMany(ctx context.Context, rows []CreatorApplicationConsentRow) error
	ListByApplicationID(ctx context.Context, applicationID string) ([]*CreatorApplicationConsentRow, error)
}

type creatorApplicationConsentRepository struct {
	db dbutil.DB
}

// InsertMany writes every consent row in a single INSERT. Empty input is a
// no-op.
func (r *creatorApplicationConsentRepository) InsertMany(ctx context.Context, rows []CreatorApplicationConsentRow) error {
	if len(rows) == 0 {
		return nil
	}
	qb := sq.Insert(TableCreatorApplicationConsents).Columns(creatorApplicationConsentInsertColumns...)
	for _, row := range rows {
		qb = insertEntities(qb, creatorApplicationConsentInsertMapper, creatorApplicationConsentInsertColumns, row)
	}
	_, err := dbutil.Exec(ctx, r.db, qb)
	return err
}

// ListByApplicationID returns every consent row tied to the given application.
// No DB-level ORDER BY: the service places the rows in canonical
// ConsentTypeValues order in memory, so the SQL ordering would be wasted work.
func (r *creatorApplicationConsentRepository) ListByApplicationID(ctx context.Context, applicationID string) ([]*CreatorApplicationConsentRow, error) {
	q := sq.Select(creatorApplicationConsentSelectColumns...).
		From(TableCreatorApplicationConsents).
		Where(sq.Eq{CreatorApplicationConsentColumnApplicationID: applicationID})
	return dbutil.Many[CreatorApplicationConsentRow](ctx, r.db, q)
}
