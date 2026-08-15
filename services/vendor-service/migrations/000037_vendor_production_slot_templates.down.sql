ALTER TABLE production_slot_templates DROP CONSTRAINT IF EXISTS production_slot_templates_timezone_check;
ALTER TABLE production_slot_templates DROP COLUMN IF EXISTS generation_horizon_days;
ALTER TABLE production_slot_templates DROP COLUMN IF EXISTS timezone;
