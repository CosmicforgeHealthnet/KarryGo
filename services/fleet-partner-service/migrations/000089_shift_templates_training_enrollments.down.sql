DROP TABLE IF EXISTS fleet_partner_training_enrollments;
DROP INDEX IF EXISTS uq_fleet_partner_shifts_template_day;
ALTER TABLE fleet_partner_shifts DROP COLUMN IF EXISTS template_local_date;
ALTER TABLE fleet_partner_shifts DROP COLUMN IF EXISTS template_id;
DROP TABLE IF EXISTS fleet_partner_shift_templates;
