---
description: Create a detailed implementation plan for a feature or change
---

# Implementation Planning

Create a comprehensive implementation plan for: $ARGUMENTS

## Task 1: Understand Requirements

Parse the request and clarify:
- Core requirements (must-have)
- Nice-to-have features
- Out of scope items
- Success criteria

## Task 2: Analyze Current State

1. Read relevant existing code
2. Identify patterns to follow
3. Note any technical debt to address
4. Check for existing tests to maintain

## Task 3: Create Implementation Plan

Write a plan to `specs/plans/[feature-name]-plan.md`:

```markdown
# Implementation Plan: [Feature Name]

## Overview
[1-2 sentence description]

## Requirements
- REQ-1: [requirement]
- REQ-2: [requirement]

## Files to Modify
| File | Changes |
|------|---------|
| path/to/file.ts | [what to change] |

## Files to Create
| File | Purpose |
|------|---------|
| path/to/new-file.ts | [purpose] |

## Implementation Steps
1. [ ] Step 1: [description]
2. [ ] Step 2: [description]
3. [ ] Step 3: [description]

## Testing Strategy
- Unit tests: [what to test]
- Integration tests: [what to test]
- Manual testing: [what to verify]

## Risk Assessment
| Risk | Likelihood | Mitigation |
|------|------------|------------|
| [risk] | Low/Med/High | [how to mitigate] |

## Rollback Plan
[How to revert if something goes wrong]
```

## Task 4: Review Checkpoint

Present the plan and ask:
"Does this plan look good? Any adjustments before I start building?"
