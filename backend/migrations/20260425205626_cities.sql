-- +goose Up
CREATE TABLE cities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_cities_active ON cities(active) WHERE active = TRUE;

-- 17 cities mirroring frontend/landing/src/content.ts.
-- Top-3 metros first (sort_order 10/20/30), the rest alphabetical with a
-- step of 10 — so future inserts can slot in without a renumber.
INSERT INTO cities (code, name, sort_order) VALUES
    ('almaty',        'Алматы',           10),
    ('astana',        'Астана',           20),
    ('shymkent',      'Шымкент',          30),
    ('aktau',         'Актау',           100),
    ('aktobe',        'Актобе',          110),
    ('atyrau',        'Атырау',          120),
    ('karaganda',     'Караганда',       130),
    ('kostanay',      'Костанай',        140),
    ('kyzylorda',     'Кызылорда',       150),
    ('pavlodar',      'Павлодар',        160),
    ('petropavlovsk', 'Петропавловск',   170),
    ('semey',         'Семей',           180),
    ('taraz',         'Тараз',           190),
    ('temirtau',      'Темиртау',        200),
    ('turkistan',     'Туркестан',       210),
    ('oral',          'Уральск',         220),
    ('oskemen',       'Усть-Каменогорск', 230);

-- +goose Down
DROP TABLE IF EXISTS cities;
