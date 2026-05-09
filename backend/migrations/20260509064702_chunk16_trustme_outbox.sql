-- +goose Up
-- +goose StatementBegin
-- chunk 16 — TrustMe outbox-worker.
--
-- Добавляет универсальную таблицу `contracts` (Decision #8 intent-v2),
-- reverse-FK на стороне `campaign_creators.contract_id`, расширяет
-- state-машину `campaign_creators` тремя новыми статусами и кладёт два
-- партиальных индекса под Phase 1 / Phase 0 SELECT'ы worker'а
-- (Decision #10).

CREATE TABLE contracts (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_kind           TEXT NOT NULL,
    trustme_document_id    TEXT,
    trustme_short_url      TEXT,
    trustme_status_code    INT  NOT NULL DEFAULT 0,
    unsigned_pdf_content   BYTEA,
    signed_pdf_content     BYTEA,
    initiated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    signed_at              TIMESTAMPTZ,
    declined_at            TIMESTAMPTZ,
    webhook_received_at    TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT contracts_subject_kind_check
        CHECK (subject_kind IN ('campaign_creator')),
    CONSTRAINT contracts_trustme_document_id_unique
        UNIQUE (trustme_document_id),
    CONSTRAINT contracts_trustme_status_code_range
        CHECK (trustme_status_code BETWEEN 0 AND 9)
);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE campaign_creators
    ADD COLUMN contract_id UUID,
    ADD CONSTRAINT campaign_creators_contract_id_fk
        FOREIGN KEY (contract_id) REFERENCES contracts(id) ON DELETE SET NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE campaign_creators
    DROP CONSTRAINT campaign_creators_status_check;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE campaign_creators
    ADD CONSTRAINT campaign_creators_status_check
        CHECK (status IN ('planned', 'invited', 'declined', 'agreed',
                          'signing', 'signed', 'signing_declined'));
-- +goose StatementEnd

-- +goose StatementBegin
-- Phase 1 outbox: выбираем `agreed` без contract_id, ORDER BY decided_at.
CREATE INDEX idx_campaign_creators_outbox
    ON campaign_creators (decided_at)
    WHERE status = 'agreed' AND contract_id IS NULL;
-- +goose StatementEnd

-- +goose StatementBegin
-- Phase 0 recovery: orphan'ы с trustme_document_id IS NULL.
CREATE INDEX idx_contracts_orphan
    ON contracts (subject_kind, initiated_at)
    WHERE trustme_document_id IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_contracts_orphan;
DROP INDEX IF EXISTS idx_campaign_creators_outbox;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE campaign_creators
    DROP CONSTRAINT campaign_creators_status_check;
-- +goose StatementEnd

-- +goose StatementBegin
-- Если в БД уже есть ряды с новыми статусами (signing/signed/signing_declined),
-- этот шаг упадёт: revert невозможен без manual cleanup. Это намеренная защита
-- от потери данных при откате уже работающего outbox worker'а.
ALTER TABLE campaign_creators
    ADD CONSTRAINT campaign_creators_status_check
        CHECK (status IN ('planned', 'invited', 'declined', 'agreed'));
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE campaign_creators
    DROP CONSTRAINT IF EXISTS campaign_creators_contract_id_fk,
    DROP COLUMN IF EXISTS contract_id;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS contracts;
-- +goose StatementEnd
