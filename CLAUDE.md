# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

UGCBoost — full-stack product (backend, frontend, landing pages, external integrations).

## Communication

All communication in this project is in **Russian**. Product documentation (briefs, PRDs, specs) is written in **Russian**. Code, variable names, and commits stay in English.

## Team

- **Alikhan Murzayev** (alikhan) — tech lead, architecture, backend
- **Aidana** (aidana) — product, business requirements, documentation, landing pages

## Development Methodology

BMad framework (modules: Core, BMM, CIS, TEA). Use BMad skills and agents for planning, design, and implementation workflows.

## BMad Setup

BMad config files (`_bmad/*/config.yaml`) are per-user and gitignored. After cloning, run `/bmad-init` with:
- communication_language: Russian
- document_output_language: Russian
- output_folder: _bmad-output
- project_name: ugcboost

## Git Workflow

- **Branch naming**: `<username>/<description>` (e.g. `alikhan/backend-auth`)
- **No direct pushes to main** — only via Pull Request with 1 approval
- **Commit messages**: concise, in English, describe the "why"
