-- Referral balances and funding-aware wallet payments.
--
-- Referral credits land in dedicated accounts (customer_referral_available,
-- provider_referral_available, customer_promotional, provider_promotional),
-- funded by the debit-normal platform_referral_expense account. Accounts are
-- created on demand by the application, so no rows are seeded here.
--
-- funding_breakdown records which balances paid a wallet-funded job
-- ({"customer_promotional": 20000, "customer_available": 80000}), so a refund
-- returns money to the balance it came from instead of turning spend-only
-- promotional credit into ordinary wallet money.
ALTER TABLE payment_intents
    ADD COLUMN IF NOT EXISTS funding_breakdown jsonb NOT NULL DEFAULT '{}'::jsonb;
