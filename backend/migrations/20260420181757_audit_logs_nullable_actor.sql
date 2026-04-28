-- +goose Up
-- Public-surface actions (e.g. creator application submission) record audit
-- entries without an authenticated actor. Allow NULL actor_id so those rows
-- can land while keeping the FK to users for everything else.
ALTER TABLE audit_logs ALTER COLUMN actor_id DROP NOT NULL;

-- +goose Down
-- Backfill stranded rows with a synthetic system actor would be required
-- before re-enabling NOT NULL. Leaving as-is: the down path is rarely taken.
ALTER TABLE audit_logs ALTER COLUMN actor_id SET NOT NULL;
