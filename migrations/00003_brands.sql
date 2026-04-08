-- +goose Up
CREATE TABLE brands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    logo_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE brand_managers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    brand_id UUID NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, brand_id)
);

CREATE INDEX idx_brand_managers_user ON brand_managers(user_id);
CREATE INDEX idx_brand_managers_brand ON brand_managers(brand_id);

-- +goose Down
DROP TABLE IF EXISTS brand_managers;
DROP TABLE IF EXISTS brands;
