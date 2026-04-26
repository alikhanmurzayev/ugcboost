-- +goose Up
ALTER TABLE creator_applications ALTER COLUMN address DROP NOT NULL;

-- +goose Down
-- Existing rows with NULL would block SET NOT NULL; coerce them to empty string
-- so the schema can return to its previous shape. This is destructive of the
-- "we don't have an address yet" signal — there is no way back to NULL after
-- the down migration runs, by design.
UPDATE creator_applications SET address = '' WHERE address IS NULL;
ALTER TABLE creator_applications ALTER COLUMN address SET NOT NULL;
