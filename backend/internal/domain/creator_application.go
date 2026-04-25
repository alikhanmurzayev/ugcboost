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
)

// SocialPlatformValues is the canonical list of accepted platforms.
// Used by services/handlers when iterating and by tests for coverage.
var SocialPlatformValues = []string{SocialPlatformInstagram, SocialPlatformTikTok}

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
	// 422 — at least one of the four mandatory consents is missing.
	CodeMissingConsent = "MISSING_CONSENT"
)

// ErrCreatorApplicationDuplicate is the sentinel the repository raises when
// the partial unique index on creator_applications(iin) WHERE status IN
// ('pending','approved','blocked') rejects an INSERT (concurrent submit lost
// the race after the service's HasActiveByIIN check). The service converts it
// into a business error with CodeCreatorApplicationDuplicate.
var ErrCreatorApplicationDuplicate = errors.New("creator application with this iin is already active")

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
	LastName         string
	FirstName        string
	MiddleName       *string
	IIN              string
	Phone            string
	City             string
	Address          string
	CategoryCodes    []string
	Socials          []SocialAccountInput
	Consents         ConsentsInput
	IPAddress        string
	UserAgent        string
	AgreementVersion string
	PrivacyVersion   string
	Now              time.Time
}

// SocialAccountInput is one validated handle on a known platform.
type SocialAccountInput struct {
	Platform string
	Handle   string
}

// ConsentsInput captures the four mandatory acceptance flags. Service layer
// rejects the request if any field is false.
type ConsentsInput struct {
	Processing  bool
	ThirdParty  bool
	CrossBorder bool
	Terms       bool
}

// AsMap returns the consents keyed by their canonical type string in the
// order defined by ConsentTypeValues. Services iterate this map to write
// one consent row per type.
func (c ConsentsInput) AsMap() map[string]bool {
	return map[string]bool{
		ConsentTypeProcessing:  c.Processing,
		ConsentTypeThirdParty:  c.ThirdParty,
		ConsentTypeCrossBorder: c.CrossBorder,
		ConsentTypeTerms:       c.Terms,
	}
}

// CreatorApplicationSubmission is what the service returns to the handler.
type CreatorApplicationSubmission struct {
	ApplicationID string
	BirthDate     time.Time
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
