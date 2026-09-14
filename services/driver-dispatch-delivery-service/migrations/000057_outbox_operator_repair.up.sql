-- Operator repair audit for the Fleet delivery outbox.
--
-- Taxi and Hauling both expose an operator surface that lists stuck
-- completed-work deliveries and requeues a dead-lettered one. Dispatch had
-- the same outbox and the same worker but no surface, so repairing a
-- dead-lettered dispatch event meant editing this table by hand.
--
-- A requeue is a REDELIVERY of the original event, never a new one: the
-- payload, the aggregate version and the idempotency identity are all left
-- untouched. These columns record that a human intervened, which is the one
-- thing a plain status reset would lose.
ALTER TABLE dispatch_marketplace_outbox
    ADD COLUMN IF NOT EXISTS requeue_count INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_requeued_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS last_requeued_by TEXT NOT NULL DEFAULT '';

ALTER TABLE dispatch_marketplace_outbox
    ADD CONSTRAINT chk_dispatch_outbox_requeue_count CHECK (requeue_count >= 0);

-- Operators look for what is stuck, which is a scan over status and age.
CREATE INDEX IF NOT EXISTS idx_dispatch_outbox_operator_state
    ON dispatch_marketplace_outbox (target_service, status, created_at DESC);

COMMENT ON COLUMN dispatch_marketplace_outbox.requeue_count IS
    'How many times an operator returned this event to the queue. The event identity, payload and aggregate version are never changed by a requeue.';
