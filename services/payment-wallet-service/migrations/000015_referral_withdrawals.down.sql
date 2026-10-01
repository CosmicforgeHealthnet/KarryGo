ALTER TABLE withdrawals DROP CONSTRAINT IF EXISTS withdrawals_funding_source_check;
ALTER TABLE withdrawals DROP COLUMN IF EXISTS funding_source;
