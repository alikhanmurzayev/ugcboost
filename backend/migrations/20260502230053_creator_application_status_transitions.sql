-- +goose Up
-- Persistent history of creator-application status transitions (chunk 8 of
-- creator-onboarding-roadmap). Audit_logs answers "what happened"; this
-- table answers "how the status moved" — explicit from/to columns let
-- analytics build funnels without re-parsing JSON metadata.
--
-- ON DELETE CASCADE on application_id: the application is the aggregate
-- root, transition history is meaningless without it. ON DELETE SET NULL
-- on actor_id: losing "who pressed the button" when an admin row is
-- purged is acceptable — the timeline itself stays intact.
--
-- No backfill: the table is empty at deploy and starts gaining rows from
-- the first real transition (chunk 8 webhook + chunk 9 manual verify).
-- No CHECK on `reason`: it is TEXT for forward compatibility; valid
-- values are enumerated as constants in `domain` (TransitionReason*).

CREATE TABLE creator_application_status_transitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES creator_applications(id) ON DELETE CASCADE,
    from_status TEXT,
    to_status TEXT NOT NULL,
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX creator_application_status_transitions_application_idx
    ON creator_application_status_transitions(application_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS creator_application_status_transitions;
