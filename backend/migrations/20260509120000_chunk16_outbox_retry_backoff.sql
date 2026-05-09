-- +goose Up
-- +goose StatementBegin

-- Per-row retry backoff для outbox-worker'а: каждый failed Phase 2c send /
-- Phase 0 resend записывает last_error_code/message + сдвигает next_retry_at
-- на cfg.TrustMeRetryBackoff. SelectOrphansForRecovery затем фильтрует по
-- (next_retry_at IS NULL OR next_retry_at <= now()), чтобы кривой orphan
-- (1219, плохой ИИН и т.п.) уходил в backoff и не блокировал слоты для
-- свежих контрактов.
ALTER TABLE contracts
    ADD COLUMN next_retry_at      TIMESTAMPTZ,
    ADD COLUMN last_error_code    TEXT,
    ADD COLUMN last_error_message TEXT,
    ADD COLUMN last_attempted_at  TIMESTAMPTZ;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE contracts
    DROP COLUMN next_retry_at,
    DROP COLUMN last_error_code,
    DROP COLUMN last_error_message,
    DROP COLUMN last_attempted_at;

-- +goose StatementEnd
