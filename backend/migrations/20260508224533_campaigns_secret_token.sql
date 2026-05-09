-- +goose Up
-- +goose StatementBegin
ALTER TABLE campaigns ADD COLUMN secret_token TEXT;

-- Backfill только валидных tma_url: last-segment ≥16 URL-safe chars.
-- Прод-записи с пустым/невалидным tma_url остаются с secret_token = NULL —
-- они недоступны через TMA до явного обновления URL администратором.
UPDATE campaigns
SET secret_token = regexp_replace(tma_url, '^.*/', '')
WHERE tma_url IS NOT NULL
  AND tma_url <> ''
  AND regexp_replace(tma_url, '^.*/', '') ~ '^[A-Za-z0-9_-]{16,}$';

CREATE UNIQUE INDEX campaigns_secret_token_uniq
  ON campaigns (secret_token)
  WHERE secret_token IS NOT NULL AND is_deleted = false;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS campaigns_secret_token_uniq;
ALTER TABLE campaigns DROP COLUMN IF EXISTS secret_token;
-- +goose StatementEnd
