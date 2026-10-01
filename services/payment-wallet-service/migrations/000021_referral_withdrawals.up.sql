-- Which balance a withdrawal is paid from. Job earnings (provider_payable) is
-- every withdrawal so far; referral earnings (provider_referral_available)
-- has its own endpoint, minimum and eligibility check. A failed or cancelled
-- withdrawal is restored to the balance it came from.
ALTER TABLE withdrawals
    ADD COLUMN IF NOT EXISTS funding_source text NOT NULL DEFAULT 'job_earnings';
ALTER TABLE withdrawals DROP CONSTRAINT IF EXISTS withdrawals_funding_source_check;
ALTER TABLE withdrawals ADD CONSTRAINT withdrawals_funding_source_check
    CHECK (funding_source IN ('job_earnings', 'referral_earnings'));
