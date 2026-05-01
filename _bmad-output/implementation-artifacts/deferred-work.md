---
title: 'Deferred work — surfaced incidentally during reviews'
last_updated: '2026-05-01'
---

## From PR `gh-22-strict-server-mode` review (2026-05-01)

### Pre-existing edges, not caused by strict-server migration

- **`audit.go:73` — `int(total)` overflow on 32-bit platforms.** `auditService.List` returns `int64`; current handler casts to `int`. On a 32-bit ARM/RPi build, audit_logs > 2.1B would wrap to negative `Total`. Either extend `ListAuditLogsData.Total` to `int64` in OpenAPI, or clamp at `math.MaxInt32`. Pre-existing, ported as-is.
- **`creator_application.go:34-36` — UA truncation by bytes can split a UTF-8 sequence.** `ua[:maxUserAgentLength]` cuts on a byte boundary; mid-rune cuts produce invalid UTF-8 in DB rows. Replace with rune-aware truncation. Pre-existing, ported as-is.
- **`auth.go:Logout` — clear-cookie not emitted when `userID == ""`.** When the access-token middleware leaves no `userID` in ctx, Logout returns 401 and does **not** emit the clear cookie. A real-world client whose access token expired between login and logout still has its refresh cookie alive. Pre-existing UX/security trade-off; revisit when reworking session expiry.
- **`auth.go:Logout` — returns 200 even if `authService.Logout` fails to revoke refresh tokens.** Cookie is cleared client-side but server-side refresh tokens may stay valid until natural expiry. Pre-existing; product call.

### Hardenings worth a follow-up PR

- **Narrow `RefreshCookieFromContext` injection to the `/auth/refresh` route group.** Currently `RequestMeta` middleware sets the raw refresh-token in request `context.Context` for **every** request — any downstream code that dumps `ctx` (logging, recovery, debug middleware) leaks the secret. Mount a separate small middleware on the refresh route only, leave `RequestMeta` to UA-only. Standards-auditor + blind-hunter both flagged this.
- **Length cap for User-Agent inside `RequestMeta` middleware.** Today `RequestMeta` stores `r.UserAgent()` raw; the standard 1 MB header bound from `http.Server` is the only ceiling. Apply a defensive cap (e.g. 4096 bytes) inside the middleware so any future UA consumer that forgets to truncate cannot blow up logs/DB.

### Standard / docs additions

- **Document strict-server `Set-Cookie` single-value constraint in `docs/standards/backend-codegen.md`.** Generated `Visit*Response` writes via `w.Header().Set("Set-Cookie", value)` — replaces any earlier value. If a future endpoint needs two cookies on one response (CSRF + refresh), the second `Set` will silently overwrite. Add a `## Что ревьюить` bullet covering this constraint and its workarounds.
- **`Set-Cookie` description in `openapi.yaml`.** Headers declared as `type: string` without describing required attributes (`HttpOnly`, `SameSite=Strict`, `Secure` in production). Add `description:` to make the contract self-documenting for clients.
