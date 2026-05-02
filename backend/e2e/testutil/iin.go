package testutil

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
)

// UniqueIIN returns a fresh, checksum-valid Kazakhstani IIN. Birth year (1985..2005),
// month, day (1..28 to dodge calendar edge cases), and 4-digit serial are
// drawn from crypto/rand. The pool of valid IINs is on the order of 70M, so
// concurrent test runs across multiple go test processes (each with its own
// memory) are extremely unlikely to collide on the partial unique index that
// guards active creator applications. Atomic counters were rejected because
// they reset to zero in every fresh test process and produce identical IINs
// across parallel package runs.
//
// Rare prefixes whose checksum hits the "both passes yield 10" corner are
// simply re-rolled.
func UniqueIIN() string {
	for {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			panic(fmt.Errorf("crypto/rand failed: %w", err))
		}
		year := 1985 + int(b[0])%21
		month := 1 + int(b[1])%12
		day := 1 + int(b[2])%28
		serial := int(binary.BigEndian.Uint32(b[3:7])) % 10000
		// IIN encodes year as YY (last two digits); the century byte (3 here)
		// disambiguates 1900s vs 2000s, so a 1985 / 2005 / 1995 etc. pair is
		// distinguishable by the seventh digit, not the YY prefix alone.
		century := 3
		if year >= 2000 {
			century = 4
		}
		yy := year % 100
		prefix := fmt.Sprintf("%02d%02d%02d%d%04d", yy, month, day, century, serial)
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
