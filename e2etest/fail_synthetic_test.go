package e2etest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Synthetic test — must fail. Delete after verifying CI reports.
func TestSyntheticFail_DeleteMeAfterCICheck(t *testing.T) {
	assert.Equal(t, "expected", "actual", "this synthetic test is supposed to fail")
}
