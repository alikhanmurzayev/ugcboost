-- +goose Up
-- Целевая 7-статусная стейт-машина заявки креатора
-- (см. _bmad-output/planning-artifacts/creator-application-state-machine.md):
--   active: verification, moderation, awaiting_contract, contract_sent
--   terminal: signed, rejected, withdrawn
--
-- Маппинг старых значений:
--   pending  → verification (backfill)
--   rejected → rejected     (терминал, 1:1)
--   approved → ABORT (нет 1:1 в новой модели — bizdev должен решить)
--   blocked  → ABORT (нет 1:1 в новой модели — bizdev должен решить)
--
-- Стратегия Up: fail-fast guard на approved/blocked → транзитный CHECK
-- (старые + новые) → backfill pending → verification → финальный CHECK
-- (только новые) → перестройка partial unique index по 4 активным.
--
-- ВНИМАНИЕ: эта миграция должна выполняться в одной транзакции
-- (default goose-поведение). Не добавлять goose-аннотацию NO TRANSACTION
-- сверху файла: промежуточные состояния (DROP CHECK без ADD, DROP INDEX
-- без CREATE) не должны быть наблюдаемы извне.

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM creator_applications WHERE status IN ('approved', 'blocked')) THEN
        RAISE EXCEPTION 'creator_applications has rows with status approved/blocked; no 1:1 mapping in target 7-state machine, manual review required before applying this migration';
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE creator_applications
    DROP CONSTRAINT IF EXISTS creator_applications_status_check;

ALTER TABLE creator_applications
    ADD CONSTRAINT creator_applications_status_check
    CHECK (status IN (
        'pending', 'approved', 'rejected', 'blocked',
        'verification', 'moderation', 'awaiting_contract', 'contract_sent',
        'signed', 'withdrawn'
    ));

UPDATE creator_applications SET status = 'verification' WHERE status = 'pending';

ALTER TABLE creator_applications
    DROP CONSTRAINT IF EXISTS creator_applications_status_check;

ALTER TABLE creator_applications
    ADD CONSTRAINT creator_applications_status_check
    CHECK (status IN (
        'verification', 'moderation', 'awaiting_contract', 'contract_sent',
        'signed', 'rejected', 'withdrawn'
    ));

DROP INDEX IF EXISTS creator_applications_iin_active_idx;

CREATE UNIQUE INDEX creator_applications_iin_active_idx
    ON creator_applications(iin)
    WHERE status IN ('verification', 'moderation', 'awaiting_contract', 'contract_sent');

-- +goose Down
-- Down безопасен только до первого реального перехода. Ряды со статусом
-- moderation/awaiting_contract/contract_sent/signed/withdrawn не имеют
-- 1:1-аналога в старой 4-статусной модели и валят финальный CHECK.
-- Ряды со статусом rejected откатываются 1:1 (терминал есть в обеих
-- моделях). verification → pending — обратный backfill.

DROP INDEX IF EXISTS creator_applications_iin_active_idx;

ALTER TABLE creator_applications
    DROP CONSTRAINT IF EXISTS creator_applications_status_check;

ALTER TABLE creator_applications
    ADD CONSTRAINT creator_applications_status_check
    CHECK (status IN (
        'pending', 'approved', 'rejected', 'blocked',
        'verification', 'moderation', 'awaiting_contract', 'contract_sent',
        'signed', 'withdrawn'
    ));

UPDATE creator_applications SET status = 'pending' WHERE status = 'verification';

ALTER TABLE creator_applications
    DROP CONSTRAINT IF EXISTS creator_applications_status_check;

ALTER TABLE creator_applications
    ADD CONSTRAINT creator_applications_status_check
    CHECK (status IN ('pending', 'approved', 'rejected', 'blocked'));

CREATE UNIQUE INDEX creator_applications_iin_active_idx
    ON creator_applications(iin)
    WHERE status IN ('pending', 'approved', 'blocked');
