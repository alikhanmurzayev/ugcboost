-- +goose Up
-- M2M attachment of creators to campaigns (chunk 10 of campaign-roadmap).
--
-- One row per (campaign, creator) pair. The lifecycle of the row is the
-- creator's state within the campaign: `planned` after admin add → `invited`
-- after notify → `declined` / `agreed` after the creator's TMA decision.
-- Chunk 10 only seeds rows in `planned`; the remaining transitions land in
-- chunks 12 and 14.
--
-- Constraint names are stable identifiers so backend-repository
-- pgErr.ConstraintName handling can switch on them without re-discovery on
-- schema changes:
--   * `campaign_creators_campaign_creator_unique` — disambiguates the
--     re-add 23505 race into ErrCreatorAlreadyInCampaign.
--   * `campaign_creators_campaign_id_fk` — 23503 with this name means the
--     campaign vanished mid-batch, mapped to ErrCampaignNotFound.
--   * `campaign_creators_creator_id_fk` — 23503 with this name means the
--     creatorId is bogus, mapped to ErrCampaignCreatorCreatorNotFound.
--
-- No DEFAULT on `status`: the value is a business decision (the service
-- always seeds `planned` in chunk 10, downstream chunks set other states),
-- so per backend-repository.md § Целостность данных it stays out of the
-- schema. Counters default to 0 because zero is the only meaningful initial
-- value (pure integrity, no business semantics). Format / regex CHECKs on
-- TEXT remain on the service layer.

CREATE TABLE campaign_creators (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id     UUID NOT NULL,
    creator_id      UUID NOT NULL,
    status          TEXT NOT NULL,
    invited_at      TIMESTAMPTZ,
    invited_count   INT NOT NULL DEFAULT 0,
    reminded_at     TIMESTAMPTZ,
    reminded_count  INT NOT NULL DEFAULT 0,
    decided_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT campaign_creators_status_check CHECK (status IN ('planned', 'invited', 'declined', 'agreed')),
    CONSTRAINT campaign_creators_campaign_creator_unique UNIQUE (campaign_id, creator_id),
    CONSTRAINT campaign_creators_campaign_id_fk FOREIGN KEY (campaign_id) REFERENCES campaigns(id),
    CONSTRAINT campaign_creators_creator_id_fk  FOREIGN KEY (creator_id)  REFERENCES creators(id)
);

-- +goose Down
DROP TABLE IF EXISTS campaign_creators;
