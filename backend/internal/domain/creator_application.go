package domain

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"time"
)

const (
	CreatorApplicationStatusVerification     = "verification"
	CreatorApplicationStatusModeration       = "moderation"
	CreatorApplicationStatusAwaitingContract = "awaiting_contract"
	CreatorApplicationStatusContractSent     = "contract_sent"
	CreatorApplicationStatusSigned           = "signed"
	CreatorApplicationStatusRejected         = "rejected"
	CreatorApplicationStatusWithdrawn        = "withdrawn"
)

var CreatorApplicationActiveStatuses = []string{
	CreatorApplicationStatusVerification,
	CreatorApplicationStatusModeration,
	CreatorApplicationStatusAwaitingContract,
	CreatorApplicationStatusContractSent,
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

// SocialVerificationMethod values mirror the enum in openapi.yaml. "auto"
// covers verification via SendPulse webhook (Instagram DM), "manual" covers
// admin-driven verification from the application drawer.
const (
	SocialVerificationMethodAuto   = "auto"
	SocialVerificationMethodManual = "manual"
)

// SocialVerificationMethodValues is the canonical list of accepted verification
// methods, used by handlers/tests when iterating.
var SocialVerificationMethodValues = []string{SocialVerificationMethodAuto, SocialVerificationMethodManual}

// Verification code: an "UGC-NNNNNN" identifier persisted on the application
// at submit time. The creator copies it from the TMA and sends it in an IG DM
// (auto path) or an admin matches it during manual verification. Format
// integrity lives in the service — there is no DB CHECK on the column.
const (
	VerificationCodePrefix                = "UGC-"
	VerificationCodeDigits                = 6
	VerificationCodeMaxGenerationAttempts = 20
)

// verificationCodeRandomMax is the upper bound (exclusive) for crypto/rand
// when sampling a digit sequence. Equal to 10**VerificationCodeDigits.
var verificationCodeRandomMax = big.NewInt(1_000_000)

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
// CreatorApplicationActiveStatuses rejects an INSERT (concurrent submit lost
// the race after the service's HasActiveByIIN check). The service converts
// it into a business error with CodeCreatorApplicationDuplicate.
var ErrCreatorApplicationDuplicate = errors.New("creator application with this iin is already active")

// ErrTelegramApplicationLinkConflict is raised by the repo on a PRIMARY KEY
// violation on creator_application_telegram_links(application_id) — the
// application is already linked. The service re-reads the row to decide
// idempotent vs business error.
var ErrTelegramApplicationLinkConflict = errors.New("telegram link for this application already exists")

// ErrCreatorApplicationVerificationCodeConflict is raised by the repo on a
// 23505 against the partial unique index over verification_code WHERE
// status='verification'. The service catches it and retries Submit with a
// freshly-generated code (cenkalti/backoff/v5, max VerificationCodeMaxGenerationAttempts).
var ErrCreatorApplicationVerificationCodeConflict = errors.New("creator application verification code collides with an existing verification-status row")

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
// application. Carries verification state so admin and creator surfaces can
// reflect "verified / by whom / when / how" without a separate fetch.
type CreatorApplicationDetailSocial struct {
	Platform         string
	Handle           string
	Verified         bool
	Method           *string
	VerifiedByUserID *string
	VerifiedAt       *time.Time
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

// Sort fields supported by the admin list endpoint
// (POST /creators/applications/list). The repo translates each value into a
// SQL column / expression; unknown values are rejected upstream by the
// handler with CodeValidation.
const (
	CreatorApplicationSortCreatedAt = "created_at"
	CreatorApplicationSortUpdatedAt = "updated_at"
	CreatorApplicationSortFullName  = "full_name"
	CreatorApplicationSortBirthDate = "birth_date"
	CreatorApplicationSortCityName  = "city_name"
)

// CreatorApplicationListSortFieldValues is the canonical, ordered list of
// supported sort fields. Iterating it gives the validator a single source of
// truth instead of a switch with hard-coded literals.
var CreatorApplicationListSortFieldValues = []string{
	CreatorApplicationSortCreatedAt,
	CreatorApplicationSortUpdatedAt,
	CreatorApplicationSortFullName,
	CreatorApplicationSortBirthDate,
	CreatorApplicationSortCityName,
}

// SortOrderAsc / SortOrderDesc — case-sensitive direction tokens accepted by
// the admin list endpoint.
const (
	SortOrderAsc  = "asc"
	SortOrderDesc = "desc"
)

// SortOrderValues lists every accepted direction token.
var SortOrderValues = []string{SortOrderAsc, SortOrderDesc}

// Pagination + filter bounds for the admin list endpoint. oapi-codegen does
// NOT enforce OpenAPI's minimum/maximum/maxLength at runtime, so the handler
// validates each constraint explicitly. The hard caps here protect both the
// instance (huge OFFSETs / megabyte ILIKE patterns / billion-element arrays
// = DoS vector) and the user (silent no-op filters when boundaries are
// quietly ignored).
const (
	CreatorApplicationListPageMin            = 1
	CreatorApplicationListPageMax            = 100_000
	CreatorApplicationListPerPageMin         = 1
	CreatorApplicationListPerPageMax         = 200
	CreatorApplicationListSearchMaxLen       = 128
	CreatorApplicationListAgeMin             = 0
	CreatorApplicationListAgeMax             = 120
	CreatorApplicationListCityCodeMaxLen     = 64
	CreatorApplicationListCategoryCodeMaxLen = 64
	CreatorApplicationListFilterArrayMax     = 50
)

// CreatorApplicationListInput is the validated read aggregate the service
// receives from the handler. Pointers / nullable types denote optional filters
// — nil/empty means "do not apply this filter". Statuses/Cities/Categories are
// any-of arrays.
type CreatorApplicationListInput struct {
	Statuses       []string
	Cities         []string
	Categories     []string
	DateFrom       *time.Time
	DateTo         *time.Time
	AgeFrom        *int
	AgeTo          *int
	TelegramLinked *bool
	Search         string
	Sort           string
	Order          string
	Page           int
	PerPage        int
}

// CreatorApplicationListItem is one row in the admin list response. The shape
// is intentionally lean — phone, address, consents and the full Telegram link
// aggregate are deliberately absent (admins fetch those via GET
// /creators/applications/{id}). Categories and CityCode hold raw codes; the
// handler resolves dictionary names at presentation time so service/repo stay
// presentation-free.
type CreatorApplicationListItem struct {
	ID             string
	Status         string
	LastName       string
	FirstName      string
	MiddleName     *string
	BirthDate      time.Time
	CityCode       string
	Categories     []string
	Socials        []CreatorApplicationDetailSocial
	TelegramLinked bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CreatorApplicationListPage is what the service returns: the page slice plus
// pagination echo (page/perPage) and the total count over the unpaginated
// filter set.
type CreatorApplicationListPage struct {
	Items   []*CreatorApplicationListItem
	Total   int64
	Page    int
	PerPage int
}

// CreatorApplicationAllStatuses is the canonical list of every legal status
// value (active + terminal). Use this instead of an ad-hoc literal list when a
// caller needs to validate that an external status string belongs to the
// state machine — e.g. when filtering counts coming back from the DB during
// a rolling deployment where a newer pod could have written a status the
// older pod does not yet recognise.
var CreatorApplicationAllStatuses = []string{
	CreatorApplicationStatusVerification,
	CreatorApplicationStatusModeration,
	CreatorApplicationStatusAwaitingContract,
	CreatorApplicationStatusContractSent,
	CreatorApplicationStatusSigned,
	CreatorApplicationStatusRejected,
	CreatorApplicationStatusWithdrawn,
}

// IsValidCreatorApplicationStatus reports whether s is one of the seven
// canonical lifecycle statuses. The check is whitelist-based so a typo or a
// newly-added status (rolling deploy) is rejected, not silently accepted.
func IsValidCreatorApplicationStatus(s string) bool {
	for _, v := range CreatorApplicationAllStatuses {
		if s == v {
			return true
		}
	}
	return false
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

// GenerateVerificationCode returns a fresh "UGC-NNNNNN" identifier sampled
// from crypto/rand. Callers should retry on
// ErrCreatorApplicationVerificationCodeConflict via cenkalti/backoff.
func GenerateVerificationCode() (string, error) {
	n, err := rand.Int(rand.Reader, verificationCodeRandomMax)
	if err != nil {
		return "", fmt.Errorf("verification code rand: %w", err)
	}
	return fmt.Sprintf("%s%0*d", VerificationCodePrefix, VerificationCodeDigits, n.Int64()), nil
}
