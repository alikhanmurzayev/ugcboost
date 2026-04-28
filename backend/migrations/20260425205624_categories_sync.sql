-- +goose Up
-- Sync categories with the EFW landing PR #20:
-- 1) add sort_order so the picker order is owned by the catalogue, not the client
-- 2) drop 'gaming' (not on the landing) and add 'home_diy' / 'animals' / 'other'
-- 3) update names to match the long-form labels shown on the landing
ALTER TABLE categories ADD COLUMN sort_order INT NOT NULL DEFAULT 0;

UPDATE categories SET name = 'Мода / Стиль',           sort_order = 10  WHERE code = 'fashion';
UPDATE categories SET name = 'Бьюти (макияж, уход)',   sort_order = 20  WHERE code = 'beauty';
UPDATE categories SET name = 'Лайфстайл',              sort_order = 30  WHERE code = 'lifestyle';
UPDATE categories SET name = 'Еда / Рестораны',        sort_order = 40  WHERE code = 'food';
UPDATE categories SET name = 'Путешествия',            sort_order = 50  WHERE code = 'travel';
UPDATE categories SET name = 'Фитнес / Здоровье / ЗОЖ', sort_order = 60 WHERE code = 'fitness';
UPDATE categories SET name = 'Мама и дети / Семья',    sort_order = 70  WHERE code = 'parenting';
UPDATE categories SET name = 'Авто',                   sort_order = 80  WHERE code = 'auto';
UPDATE categories SET name = 'Тех / Гаджеты',          sort_order = 90  WHERE code = 'tech';

DELETE FROM categories WHERE code = 'gaming';

INSERT INTO categories (code, name, sort_order) VALUES
    ('home_diy', 'Дом / Интерьер / DIY', 100),
    ('animals',  'Животные',             110),
    ('other',    'Другое',               999);

-- +goose Down
DELETE FROM categories WHERE code IN ('home_diy', 'animals', 'other');
INSERT INTO categories (code, name) VALUES ('gaming', 'Игры') ON CONFLICT DO NOTHING;
UPDATE categories SET name = 'Бьюти'        WHERE code = 'beauty';
UPDATE categories SET name = 'Мода'         WHERE code = 'fashion';
UPDATE categories SET name = 'Еда'          WHERE code = 'food';
UPDATE categories SET name = 'Фитнес'       WHERE code = 'fitness';
UPDATE categories SET name = 'Лайфстайл'    WHERE code = 'lifestyle';
UPDATE categories SET name = 'Технологии'   WHERE code = 'tech';
UPDATE categories SET name = 'Путешествия'  WHERE code = 'travel';
UPDATE categories SET name = 'Родительство' WHERE code = 'parenting';
UPDATE categories SET name = 'Авто'         WHERE code = 'auto';
ALTER TABLE categories DROP COLUMN sort_order;
