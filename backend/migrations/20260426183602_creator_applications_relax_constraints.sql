-- +goose Up
-- IIN format CHECK уезжает в backend (domain.ValidateIIN). Миграция не должна
-- содержать regex'ы — формат может поменяться, и в БД защита всё равно
-- дублирует backend-валидацию.
ALTER TABLE creator_applications
    DROP CONSTRAINT IF EXISTS creator_applications_iin_check;

-- DEFAULT 'pending' уезжает в backend (service.CreatorApplicationService.Submit
-- теперь явно проставляет domain.CreatorApplicationStatusPending). Бизнес-
-- defaults определяет код, не БД. CHECK на enum-значения остаётся —
-- это data integrity, а не business default.
ALTER TABLE creator_applications
    ALTER COLUMN status DROP DEFAULT;

-- +goose Down
ALTER TABLE creator_applications
    ALTER COLUMN status SET DEFAULT 'pending';

ALTER TABLE creator_applications
    ADD CONSTRAINT creator_applications_iin_check CHECK (iin ~ '^[0-9]{12}$');
