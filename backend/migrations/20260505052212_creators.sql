-- +goose Up
-- Foundation tables for the creator entity (chunk 18a).
--
-- Three tables are introduced empty: creators (the snapshot row taken when
-- an admin approves a CreatorApplication) and the M:N children
-- creator_socials / creator_categories. They sit unused until chunk 18b
-- (approve action) starts populating them — this migration is the
-- "infrastructure without effect" half of the foundation slice.
--
-- Constraint names are stable identifiers (suffixed `_unique`) instead of
-- the auto-generated ones so backend-repository pgErr.ConstraintName
-- handling can switch on them without re-discovery on schema changes.
--
-- No regex / length CHECK on TEXT columns and no business defaults — those
-- belong in the service layer per backend-repository.md § Целостность данных.
-- TIMESTAMPTZ defaults stamp creation at the schema level because they are
-- pure DB integrity (no business meaning).

CREATE TABLE creators (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    iin                   TEXT NOT NULL,
    last_name             TEXT NOT NULL,
    first_name            TEXT NOT NULL,
    middle_name           TEXT,
    birth_date            DATE NOT NULL,
    phone                 TEXT NOT NULL,
    city_code             TEXT NOT NULL REFERENCES cities(code),
    address               TEXT,
    category_other_text   TEXT,
    telegram_user_id      BIGINT NOT NULL CHECK (telegram_user_id > 0),
    telegram_username     TEXT,
    telegram_first_name   TEXT,
    telegram_last_name    TEXT,
    source_application_id UUID NOT NULL REFERENCES creator_applications(id),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT creators_iin_unique UNIQUE (iin),
    CONSTRAINT creators_telegram_user_id_unique UNIQUE (telegram_user_id),
    CONSTRAINT creators_source_application_id_unique UNIQUE (source_application_id)
);

CREATE TABLE creator_socials (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    creator_id          UUID NOT NULL REFERENCES creators(id) ON DELETE CASCADE,
    platform            TEXT NOT NULL CHECK (platform IN ('instagram', 'tiktok', 'threads')),
    handle              TEXT NOT NULL,
    verified            BOOLEAN NOT NULL DEFAULT false,
    method              TEXT,
    verified_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    verified_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT creator_socials_creator_platform_handle_unique UNIQUE (creator_id, platform, handle)
);

CREATE INDEX creator_socials_creator_id_idx ON creator_socials(creator_id);

CREATE TABLE creator_categories (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    creator_id    UUID NOT NULL REFERENCES creators(id) ON DELETE CASCADE,
    category_code TEXT NOT NULL REFERENCES categories(code),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT creator_categories_creator_category_code_unique UNIQUE (creator_id, category_code)
);

CREATE INDEX creator_categories_creator_id_idx ON creator_categories(creator_id);
CREATE INDEX creator_categories_category_code_idx ON creator_categories(category_code);

-- +goose Down
DROP TABLE IF EXISTS creator_categories;
DROP TABLE IF EXISTS creator_socials;
DROP TABLE IF EXISTS creators;
