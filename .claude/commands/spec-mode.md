---
description: Инкрементальный дизайн интента под bmad-quick-dev
---

# Spec Mode: incremental intent design before bmad-quick-dev

## RULES

- READ-ONLY mode. Do not edit code or run migrations. A parallel coding
  agent may be working — do not interfere.
- ITERATIVE design. Start with a 1–2 sentence thesis.
- HARD CAP each message at ~25 lines. Exception: the final concept
  restate at CHECKPOINT.
- DO NOT FANTASIZE. If intent is unclear, HALT and ask.

## INSTRUCTIONS

1. Read intent: argument or recent conversation. If empty, HALT and ask.
2. Resume check. If a recent
   `_bmad-output/implementation-artifacts/intent-*.md` matches the
   intent: HALT and ask `[R] Resume` | `[N] Start new`.
3. Investigate before asking. Read related planning artifacts, the
   roadmap section, the relevant codebase area. Use sub-agents
   (Explore) for broad scans. Do not ask what you can read.
4. Form a 1–2 sentence thesis. HALT for ack.
5. Refine aspects in the order that fits the task. Common ones:
   user-facing flow, API surface, domain entities, fields &
   validations, transactions/idempotency/audit, tests. Halt after
   each. Order is not fixed; subset is fine.
6. After every approved decision: (a) overwrite
   `_bmad-output/implementation-artifacts/intent-{slug}.md` with the
   full current concept; (b) post a short diff summary in chat
   (≤25 lines) — what changed since last iteration. Ask the human
   for slug if not obvious.
7. COHESION CHECK before final hand-off. Migration, audit, tests,
   notifications — anything obvious missing? If yes, HALT and ask.
8. CHECKPOINT — when human says "enough" or you assess sufficient:
   restate the full concept in chat (one message — exception to the
   25-line cap). HALT and ask `[B] Hand off to bmad-quick-dev`
   | `[E] Edit`.
9. On B: invoke bmad-quick-dev with path to `intent-{slug}.md`.
   When bmad-quick-dev's CHECKPOINT 1 approves the spec, delete
   the intent file (it served its purpose).

## NEXT

bmad-quick-dev step-02 (after CHECKPOINT B).
