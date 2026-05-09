-- +goose Up
-- +goose StatementBegin
-- Adds storage for the contract-template PDF that the admin uploads via PUT
-- /campaigns/{id}/contract-template (chunk 9a). The column is NOT NULL with
-- DEFAULT '\x' so existing rows pick up an empty bytea instead of NULL — the
-- "is template loaded?" check stays a single `octet_length(...) > 0` SQL
-- expression rather than a tri-state NULL/empty/non-empty fork. The PDF
-- itself is never SELECTed by default; reads go through the dedicated GET
-- /campaigns/{id}/contract-template endpoint.
ALTER TABLE campaigns
    ADD COLUMN contract_template_pdf BYTEA NOT NULL DEFAULT '\x';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE campaigns
    DROP COLUMN IF EXISTS contract_template_pdf;
-- +goose StatementEnd
