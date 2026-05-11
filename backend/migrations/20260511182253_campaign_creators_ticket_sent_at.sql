-- +goose Up
ALTER TABLE campaign_creators
    ADD COLUMN ticket_sent_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE campaign_creators
    DROP COLUMN ticket_sent_at;
