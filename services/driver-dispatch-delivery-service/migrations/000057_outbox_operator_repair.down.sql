DROP INDEX IF EXISTS idx_dispatch_outbox_operator_state;

ALTER TABLE dispatch_marketplace_outbox
    DROP CONSTRAINT IF EXISTS chk_dispatch_outbox_requeue_count;

ALTER TABLE dispatch_marketplace_outbox
    DROP COLUMN IF EXISTS requeue_count,
    DROP COLUMN IF EXISTS last_requeued_at,
    DROP COLUMN IF EXISTS last_requeued_by;
