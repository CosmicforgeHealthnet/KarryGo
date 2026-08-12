-- Rollback: remove provider allocations, provider withdrawals and the
-- sub-wallet ownership metadata.
--
-- Journals posted by these flows are deliberately NOT deleted. A rollback of
-- the operation records must never rewrite the ledger; the postings remain and
-- stay balanced.

DELETE FROM finance_approval_policies WHERE operation = 'PROVIDER_WITHDRAWAL_APPROVAL';

DELETE FROM finance_role_permissions
WHERE permission_code IN ('finance.provider_allocation.post', 'finance.provider_allocation.reverse',
                          'finance.provider_withdrawal.request', 'finance.provider_withdrawal.approve',
                          'finance.provider_withdrawal.view');

DELETE FROM finance_permissions
WHERE code IN ('finance.provider_allocation.post', 'finance.provider_allocation.reverse',
               'finance.provider_withdrawal.request', 'finance.provider_withdrawal.approve',
               'finance.provider_withdrawal.view');

DROP INDEX IF EXISTS idx_finance_provider_withdrawals_open;
DROP INDEX IF EXISTS idx_finance_provider_withdrawals_lease;
DROP INDEX IF EXISTS idx_finance_provider_withdrawals_due;
DROP INDEX IF EXISTS idx_finance_provider_withdrawals_account;
DROP INDEX IF EXISTS idx_finance_provider_withdrawals_idempotency;
DROP TABLE IF EXISTS finance_provider_withdrawals;

DROP INDEX IF EXISTS idx_finance_provider_allocations_single_reversal;
DROP INDEX IF EXISTS idx_finance_provider_allocations_org;
DROP INDEX IF EXISTS idx_finance_provider_allocations_account;
DROP INDEX IF EXISTS idx_finance_provider_allocations_business_purpose;
DROP INDEX IF EXISTS idx_finance_provider_allocations_idempotency;
DROP TABLE IF EXISTS finance_provider_allocations;

DROP INDEX IF EXISTS idx_finance_earning_accounts_provider;
DROP INDEX IF EXISTS idx_finance_earning_accounts_org_wallet;
DROP INDEX IF EXISTS idx_finance_earning_accounts_membership;

ALTER TABLE finance_earning_accounts
    DROP COLUMN IF EXISTS provider_service_category,
    DROP COLUMN IF EXISTS organization_membership_id,
    DROP COLUMN IF EXISTS provider_id,
    DROP COLUMN IF EXISTS organization_wallet_id;
