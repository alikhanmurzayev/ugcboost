package testutil

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
)

// iinSerialCounter backs UniqueIIN so parallel tests never collide on the
// partial unique index guarding active creator applications.
var iinSerialCounter uint64

// UniqueIIN returns a fresh, checksum-valid Kazakhstani IIN. The birthdate is
// fixed to 1995-05-15 (well over 18 years old on any reasonable test run) and
// only the 4-digit serial portion varies. Rare serials that end up with a
// checksum collision (both passes yield 10) are simply skipped.
func UniqueIIN() string {
	for {
		serialInt := atomic.AddUint64(&iinSerialCounter, 1) % 10000
		serial := fmt.Sprintf("%04d", serialInt)
		// YYMMDD = 950515, century byte 3 = male, 1900s.
		prefix := "950515" + "3" + serial
		if control, ok := iinControl(prefix); ok {
			return prefix + strconv.Itoa(control)
		}
	}
}

// iinControl computes the 12th (checksum) digit for an 11-digit IIN prefix
// using the two-pass Republic of Kazakhstan algorithm. ok is false when both
// passes yield modulus 10 — the caller should generate a different prefix.
func iinControl(first11 string) (int, bool) {
	digits := make([]int, 11)
	for i, r := range first11 {
		digits[i] = int(r - '0')
	}
	weights1 := [11]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	sum := 0
	for i := 0; i < 11; i++ {
		sum += digits[i] * weights1[i]
	}
	mod := sum % 11
	if mod == 10 {
		weights2 := [11]int{3, 4, 5, 6, 7, 8, 9, 10, 11, 1, 2}
		sum2 := 0
		for i := 0; i < 11; i++ {
			sum2 += digits[i] * weights2[i]
		}
		mod = sum2 % 11
		if mod == 10 {
			return 0, false
		}
	}
	return mod, true
}

// RegisterCreatorApplicationCleanup schedules a POST /test/cleanup-entity for
// the creator application after the test. Used by tests that learn the
// application id from a response and still want the cleanup stack to remove
// it (plus cascaded socials/categories/consents) before the next run.
func RegisterCreatorApplicationCleanup(t *testing.T, applicationID string) {
	t.Helper()
	RegisterCleanup(t, func(ctx context.Context) error {
		tc := NewTestClient(t)
		resp, err := tc.CleanupEntityWithResponse(ctx, testclient.CleanupEntityJSONRequestBody{
			Type: testclient.CreatorApplication,
			Id:   applicationID,
		})
		if err != nil {
			return fmt.Errorf("cleanup creator application %s: %w", applicationID, err)
		}
		if resp.StatusCode() != http.StatusNoContent && resp.StatusCode() != http.StatusNotFound {
			return fmt.Errorf("cleanup creator application %s: unexpected status %d", applicationID, resp.StatusCode())
		}
		return nil
	})
}
