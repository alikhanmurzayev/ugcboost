-- +goose Up
-- chunk-7 миграция объявила FK creator_application_socials.verified_by_user_id
-- → users.id с ON DELETE SET NULL, но в применённой схеме она оказалась без
-- ON DELETE clause (NO ACTION default). Любая попытка удалить пользователя,
-- который вручную верифицировал хотя бы одну соцсеть, ловит 23503 — что
-- блокирует e2e-cleanup для chunk-10 happy-path и для будущих admin-driven
-- сценариев (rollback users, /test/cleanup-entity).
--
-- Дроп + добавление с явным ON DELETE SET NULL в той же транзакции —
-- эквивалент SET NULL для будущих DELETE'ов без миграции данных.
ALTER TABLE creator_application_socials
    DROP CONSTRAINT creator_application_socials_verified_by_user_id_fkey,
    ADD CONSTRAINT creator_application_socials_verified_by_user_id_fkey
        FOREIGN KEY (verified_by_user_id) REFERENCES users(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE creator_application_socials
    DROP CONSTRAINT creator_application_socials_verified_by_user_id_fkey,
    ADD CONSTRAINT creator_application_socials_verified_by_user_id_fkey
        FOREIGN KEY (verified_by_user_id) REFERENCES users(id);
