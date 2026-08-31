DROP TABLE IF EXISTS dispatch_manifest_custody_operations;
DROP TABLE IF EXISTS dispatch_replan_operations;
DROP INDEX IF EXISTS dispatch_manifest_one_active_logical;
DROP INDEX IF EXISTS dispatch_manifest_logical_version_unique;
DROP INDEX IF EXISTS dispatch_delivery_mission_logical_ref;
ALTER TABLE dispatch_delivery_missions DROP COLUMN IF EXISTS logical_mission_ref;
DROP INDEX IF EXISTS dispatch_parcel_scannable_unique;
DROP INDEX IF EXISTS dispatch_parcel_karrygo_ref_unique;
ALTER TABLE dispatch_parcels DROP COLUMN IF EXISTS scannable_code, DROP COLUMN IF EXISTS karrygo_parcel_ref;
DROP TRIGGER IF EXISTS dispatch_assignment_audit_append_only ON dispatch_assignment_audit;
DROP FUNCTION IF EXISTS dispatch_reject_assignment_audit_mutation();
DROP TABLE IF EXISTS dispatch_assignment_audit;
DROP INDEX IF EXISTS dispatch_queue_wallet_operation_unique;
ALTER TABLE dispatch_queue_entries
    DROP COLUMN IF EXISTS wallet_error_code,
    DROP COLUMN IF EXISTS wallet_state,
    DROP COLUMN IF EXISTS wallet_operation_ref,
    DROP COLUMN IF EXISTS serviceability_ref,
    DROP COLUMN IF EXISTS search_policy_version,
    DROP COLUMN IF EXISTS priority_policy_version,
    DROP COLUMN IF EXISTS priority_reason,
    DROP COLUMN IF EXISTS service_tier,
    DROP COLUMN IF EXISTS max_radius_km,
    DROP COLUMN IF EXISTS current_radius_km,
    DROP COLUMN IF EXISTS initial_radius_km,
    DROP COLUMN IF EXISTS strategy,
    DROP COLUMN IF EXISTS customer_decision_deadline;
DROP INDEX IF EXISTS dispatch_delivery_mission_one_booking;
DROP TABLE IF EXISTS dispatch_mission_execution_links;
