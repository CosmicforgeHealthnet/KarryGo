DELETE FROM fleet_partner_role_permissions WHERE permission_code IN ('rescue.read','rescue.manage','assets.costs.read','assets.costs.manage');
DELETE FROM fleet_partner_permissions WHERE code IN ('rescue.read','rescue.manage','assets.costs.read','assets.costs.manage');

DROP TABLE IF EXISTS fleet_partner_asset_cost_entries;
DROP TABLE IF EXISTS fleet_partner_tyre_events;
DROP TABLE IF EXISTS fleet_partner_tyres;
DROP TABLE IF EXISTS fleet_partner_asset_defect_history;
DROP TABLE IF EXISTS fleet_partner_asset_defects;
DROP TABLE IF EXISTS fleet_partner_meter_readings;

ALTER TABLE fleet_partner_asset_assignments DROP COLUMN IF EXISTS end_reason;
ALTER TABLE fleet_partner_asset_assignments DROP COLUMN IF EXISTS ended_by;

-- The original alert_type list cannot hold the widened types; alerts of those
-- types (and their history) are removed before the constraint is restored.
DELETE FROM fleet_partner_operational_alert_history WHERE alert_id IN (SELECT id FROM fleet_partner_operational_alerts WHERE alert_type IN (
    'RESCUE_REQUIRED','ASSET_BREAKDOWN','CRITICAL_INSPECTION','CRITICAL_MAINTENANCE_OVERDUE','PART_REORDER_REQUIRED',
    'UNSAFE_VEHICLE_DECLINE','CRITICAL_DEFECT','METER_READING_ANOMALY'));
DELETE FROM fleet_partner_operational_alerts WHERE alert_type IN (
    'RESCUE_REQUIRED','ASSET_BREAKDOWN','CRITICAL_INSPECTION','CRITICAL_MAINTENANCE_OVERDUE','PART_REORDER_REQUIRED',
    'UNSAFE_VEHICLE_DECLINE','CRITICAL_DEFECT','METER_READING_ANOMALY');
ALTER TABLE fleet_partner_operational_alerts DROP CONSTRAINT fleet_partner_operational_alerts_alert_type_check;
ALTER TABLE fleet_partner_operational_alerts ADD CONSTRAINT fleet_partner_operational_alerts_alert_type_check CHECK (alert_type IN (
    'ASSIGNMENT_EXPIRED','PROVIDER_DECLINED','NO_ELIGIBLE_PROVIDER','ROUTE_DELAY','ROUTE_DEVIATION','GEOFENCE_ENTRY','GEOFENCE_EXIT',
    'TRACKING_STALE','TRACKING_GAP','VEHICLE_OR_TRUCK_UNAVAILABLE','SHIPMENT_AT_RISK','SLA_AT_RISK','EXECUTION_FAILED',
    'CANCELLATION_REQUIRES_RECONCILIATION'));

DROP TABLE IF EXISTS fleet_partner_rescue_case_history;
DROP TABLE IF EXISTS fleet_partner_rescue_cases;
