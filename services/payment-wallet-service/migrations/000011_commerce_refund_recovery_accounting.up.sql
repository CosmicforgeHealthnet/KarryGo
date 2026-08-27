-- Allocation-local recovery accounting prevents a refund for one VendorOrder
-- from consuming another VendorOrder's balance in the shared parent escrow.
-- These are mutable execution counters; the immutable commercial snapshot
-- remains guarded by the 000010 trigger.

ALTER TABLE commerce_payment_allocations
    ADD COLUMN refund_customer_minor bigint NOT NULL DEFAULT 0 CHECK (refund_customer_minor >= 0),
    ADD COLUMN refund_platform_funding_minor bigint NOT NULL DEFAULT 0 CHECK (refund_platform_funding_minor >= 0),
    ADD COLUMN refund_other_funding_minor bigint NOT NULL DEFAULT 0 CHECK (refund_other_funding_minor >= 0),
    ADD COLUMN refund_escrow_recovery_minor bigint NOT NULL DEFAULT 0 CHECK (refund_escrow_recovery_minor >= 0),
    ADD COLUMN refund_vendor_recovery_minor bigint NOT NULL DEFAULT 0 CHECK (refund_vendor_recovery_minor >= 0),
    ADD COLUMN refund_provider_recovery_minor bigint NOT NULL DEFAULT 0 CHECK (refund_provider_recovery_minor >= 0),
    ADD COLUMN refund_platform_recovery_minor bigint NOT NULL DEFAULT 0 CHECK (refund_platform_recovery_minor >= 0),
    ADD COLUMN refund_platform_loss_funding_minor bigint NOT NULL DEFAULT 0 CHECK (refund_platform_loss_funding_minor >= 0),
    ADD COLUMN refund_vendor_merchandise_reduction_minor bigint NOT NULL DEFAULT 0 CHECK (refund_vendor_merchandise_reduction_minor >= 0),
    ADD COLUMN refund_vendor_delivery_reduction_minor bigint NOT NULL DEFAULT 0 CHECK (refund_vendor_delivery_reduction_minor >= 0),
    ADD COLUMN refund_provider_delivery_reduction_minor bigint NOT NULL DEFAULT 0 CHECK (refund_provider_delivery_reduction_minor >= 0),
    ADD COLUMN refund_platform_revenue_reduction_minor bigint NOT NULL DEFAULT 0 CHECK (refund_platform_revenue_reduction_minor >= 0),
    ADD COLUMN refund_authorization_release_minor bigint NOT NULL DEFAULT 0 CHECK (refund_authorization_release_minor >= 0);

ALTER TABLE commerce_payment_allocations
    ADD CONSTRAINT commerce_refund_customer_bound_chk CHECK (refund_customer_minor <= max_authorized_minor),
    ADD CONSTRAINT commerce_refund_platform_funding_bound_chk CHECK (refund_platform_funding_minor <= platform_product_discount_minor + platform_delivery_subsidy_minor),
    ADD CONSTRAINT commerce_refund_other_funding_bound_chk CHECK (refund_other_funding_minor <= other_funding_minor),
    ADD CONSTRAINT commerce_refund_escrow_recovery_bound_chk CHECK (refund_escrow_recovery_minor <= max_authorized_minor + platform_product_discount_minor + platform_delivery_subsidy_minor + other_funding_minor),
    ADD CONSTRAINT commerce_refund_vendor_recovery_bound_chk CHECK (refund_vendor_recovery_minor <= vendor_merchandise_payable_minor + vendor_delivery_payable_minor),
    ADD CONSTRAINT commerce_refund_provider_recovery_bound_chk CHECK (refund_provider_recovery_minor <= provider_delivery_payable_minor),
    ADD CONSTRAINT commerce_refund_platform_recovery_bound_chk CHECK (refund_platform_recovery_minor <= platform_revenue_minor),
    ADD CONSTRAINT commerce_refund_vendor_merchandise_reduction_chk CHECK (refund_vendor_merchandise_reduction_minor <= vendor_merchandise_payable_minor),
    ADD CONSTRAINT commerce_refund_vendor_delivery_reduction_chk CHECK (refund_vendor_delivery_reduction_minor <= vendor_delivery_payable_minor),
    ADD CONSTRAINT commerce_refund_provider_delivery_reduction_chk CHECK (refund_provider_delivery_reduction_minor <= provider_delivery_payable_minor),
    ADD CONSTRAINT commerce_refund_platform_revenue_reduction_chk CHECK (refund_platform_revenue_reduction_minor <= platform_revenue_minor),
    ADD CONSTRAINT commerce_refund_authorization_release_chk CHECK (refund_authorization_release_minor <= max_authorized_minor - customer_payable_minor);

COMMENT ON COLUMN commerce_payment_allocations.refund_platform_loss_funding_minor IS
    'Explicit platform expense used when policy preserves provider earning while refunding the customer; never customer or sibling escrow money.';
