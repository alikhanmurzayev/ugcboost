-- +goose Up
-- UTM tracking metadata captured by the landing form (last-click model in
-- sessionStorage). All five fields are nullable text — never validated
-- against a catalogue, length-bounded only at the API layer (maxLength=256).
ALTER TABLE creator_applications ADD COLUMN utm_source TEXT NULL;
ALTER TABLE creator_applications ADD COLUMN utm_medium TEXT NULL;
ALTER TABLE creator_applications ADD COLUMN utm_campaign TEXT NULL;
ALTER TABLE creator_applications ADD COLUMN utm_term TEXT NULL;
ALTER TABLE creator_applications ADD COLUMN utm_content TEXT NULL;

-- +goose Down
ALTER TABLE creator_applications DROP COLUMN utm_content;
ALTER TABLE creator_applications DROP COLUMN utm_term;
ALTER TABLE creator_applications DROP COLUMN utm_campaign;
ALTER TABLE creator_applications DROP COLUMN utm_medium;
ALTER TABLE creator_applications DROP COLUMN utm_source;
