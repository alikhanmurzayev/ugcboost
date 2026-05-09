package domain

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateContractTemplatePDF(t *testing.T) {
	t.Parallel()

	t.Run("empty pdf — required", func(t *testing.T) {
		t.Parallel()
		err := ValidateContractTemplatePDF(0, nil)
		var cve *ContractValidationError
		require.ErrorAs(t, err, &cve)
		require.Equal(t, CodeContractRequired, cve.Code)
		require.Empty(t, cve.Missing)
		require.Empty(t, cve.Unknown)
	})

	t.Run("missing all placeholders", func(t *testing.T) {
		t.Parallel()
		err := ValidateContractTemplatePDF(1024, nil)
		var cve *ContractValidationError
		require.ErrorAs(t, err, &cve)
		require.Equal(t, CodeContractMissingPlaceholder, cve.Code)
		require.Equal(t, KnownContractPlaceholders, cve.Missing)
	})

	t.Run("missing one placeholder", func(t *testing.T) {
		t.Parallel()
		err := ValidateContractTemplatePDF(1024, []string{"CreatorFIO", "CreatorIIN"})
		var cve *ContractValidationError
		require.ErrorAs(t, err, &cve)
		require.Equal(t, CodeContractMissingPlaceholder, cve.Code)
		require.Equal(t, []string{"IssuedDate"}, cve.Missing)
	})

	t.Run("unknown placeholder among knowns", func(t *testing.T) {
		t.Parallel()
		err := ValidateContractTemplatePDF(1024, []string{"CreatorFIO", "CreatorIIN", "IssuedDate", "CreatorEmail"})
		var cve *ContractValidationError
		require.ErrorAs(t, err, &cve)
		require.Equal(t, CodeContractUnknownPlaceholder, cve.Code)
		require.Equal(t, []string{"CreatorEmail"}, cve.Unknown)
	})

	t.Run("multiple unknown placeholders dedup", func(t *testing.T) {
		t.Parallel()
		err := ValidateContractTemplatePDF(1024, []string{
			"CreatorFIO", "CreatorIIN", "IssuedDate",
			"CreatorEmail", "BrandName", "CreatorEmail",
		})
		var cve *ContractValidationError
		require.ErrorAs(t, err, &cve)
		require.Equal(t, CodeContractUnknownPlaceholder, cve.Code)
		require.ElementsMatch(t, []string{"CreatorEmail", "BrandName"}, cve.Unknown)
	})

	t.Run("repeated knowns across pages — accepted", func(t *testing.T) {
		t.Parallel()
		err := ValidateContractTemplatePDF(1024, []string{
			"CreatorFIO", "CreatorFIO", "CreatorIIN", "IssuedDate", "IssuedDate",
		})
		require.NoError(t, err)
	})

	t.Run("missing wins over unknown", func(t *testing.T) {
		t.Parallel()
		err := ValidateContractTemplatePDF(1024, []string{"CreatorFIO", "FooBar"})
		var cve *ContractValidationError
		require.ErrorAs(t, err, &cve)
		require.Equal(t, CodeContractMissingPlaceholder, cve.Code)
	})

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		err := ValidateContractTemplatePDF(2048, []string{"CreatorFIO", "CreatorIIN", "IssuedDate"})
		require.NoError(t, err)
	})
}

func TestErrContractTemplateNotFound_isSentinel(t *testing.T) {
	t.Parallel()
	require.True(t, errors.Is(ErrContractTemplateNotFound, ErrContractTemplateNotFound))
}
