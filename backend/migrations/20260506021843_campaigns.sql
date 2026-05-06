-- +goose Up
-- Foundation table for the campaign entity (chunk #3 of campaign-roadmap).
--
-- Minimal MVP shape: a campaign is just {name, tma_url}. The TMA-side ТЗ
-- landing lives at a hardcoded secret URL inside the TMA app — the URL is
-- only stored here to be embedded into outbound creator-invite messages.
--
-- Constraint name `campaigns_name_active_unique` is a stable identifier so
-- backend-repository pgErr.ConstraintName handling can switch on it without
-- re-discovery on schema changes. The index is partial (`WHERE is_deleted =
-- false`) so a soft-deleted campaign frees its name for reuse — this is why
-- it's an INDEX with WHERE clause, not a table-level UNIQUE constraint.
--
-- No regex / length CHECK on TEXT columns and no business defaults beyond
-- pure DB integrity (`is_deleted` default = false / TIMESTAMPTZ defaults
-- stamping creation) — format validations belong in the service layer per
-- backend-repository.md § Целостность данных.

CREATE TABLE campaigns (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    tma_url    TEXT NOT NULL,
    is_deleted BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX campaigns_name_active_unique
    ON campaigns(name)
    WHERE is_deleted = false;

-- +goose Down
DROP INDEX IF EXISTS campaigns_name_active_unique;
DROP TABLE IF EXISTS campaigns;
