-- +goose Up
-- One Telegram account may legitimately link to several applications
-- (re-apply after rejection); positivity check moved to service layer.
ALTER TABLE creator_application_telegram_links
    DROP CONSTRAINT creator_application_telegram_links_telegram_user_id_key,
    DROP CONSTRAINT creator_application_telegram_links_telegram_user_id_check;

-- +goose Down
ALTER TABLE creator_application_telegram_links
    ADD CONSTRAINT creator_application_telegram_links_telegram_user_id_key
        UNIQUE (telegram_user_id),
    ADD CONSTRAINT creator_application_telegram_links_telegram_user_id_check
        CHECK (telegram_user_id > 0);
