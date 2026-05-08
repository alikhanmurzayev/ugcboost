package domain

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
)

const (
	CreatorApplicationStatusVerification = "verification"
	CreatorApplicationStatusModeration   = "moderation"
	CreatorApplicationStatusApproved     = "approved"
	CreatorApplicationStatusRejected     = "rejected"
	CreatorApplicationStatusWithdrawn    = "withdrawn"
)

var CreatorApplicationActiveStatuses = []string{
	CreatorApplicationStatusVerification,
	CreatorApplicationStatusModeration,
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
	// 404 — manual verify target social row does not belong to the application
	// (or does not exist at all). The application-level NOT_FOUND lives under
	// the shared CodeNotFound, but the social-level miss carries its own code
	// so admins know whether to refresh the drawer or refetch the list.
	CodeCreatorApplicationSocialNotFound = "CREATOR_APPLICATION_SOCIAL_NOT_FOUND"
	// 409 — manual verify target social row is already verified (auto or
	// manual). Idempotency-bearing — repeats are user errors, not no-ops.
	CodeCreatorApplicationSocialAlreadyVerified = "CREATOR_APPLICATION_SOCIAL_ALREADY_VERIFIED"
	// 422 — manual verify is only legal while the application sits in
	// `verification`. Once it has moved on (moderation/approved/...) the action
	// is rejected so admins do not silently re-trigger transitions.
	CodeCreatorApplicationNotInVerification = "CREATOR_APPLICATION_NOT_IN_VERIFICATION"
	// 422 — manual verify refuses to run when the creator has not opened the
	// Telegram bot via /start. Without the link there is no chat to notify
	// downstream and the moderator cannot follow up out-of-band.
	CodeCreatorApplicationTelegramNotLinked = "CREATOR_APPLICATION_TELEGRAM_NOT_LINKED"
	// 422 — admin reject is only legal while the application sits in
	// `verification` or `moderation`. Any other status (including a second
	// reject of an already-rejected application) is refused so the operation
	// cannot silently re-trigger transitions or contradict the state machine.
	CodeCreatorApplicationNotRejectable = "CREATOR_APPLICATION_NOT_REJECTABLE"
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

// ErrCreatorApplicationNotFound — no application exists for the supplied id.
// Service raises it instead of leaking the underlying sql.ErrNoRows so the
// handler maps a single sentinel to the user-facing 404 NOT_FOUND code.
var ErrCreatorApplicationNotFound = errors.New("creator application not found")

// ErrCreatorApplicationNotInVerification — the application is no longer in
// the `verification` status, so manual verify cannot run. 422.
var ErrCreatorApplicationNotInVerification = errors.New("creator application is not in verification status")

// ErrCreatorApplicationSocialNotFound — the supplied socialID does not point
// at a row attached to this application (404).
var ErrCreatorApplicationSocialNotFound = errors.New("creator application social not found")

// ErrCreatorApplicationSocialAlreadyVerified — the targeted social is
// already verified (409). Repeats are user errors, not idempotency.
var ErrCreatorApplicationSocialAlreadyVerified = errors.New("creator application social is already verified")

// ErrCreatorApplicationTelegramNotLinked — the creator has not opened the
// Telegram bot via /start, so we cannot manually verify yet (422). The
// admin should ask the creator to open the deep-link first.
var ErrCreatorApplicationTelegramNotLinked = errors.New("creator application has no telegram link")

// ErrCreatorApplicationNotRejectable — admin reject was attempted on an
// application whose current status is neither `verification` nor
// `moderation`. Includes the case of repeating reject on an already-
// rejected application. 422.
var ErrCreatorApplicationNotRejectable = errors.New("creator application is not in a rejectable status")

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
	VerificationCode  string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Categories        []string
	Socials           []CreatorApplicationDetailSocial
	Consents          []CreatorApplicationDetailConsent
	TelegramLink      *CreatorApplicationTelegramLink
	Rejection         *CreatorApplicationRejection
}

// CreatorApplicationRejection mirrors the wire-level CreatorApplicationRejection
// schema and is populated only for applications in the `rejected` status. The
// fields are derived from the latest creator_application_status_transitions row
// where to_status='rejected' (from_status / created_at / actor_id).
type CreatorApplicationRejection struct {
	FromStatus       string
	RejectedAt       time.Time
	RejectedByUserID string
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
// ID surfaces here (and through the API) because the manual-verify action
// targets a specific social row by uuid — admins must see which row their
// click will mutate.
type CreatorApplicationDetailSocial struct {
	ID               string
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
	CreatorApplicationStatusApproved,
	CreatorApplicationStatusRejected,
	CreatorApplicationStatusWithdrawn,
}

// IsValidCreatorApplicationStatus reports whether s is one of the canonical
// lifecycle statuses. The check is whitelist-based so a typo or a newly-added
// status (rolling deploy) is rejected, not silently accepted.
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

// verificationCodeParseRegex matches a single "UGC-NNNNNN" token anywhere in
// the haystack, case-insensitive. Word boundaries on each side reject longer
// digit runs like "UGC-1234567" — without them a 7-digit typo would extract
// a candidate matching some other application's code and trigger a wrong
// verification. The first match wins; ParseVerificationCode upper-cases it
// before returning.
var verificationCodeParseRegex = regexp.MustCompile(`(?i)\bUGC-[0-9]{6}\b`)

// ParseVerificationCode extracts the first UGC-NNNNNN token from a free-form
// text and returns it normalised to upper-case. The boolean is false when no
// token is present. Used by the SendPulse webhook to fish a code out of an
// Instagram DM body.
func ParseVerificationCode(text string) (string, bool) {
	match := verificationCodeParseRegex.FindString(text)
	if match == "" {
		return "", false
	}
	return strings.ToUpper(match), true
}

// NormalizeInstagramHandle is the canonical form persisted in
// creator_application_socials.handle for IG accounts: trim whitespace, drop
// every leading and trailing '@', lowercase the rest. Applied at submit
// time, in the SendPulse webhook ingestion path and in the backfill
// migration so the strict equality check between webhook payload and
// stored handle stays sound. Trim both sides to mirror the SQL backfill
// (`trim(BOTH '@' FROM handle)`) — divergent normalisation rules between
// the migration and the live code would silently corrupt the comparison.
func NormalizeInstagramHandle(h string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(h), "@"))
}

// creatorApplicationAllowedTransitions is the declarative state machine for
// creator applications. Only transitions explicitly mapped to true here are
// legal; everything else fails IsCreatorApplicationTransitionAllowed and the
// service helper returns ErrInvalidStatusTransition.
var creatorApplicationAllowedTransitions = map[string]map[string]bool{
	CreatorApplicationStatusVerification: {
		CreatorApplicationStatusModeration: true,
		CreatorApplicationStatusRejected:   true,
	},
	CreatorApplicationStatusModeration: {
		CreatorApplicationStatusApproved: true,
		CreatorApplicationStatusRejected: true,
	},
}

// IsCreatorApplicationTransitionAllowed reports whether moving an application
// from `from` to `to` is permitted by the state machine. Identity transitions
// (`from == to`) are not allowed.
func IsCreatorApplicationTransitionAllowed(from, to string) bool {
	allowed, ok := creatorApplicationAllowedTransitions[from]
	if !ok {
		return false
	}
	return allowed[to]
}

// ErrInvalidStatusTransition is the sentinel raised when the service helper
// is asked to transition an application along an edge that is not declared
// in creatorApplicationAllowedTransitions. Surfaces in stdout-логах
// приложения via wrapped errors, never in user-facing copy.
var ErrInvalidStatusTransition = errors.New("invalid creator application status transition")

// TransitionReason* values are persisted in
// creator_application_status_transitions.reason. The column itself is TEXT
// (free-form for forward compatibility) but Go callers must use one of these
// constants — readers / dashboards rely on the bounded vocabulary.
const (
	TransitionReasonInstagramAuto = "instagram_auto"
	TransitionReasonManualVerify  = "manual_verify"
	TransitionReasonReject        = "reject_admin"
	TransitionReasonApprove       = "approve_admin"
)

// VerifyInstagramStatus is the explicit outcome the service returns to the
// SendPulse webhook handler. The handler always responds 200 `{}` regardless,
// but distinct values give logs / metrics / unit tests a single source of
// truth for the no-op vs side-effecting branches.
type VerifyInstagramStatus string

const (
	// VerifyInstagramStatusVerified — full happy path executed: social row
	// updated to verified=true (with optional self-fix of `handle`),
	// application moved verification → moderation, audit + transition rows
	// committed, Telegram notification scheduled (or skipped on missing link).
	VerifyInstagramStatusVerified VerifyInstagramStatus = "verified"
	// VerifyInstagramStatusNoop — social row was already verified=true; we
	// do not touch state and skip Telegram. Acts as the idempotency branch
	// for repeat webhook deliveries.
	VerifyInstagramStatusNoop VerifyInstagramStatus = "noop"
	// VerifyInstagramStatusNotFound — no application in `verification`
	// status carries the parsed code. Does not commit anything.
	VerifyInstagramStatusNotFound VerifyInstagramStatus = "not_found"
	// VerifyInstagramStatusNoIGSocial — application matched, but it has no
	// Instagram social attached (only TikTok / Threads). Does not commit
	// anything.
	VerifyInstagramStatusNoIGSocial VerifyInstagramStatus = "no_ig_social"
)
