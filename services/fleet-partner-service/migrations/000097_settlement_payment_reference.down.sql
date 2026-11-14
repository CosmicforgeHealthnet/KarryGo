ALTER TABLE fleet_partner_managed_work_settlement_instructions
    DROP COLUMN IF EXISTS payment_source_reference;

ALTER TABLE fleet_partner_completed_work_records
    DROP COLUMN IF EXISTS payment_source_reference;
