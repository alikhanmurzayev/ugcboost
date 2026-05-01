package domain

import (
	"errors"
	"regexp"
	"time"
)

// CreatorApplicationStatus values stored in creator_applications.status.
// "Active" statuses are the ones that block a duplicate IIN submission;
// rejected applicants may re-apply (FR17).
const (
	CreatorApplicationStatusPending  = "pending"
	CreatorApplicationStatusApproved = "approved"
	CreatorApplicationStatusRejected = "rejected"
	CreatorApplicationStatusBlocked  = "blocked"
)

// CreatorApplicationActiveStatuses lists statuses that count as "in progress"
// for the IIN uniqueness rule. Mirrors the partial unique index in the
// 20260420181753_creator_applications.sql migration.
var CreatorApplicationActiveStatuses = []string{
	CreatorApplicationStatusPending,
	CreatorApplicationStatusApproved,
	CreatorApplicationStatusBlocked,
}

// SocialPlatform values mirror the enum in openapi.yaml. Adding a new value
// requires updating the enum, the CHECK constraint in the socials migration
// and the SocialPlatformValues registry below.
const (
	SocialPlatformInstagram = "instagram"
	SocialPlatformTikTok    = "tiktok"
	SocialPlatformThreads   = "threads"
)

// SocialPlatformValues is the canonical list of accepted platforms.
// Used by services/handlers when iterating and by tests for coverage.
var SocialPlatformValues = []string{SocialPlatformInstagram, SocialPlatformTikTok, SocialPlatformThreads}

// MaxCategoriesPerApplication caps how many category codes a creator may
// pick on the landing form. The landing UI enforces this client-side; the
// service rejects anything over the limit with 422 VALIDATION_ERROR so a
// non-browser client cannot bypass it.
const MaxCategoriesPerApplication = 3

// CategoryCodeOther is the category code that triggers the free-text
// "categoryOtherText" field on the submission DTO. When this code is in the
// selected categories, categoryOtherText becomes required.
const CategoryCodeOther = "other"

// ConsentType values stored in creator_application_consents.consent_type.
// Each application MUST record exactly one row per type (FR4).
const (
	ConsentTypeProcessing  = "processing"
	ConsentTypeThirdParty  = "third_party"
	ConsentTypeCrossBorder = "cross_border"
	ConsentTypeTerms       = "terms"
)

// ConsentTypeValues is the canonical ordered list. Submission services walk
// it to write the four consent rows in deterministic order.
var ConsentTypeValues = []string{
	ConsentTypeProcessing,
	ConsentTypeThirdParty,
	ConsentTypeCrossBorder,
	ConsentTypeTerms,
}

// Application-specific error codes carried in 4xx responses.
const (
	// 409 — IIN already has an active application.
	CodeCreatorApplicationDuplicate = "CREATOR_APPLICATION_DUPLICATE"
	// 422 — a category code in the request is unknown or inactive.
	CodeUnknownCategory = "UNKNOWN_CATEGORY"
	// 422 — the city code in the request is unknown or inactive.
	CodeUnknownCity = "UNKNOWN_CITY"
	// 422 — at least one of the four mandatory consents is missing.
	CodeMissingConsent = "MISSING_CONSENT"
	// 409 — application is already linked to a different Telegram account.
	CodeTelegramApplicationAlreadyLinked = "TELEGRAM_APPLICATION_ALREADY_LINKED"
)

// Telegram metadata length caps applied at the service layer before persisting,
// to prevent attacker-controlled mega-strings.
const (
	TelegramUsernameMaxLen = 64
	TelegramNameMaxLen     = 256
)

// ErrCreatorApplicationDuplicate is the sentinel the repository raises when
// the partial unique index on creator_applications(iin) WHERE status IN
// ('pending','approved','blocked') rejects an INSERT (concurrent submit lost
// the race after the service's HasActiveByIIN check). The service converts it
// into a business error with CodeCreatorApplicationDuplicate.
var ErrCreatorApplicationDuplicate = errors.New("creator application with this iin is already active")

// ErrTelegramApplicationLinkConflict is raised by the repo on a PRIMARY KEY
// violation on creator_application_telegram_links(application_id) — the
// application is already linked. The service re-reads the row to decide
// idempotent vs business error.
var ErrTelegramApplicationLinkConflict = errors.New("telegram link for this application already exists")

// SocialHandleRegex is the validation pattern applied to handles after they
// are trimmed, stripped of leading '@' and lowercased. Current scope (IG/TT)
// shares the same permissive subset — letters, digits, dot, underscore, up to
// 30 chars. Extending this regex when a new platform lands must go through
// Ask First in the spec.
var SocialHandleRegex = regexp.MustCompile(`^[a-z0-9._]{1,30}$`)

// CreatorApplicationInput is what the service receives after the handler has
// parsed and lightly validated the HTTP request. It carries the raw consent
// metadata (IP, User-Agent, document versions) so the service can persist a
// faithful audit trail without ever touching net/http.
type CreatorApplicationInput struct {
	LastName          string
	FirstName         string
	MiddleName        *string
	IIN               string
	Phone             string
	CityCode          string
	Address           *string
	CategoryCodes     []string
	CategoryOtherText *string
	Socials           []SocialAccountInput
	Consents          ConsentsInput
	IPAddress         string
	UserAgent         string
	AgreementVersion  string
	PrivacyVersion    string
	Now               time.Time
}

// SocialAccountInput is one validated handle on a known platform.
type SocialAccountInput struct {
	Platform string
	Handle   string
}

// ConsentsInput captures the single "accepted everything" flag the landing
// form sends. The legal model (privacy-policy.md §9.2) treats acceptance of
// the Privacy Policy as unconditional consent for processing, third-party
// transfer, cross-border transfer and the user agreement, so a single flag
// from the client is enough — the service still writes one row per consent
// type into creator_application_consents.
type ConsentsInput struct {
	AcceptedAll bool
}

// CreatorApplicationSubmission is what the service returns to the handler.
type CreatorApplicationSubmission struct {
	ApplicationID string
	BirthDate     time.Time
}

// CreatorApplicationDetail is the full read aggregate returned by the
// admin-only GET /creators/applications/{id} endpoint. It bundles the main
// application row with its three associated collections so the handler can
// shape one self-contained response — no extra round trips needed.
//
// Categories and CityCode carry only the raw codes stored in the database —
// the handler layer resolves them against DictionaryService at read time.
// Keeping the domain free of human-readable names means service/repo do not
// depend on the dictionary cache and stay one source of truth: data, not
// presentation.
type CreatorApplicationDetail struct {
	ID                string
	LastName          string
	FirstName         string
	MiddleName        *string
	IIN               string
	BirthDate         time.Time
	Phone             string
	CityCode          string
	Address           *string
	CategoryOtherText *string
	Status            string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Categories        []string
	Socials           []CreatorApplicationDetailSocial
	Consents          []CreatorApplicationDetailConsent
	TelegramLink      *CreatorApplicationTelegramLink
}

// CreatorApplicationTelegramLink describes the Telegram account bound to an
// application. Nil means the creator has not opened the bot yet.
type CreatorApplicationTelegramLink struct {
	ApplicationID     string
	TelegramUserID    int64
	TelegramUsername  *string
	TelegramFirstName *string
	TelegramLastName  *string
	LinkedAt          time.Time
}

// TelegramLinkInput is what the service receives from the bot's /start handler.
// Username/first/last may legitimately be absent (Telegram users can hide them).
type TelegramLinkInput struct {
	ApplicationID     string
	TelegramUserID    int64
	TelegramUsername  *string
	TelegramFirstName *string
	TelegramLastName  *string
}

// CreatorApplicationDetailSocial is one social account attached to the
// application — exactly what was persisted at submit time.
type CreatorApplicationDetailSocial struct {
	Platform string
	Handle   string
}

// CreatorApplicationDetailConsent is one consent record persisted at submit
// time. The handler returns these in canonical ConsentTypeValues order so
// admins always see them in the same sequence regardless of DB ordering.
type CreatorApplicationDetailConsent struct {
	ConsentType     string
	AcceptedAt      time.Time
	DocumentVersion string
	IPAddress       string
	UserAgent       string
}

// DocumentVersionFor returns the document version stamp recorded against the
// given consent type. Both processing and third_party / cross_border consents
// reference the privacy policy; terms references the user agreement.
func DocumentVersionFor(consentType, agreementVersion, privacyVersion string) string {
	switch consentType {
	case ConsentTypeTerms:
		return agreementVersion
	default:
		return privacyVersion
	}
}
