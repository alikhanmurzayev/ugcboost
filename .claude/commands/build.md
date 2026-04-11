---
description: Execute an implementation plan step by step
---

# Build Execution

Execute the implementation plan for: $ARGUMENTS

## Pre-Build Checks

1. Read the plan from `specs/plans/[feature]-plan.md`
2. Verify we're on the correct git branch
3. Ensure working directory is clean (commit any WIP first)

## Build Process

For each step in the implementation plan:

### Step Execution Pattern

1. **Read** the current state of files to modify
2. **Implement** the change following project conventions
3. **Verify** the change works (run tests if applicable)
4. **Commit** with a descriptive message

### Commit Message Format
```
[type]: [description]

- [detail 1]
- [detail 2]

Part of: [feature-name]
```

Types: feat, fix, refactor, test, docs, chore

## Progress Tracking

After each step, update `specs/plans/[feature]-progress.md`:

```markdown
# Progress: [Feature Name]

## Completed
- [x] Step 1: [description] - [timestamp]
- [x] Step 2: [description] - [timestamp]

## In Progress
- [ ] Step 3: [description]

## Remaining
- [ ] Step 4: [description]
- [ ] Step 5: [description]

## Blockers
[Any issues encountered]

## Notes
[Decisions made, deviations from plan]
```

## Post-Build

1. Run full test suite
2. Update plan status
3. Report completion summary

## Human Checkpoint

After completing all steps, present:
- Summary of changes made
- Any deviations from the plan
- Test results
- "Ready for review, or any changes needed?"
