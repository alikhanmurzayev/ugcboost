package domain

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	campaignNameMaxLen   = 255
	campaignTmaURLMaxLen = 2048

	// secretTokenPattern enforces the format of the TMA-side secret token: 16
	// to 256 URL-safe chars (letters/digits/underscore/hyphen). Defence in
	// depth — both the ADMIN POST/PATCH and the TMA path-param are checked
	// against this regex before any DB lookup, so neither a one-character
	// nor a megabyte-sized token can reach `WHERE secret_token = $1`. 256 is
	// far above any realistic UUID / base64 token (22-32 chars typical) but
	// keeps a hard cap on attacker-controlled path-param length.
	secretTokenPattern = `^[A-Za-z0-9_-]{16,256}$`
)

// secretTokenRe is the compiled regex matching the secret_token format.
// Exported so middleware/handler/repo packages can early-reject malformed
// tokens without re-compiling the same pattern.
var secretTokenRe = regexp.MustCompile(secretTokenPattern)

// SecretTokenRegex returns the compiled regex matching the secret_token
// format. Used by handler-side early-reject to keep the regex in one place.
func SecretTokenRegex() *regexp.Regexp { return secretTokenRe }

// Campaign is the domain projection of a marketing campaign. JSON tags are
// snake_case because the struct is serialized into audit_logs.new_value as-is
// — adding a field to the struct extends the audit payload automatically.
type Campaign struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	TmaURL    string    `json:"tma_url"`
	IsDeleted bool      `json:"is_deleted"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CampaignInput is the mutable subset of a campaign for create/update services.
type CampaignInput struct {
	Name   string
	TmaURL string
}

// ErrCampaignNameTaken is the sentinel raised by CampaignRepo.Create when a
// 23505 fires on the partial unique index campaigns_name_active_unique —
// another non-deleted campaign already uses this name. respondError maps any
// *BusinessError to 409 + Code + Message via errors.As.
var ErrCampaignNameTaken = NewBusinessError(
	CodeCampaignNameTaken,
	"Кампания с таким названием уже есть. Выберите другое название или удалите старую кампанию.",
)

// ErrCampaignNotFound is raised by CampaignService.GetByID when the lookup
// hits sql.ErrNoRows. respondError maps it to 404 CAMPAIGN_NOT_FOUND.
var ErrCampaignNotFound = errors.New("campaign not found")

// ErrCampaignTmaURLLocked is raised by UpdateCampaign when the request flips
// tma_url while at least one creator in this campaign has already been
// invited (invited_count > 0). The previous URL is embedded in the inline
// web_app button of bot messages already delivered to creators; flipping it
// would silently break those links.
var ErrCampaignTmaURLLocked = NewValidationError(
	CodeCampaignTmaURLLocked,
	"Нельзя изменить ссылку на ТЗ — приглашения по текущей ссылке уже отправлены креаторам.",
)

// ErrCampaignIdsTooMany is raised by ApproveCreatorApplication when the
// optional `campaignIds` payload exceeds the per-call cap.
var ErrCampaignIdsTooMany = NewValidationError(
	CodeCampaignIdsTooMany,
	"Слишком много кампаний. За один approve можно добавить креатора не более чем в 20 кампаний.",
)

// ErrCampaignIdsDuplicates is raised by ApproveCreatorApplication when the
// `campaignIds` payload contains duplicate UUIDs.
var ErrCampaignIdsDuplicates = NewValidationError(
	CodeCampaignIdsDuplicates,
	"В списке кампаний есть дубликаты. Уберите повторы и повторите.",
)

// ErrCampaignNotAvailableForAdd is raised by CampaignService.AssertActiveCampaigns
// when at least one of the requested campaigns is missing or soft-deleted.
// Single code for both cases — admin's expected reaction is the same: refresh
// the campaign list and retry.
var ErrCampaignNotAvailableForAdd = NewValidationError(
	CodeCampaignNotAvailableForAdd,
	"Одна или несколько выбранных кампаний недоступны. Обновите список и попробуйте снова.",
)

// ErrInvalidTmaURL is raised by ValidateCampaignTmaURL when the supplied
// tma_url is not a valid absolute URL OR its last path segment fails the
// secret_token format check (`^[A-Za-z0-9_-]{16,}$`). Empty input is
// allowed (legacy / draft campaign). Mapped to 422 INVALID_TMA_URL.
var ErrInvalidTmaURL = NewValidationError(
	CodeInvalidTmaURL,
	"Последний сегмент ссылки на TMA должен быть длиной от 16 символов и содержать только латинские буквы, цифры, '_' и '-'. Проверьте URL.",
)

// ErrTmaURLConflict is raised by CampaignRepo.Create / Update when a 23505
// fires on `campaigns_secret_token_uniq` — another live campaign already
// uses this secret_token. Mapped to 422 TMA_URL_CONFLICT.
var ErrTmaURLConflict = NewValidationError(
	CodeTmaURLConflict,
	"Эта ссылка на TMA уже используется в другой кампании. Сгенерируйте новую ссылку.",
)

// NewErrCampaignAddAfterApproveFailed is constructed by ApproveApplication
// when the post-tx1 add-loop fails on a specific campaign. The text spells
// out that the creator is already created so the admin does not retry the
// approve and includes both the new creator id (so the admin can find them
// in /creators without searching by IIN) and the campaign display label
// (name when the post-fail lookup succeeds, UUID fallback when it fails).
func NewErrCampaignAddAfterApproveFailed(creatorID, campaignDisplay string) *ValidationError {
	return NewValidationError(
		CodeCampaignAddAfterApproveFailed,
		"Не удалось добавить креатора (id "+creatorID+") в кампанию «"+campaignDisplay+
			"». Креатор уже создан — найдите его в разделе «Креаторы» по id и добавьте в кампанию вручную.",
	)
}

// ValidateCampaignName enforces the trim + non-empty + ≤255 contract on the
// campaign display name. Returns the trimmed value so callers don't have to
// re-trim before passing the name downstream — single source of truth for
// normalization plus error.
func ValidateCampaignName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", NewValidationError(
			CodeCampaignNameRequired,
			"Название кампании обязательно. Укажите хотя бы один непробельный символ.",
		)
	}
	if utf8.RuneCountInString(name) > campaignNameMaxLen {
		return "", NewValidationError(
			CodeCampaignNameTooLong,
			"Название кампании слишком длинное. Сократите до 255 символов.",
		)
	}
	return name, nil
}

// ValidateCampaignTmaURL trims the supplied tma_url and enforces the
// secret_token contract for the live-campaign TMA flow. Empty input is
// allowed — legacy / draft campaigns stay reachable in admin without a TMA
// link and surface as `secret_token = NULL` in the DB. Non-empty input must
// parse as an absolute URL (scheme + host) and its last path segment must
// match `^[A-Za-z0-9_-]{16,}$`. The secret_token integrity check lives here
// so a one-character token can never reach the DB and a partial UNIQUE
// index lookup never sees a malformed value.
//
// Security note: the value is admin-controlled but downstream code embedding
// it into outbound Telegram messages MUST escape it for the target surface
// (Markdown / HTML); the format check does not substitute for that escape.
func ValidateCampaignTmaURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if utf8.RuneCountInString(raw) > campaignTmaURLMaxLen {
		return "", NewValidationError(
			CodeCampaignTmaURLTooLong,
			"Ссылка на TMA-страницу слишком длинная. Сократите URL до 2048 символов.",
		)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", ErrInvalidTmaURL
	}
	if !secretTokenRe.MatchString(lastSegment(u.Path)) {
		return "", ErrInvalidTmaURL
	}
	return raw, nil
}

// ExtractSecretToken returns the last path segment of the supplied tma_url
// or the empty string when the input is empty / unparseable. Callers pair
// this with ValidateCampaignTmaURL — the validator gates format, this
// helper extracts the raw token for the DB column.
func ExtractSecretToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return lastSegment(u.Path)
}

func lastSegment(p string) string {
	p = strings.TrimRight(p, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

const (
	CampaignSortCreatedAt = "created_at"
	CampaignSortUpdatedAt = "updated_at"
	CampaignSortName      = "name"
)

var CampaignListSortFieldValues = []string{
	CampaignSortCreatedAt,
	CampaignSortUpdatedAt,
	CampaignSortName,
}

const (
	CampaignListPageMin      = 1
	CampaignListPageMax      = 100_000
	CampaignListPerPageMin   = 1
	CampaignListPerPageMax   = 200
	CampaignListSearchMaxLen = 128
)

type CampaignListInput struct {
	Search    string
	IsDeleted *bool
	Sort      string
	Order     string
	Page      int
	PerPage   int
}

type CampaignListPage struct {
	Items   []*Campaign
	Total   int64
	Page    int
	PerPage int
}
