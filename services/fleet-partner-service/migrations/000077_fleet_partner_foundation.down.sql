DROP TABLE IF EXISTS fleet_partner_reconciliation_items;
DROP TABLE IF EXISTS fleet_partner_compensation_adjustments;
DROP TABLE IF EXISTS fleet_partner_managed_work_settlement_instructions;
DROP TABLE IF EXISTS fleet_partner_compensation_resolutions;
DROP TABLE IF EXISTS fleet_partner_completed_work_records;
DROP TABLE IF EXISTS fleet_partner_work_sessions;
DROP INDEX IF EXISTS uq_fleet_member_comp_assignment_open;
DROP TABLE IF EXISTS fleet_partner_membership_compensation_assignments;
DROP TABLE IF EXISTS fleet_partner_compensation_plan_rules;
DROP TABLE IF EXISTS fleet_partner_compensation_plan_versions;
DROP TABLE IF EXISTS fleet_partner_compensation_plans;
ALTER TABLE fleet_partner_organization_settings
    DROP COLUMN IF EXISTS financial_visibility_policy,
    DROP COLUMN IF EXISTS operating_mode;
