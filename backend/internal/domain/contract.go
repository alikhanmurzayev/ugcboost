package domain

import (
	"errors"
	"fmt"
	"strings"
)

var KnownContractPlaceholders = []string{
	"CreatorFIO",
	"CreatorIIN",
	"IssuedDate",
}

type ContractValidationError struct {
	Code    string
	Message string
	Missing []string
	Unknown []string
}

func (e *ContractValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func NewContractRequiredError() *ContractValidationError {
	return &ContractValidationError{
		Code:    CodeContractRequired,
		Message: "Загрузите PDF-шаблон договора. Файл не должен быть пустым.",
	}
}

func NewContractInvalidPDFError() *ContractValidationError {
	return &ContractValidationError{
		Code:    CodeContractInvalidPDF,
		Message: "Файл не распознаётся как PDF.",
	}
}

func NewContractMissingPlaceholderError(missing []string) *ContractValidationError {
	return &ContractValidationError{
		Code: CodeContractMissingPlaceholder,
		Message: fmt.Sprintf(
			"В шаблоне не найдены обязательные плейсхолдеры: %s. Проверьте, что они написаны в формате {{Name}} (например, {{CreatorFIO}}).",
			joinPlaceholderNames(missing),
		),
		Missing: missing,
	}
}

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

// NewContractTemplateRequiredForNotifyError — раздача приглашений требует
// загруженного шаблона договора (chunk 16 будет рендерить и слать его).
func NewContractTemplateRequiredForNotifyError() *ValidationError {
	return NewValidationError(
		CodeContractTemplateRequired,
		"Загрузите PDF-шаблон договора в кампанию, прежде чем приглашать креаторов.",
	)
}

var ErrContractTemplateNotFound = errors.New("contract template not found")

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

func joinPlaceholderNames(names []string) string {
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = "{{" + n + "}}"
	}
	return strings.Join(parts, ", ")
}
