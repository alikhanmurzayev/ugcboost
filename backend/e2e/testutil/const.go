package testutil

// Package-local constants used across the e2e test helpers. The e2e module
// is intentionally isolated from the backend internals — it can't reuse
// constants from `internal/*`, so the names that repeat in tests live here.
const (
	// DefaultPassword is the canonical password used for every seeded test
	// user. It satisfies the 6-char minimum enforced by the backend.
	DefaultPassword = "testpass123"

	// RefreshCookieName is the cookie the backend sets for the refresh token
	// after a successful login. Tests inspect it when verifying auth flows.
	RefreshCookieName = "refresh_token"

	// EnvCleanup is the env var that controls whether per-test cleanups run
	// (default true). Set to "false" to keep test data for debugging.
	EnvCleanup = "E2E_CLEANUP"

	// HeaderCFConnectingIP is the Cloudflare-set header that carries the real
	// client IP through the proxy chain. Tests send it via PostRaw to verify
	// the backend's RealIP middleware picks it up over X-Forwarded-For.
	HeaderCFConnectingIP = "CF-Connecting-IP"
)
