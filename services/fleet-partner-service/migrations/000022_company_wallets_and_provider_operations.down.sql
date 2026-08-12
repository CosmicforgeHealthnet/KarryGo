-- Rollback Section 13 company wallet and provider operation structures.

DELETE FROM fleet_partner_role_permissions
WHERE permission_code IN ('wallet.read', 'wallet.transactions.read', 'wallet.provider_subwallet.read',
                          'wallet.provider_allocations.create', 'wallet.provider_allocations.approve',
                          'wallet.provider_withdrawals.read', 'wallet.provider_withdrawals.approve',
                          'wallet.provider_withdrawals.reject', 'wallet.policies.manage');

DELETE FROM fleet_partner_permissions
WHERE code IN ('wallet.read', 'wallet.transactions.read', 'wallet.provider_subwallet.read',
               'wallet.provider_allocations.create', 'wallet.provider_allocations.approve',
               'wallet.provider_withdrawals.read', 'wallet.provider_withdrawals.approve',
               'wallet.provider_withdrawals.reject', 'wallet.policies.manage');

DROP INDEX IF EXISTS idx_fleet_wallet_transactions_subject;
DROP INDEX IF EXISTS idx_fleet_wallet_transactions_cursor;
DROP TABLE IF EXISTS fleet_partner_wallet_transaction_projections;

DROP TRIGGER IF EXISTS trg_fleet_withdrawal_history_append_only
    ON fleet_partner_provider_withdrawal_history;
DROP INDEX IF EXISTS idx_fleet_withdrawal_history_withdrawal;
DROP TABLE IF EXISTS fleet_partner_provider_withdrawal_history;

DROP INDEX IF EXISTS idx_fleet_withdrawal_requests_open;
DROP INDEX IF EXISTS idx_fleet_withdrawal_requests_status;
DROP INDEX IF EXISTS idx_fleet_withdrawal_requests_membership;
DROP INDEX IF EXISTS idx_fleet_withdrawal_requests_org;
DROP TABLE IF EXISTS fleet_partner_provider_withdrawal_requests;

DROP TRIGGER IF EXISTS trg_fleet_allocation_history_append_only
    ON fleet_partner_provider_allocation_history;
DROP INDEX IF EXISTS idx_fleet_allocation_history_instruction;
DROP TABLE IF EXISTS fleet_partner_provider_allocation_history;

DROP INDEX IF EXISTS idx_fleet_allocation_instructions_business_purpose;
DROP INDEX IF EXISTS idx_fleet_allocation_instructions_status;
DROP INDEX IF EXISTS idx_fleet_allocation_instructions_branch;
DROP INDEX IF EXISTS idx_fleet_allocation_instructions_membership;
DROP INDEX IF EXISTS idx_fleet_allocation_instructions_org;
DROP TABLE IF EXISTS fleet_partner_provider_allocation_instructions;

DROP TABLE IF EXISTS fleet_partner_organization_withdrawal_policies;

DROP INDEX IF EXISTS idx_fleet_partner_subwallet_links_active;

ALTER TABLE fleet_partner_provider_subwallet_links
    DROP CONSTRAINT IF EXISTS fleet_partner_provider_subwallet_links_freshness_check;
ALTER TABLE fleet_partner_provider_subwallet_links
    DROP CONSTRAINT IF EXISTS fleet_partner_provider_subwallet_links_status_check;

-- Rows added while the extended states were available are normalized back into
-- the original vocabulary before the narrower constraint is restored; a
-- rollback that failed on its own data would leave the schema half-migrated.
UPDATE fleet_partner_provider_subwallet_links
SET status = CASE
        WHEN status IN ('PROVISIONING') THEN 'REQUESTED'
        WHEN status IN ('SUSPENDED', 'RESTRICTED', 'CLOSED') THEN 'DISABLED'
        WHEN status IN ('UNKNOWN', 'STALE', 'RECONCILING') THEN 'RECONCILIATION_REQUIRED'
        ELSE status
    END
WHERE status NOT IN ('NOT_REQUESTED', 'REQUESTED', 'ACTIVE', 'FAILED_RETRYABLE',
                     'FAILED_PERMANENT', 'RECONCILIATION_REQUIRED', 'DISABLED');

ALTER TABLE fleet_partner_provider_subwallet_links
    ADD CONSTRAINT fleet_partner_provider_subwallet_links_status_check
    CHECK (status IN ('NOT_REQUESTED', 'REQUESTED', 'ACTIVE', 'FAILED_RETRYABLE',
                      'FAILED_PERMANENT', 'RECONCILIATION_REQUIRED', 'DISABLED'));

ALTER TABLE fleet_partner_provider_subwallet_links
    DROP COLUMN IF EXISTS last_error_code,
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS freshness_state,
    DROP COLUMN IF EXISTS currency,
    DROP COLUMN IF EXISTS provider_service,
    DROP COLUMN IF EXISTS provider_id;

ALTER TABLE fleet_partner_organization_wallet_provisioning
    DROP CONSTRAINT IF EXISTS fleet_partner_organization_wallet_freshness_check;
ALTER TABLE fleet_partner_organization_wallet_provisioning
    DROP CONSTRAINT IF EXISTS fleet_partner_organization_wallet_provisioning_status_check;

UPDATE fleet_partner_organization_wallet_provisioning
SET status = CASE
        WHEN status IN ('SUSPENDED', 'RESTRICTED', 'CLOSED') THEN 'FAILED_PERMANENT'
        WHEN status IN ('UNKNOWN', 'STALE', 'RECONCILING') THEN 'RECONCILIATION_REQUIRED'
        ELSE status
    END
WHERE status NOT IN ('NOT_REQUESTED', 'REQUESTED', 'PROVISIONING', 'ACTIVE',
                     'FAILED_RETRYABLE', 'FAILED_PERMANENT', 'RECONCILIATION_REQUIRED');

ALTER TABLE fleet_partner_organization_wallet_provisioning
    ADD CONSTRAINT fleet_partner_organization_wallet_provisioning_status_check
    CHECK (status IN ('NOT_REQUESTED', 'REQUESTED', 'PROVISIONING', 'ACTIVE',
                      'FAILED_RETRYABLE', 'FAILED_PERMANENT', 'RECONCILIATION_REQUIRED'));

ALTER TABLE fleet_partner_organization_wallet_provisioning
    DROP COLUMN IF EXISTS correlation_id,
    DROP COLUMN IF EXISTS reconciliation_state,
    DROP COLUMN IF EXISTS freshness_state,
    DROP COLUMN IF EXISTS currency;
