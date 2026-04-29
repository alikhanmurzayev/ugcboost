-- +goose Up
CREATE TABLE creator_application_telegram_links (
    application_id      UUID PRIMARY KEY
                        REFERENCES creator_applications(id) ON DELETE CASCADE,
    telegram_user_id    BIGINT NOT NULL UNIQUE,
    telegram_username   TEXT,
    telegram_first_name TEXT,
    telegram_last_name  TEXT,
    linked_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS creator_application_telegram_links;
