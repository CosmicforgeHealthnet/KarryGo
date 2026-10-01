CREATE INDEX IF NOT EXISTS idx_customer_movement_outbox_reconcile
 ON customer_movement_outbox(source_ref,source_version DESC,id DESC);
