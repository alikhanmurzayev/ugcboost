-- +goose Up
-- State-machine v2 заявки креатора
-- (см. _bmad-output/planning-artifacts/creator-application-state-machine.md):
--   active:   verification, moderation
--   terminal: approved, rejected, withdrawn
--
-- Дропаемые статусы относительно v1 (миграция 20260501222829):
--   awaiting_contract, contract_sent, signed
-- Pipeline вокруг договора уехал в campaign-roadmap (см. revision
-- от 2026-05-03 в creator-onboarding-roadmap.md). Backfill не нужен:
-- prod-данных в дропаемых статусах нет — ни одна заявка туда не доходила.
--
-- Стратегия Up: fail-fast guard на awaiting_contract/contract_sent/signed
-- → транзитный CHECK (старые + approved) → финальный CHECK (только v2)
-- → перестройка partial unique index по 2 активным.
--
-- ВНИМАНИЕ: эта миграция должна выполняться в одной транзакции
-- (default goose-поведение). Не добавлять goose-аннотацию NO TRANSACTION
-- сверху файла: промежуточные состояния (DROP CHECK без ADD, DROP INDEX
-- без CREATE) не должны быть наблюдаемы извне.

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM creator_applications WHERE status IN ('awaiting_contract', 'contract_sent', 'signed')) THEN
        RAISE EXCEPTION 'creator_applications has rows with status awaiting_contract/contract_sent/signed; no automatic mapping in v2 state-machine, manual review required before applying this migration';
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE creator_applications
    DROP CONSTRAINT IF EXISTS creator_applications_status_check;

ALTER TABLE creator_applications
    ADD CONSTRAINT creator_applications_status_check
    CHECK (status IN (
        'verification', 'moderation', 'awaiting_contract', 'contract_sent',
        'signed', 'rejected', 'withdrawn', 'approved'
    ));

ALTER TABLE creator_applications
    DROP CONSTRAINT IF EXISTS creator_applications_status_check;

ALTER TABLE creator_applications
    ADD CONSTRAINT creator_applications_status_check
    CHECK (status IN (
        'verification', 'moderation', 'approved', 'rejected', 'withdrawn'
    ));

DROP INDEX IF EXISTS creator_applications_iin_active_idx;

CREATE UNIQUE INDEX creator_applications_iin_active_idx
    ON creator_applications(iin)
    WHERE status IN ('verification', 'moderation');

-- +goose Down
-- Down безопасен только до подключения transition в `approved`
-- (chunk 18). Ряд со статусом approved не имеет 1:1-аналога в v1 —
-- guard ниже не пускает откат. Active-set partial unique index
-- возвращается к v1-варианту по 4 статусам.

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM creator_applications WHERE status = 'approved') THEN
        RAISE EXCEPTION 'creator_applications has rows with status approved; no 1:1 mapping in v1 state-machine, manual review required before reverting this migration';
    END IF;
END $$;
-- +goose StatementEnd

DROP INDEX IF EXISTS creator_applications_iin_active_idx;

ALTER TABLE creator_applications
    DROP CONSTRAINT IF EXISTS creator_applications_status_check;

ALTER TABLE creator_applications
    ADD CONSTRAINT creator_applications_status_check
    CHECK (status IN (
        'verification', 'moderation', 'awaiting_contract', 'contract_sent',
        'signed', 'rejected', 'withdrawn'
    ));

CREATE UNIQUE INDEX creator_applications_iin_active_idx
    ON creator_applications(iin)
    WHERE status IN ('verification', 'moderation', 'awaiting_contract', 'contract_sent');
