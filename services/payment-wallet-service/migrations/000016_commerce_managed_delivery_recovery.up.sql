-- Section 12 WAL-029/030: refunding and disputing a corporate Commerce delivery.
--
-- A refund used to take the delivery money back from one place, because a
-- platform delivery used to have one recipient. A corporate delivery has two:
-- the provider received what their compensation plan entitled them to -- often
-- nothing, because a salaried rider is paid by their employer -- and the
-- organization received the rest.
--
-- Recovering it all from the provider would claw back money a salaried rider
-- never had and leave the organization holding money that was refunded to the
-- customer. Both are real losses to a real party, and neither shows up in the
-- totals.

ALTER TABLE commerce_payment_allocations
    -- What has been recovered from the organization, mirroring the provider,
    -- vendor and platform recovery counters that already exist.
    ADD COLUMN IF NOT EXISTS refund_organization_recovery_minor bigint NOT NULL DEFAULT 0
        CHECK (refund_organization_recovery_minor >= 0);

ALTER TABLE commerce_delivery_executions
    -- Per-mission recovery, so the evidence a refund is computed against is the
    -- same evidence that recorded the original payment. Recomputing the split
    -- from today's compensation policy would apply a plan the provider was
    -- never paid under.
    ADD COLUMN IF NOT EXISTS refunded_provider_minor bigint NOT NULL DEFAULT 0
        CHECK (refunded_provider_minor >= 0),
    ADD COLUMN IF NOT EXISTS refunded_organization_minor bigint NOT NULL DEFAULT 0
        CHECK (refunded_organization_minor >= 0);

-- A party can never give back more than they were paid.
ALTER TABLE commerce_delivery_executions
    DROP CONSTRAINT IF EXISTS commerce_delivery_execution_recovery_bounds;
ALTER TABLE commerce_delivery_executions
    ADD CONSTRAINT commerce_delivery_execution_recovery_bounds
        CHECK (refunded_provider_minor <= provider_share_minor
           AND refunded_organization_minor <= organization_share_minor);

-- A dispute reversal is its own operation. It shares the recovery pool with an
-- ordinary refund -- they decrement the same counters -- so the two can never
-- between them take back more than was paid, in either order.
ALTER TABLE commerce_financial_operations
    DROP CONSTRAINT IF EXISTS commerce_financial_operations_operation_type_check;
ALTER TABLE commerce_financial_operations
    ADD CONSTRAINT commerce_financial_operations_operation_type_check
        CHECK (operation_type IN (
            'HOLD','VENDOR_RELEASE','DELIVERY_RELEASE','PARTIAL_REFUND','DISPUTE_REVERSAL'
        ));

-- One dispute reversal per allocation item, matching the refund rule.
CREATE UNIQUE INDEX IF NOT EXISTS commerce_dispute_allocation_item_idx
    ON commerce_financial_operations(allocation_id, item_reference)
    WHERE operation_type = 'DISPUTE_REVERSAL';

-- WAL-026. One vendor order can be delivered by more than one mission, and each
-- mission is its own authoritative release. The previous index allowed only one
-- DELIVERY_RELEASE per allocation, which made a second mission impossible --
-- the release failed outright rather than being recorded.
--
-- Vendor release keeps its one-per-allocation rule; delivery release becomes
-- one per mission. An independent delivery carries an empty mission reference
-- and so is still limited to one, exactly as before.
DROP INDEX IF EXISTS commerce_release_allocation_type_idx;

CREATE UNIQUE INDEX IF NOT EXISTS commerce_vendor_release_allocation_idx
    ON commerce_financial_operations(allocation_id)
    WHERE operation_type = 'VENDOR_RELEASE';

CREATE UNIQUE INDEX IF NOT EXISTS commerce_delivery_release_mission_idx
    ON commerce_financial_operations(allocation_id, item_reference)
    WHERE operation_type = 'DELIVERY_RELEASE';
