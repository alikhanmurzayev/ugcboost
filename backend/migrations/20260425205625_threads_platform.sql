-- +goose Up
-- The platform column is a TEXT with a CHECK constraint (not a Postgres ENUM).
-- Re-create the CHECK to add 'threads' alongside 'instagram' and 'tiktok'.
ALTER TABLE creator_application_socials
    DROP CONSTRAINT creator_application_socials_platform_check;
ALTER TABLE creator_application_socials
    ADD CONSTRAINT creator_application_socials_platform_check
    CHECK (platform IN ('instagram', 'tiktok', 'threads'));

-- +goose Down
ALTER TABLE creator_application_socials
    DROP CONSTRAINT creator_application_socials_platform_check;
ALTER TABLE creator_application_socials
    ADD CONSTRAINT creator_application_socials_platform_check
    CHECK (platform IN ('instagram', 'tiktok'));
