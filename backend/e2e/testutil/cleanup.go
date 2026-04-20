package testutil

import (
	"context"
	"os"
	"testing"
	"time"
)

// cleanupTimeout bounds any single cleanup call so a hung request can't stall
// the whole test run on shutdown.
const cleanupTimeout = 10 * time.Second

// RegisterCleanup schedules fn to run after the test finishes. Honors
// E2E_CLEANUP=false by skipping the call — that mode leaves test data in the
// DB so a developer can inspect state after a failure.
//
// Cleanups are registered via t.Cleanup, which runs them in LIFO order.
// Composable setup helpers register their cleanups in the same order they
// create rows, so LIFO means children are removed before parents — which
// respects foreign key constraints without the caller having to think about
// ordering.
//
// A cleanup failure is logged (t.Logf) instead of failing the test: one bad
// cleanup must not mask other cleanups from running, nor flip a passing test
// red because of residual state.
func RegisterCleanup(t *testing.T, fn func(context.Context) error) {
	t.Helper()
	t.Cleanup(func() {
		if os.Getenv(EnvCleanup) == "false" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		if err := fn(ctx); err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	})
}
