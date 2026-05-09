package domain

import (
	"errors"
	"fmt"
	"strings"
)

// KnownContractPlaceholders is the canonical list of placeholder names the
// contract-template validator and the chunk-16 outbox renderer both rely on.
// Must stay aligned with the strings the production extractor recognises and
// the values `domain.ContractData.Get` knows how to substitute. Order matches
// the slot order in Аидана's reference PDF.
var KnownContractPlaceholders = []string{
	"CreatorFIO",
	"CreatorIIN",
	"IssuedDate",
}

// ContractValidationError carries an admin-facing failure for the
// PUT /campaigns/{id}/contract-template flow with optional structured details.
// `Missing` is set only for CONTRACT_MISSING_PLACEHOLDER; `Unknown` is set
// only for CONTRACT_UNKNOWN_PLACEHOLDER. The handler maps the struct onto
// `ContractValidationErrorResponse` from the OpenAPI contract.
type ContractValidationError struct {
	Code    string
	Message string
	Missing []string
	Unknown []string
}

// Error renders the validation error as `code: message`, mirroring
// *ValidationError so logs are consistent.
func (e *ContractValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewContractRequiredError wraps the empty-body upload (Content-Length: 0
// or zero-length stream).
func NewContractRequiredError() *ContractValidationError {
	return &ContractValidationError{
		Code:    CodeContractRequired,
		Message: "Загрузите PDF-шаблон договора. Файл не должен быть пустым.",
	}
}

// NewContractInvalidPDFError wraps the case where the bytes do not parse as a
// PDF (encrypted PDF, non-PDF MIME, password-protected, truncated…).
func NewContractInvalidPDFError() *ContractValidationError {
	return &ContractValidationError{
		Code:    CodeContractInvalidPDF,
		Message: "Файл не распознаётся как PDF. Проверьте, что вы экспортировали документ из Google Docs через File → Download → PDF.",
	}
}

// NewContractMissingPlaceholderError carries the known placeholders absent
// from the uploaded PDF. The Missing slice is preserved for the structured
// `details.missing` field on the wire.
func NewContractMissingPlaceholderError(missing []string) *ContractValidationError {
	return &ContractValidationError{
		Code: CodeContractMissingPlaceholder,
		Message: fmt.Sprintf(
			"В шаблоне не найдены обязательные плейсхолдеры: %s. Проверьте, что они написаны в формате {{Name}} (CamelCase, без подчёркиваний) и каждый — на отдельной строке.",
			joinPlaceholderNames(missing),
		),
		Missing: missing,
	}
}

// NewContractUnknownPlaceholderError carries placeholders found in the PDF
// that are not part of KnownContractPlaceholders.
func NewContractUnknownPlaceholderError(unknown []string) *ContractValidationError {
	return &ContractValidationError{
		Code: CodeContractUnknownPlaceholder,
		Message: fmt.Sprintf(
			"В шаблоне найдены незнакомые плейсхолдеры: %s. Известные плейсхолдеры: %s.",
			joinPlaceholderNames(unknown),
			joinPlaceholderNames(KnownContractPlaceholders),
		),
		Unknown: unknown,
	}
}

// ErrContractTemplateNotFound is the sentinel raised by
// CampaignService.GetContractTemplate on a live campaign whose
// contract_template_pdf column is empty. respondError translates it into 404
// CONTRACT_TEMPLATE_NOT_FOUND.
var ErrContractTemplateNotFound = errors.New("contract template not found")

// ValidateContractTemplatePDF runs the pure-domain checks on a candidate
// upload: empty body, missing known placeholders, unknown placeholders. PDF
// parsing failure is detected upstream by the Extractor and surfaces as
// NewContractInvalidPDFError independently of this function.
//
// placeholderNames is the raw list of names extracted from the PDF (with
// duplicates allowed — the same placeholder repeated across pages is
// expected). Returns *ContractValidationError on the first failure or nil on
// success.
func ValidateContractTemplatePDF(pdfLen int, placeholderNames []string) error {
	if pdfLen == 0 {
		return NewContractRequiredError()
	}

	found := make(map[string]struct{}, len(placeholderNames))
	for _, n := range placeholderNames {
		found[n] = struct{}{}
	}
	knownSet := make(map[string]struct{}, len(KnownContractPlaceholders))
	for _, k := range KnownContractPlaceholders {
		knownSet[k] = struct{}{}
	}

	var missing []string
	for _, k := range KnownContractPlaceholders {
		if _, ok := found[k]; !ok {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return NewContractMissingPlaceholderError(missing)
	}

	var unknown []string
	seen := make(map[string]struct{}, len(placeholderNames))
	for _, n := range placeholderNames {
		if _, ok := knownSet[n]; ok {
			continue
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		unknown = append(unknown, n)
	}
	if len(unknown) > 0 {
		return NewContractUnknownPlaceholderError(unknown)
	}

	return nil
}

// joinPlaceholderNames renders a list of names as `{{Name1}}, {{Name2}}` for
// user-facing error messages.
func joinPlaceholderNames(names []string) string {
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = "{{" + n + "}}"
	}
	return strings.Join(parts, ", ")
}
