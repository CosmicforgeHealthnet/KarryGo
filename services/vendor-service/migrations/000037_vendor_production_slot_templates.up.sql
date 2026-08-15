ALTER TABLE production_slot_templates
    ADD COLUMN IF NOT EXISTS timezone text NOT NULL DEFAULT 'UTC',
    ADD COLUMN IF NOT EXISTS generation_horizon_days integer NOT NULL DEFAULT 90
        CHECK (generation_horizon_days BETWEEN 1 AND 366);

ALTER TABLE production_slot_templates
    ADD CONSTRAINT production_slot_templates_timezone_check
        CHECK (char_length(timezone) BETWEEN 3 AND 100);
