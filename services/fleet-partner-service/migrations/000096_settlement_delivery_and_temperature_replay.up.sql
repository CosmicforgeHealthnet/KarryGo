-- FLEET-13.11: deliver managed settlement instructions to payment-wallet.
--
-- An instruction used to stop at CREATED: nothing ever handed it to the money
-- authority. The delivery worker now drives it to POSTED (Wallet confirmed the
-- journal) or FAILED_TERMINAL (Wallet refused it as stated; resending it
-- unchanged cannot help). PENDING_WALLET and RECONCILIATION_REQUIRED keep
-- their existing meaning: a transient failure, and an outcome that must be
-- read back rather than guessed.
ALTER TABLE fleet_partner_managed_work_settlement_instructions
    DROP CONSTRAINT IF EXISTS fleet_partner_managed_work_settlement_instructions_status_check;
ALTER TABLE fleet_partner_managed_work_settlement_instructions
    ADD CONSTRAINT fleet_partner_managed_work_settlement_instructions_status_check
    CHECK (status IN ('CREATED','PENDING_WALLET','RECONCILIATION_REQUIRED','POSTED','FAILED_TERMINAL','SUPERSEDED','CANCELLED'));

ALTER TABLE fleet_partner_managed_work_settlement_instructions
    ADD COLUMN IF NOT EXISTS wallet_posting_operation_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS wallet_settlement_reference text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS delivery_attempts integer NOT NULL DEFAULT 0 CHECK (delivery_attempts >= 0),
    ADD COLUMN IF NOT EXISTS last_error_code text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS posted_at timestamptz,
    ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

-- Instructions the recovery sweep still has to deliver.
CREATE INDEX IF NOT EXISTS idx_fleet_settlement_instruction_undelivered
    ON fleet_partner_managed_work_settlement_instructions (created_at)
    WHERE status IN ('CREATED','PENDING_WALLET','RECONCILIATION_REQUIRED');

-- FLEET-20.7: a temperature reading is replayed per record, not per
-- organization. One sensor may serve several cold-chain records over its
-- life, and the same (sensor, source version) sent against a second record
-- previously returned the first record's event instead of being recorded.
ALTER TABLE fleet_partner_temperature_events
    DROP CONSTRAINT IF EXISTS fleet_partner_temperature_eve_organization_id_sensor_refere_key;
ALTER TABLE fleet_partner_temperature_events
    ADD CONSTRAINT fleet_partner_temperature_events_record_replay_key
    UNIQUE (organization_id, record_id, sensor_reference, source_version);
