---
description: Scout and analyze codebase before implementing changes
---

# Scout Mission

Investigate the codebase to understand the implementation area for: $ARGUMENTS

## Task 1: Understand the Request

Parse the user's request and identify:
- What feature/change/fix is being requested
- What files/components are likely involved
- What dependencies might be affected

## Task 2: Codebase Exploration

1. Run `git ls-files` to see project structure
2. Use Glob and Grep to find relevant files
3. Read key files to understand:
   - Current implementation patterns
   - Related functionality
   - Test patterns used

## Task 3: Report Findings

Create a scout report with:

### Affected Areas
- List files that will need changes
- Note dependencies between files

### Implementation Patterns
- How similar features are implemented
- Conventions to follow
- Test patterns to use

### Risks & Considerations
- Breaking changes to watch for
- Edge cases to handle
- Security considerations

### Recommended Approach
- Suggested order of changes
- Key decisions to make
- Alternative approaches (if applicable)

## Task 4: Await Instructions

Present the scout report and ask:
"Ready to proceed with implementation, or would you like me to explore further?"
