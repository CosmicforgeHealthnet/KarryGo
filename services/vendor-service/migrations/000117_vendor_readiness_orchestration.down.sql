-- Reverse of 000117.
--
-- commerce_vendor_order_readiness itself survives: it predates this migration
-- (000091) and the production timing projection still writes to it. Only the
-- columns, constraints and trigger this migration added come off.
DELETE FROM vendor_permissions
 WHERE code IN ('vendor.readiness.read', 'vendor.readiness.manage',
                'vendor.orchestration.read', 'vendor.orchestration.override');

DROP TRIGGER IF EXISTS vendor_orchestration_events_append_only ON vendor_orchestration_events;
DROP TABLE IF EXISTS vendor_orchestration_events;
DROP FUNCTION IF EXISTS reject_orchestration_event_mutation();

DROP TRIGGER IF EXISTS vendor_orchestration_release_pickups ON vendor_fulfilment_orchestrations;
DROP INDEX IF EXISTS vendor_orchestration_pickups_live_idx;
DROP TABLE IF EXISTS vendor_orchestration_pickups;
DROP FUNCTION IF EXISTS release_orchestration_pickups();

DROP TRIGGER IF EXISTS vendor_orchestration_frozen_guard ON vendor_fulfilment_orchestrations;
DROP TABLE IF EXISTS vendor_fulfilment_orchestrations;
DROP FUNCTION IF EXISTS protect_frozen_requirements();

DROP TABLE IF EXISTS vendor_readiness_rules;
DROP TABLE IF EXISTS vendor_branch_readiness_profiles;

DROP TRIGGER IF EXISTS vendor_readiness_history_append_only ON vendor_readiness_history;
DROP TABLE IF EXISTS vendor_readiness_history;
DROP FUNCTION IF EXISTS reject_readiness_history_mutation();

DROP TRIGGER IF EXISTS commerce_vendor_order_readiness_first_ready_guard
    ON commerce_vendor_order_readiness;
DROP FUNCTION IF EXISTS protect_first_actual_ready_at();

DROP INDEX IF EXISTS commerce_vendor_order_readiness_signal_idx;
DROP INDEX IF EXISTS commerce_vendor_order_readiness_state_idx;

ALTER TABLE commerce_vendor_order_readiness
    DROP CONSTRAINT IF EXISTS commerce_vendor_order_readiness_window_check,
    DROP CONSTRAINT IF EXISTS commerce_vendor_order_readiness_version_check,
    DROP CONSTRAINT IF EXISTS commerce_vendor_order_readiness_source_check,
    DROP CONSTRAINT IF EXISTS commerce_vendor_order_readiness_confidence_check,
    DROP CONSTRAINT IF EXISTS commerce_vendor_order_readiness_state_check;

ALTER TABLE commerce_vendor_order_readiness
    DROP COLUMN IF EXISTS last_signal_at,
    DROP COLUMN IF EXISTS delay_reason,
    DROP COLUMN IF EXISTS block_reason,
    DROP COLUMN IF EXISTS source,
    DROP COLUMN IF EXISTS confidence_band,
    DROP COLUMN IF EXISTS confidence_score,
    DROP COLUMN IF EXISTS first_actual_ready_at,
    DROP COLUMN IF EXISTS actual_ready_at,
    DROP COLUMN IF EXISTS vendor_estimated_ready_at,
    DROP COLUMN IF EXISTS predicted_ready_end,
    DROP COLUMN IF EXISTS predicted_ready_start,
    DROP COLUMN IF EXISTS prep_started_at,
    DROP COLUMN IF EXISTS readiness_version,
    DROP COLUMN IF EXISTS substate,
    DROP COLUMN IF EXISTS state,
    DROP COLUMN IF EXISTS vendor_type;
