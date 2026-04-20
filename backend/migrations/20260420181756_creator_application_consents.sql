-- +goose Up
CREATE TABLE creator_application_consents (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id   UUID NOT NULL REFERENCES creator_applications(id) ON DELETE CASCADE,
    consent_type     TEXT NOT NULL CHECK (consent_type IN ('processing','third_party','cross_border','terms')),
    accepted_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    document_version TEXT NOT NULL,
    ip_address       TEXT NOT NULL,
    user_agent       TEXT NOT NULL,
    UNIQUE (application_id, consent_type)
);

CREATE INDEX idx_creator_application_consents_app ON creator_application_consents(application_id);

-- +goose Down
DROP TABLE IF EXISTS creator_application_consents;
