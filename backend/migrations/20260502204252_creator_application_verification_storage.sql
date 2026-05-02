-- +goose Up
-- Бэк-фундамент верификации (chunk 7 creator-onboarding-roadmap):
--   * verification_code на creator_applications — идентификатор-код,
--     который креатор отправит в IG DM. Формат UGC-NNNNNN, целостность
--     формата держит сервис (нет CHECK), partial unique только пока
--     заявка в статусе verification — после перехода в moderation код
--     перестаёт быть constraint'ом и может позже переиспользоваться.
--   * 4 поля верификации на creator_application_socials: verified,
--     method, verified_by_user_id, verified_at. Никаких CHECK на
--     согласованность — целостность 4 полей держит сервис.
--
-- Существующие заявки бэкфилятся в этой же транзакции (DO-блок,
-- retry до 10 на unique_violation, RAISE EXCEPTION на 11-й попытке).
-- ВНИМАНИЕ: миграция в одной транзакции (default goose). Не добавлять
-- NO TRANSACTION — промежуточное состояние "verification_code NULL"
-- не должно быть наблюдаемо извне.

ALTER TABLE creator_applications
    ADD COLUMN verification_code TEXT;

-- ON DELETE SET NULL: losing "who verified this" when an admin row is purged
-- is acceptable; we still want the verified=true / verified_at history to
-- survive. Without an explicit clause we default to NO ACTION, which would
-- block any future hard-delete of an admin row that ever pressed Verify.
ALTER TABLE creator_application_socials
    ADD COLUMN verified            BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN method              TEXT,
    ADD COLUMN verified_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN verified_at         TIMESTAMPTZ;

CREATE UNIQUE INDEX creator_applications_verification_code_verification_idx
    ON creator_applications(verification_code)
    WHERE status = 'verification';

-- Backfill uses cryptographic random (`gen_random_bytes`) rather than the
-- PRNG-based `random()`: the verification code is going to be matched against
-- IG DM messages once chunk 8 lands, so a guessable seed-replayable code is
-- a security regression. Convert 4 random bytes → 32-bit unsigned, modulo
-- 1_000_000 to land in the same 6-digit space as the runtime helper.
-- +goose StatementBegin
DO $$
DECLARE
    rec       RECORD;
    new_code  TEXT;
    attempt   INT;
BEGIN
    FOR rec IN
        SELECT id FROM creator_applications
        WHERE verification_code IS NULL
        ORDER BY created_at
    LOOP
        attempt := 0;
        LOOP
            new_code := 'UGC-' || LPAD(((('x' || encode(gen_random_bytes(4), 'hex'))::bit(32)::int8 & x'7fffffff'::int8) % 1000000)::TEXT, 6, '0');
            BEGIN
                UPDATE creator_applications
                SET verification_code = new_code
                WHERE id = rec.id;
                EXIT;
            EXCEPTION WHEN unique_violation THEN
                attempt := attempt + 1;
                IF attempt >= 10 THEN
                    RAISE EXCEPTION 'failed to generate unique verification_code for application % after 10 attempts', rec.id;
                END IF;
            END;
        END LOOP;
    END LOOP;
END $$;
-- +goose StatementEnd

ALTER TABLE creator_applications
    ALTER COLUMN verification_code SET NOT NULL;

-- +goose Down
ALTER TABLE creator_applications
    ALTER COLUMN verification_code DROP NOT NULL;

DROP INDEX IF EXISTS creator_applications_verification_code_verification_idx;

ALTER TABLE creator_application_socials
    DROP COLUMN IF EXISTS verified_at,
    DROP COLUMN IF EXISTS verified_by_user_id,
    DROP COLUMN IF EXISTS method,
    DROP COLUMN IF EXISTS verified;

ALTER TABLE creator_applications
    DROP COLUMN IF EXISTS verification_code;
