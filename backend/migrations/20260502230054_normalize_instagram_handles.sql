-- +goose Up
-- Backfill normalisation of legacy Instagram handles in
-- creator_application_socials. From chunk 6 (and earlier) the Submit
-- service already lowercases + strips leading '@', but the existing 20
-- prod rows were inserted before that became canonical. The chunk 8
-- SendPulse webhook performs strict equality between the lowercased IG
-- username it receives and the stored handle, so any "@User" / "USER"
-- legacy row would silently fail to match without this migration.
--
-- trim(BOTH '@' FROM handle) drops every '@' on either side; no IG
-- handle legitimately contains '@' inside, and the application form
-- already rejects it. No-op for rows already in canonical form.
--
-- Down is intentionally a no-op: the original casing/leading-'@' is
-- not recoverable from the normalised value.

UPDATE creator_application_socials
SET handle = lower(trim(BOTH '@' FROM handle))
WHERE platform = 'instagram'
  AND handle <> lower(trim(BOTH '@' FROM handle));

-- +goose Down
SELECT 1; -- normalisation is irreversible by design
