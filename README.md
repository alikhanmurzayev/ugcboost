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

## Backend CORS_ORIGINS

`backend/.env` (gitignored, per-developer) and the staging/prod env templates
must list every frontend origin that the backend should accept. The default in
`backend/internal/config/config.go` covers only `5173,5174` (web vite dev). For
local work / docker / e2e the env should include:

| Origin | Used by |
|---|---|
| `http://localhost:5173` | web vite dev (`npm run dev`) |
| `http://localhost:5174` | tma vite dev |
| `http://localhost:3001` | web docker (`make start-web`) |
| `http://localhost:3002` | tma docker (`make start-tma`) |
| `http://localhost:3003` | landing docker (`make start-landing`) |
| `http://localhost:4321` | landing astro dev / playwright webServer |

On staging / prod replace localhost entries with the real public origins.
