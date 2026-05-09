---
title: 'Deferred Work — TMA decision flow + secret_token integrity'
created: '2026-05-09'
source_spec: 'spec-creator-campaign-decision.md'
---

# Deferred Work

Surfaced during step-04-review (3 reviewers: blind, edge case, acceptance).
Items below are real but did not block ship — either pre-existing, low
severity, or require structural changes outside this story's scope.

## Backend

- **Soft-delete race after authz** (edge case hunter, medium)
  `AuthzService.AuthorizeTMACampaignDecision` returns a live campaign row,
  but `service.ApplyDecision` opens its own tx and only locks the
  `campaign_creators` row. Between the two, an admin DELETE on the
  campaign flips `is_deleted=true` — the decision still commits + audits
  against a soft-deleted campaign. Fix requires extending
  `TmaCampaignCreatorRepoFactory` with a `CampaignRepo` and re-checking
  `IsDeleted` inside the WithTx after `GetByIDForUpdate`. Race window is
  narrow (admin DELETE during creator's tap), and the audit row is
  recoverable via undelete + manual cleanup.

- **`sql.ErrNoRows` from `GetByIDForUpdate` → 500 instead of granular
  404** (edge case hunter, low)
  If admin removes the campaign_creator row between authz lookup and
  `GetByIDForUpdate`, the wrapped error falls through `respondError`
  without a `errors.Is(sql.ErrNoRows)` mapping for this layer → generic
  500. Spec calls for 404 `CAMPAIGN_CREATOR_NOT_FOUND`. Same race-window
  caveat as above.

- **`authDate.After(now)` zero-skew tolerance** (edge case hunter, low)
  No grace for clock-skew on the WebApp client. A device with +1s drift
  gets 401. Telegram-mediated initData is usually clock-correct, but a
  small skew window (e.g., 60s) would harden against false 401s.

- **`repository.Update` always rewrites `secret_token`** (blind hunter, low)
  Even on a no-op PATCH where `tma_url` is unchanged, the SET clause
  overwrites `secret_token` and bumps `updated_at`. Generates redundant
  `campaign_update` audit rows. Fix: skip `secret_token` from SetMap when
  unchanged (requires comparing old + new at service level).

- **`ValidateCampaignTmaURL` does not enforce a scheme/host whitelist**
  (blind hunter, medium)
  Admin can register `tma_url = "https://attacker.com/path/<valid-token>"`
  and `Notify` will embed that URL in Telegram WebApp buttons. Admin
  trust is the current mitigation; if admin compromise becomes a concern,
  add an allow-list (`tma.ugcboost.kz`, `t.me`).

- **`SignTMAInitData` test endpoint without `bot_token` runtime guard**
  (blind hunter, medium)
  Production deployment guards via `EnableTestEndpoints=false` flag —
  `cmd/api/main.go` blocks registration in production. If the flag ever
  leaks, the handler accepts empty `bot_token` and produces predictable
  signatures. Defense-in-depth: panic if `cfg.IsProduction()` even when
  the endpoint is reachable.

- **Long-tail soft-delete + un-soft-delete + partial UNIQUE race**
  (edge case hunter, medium)
  Hypothetical: if a future feature undoes `is_deleted=true`, flipping
  `is_deleted=false` could trip `campaigns_secret_token_uniq` 23505 if
  another live campaign now uses the same token. No un-soft-delete
  feature exists today.

## E2E coverage

- **`secret_token IS NULL` direct DB check for empty `tma_url`**
  (acceptance auditor, low)
  e2e module has no DB connection helper — verifying nullability requires
  adding a test endpoint or `pgx.Connect` in testutil. Currently covered
  by repo unit test (`TestCreate_emptyToken`) + backend self-check #12
  via psql.

- **Soft-deleted campaign explicit e2e** (acceptance auditor, low)
  Spec §I/O Matrix lists "campaign soft-deleted → 404" as a separate
  branch; current e2e covers the structurally equivalent "unknown
  secret_token → 404" path. Repo's `GetBySecretToken` filters
  `is_deleted=false` so behavior is identical.

- **e2e cleanup LIFO ordering** (edge case hunter, low)
  `SetupCampaignWithInvitedCreator` registers cleanups in
  campaign → creator → campaign_creator order. LIFO drains
  campaign_creator → creator → campaign — FK chain unwinds correctly
  *today*, but adding any cleanup inside `SetupApprovedCreator` could
  invert the order. Pin the contract with a short test.

## Frontend

- **`agree.data ?? decline.data` precedence** (blind hunter, low)
  If a creator successfully agrees and somehow triggers a decline
  mutation in the same mount (currently impossible — disabled buttons +
  AcceptedView mounted), the `??` keeps `agree.data` and silently
  ignores `decline.data`. Mutually-exclusive UI flows make this
  unreachable in practice; flagged for future refactor awareness.

- **`getCampaignByToken` always returns** (blind hunter, low)
  Switched from `CampaignBrief | undefined` to `genericBrief()` fallback.
  Unknown / malformed tokens now render the brief page instead of
  NotFoundPage. Anti-fingerprint is preserved at the backend (single 403
  body for "not registered" vs "not invited"); the frontend leak is
  minimal (page renders before submit fails). Documented as a UX choice.

- **No status refetch on TMA reload** (edge case hunter, low)
  After a creator agrees, reloading the TMA page rerenders the brief
  with both buttons enabled — only the click → 200 + `alreadyDecided=true`
  reveals the decided state. A backend `GET /tma/campaigns/{token}` for
  the current row state would close this gap, but spec did not include
  it (and the current UX hits the no-op idempotent path on click, so
  it's safe).

## Cosmetic / rejected

- **`useDecision.ts` lives in `features/campaign/` not `features/campaign/hooks/`**
  (acceptance auditor) — cosmetic. Spec referenced a `hooks/` subdirectory
  that does not exist in the project; no other hooks file uses that path.
  Rejected.
