-- Taxi Goods plan binding compatibility model.
--
-- Legacy quote-bound missions (created before Vendor sent a fulfilment plan,
-- or by callers that still only send a quote) have no plan identity. They are
-- recorded explicitly as LEGACY_QUOTE with an empty plan_ref and version 0,
-- never with a generated placeholder that could collide with a real plan in
-- uq_taxi_goods_plan_slot. PLAN_BOUND missions always carry a real plan_ref
-- and a plan_version >= 1.
ALTER TABLE taxi_goods_missions
  ADD COLUMN IF NOT EXISTS plan_binding_mode TEXT NOT NULL DEFAULT 'LEGACY_QUOTE';

UPDATE taxi_goods_missions
   SET plan_binding_mode = 'PLAN_BOUND'
 WHERE plan_ref <> '' AND plan_version >= 1 AND plan_binding_mode <> 'PLAN_BOUND';

ALTER TABLE taxi_goods_missions DROP CONSTRAINT IF EXISTS taxi_goods_missions_plan_binding_mode_check;
ALTER TABLE taxi_goods_missions ADD CONSTRAINT taxi_goods_missions_plan_binding_mode_check CHECK (
  (plan_binding_mode = 'LEGACY_QUOTE' AND plan_ref = '' AND plan_version = 0 AND quote_id IS NOT NULL)
  OR (plan_binding_mode = 'PLAN_BOUND' AND plan_ref <> '' AND plan_version >= 1)
);
