-- Default withdrawal account support for provider bank accounts.
-- Accounts are soft-removed (status = 'removed') because withdrawals hold a
-- NOT NULL foreign key to provider_bank_accounts.

ALTER TABLE provider_bank_accounts
    ADD COLUMN IF NOT EXISTS is_default boolean NOT NULL DEFAULT false;

-- At most one default account per provider.
CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_bank_accounts_default
    ON provider_bank_accounts (provider_type, provider_id)
    WHERE is_default;

-- Backfill: providers with existing accounts get their newest active account
-- as the default so the app always has a preselected payout account.
UPDATE provider_bank_accounts pba
SET is_default = true
WHERE pba.status = 'active'
  AND pba.id = (
      SELECT other.id
      FROM provider_bank_accounts other
      WHERE other.provider_type = pba.provider_type
        AND other.provider_id = pba.provider_id
        AND other.status = 'active'
      ORDER BY other.created_at DESC
      LIMIT 1
  )
  AND NOT EXISTS (
      SELECT 1
      FROM provider_bank_accounts existing
      WHERE existing.provider_type = pba.provider_type
        AND existing.provider_id = pba.provider_id
        AND existing.is_default
  );
