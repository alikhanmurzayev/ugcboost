-- +goose Up
-- Free-text description used when the creator picks the 'other' category.
-- Required at the service layer when 'other' is in the selected codes; the
-- column itself stays nullable so applications without 'other' don't carry
-- empty strings.
ALTER TABLE creator_applications ADD COLUMN category_other_text TEXT NULL;

-- +goose Down
ALTER TABLE creator_applications DROP COLUMN category_other_text;
