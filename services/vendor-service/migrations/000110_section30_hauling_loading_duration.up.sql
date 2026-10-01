-- Section 30 additive loading-duration evidence.
--
-- The checkout quote plan and the activated operational plan both preserve
-- this bounded scheduling value so a later mission creation/recovery pass does
-- not silently fall back to the legacy loading policy.

ALTER TABLE commerce_fulfilment_missions
  ADD COLUMN IF NOT EXISTS loading_duration_seconds BIGINT NOT NULL DEFAULT 0
    CHECK (loading_duration_seconds BETWEEN 0 AND 28800);

ALTER TABLE commerce_fulfilment_plan_missions
  ADD COLUMN IF NOT EXISTS loading_duration_seconds BIGINT NOT NULL DEFAULT 0
    CHECK (loading_duration_seconds BETWEEN 0 AND 28800);
