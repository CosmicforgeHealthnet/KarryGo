-- Pre-custody Vendor-owned to platform delivery responsibility transition.
-- The checkout allocation remains immutable evidence; these effective fields
-- are the operational payable owner selected by an audited Wallet command.
ALTER TABLE commerce_payment_allocations
    ADD COLUMN effective_delivery_responsibility text,
    ADD COLUMN effective_vendor_delivery_payable_minor bigint,
    ADD COLUMN effective_provider_delivery_payable_minor bigint,
    ADD COLUMN fallback_platform_funding_minor bigint NOT NULL DEFAULT 0,
    ADD COLUMN delivery_transition_version bigint NOT NULL DEFAULT 0,
    ADD COLUMN delivery_transition_ref text NOT NULL DEFAULT '';

UPDATE commerce_payment_allocations
   SET effective_delivery_responsibility = delivery_responsibility,
       effective_vendor_delivery_payable_minor = vendor_delivery_payable_minor,
       effective_provider_delivery_payable_minor = provider_delivery_payable_minor;

ALTER TABLE commerce_payment_allocations
    ALTER COLUMN effective_delivery_responsibility SET NOT NULL,
    ALTER COLUMN effective_vendor_delivery_payable_minor SET NOT NULL,
    ALTER COLUMN effective_provider_delivery_payable_minor SET NOT NULL,
    ADD CONSTRAINT commerce_effective_delivery_responsibility_check
        CHECK (effective_delivery_responsibility IN ('NONE','PLATFORM','VENDOR')),
    ADD CONSTRAINT commerce_effective_vendor_delivery_payable_check
        CHECK (effective_vendor_delivery_payable_minor >= 0),
    ADD CONSTRAINT commerce_effective_provider_delivery_payable_check
        CHECK (effective_provider_delivery_payable_minor >= 0),
    ADD CONSTRAINT commerce_effective_delivery_total_check
        CHECK (effective_vendor_delivery_payable_minor + effective_provider_delivery_payable_minor =
               vendor_delivery_payable_minor + provider_delivery_payable_minor + fallback_platform_funding_minor),
    ADD CONSTRAINT commerce_fallback_platform_funding_check
        CHECK (fallback_platform_funding_minor >= 0);

-- Migration 000010 created this bound as an unnamed table CHECK. Replace it
-- with the effective payable bound so a platform-funded fallback can later be
-- released without mutating the immutable checkout allocation.
DO $$
DECLARE
    constraint_name name;
BEGIN
    SELECT conname
      INTO constraint_name
      FROM pg_constraint
     WHERE conrelid = 'commerce_payment_allocations'::regclass
       AND contype = 'c'
       AND pg_get_constraintdef(oid) LIKE '%delivery_released_minor%vendor_delivery_payable_minor%provider_delivery_payable_minor%'
       AND pg_get_constraintdef(oid) NOT LIKE '%effective_%'
     LIMIT 1;

    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE commerce_payment_allocations DROP CONSTRAINT %I', constraint_name);
    END IF;
END $$;

ALTER TABLE commerce_payment_allocations
    ADD CONSTRAINT commerce_delivery_release_effective_bound_check
        CHECK (delivery_released_minor <=
               effective_vendor_delivery_payable_minor + effective_provider_delivery_payable_minor);

CREATE TABLE commerce_delivery_responsibility_transitions (
    id text PRIMARY KEY,
    allocation_id text NOT NULL REFERENCES commerce_payment_allocations(id) ON DELETE RESTRICT,
    vendor_order_id text NOT NULL,
    operation_ref text NOT NULL,
    idempotency_key text NOT NULL,
    request_fingerprint text NOT NULL,
    from_responsibility text NOT NULL CHECK (from_responsibility IN ('VENDOR','PLATFORM')),
    to_responsibility text NOT NULL CHECK (to_responsibility = 'PLATFORM'),
    vendor_delivery_payable_before_minor bigint NOT NULL CHECK (vendor_delivery_payable_before_minor >= 0),
    provider_delivery_payable_before_minor bigint NOT NULL CHECK (provider_delivery_payable_before_minor >= 0),
    vendor_delivery_payable_after_minor bigint NOT NULL CHECK (vendor_delivery_payable_after_minor = 0),
    provider_delivery_payable_after_minor bigint NOT NULL CHECK (provider_delivery_payable_after_minor >= 0),
    platform_execution_cost_minor bigint NOT NULL CHECK (platform_execution_cost_minor >= 0),
    fallback_platform_funding_minor bigint NOT NULL CHECK (fallback_platform_funding_minor >= 0),
    customer_contribution_minor bigint NOT NULL CHECK (customer_contribution_minor >= 0),
    vendor_subsidy_minor bigint NOT NULL CHECK (vendor_subsidy_minor >= 0),
    platform_subsidy_minor bigint NOT NULL CHECK (platform_subsidy_minor >= 0),
    status text NOT NULL CHECK (status = 'APPLIED'),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (operation_ref),
    UNIQUE (idempotency_key),
    UNIQUE (allocation_id)
);

CREATE INDEX commerce_delivery_transition_vendor_order_idx
    ON commerce_delivery_responsibility_transitions(vendor_order_id, created_at DESC);
