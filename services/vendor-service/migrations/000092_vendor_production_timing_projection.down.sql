DROP INDEX IF EXISTS commerce_order_missions_timing_idx;
DROP INDEX IF EXISTS commerce_vendor_order_readiness_eta_idx;

ALTER TABLE commerce_order_missions
    DROP COLUMN IF EXISTS logistics_timing_version,
    DROP COLUMN IF EXISTS logistics_timing_source,
    DROP COLUMN IF EXISTS logistics_timing_status,
    DROP COLUMN IF EXISTS logistics_eta_end,
    DROP COLUMN IF EXISTS logistics_eta_start,
    DROP COLUMN IF EXISTS scheduled_pickup_at,
    DROP COLUMN IF EXISTS pickup_not_before;

ALTER TABLE commerce_vendor_order_readiness
    DROP COLUMN IF EXISTS timing_source_evidence,
    DROP COLUMN IF EXISTS timing_revision,
    DROP COLUMN IF EXISTS customer_eta_end,
    DROP COLUMN IF EXISTS customer_eta_start,
    DROP COLUMN IF EXISTS customer_eta_status,
    DROP COLUMN IF EXISTS logistics_window_end,
    DROP COLUMN IF EXISTS logistics_window_start;
