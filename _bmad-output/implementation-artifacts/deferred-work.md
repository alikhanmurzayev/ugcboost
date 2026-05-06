# Deferred Work

Findings, surfaced by step-04 reviews, that are valid but not caused by the current story. Pre-existing patterns or cross-cutting candidates for future focused attention.

## Captured 2026-05-06 — spec-4-campaign-detail-backend

- **NewServer positional nil-args in handler tests** — every handler test that exercises only a subset of services calls `NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, ...)`. Adding a new dependency requires updating every site silently — runtime-panic risk. Candidate: functional options or a builder. Surface across `backend/internal/handler/*_test.go`.
- **Per-call repo allocation in services** — every service method does `s.repoFactory.NewXRepo(s.pool).Method(...)`, allocating a fresh repo struct on each call. Acceptable today (factory is stateless), but the idiom locks future repos out of holding any cached state. Cross-service pattern.
- **`respondError` switch ordering invariant** — specific not-found cases (`ErrCreatorNotFound`, `ErrCampaignNotFound`) must precede the generic `ErrNotFound`/`sql.ErrNoRows` arm or they get swallowed silently. Today correct; one careless append breaks it. Candidate: table-driven mapping or explicit comment / lint rule. Cross-cutting.
- **400 from path-param validator not in OpenAPI** — `chi`'s `HandleParamError` returns 400 with `error.Code=VALIDATION_ERROR` for invalid path params (e.g. `not-a-uuid`), but no operation in `openapi.yaml` declares 400 — TS clients fall back to `default: UnexpectedError`. Existing project-wide pattern (POST /campaigns shares it). Candidate: declare 400 on every path-param-bearing operation, or wrap validator errors through `RequestErrorHandlerFunc` to a 422.
- **Flake risk in e2e `WithinDuration(time.Minute)` under `t.Parallel()`** — wall-clock comparison between Go runtime and Postgres `NOW()` under parallel CI load can blow a 1-minute window. Spec for chunk #4 deliberately fixed at ~1 min; widen if flakes appear in the wild.
- **DRY for `Can*Campaign` / `Can*Brand` admin-only authz checks** — `CanCreateCampaign` and `CanGetCampaign` are byte-identical (`if role != Admin → ErrForbidden`). Project-wide pattern (also `CanViewCreator` / `CanViewCreators`). Candidate: shared `requireRole(ctx, api.Admin)` helper once a fourth duplicate appears.

## Standards-checklist candidates surfaced this round

These are review-finding patterns the reviewers flagged; should be triaged into `docs/standards/` proper before the next chunk.

- specific error sentinels in `respondError` switch must precede generic fallbacks (cross-cutting hard rule).
- do not embed DB-stored field values into parse-error messages (`fmt.Errorf("parse %s: %w", fieldName, err)` без значения) — security/logging.
- domain→api mappers for stringified-UUID DB columns should use `uuid.Parse` + error-return, never `MustParse` — backend-architecture / backend-codegen.
- OpenAPI operation must declare every HTTP code the router actually emits, including 400 from path/query/body validators — api-contract.
