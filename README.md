# UGCBoost

## Setup

1. Clone the repository:
   ```bash
   git clone git@github.com:alikhanmurzayev/ugcboost.git ~/projects/ugcboost
   ```

2. Initialize BMad (first time after cloning):
   ```bash
   claude-project ugcboost
   ```
   Then run `/bmad-init` inside Claude session. Use these settings:
   - **communication_language**: Russian
   - **document_output_language**: Russian
   - **output_folder**: `_bmad-output`
   - **project_name**: ugcboost

   BMad config is per-user (gitignored) — each team member runs this once.

## Workflow

- Branch naming: `<username>/<description>` (e.g. `alikhan/backend-auth`)
- No direct pushes to `main` — only via Pull Request with approval
- Use `gh pr create` to open PRs

## Start claude code session without tmux:
   ```bash
   claude --dangerously-skip-permissions --remote-control --effort max -c
   ```
