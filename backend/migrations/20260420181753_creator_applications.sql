-- +goose Up
CREATE TABLE creator_applications (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    last_name   TEXT NOT NULL,
    first_name  TEXT NOT NULL,
    middle_name TEXT,
    iin         TEXT NOT NULL CHECK (iin ~ '^[0-9]{12}$'),
    birth_date  DATE NOT NULL,
    phone       TEXT NOT NULL,
    city        TEXT NOT NULL,
    address     TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending','approved','rejected','blocked')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One IIN cannot have more than one application in an "active" state.
-- Rejected applications are excluded so creators can re-apply (FR17).
CREATE UNIQUE INDEX creator_applications_iin_active_idx
    ON creator_applications(iin)
    WHERE status IN ('pending','approved','blocked');

CREATE INDEX idx_creator_applications_status ON creator_applications(status);
CREATE INDEX idx_creator_applications_created_at ON creator_applications(created_at);

-- +goose Down
DROP TABLE IF EXISTS creator_applications;
