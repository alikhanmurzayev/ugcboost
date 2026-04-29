-- +goose Up
-- linked_at carries no DB-side default: the service stamps `now` from Go in
-- the same transaction as the audit row, and a column default would mask a
-- regression where the service forgets to pass the value (business defaults
-- belong in the service, not the schema — see backend-repository.md).
CREATE TABLE creator_application_telegram_links (
    application_id      UUID PRIMARY KEY
                        REFERENCES creator_applications(id) ON DELETE CASCADE,
    telegram_user_id    BIGINT NOT NULL UNIQUE,
    telegram_username   TEXT,
    telegram_first_name TEXT,
    telegram_last_name  TEXT,
    linked_at           TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS creator_application_telegram_links;
