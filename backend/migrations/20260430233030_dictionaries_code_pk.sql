-- +goose Up

-- Promote `code` from UNIQUE secondary key to PRIMARY KEY for the public
-- dictionaries (categories, cities) and switch consumer FKs to reference
-- code directly. This eliminates the UUID indirection that forced repos to
-- JOIN every time they wanted a code, and brings consumer columns under the
-- <entity>_code naming convention. Tracking: gh-25.

-- 1. creator_application_categories: backfill category_code from JOIN, then
-- drop the UUID column and its dependent constraints/indexes.
ALTER TABLE creator_application_categories ADD COLUMN category_code TEXT;

UPDATE creator_application_categories cac
   SET category_code = c.code
  FROM categories c
 WHERE c.id = cac.category_id;

-- Defensive: the existing FK to categories(id) prevents orphans, but we want
-- the migration to fail loudly if any row is left without a code rather than
-- silently DROP COLUMN and lose the data.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM creator_application_categories WHERE category_code IS NULL) THEN
        RAISE EXCEPTION 'orphan category_id rows in creator_application_categories — manual cleanup required before migration';
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE creator_application_categories ALTER COLUMN category_code SET NOT NULL;

ALTER TABLE creator_application_categories
    DROP CONSTRAINT creator_application_categories_application_id_category_id_key;
DROP INDEX IF EXISTS idx_creator_application_categories_cat;
ALTER TABLE creator_application_categories DROP COLUMN category_id;

-- 2. categories: drop UUID id, promote code to PK.
ALTER TABLE categories DROP COLUMN id;
ALTER TABLE categories DROP CONSTRAINT IF EXISTS categories_code_key;
ALTER TABLE categories ADD PRIMARY KEY (code);

-- 3. cities: drop UUID id, promote code to PK.
ALTER TABLE cities DROP COLUMN id;
ALTER TABLE cities DROP CONSTRAINT IF EXISTS cities_code_key;
ALTER TABLE cities ADD PRIMARY KEY (code);

-- 4. New FK + UNIQUE + index on category_code.
ALTER TABLE creator_application_categories
    ADD CONSTRAINT creator_application_categories_category_code_fkey
    FOREIGN KEY (category_code) REFERENCES categories(code);
ALTER TABLE creator_application_categories
    ADD CONSTRAINT creator_application_categories_application_code_uniq
    UNIQUE (application_id, category_code);
CREATE INDEX idx_creator_application_categories_cat
    ON creator_application_categories(category_code);

-- 5. creator_applications.city → city_code with FK to cities(code). The
-- column already stores codes, so RENAME is a no-op data-wise; the FK is
-- the new invariant and may surface latent orphans (unknown city codes).
ALTER TABLE creator_applications RENAME COLUMN city TO city_code;

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
          FROM creator_applications ca
          LEFT JOIN cities c ON c.code = ca.city_code
         WHERE c.code IS NULL
    ) THEN
        RAISE EXCEPTION 'orphan city_code values in creator_applications — manual cleanup required before migration';
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE creator_applications
    ADD CONSTRAINT creator_applications_city_code_fkey
    FOREIGN KEY (city_code) REFERENCES cities(code);

-- +goose Down

-- Down is intentionally not provided: the original UUID `id` values for
-- categories and cities are irrecoverable once dropped. Rolling back
-- requires a database restore from before the Up migration. Goose still
-- needs a Down section, so we raise to make the impossibility explicit
-- rather than silently no-op.
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'down migration unsupported: UUID ids cannot be restored — restore database from backup';
END $$;
-- +goose StatementEnd
