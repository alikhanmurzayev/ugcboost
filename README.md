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

## Roadmaps

Living-документы с нарезкой на чанки (один чанк ≈ один PR с релизом).

- **Активный:** [campaign-roadmap.md](_bmad-output/planning-artifacts/campaign-roadmap.md) — путь от `approved`-креатора до билета на мероприятие 13–14 мая. Дедлайн функционала — 9 мая.
- **Завершённый:** [creator-onboarding-roadmap.md](_bmad-output/planning-artifacts/archive/2026-04-30-creator-onboarding-roadmap.md) — онбординг креатора (форма заявки → верификация → approved). Закрыт 2026-05-05.

## Start claude code session without tmux:
   ```bash
   claude --dangerously-skip-permissions --remote-control --effort max -c
   ```
