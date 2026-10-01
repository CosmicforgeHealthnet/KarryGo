-- Once operational PII has been minimized, removing this boundary would make
-- canonical retained evidence visible through ordinary order reads again.
-- Refuse that unsafe rollback; an operator must retain the schema or perform
-- an explicitly reviewed forward recovery.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM vendor_privacy_retention_jobs WHERE status='MINIMIZED') THEN
        RAISE EXCEPTION 'NO SAFE DOWN: privacy minimization has already executed';
    END IF;
END $$;

DROP TRIGGER IF EXISTS commerce_parent_orders_privacy_retention_trg ON commerce_parent_orders;
DROP FUNCTION IF EXISTS vendor_enqueue_order_privacy_retention();
DROP TABLE IF EXISTS vendor_privacy_retention_jobs;
DROP TABLE IF EXISTS commerce_order_restricted_evidence;
DROP TABLE IF EXISTS vendor_data_legal_holds;
DROP INDEX IF EXISTS vendor_privacy_retention_policy_active_uq;
DROP TABLE IF EXISTS vendor_privacy_retention_policies;

ALTER TABLE commerce_parent_orders
    DROP COLUMN IF EXISTS operational_receiver_snapshot,
    DROP COLUMN IF EXISTS operational_address_snapshot;

DROP INDEX IF EXISTS vendor_idempotency_keys_actor_operation_idx;
ALTER TABLE vendor_idempotency_keys
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS completed_at,
    DROP COLUMN IF EXISTS resource_target,
    DROP COLUMN IF EXISTS operation_type,
    DROP COLUMN IF EXISTS actor_ref;
