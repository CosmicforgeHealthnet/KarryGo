DROP TABLE IF EXISTS commerce_delivery_responsibility_transitions;

ALTER TABLE commerce_payment_allocations
    DROP CONSTRAINT IF EXISTS commerce_delivery_release_effective_bound_check,
    DROP CONSTRAINT IF EXISTS commerce_effective_delivery_total_check,
    DROP CONSTRAINT IF EXISTS commerce_fallback_platform_funding_check,
    DROP CONSTRAINT IF EXISTS commerce_effective_provider_delivery_payable_check,
    DROP CONSTRAINT IF EXISTS commerce_effective_vendor_delivery_payable_check,
    DROP CONSTRAINT IF EXISTS commerce_effective_delivery_responsibility_check,
    DROP COLUMN IF EXISTS delivery_transition_ref,
    DROP COLUMN IF EXISTS delivery_transition_version,
    DROP COLUMN IF EXISTS effective_provider_delivery_payable_minor,
    DROP COLUMN IF EXISTS fallback_platform_funding_minor,
    DROP COLUMN IF EXISTS effective_vendor_delivery_payable_minor,
    DROP COLUMN IF EXISTS effective_delivery_responsibility;

ALTER TABLE commerce_payment_allocations
    ADD CONSTRAINT commerce_delivery_release_original_bound_check
        CHECK (delivery_released_minor <= vendor_delivery_payable_minor + provider_delivery_payable_minor);
