DROP TABLE IF EXISTS commerce_mission_legs;
DROP TABLE IF EXISTS commerce_mission_assignment_attempts;
DROP TABLE IF EXISTS commerce_mission_operations;
DROP TABLE IF EXISTS commerce_mission_parcels;
DROP TABLE IF EXISTS commerce_mission_stops;

DROP INDEX IF EXISTS idx_commerce_mission_runtime_queue;
DROP INDEX IF EXISTS uq_commerce_mission_canonical_ref;
DROP INDEX IF EXISTS uq_commerce_mission_caller_operation;

ALTER TABLE commerce_missions
    DROP COLUMN IF EXISTS completed_at,
    DROP COLUMN IF EXISTS cancelled_at,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS reason_code,
    DROP COLUMN IF EXISTS promised_by,
    DROP COLUMN IF EXISTS current_provider_id,
    DROP COLUMN IF EXISTS trip_id,
    DROP COLUMN IF EXISTS request_snapshot,
    DROP COLUMN IF EXISTS funding_reference,
    DROP COLUMN IF EXISTS capability_version,
    DROP COLUMN IF EXISTS service_class,
    DROP COLUMN IF EXISTS mode,
    DROP COLUMN IF EXISTS quote_fingerprint,
    DROP COLUMN IF EXISTS quote_revision,
    DROP COLUMN IF EXISTS mission_slot_ref,
    DROP COLUMN IF EXISTS plan_fingerprint,
    DROP COLUMN IF EXISTS plan_version,
    DROP COLUMN IF EXISTS plan_ref,
    DROP COLUMN IF EXISTS source_version,
    DROP COLUMN IF EXISTS next_retry_at,
    DROP COLUMN IF EXISTS assignment_attempt,
    DROP COLUMN IF EXISTS queue_state,
    DROP COLUMN IF EXISTS runtime_state,
    DROP COLUMN IF EXISTS canonical_ref,
    DROP COLUMN IF EXISTS request_fingerprint,
    DROP COLUMN IF EXISTS operation_ref,
    DROP COLUMN IF EXISTS caller_service;

ALTER TABLE commerce_delivery_quotes DROP CONSTRAINT IF EXISTS commerce_delivery_quotes_v1_route_chk;
ALTER TABLE commerce_delivery_quotes DROP CONSTRAINT IF EXISTS commerce_delivery_quotes_serviceable_chk;
ALTER TABLE commerce_delivery_quotes ADD CONSTRAINT commerce_delivery_quotes_serviceable_chk CHECK (
    NOT serviceable OR (reason_code='SERVICEABLE' AND currency<>'' AND available_providers>0)
);
ALTER TABLE commerce_delivery_quotes
    DROP COLUMN IF EXISTS estimated_seconds,
    DROP COLUMN IF EXISTS distance_meters,
    DROP COLUMN IF EXISTS route_evidence_version,
    DROP COLUMN IF EXISTS route_provider,
    DROP COLUMN IF EXISTS capability_version,
    DROP COLUMN IF EXISTS service_class,
    DROP COLUMN IF EXISTS mode,
    DROP COLUMN IF EXISTS supply_state,
    DROP COLUMN IF EXISTS route_snapshot,
    DROP COLUMN IF EXISTS request_snapshot,
    DROP COLUMN IF EXISTS contract_version;

DROP INDEX IF EXISTS idx_dispatch_outbox_pending;
ALTER TABLE dispatch_marketplace_outbox DROP COLUMN IF EXISTS target_service;
CREATE INDEX idx_dispatch_outbox_pending
    ON dispatch_marketplace_outbox(next_attempt_at)
    WHERE status IN ('PENDING','FAILED');
