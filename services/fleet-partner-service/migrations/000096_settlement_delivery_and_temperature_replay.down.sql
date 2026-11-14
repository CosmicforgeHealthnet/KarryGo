-- Reverse 000096.
--
-- The temperature constraint goes back to one reading per (organization,
-- sensor, source version). This fails if two records already hold the same
-- sensor/version: the old schema cannot represent that data, and the rows are
-- append-only, so the down migration refuses rather than deleting evidence.
ALTER TABLE fleet_partner_temperature_events
    DROP CONSTRAINT IF EXISTS fleet_partner_temperature_events_record_replay_key;
ALTER TABLE fleet_partner_temperature_events
    ADD CONSTRAINT fleet_partner_temperature_eve_organization_id_sensor_refere_key
    UNIQUE (organization_id, sensor_reference, source_version);

DROP INDEX IF EXISTS idx_fleet_settlement_instruction_undelivered;

-- POSTED and FAILED_TERMINAL do not exist in the old status set. Neither can
-- be represented honestly as CREATED or PENDING_WALLET (that would invite a
-- second delivery), so both fall back to RECONCILIATION_REQUIRED: the money
-- authority must be read before anything acts on them again.
UPDATE fleet_partner_managed_work_settlement_instructions
    SET status = 'RECONCILIATION_REQUIRED'
    WHERE status IN ('POSTED','FAILED_TERMINAL');

ALTER TABLE fleet_partner_managed_work_settlement_instructions
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS posted_at,
    DROP COLUMN IF EXISTS last_error_code,
    DROP COLUMN IF EXISTS delivery_attempts,
    DROP COLUMN IF EXISTS wallet_settlement_reference,
    DROP COLUMN IF EXISTS wallet_posting_operation_id;

ALTER TABLE fleet_partner_managed_work_settlement_instructions
    DROP CONSTRAINT IF EXISTS fleet_partner_managed_work_settlement_instructions_status_check;
ALTER TABLE fleet_partner_managed_work_settlement_instructions
    ADD CONSTRAINT fleet_partner_managed_work_settlement_instructions_status_check
    CHECK (status IN ('CREATED','PENDING_WALLET','RECONCILIATION_REQUIRED','SUPERSEDED','CANCELLED'));
