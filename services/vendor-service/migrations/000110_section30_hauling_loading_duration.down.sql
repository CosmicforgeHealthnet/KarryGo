ALTER TABLE commerce_fulfilment_plan_missions
  DROP COLUMN IF EXISTS loading_duration_seconds;

ALTER TABLE commerce_fulfilment_missions
  DROP COLUMN IF EXISTS loading_duration_seconds;
