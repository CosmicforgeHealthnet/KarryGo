DELETE FROM fleet_partner_role_permissions WHERE permission_code = 'security.audit_chain.verify';
DELETE FROM fleet_partner_permissions WHERE code = 'security.audit_chain.verify';

DROP INDEX IF EXISTS idx_fleet_partner_audit_chain_partition;

ALTER TABLE fleet_partner_audit_records
    DROP COLUMN IF EXISTS causation_id,
    DROP COLUMN IF EXISTS approval_reference,
    DROP COLUMN IF EXISTS idempotency_key_reference,
    DROP COLUMN IF EXISTS step_up_reference,
    DROP COLUMN IF EXISTS chain_partition,
    DROP COLUMN IF EXISTS previous_entry_hash,
    DROP COLUMN IF EXISTS entry_hash,
    DROP COLUMN IF EXISTS hash_version;
