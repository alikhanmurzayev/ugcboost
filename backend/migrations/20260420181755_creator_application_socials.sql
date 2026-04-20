-- +goose Up
CREATE TABLE creator_application_socials (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES creator_applications(id) ON DELETE CASCADE,
    platform       TEXT NOT NULL CHECK (platform IN ('instagram','tiktok')),
    handle         TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (application_id, platform, handle)
);

CREATE INDEX idx_creator_application_socials_app ON creator_application_socials(application_id);

-- +goose Down
DROP TABLE IF EXISTS creator_application_socials;
