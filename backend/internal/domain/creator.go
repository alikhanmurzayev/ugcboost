package domain

import (
	"errors"
	"time"
)

// ErrCreatorAlreadyExists is the sentinel raised by CreatorRepo.Create when
// a 23505 fires on the creators_iin_unique constraint — a creator with this
// IIN has already been provisioned. Maps to CodeCreatorAlreadyExists / 422.
var ErrCreatorAlreadyExists = errors.New("creator with this iin already exists")

// ErrCreatorTelegramAlreadyTaken is the sentinel raised on a 23505 against
// creators_telegram_user_id_unique — the Telegram account is already bound
// to another creator. Maps to CodeCreatorTelegramAlreadyTaken / 422.
var ErrCreatorTelegramAlreadyTaken = errors.New("creator with this telegram_user_id already exists")

// ErrCreatorApplicationNotApprovable is the sentinel raised when the
// application is not in `moderation`, or on a 23505 against
// creators_source_application_id_unique (concurrent approve lost the race).
// Maps to CodeCreatorApplicationNotApprovable / 422.
var ErrCreatorApplicationNotApprovable = errors.New("creator application is not in an approvable status")

// ErrCreatorNotFound is the sentinel raised by CreatorService.GetByID when
// the creator row does not exist. The handler maps it to 404
// CREATOR_NOT_FOUND. We translate sql.ErrNoRows in the service layer instead
// of relying on errors.Is(sql.ErrNoRows) directly so the response carries the
// creator-specific code rather than the generic NOT_FOUND fallback.
var ErrCreatorNotFound = errors.New("creator not found")

// Creator-flow user-facing codes carried in 4xx responses by approve action.
const (
	// 422 — application is not in `moderation` (e.g. verification, rejected,
	// approved, withdrawn). Also surfaces when a concurrent approve already
	// produced a creator for the same source application.
	CodeCreatorApplicationNotApprovable = "CREATOR_APPLICATION_NOT_APPROVABLE"
	// 422 — `creators.iin_unique` was violated. Kept as a real failure mode
	// for future re-application flows so the table cannot be silently
	// corrupted with two active creator rows for the same IIN.
	CodeCreatorAlreadyExists = "CREATOR_ALREADY_EXISTS"
	// 422 — `creators.telegram_user_id_unique` was violated. Same Telegram
	// account is already tied to another creator. Operator must pick whether
	// to reject the duplicate application or unlink the prior creator.
	CodeCreatorTelegramAlreadyTaken = "CREATOR_TELEGRAM_ALREADY_TAKEN"
	// 404 — creator with the requested id does not exist.
	CodeCreatorNotFound = "CREATOR_NOT_FOUND"
)

// Creator is the flat domain projection the service hands to CreatorRepo.Create.
// Mirrors repository.CreatorRow's insert tags 1:1 — the helper
// NewCreatorFromApplication composes one from the application row plus its
// Telegram link.
type Creator struct {
	IIN                 string
	LastName            string
	FirstName           string
	MiddleName          *string
	BirthDate           time.Time
	Phone               string
	CityCode            string
	Address             *string
	CategoryOtherText   *string
	TelegramUserID      int64
	TelegramUsername    *string
	TelegramFirstName   *string
	TelegramLastName    *string
	SourceApplicationID string
}

// CreatorSocial is one social account snapshot copied from the application's
// social row at approve time. The verification fields are copied 1:1 — the
// admin should see the exact verification metadata that was on the application
// at the moment of approve, even after the application row gets archived.
type CreatorSocial struct {
	CreatorID        string
	Platform         string
	Handle           string
	Verified         bool
	Method           *string
	VerifiedByUserID *string
	VerifiedAt       *time.Time
}

// CreatorCategory is one category code attached to the creator. CategoryCode
// is resolved against the active categories dictionary at read time
// (chunk 18c), so the domain layer keeps only the raw code.
type CreatorCategory struct {
	CreatorID    string
	CategoryCode string
}

// CreatorSnapshotInput captures the application + Telegram-link fields the
// service reads inside the approve transaction. NewCreatorFromApplication
// receives one of these instead of the repo row types directly to avoid a
// domain → repository import cycle (repository already imports domain for
// the sentinels above).
type CreatorSnapshotInput struct {
	ApplicationID     string
	IIN               string
	LastName          string
	FirstName         string
	MiddleName        *string
	BirthDate         time.Time
	Phone             string
	CityCode          string
	Address           *string
	CategoryOtherText *string
	TelegramUserID    int64
	TelegramUsername  *string
	TelegramFirstName *string
	TelegramLastName  *string
}

// NewCreatorFromApplication composes a Creator domain object from the source
// application snapshot. The mapping is straight 1:1; SourceApplicationID
// points back at the originating application so audit / analytics joins
// remain possible after the application row is archived.
func NewCreatorFromApplication(in CreatorSnapshotInput) *Creator {
	return &Creator{
		IIN:                 in.IIN,
		LastName:            in.LastName,
		FirstName:           in.FirstName,
		MiddleName:          in.MiddleName,
		BirthDate:           in.BirthDate,
		Phone:               in.Phone,
		CityCode:            in.CityCode,
		Address:             in.Address,
		CategoryOtherText:   in.CategoryOtherText,
		TelegramUserID:      in.TelegramUserID,
		TelegramUsername:    in.TelegramUsername,
		TelegramFirstName:   in.TelegramFirstName,
		TelegramLastName:    in.TelegramLastName,
		SourceApplicationID: in.ApplicationID,
	}
}

// CreatorAggregateSocial is one social row of a CreatorAggregate. It mirrors
// the persisted creator_socials snapshot — verification fields stay nilable
// because Threads / unverified accounts can ride alongside verified ones in
// the same aggregate.
type CreatorAggregateSocial struct {
	ID               string
	Platform         string
	Handle           string
	Verified         bool
	Method           *string
	VerifiedByUserID *string
	VerifiedAt       *time.Time
	CreatedAt        time.Time
}

// CreatorAggregateCategory is one (code, name) pair attached to a creator.
// Name is hydrated by the service against the active categories dictionary;
// when the dictionary entry has been deactivated since approval, Name falls
// back to Code so admins still see a meaningful reference.
type CreatorAggregateCategory struct {
	Code string
	Name string
}

// CreatorAggregate is the full creator profile served by GET /creators/{id}.
// It collapses the relational creators / creator_socials / creator_categories
// trio into one flat document because every consumer (admin UI, future read
// flows) wants the whole snapshot in one round-trip; the Telegram block is
// inlined for the same reason — there is no concept of a creator without a
// Telegram link, so a separate optional object would only invite null checks.
type CreatorAggregate struct {
	ID                  string
	IIN                 string
	SourceApplicationID string
	LastName            string
	FirstName           string
	MiddleName          *string
	BirthDate           time.Time
	Phone               string
	CityCode            string
	CityName            string
	Address             *string
	CategoryOtherText   *string
	TelegramUserID      int64
	TelegramUsername    *string
	TelegramFirstName   *string
	TelegramLastName    *string
	Socials             []CreatorAggregateSocial
	Categories          []CreatorAggregateCategory
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
