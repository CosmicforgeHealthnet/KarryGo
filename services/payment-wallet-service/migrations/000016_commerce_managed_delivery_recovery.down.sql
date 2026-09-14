-- Restore the old one-release-per-allocation index only where the recorded
-- data still satisfies it.
--
-- A database that actually delivered an order with two missions holds two
-- DELIVERY_RELEASE rows for one allocation. Recreating the old index there
-- would fail, and the rollback with it -- so a rollback would be impossible on
-- exactly the databases that used the feature. Where the data permits it the
-- old shape is restored exactly; where it does not, the mission-scoped index
-- stays and says so, because the alternative is deleting records of money that
-- moved.
DO $$
BEGIN
    BEGIN
        CREATE UNIQUE INDEX commerce_release_allocation_type_idx
            ON commerce_financial_operations(allocation_id, operation_type)
            WHERE operation_type IN ('VENDOR_RELEASE', 'DELIVERY_RELEASE');
        DROP INDEX IF EXISTS commerce_delivery_release_mission_idx;
        DROP INDEX IF EXISTS commerce_vendor_release_allocation_idx;
    EXCEPTION WHEN unique_violation THEN
        RAISE NOTICE 'multi-mission delivery releases exist; keeping the mission-scoped release index';
    END;
END $$;

DROP INDEX IF EXISTS commerce_dispute_allocation_item_idx;

-- DISPUTE_REVERSAL stays in the constraint on the way down, deliberately.
--
-- Rolling back the code stops new dispute reversals being created, which is
-- what a rollback is for. Narrowing the constraint would instead make the
-- schema reject financial operations that already happened -- rows recording
-- money that really moved -- and the rollback itself would fail against any
-- database where one had been recorded. A rollback that cannot run on a
-- database that used the feature is not a rollback.

ALTER TABLE commerce_delivery_executions
    DROP CONSTRAINT IF EXISTS commerce_delivery_execution_recovery_bounds;
ALTER TABLE commerce_delivery_executions
    DROP COLUMN IF EXISTS refunded_organization_minor,
    DROP COLUMN IF EXISTS refunded_provider_minor;

ALTER TABLE commerce_payment_allocations
    DROP COLUMN IF EXISTS refund_organization_recovery_minor;
