package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	campaignNameMaxLen   = 255
	campaignTmaURLMaxLen = 2048
)

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

// ErrCampaignNameTaken is the sentinel raised by CampaignRepo.Create when a
// 23505 fires on the partial unique index campaigns_name_active_unique —
// another non-deleted campaign already uses this name. respondError maps any
// *BusinessError to 409 + Code + Message via errors.As.
var ErrCampaignNameTaken = NewBusinessError(
	CodeCampaignNameTaken,
	"Кампания с таким названием уже есть. Выберите другое название или удалите старую кампанию.",
)

// ValidateCampaignName enforces the trim + non-empty + ≤255 contract on the
// campaign display name. Trim is performed inside so the validator stays the
// single source of truth — passing a whitespace-only string still fails.
func ValidateCampaignName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return NewValidationError(
			CodeCampaignNameRequired,
			"Название кампании обязательно. Укажите хотя бы один непробельный символ.",
		)
	}
	if utf8.RuneCountInString(name) > campaignNameMaxLen {
		return NewValidationError(
			CodeCampaignNameTooLong,
			"Название кампании слишком длинное. Сократите до 255 символов.",
		)
	}
	return nil
}

// ValidateCampaignTmaURL enforces the trim + non-empty + ≤2048 contract on
// the TMA-side ТЗ landing URL. Format-wise we only require non-empty — host
// differs between local / staging / production and the value lives only to
// be embedded into outbound creator-invite messages.
func ValidateCampaignTmaURL(url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return NewValidationError(
			CodeCampaignTmaURLRequired,
			"Ссылка на TMA-страницу обязательна. Укажите URL без пробелов.",
		)
	}
	if utf8.RuneCountInString(url) > campaignTmaURLMaxLen {
		return NewValidationError(
			CodeCampaignTmaURLTooLong,
			"Ссылка на TMA-страницу слишком длинная. Сократите URL до 2048 символов.",
		)
	}
	return nil
}
