-- Section 20 continuation: authoritative timing projections.
--
-- Production remains the source of production readiness, logistics remains the
-- source of mission timing, and this Orders-owned projection records the
-- customer-facing result without rewriting the immutable commercial quote.
ALTER TABLE commerce_vendor_order_readiness
    ADD COLUMN IF NOT EXISTS logistics_window_start timestamptz,
    ADD COLUMN IF NOT EXISTS logistics_window_end timestamptz,
    ADD COLUMN IF NOT EXISTS customer_eta_status text NOT NULL DEFAULT 'NOT_AVAILABLE'
        CHECK (customer_eta_status IN ('AVAILABLE','NOT_AVAILABLE')),
    ADD COLUMN IF NOT EXISTS customer_eta_start timestamptz,
    ADD COLUMN IF NOT EXISTS customer_eta_end timestamptz,
    ADD COLUMN IF NOT EXISTS timing_revision bigint NOT NULL DEFAULT 1 CHECK (timing_revision > 0),
    ADD COLUMN IF NOT EXISTS timing_source_evidence jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(timing_source_evidence) = 'object');

ALTER TABLE commerce_order_missions
    ADD COLUMN IF NOT EXISTS pickup_not_before timestamptz,
    ADD COLUMN IF NOT EXISTS scheduled_pickup_at timestamptz,
    ADD COLUMN IF NOT EXISTS logistics_eta_start timestamptz,
    ADD COLUMN IF NOT EXISTS logistics_eta_end timestamptz,
    ADD COLUMN IF NOT EXISTS logistics_timing_status text NOT NULL DEFAULT 'NOT_AVAILABLE'
        CHECK (logistics_timing_status IN ('AVAILABLE','NOT_AVAILABLE','APPLIED','NO_CHANGE','TOO_LATE_TO_RESCHEDULE','TERMINAL','UNKNOWN')),
    ADD COLUMN IF NOT EXISTS logistics_timing_source text,
    ADD COLUMN IF NOT EXISTS logistics_timing_version bigint NOT NULL DEFAULT 1 CHECK (logistics_timing_version > 0);

CREATE INDEX IF NOT EXISTS commerce_vendor_order_readiness_eta_idx
    ON commerce_vendor_order_readiness (business_id, customer_eta_status, customer_eta_end, vendor_order_id);

CREATE INDEX IF NOT EXISTS commerce_order_missions_timing_idx
    ON commerce_order_missions (parent_order_id, logistics_timing_status, updated_at);
