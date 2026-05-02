---
title: Deferred work
type: tracker
status: living
---

# Отложенная работа

Findings, всплывшие во время review, которые **не относятся к текущей задаче**, но требуют внимания позже.

## Открытые

- **2026-05-02 / spec-creator-applications-counts.md review**: дублирующиеся `extractErrorCode` / `extractErrorMessage` хелперы между `frontend/landing/src/api/dictionaries.ts` и `frontend/landing/src/api/creator-applications.ts`. Pre-existing, не в scope этого PR. Кандидат на вынос в `frontend/landing/src/api/errors.ts` или в `client.ts` рядом с `ApiError`. Severity — minor.
