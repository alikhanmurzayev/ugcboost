---
title: 'Deferred work — surfaced incidentally during reviews'
last_updated: '2026-05-01'
---

## From PR `gh-22-strict-server-mode` review (2026-05-01)

### Pre-existing edges, not caused by strict-server migration

- **`middleware/request_meta.go:24` — UA truncation by bytes can split a UTF-8 sequence.** `ua[:MaxUserAgentLength]` cuts on a byte boundary; mid-rune cuts produce invalid UTF-8 in DB rows. Replace with rune-aware truncation.

### Standard / docs additions

- **`Set-Cookie` description in `openapi.yaml`.** Headers declared as `type: string` without describing required attributes (`HttpOnly`, `SameSite=Strict`, `Secure` in production). Add `description:` to make the contract self-documenting for clients.
