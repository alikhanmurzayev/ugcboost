-- +goose Up
CREATE TABLE telegram_messages (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id             BIGINT NOT NULL,
    direction           TEXT NOT NULL
                        CHECK (direction IN ('inbound','outbound')),
    text                TEXT NOT NULL DEFAULT '',
    telegram_message_id BIGINT,
    telegram_username   TEXT,
    status              TEXT
                        CHECK (status IS NULL OR status IN ('sent','failed')),
    error               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Listing index: keyset pagination DESC by (created_at, id) per chat_id.
CREATE INDEX idx_telegram_messages_chat_created
    ON telegram_messages(chat_id, created_at DESC, id DESC);

-- Telegram resends the same Update on connectivity issues. Dedupe inbound rows
-- by (chat_id, telegram_message_id). Outbound rows are exempt — every retry
-- (Notifier.fire backoff) is a deliberately distinct attempt.
CREATE UNIQUE INDEX telegram_messages_inbound_dedup_unique
    ON telegram_messages(chat_id, telegram_message_id)
    WHERE direction = 'inbound' AND telegram_message_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS telegram_messages;
