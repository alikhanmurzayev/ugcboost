-- +goose Up
CREATE TABLE creator_application_categories (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES creator_applications(id) ON DELETE CASCADE,
    category_id    UUID NOT NULL REFERENCES categories(id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (application_id, category_id)
);

CREATE INDEX idx_creator_application_categories_app ON creator_application_categories(application_id);
CREATE INDEX idx_creator_application_categories_cat ON creator_application_categories(category_id);

-- +goose Down
DROP TABLE IF EXISTS creator_application_categories;
