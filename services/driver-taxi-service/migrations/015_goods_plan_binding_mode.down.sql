ALTER TABLE taxi_goods_missions DROP CONSTRAINT IF EXISTS taxi_goods_missions_plan_binding_mode_check;
ALTER TABLE taxi_goods_missions DROP COLUMN IF EXISTS plan_binding_mode;
