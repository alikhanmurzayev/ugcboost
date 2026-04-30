-- +goose Up
-- Re-introduce the > 0 invariant on telegram_user_id. The previous migration
-- dropped it together with the UNIQUE constraint with a comment that the
-- positivity check moved to the service layer — but the bot handler reaches
-- the service via several paths (real /start, anonymous group admin, test
-- endpoint) and a non-positive value sneaking in produces a poisoned link
-- row that matches every future "id == 0" lookup as idempotent. Defence in
-- depth: keep the check at the schema boundary too.
ALTER TABLE creator_application_telegram_links
    ADD CONSTRAINT creator_application_telegram_links_telegram_user_id_check
        CHECK (telegram_user_id > 0);

-- +goose Down
ALTER TABLE creator_application_telegram_links
    DROP CONSTRAINT creator_application_telegram_links_telegram_user_id_check;
