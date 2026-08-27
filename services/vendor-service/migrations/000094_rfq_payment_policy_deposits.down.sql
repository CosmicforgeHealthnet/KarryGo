DROP TABLE IF EXISTS commerce_rfq_payment_obligations;
ALTER TABLE commerce_rfq_checkout_bindings
    DROP COLUMN IF EXISTS payment_policy,
    DROP COLUMN IF EXISTS cancellation_policy;
ALTER TABLE commerce_rfq_quote_revisions
    DROP COLUMN IF EXISTS vendor_terms,
    DROP COLUMN IF EXISTS payment_policy,
    DROP COLUMN IF EXISTS deposit_policy,
    DROP COLUMN IF EXISTS cancellation_policy,
    DROP COLUMN IF EXISTS created_by;
