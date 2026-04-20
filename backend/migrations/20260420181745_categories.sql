-- +goose Up
CREATE TABLE categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_categories_active ON categories(active) WHERE active = TRUE;

INSERT INTO categories (code, name) VALUES
    ('beauty',     'Бьюти'),
    ('fashion',    'Мода'),
    ('food',       'Еда'),
    ('fitness',    'Фитнес'),
    ('lifestyle',  'Лайфстайл'),
    ('tech',       'Технологии'),
    ('travel',     'Путешествия'),
    ('parenting',  'Родительство'),
    ('auto',       'Авто'),
    ('gaming',     'Игры');

-- +goose Down
DROP TABLE IF EXISTS categories;
