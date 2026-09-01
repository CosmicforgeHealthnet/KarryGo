-- Delivery recovery and route-integrity state. Existing Trip, commerce
-- exception/return/provider-transfer, secure handover, routing and Wallet
-- authorities remain authoritative; these tables record the additive workflow.

ALTER TABLE dispatch_handover_challenges DROP CONSTRAINT dispatch_handover_challenges_stage_check;
ALTER TABLE dispatch_handover_challenges ADD CONSTRAINT dispatch_handover_challenges_stage_check
    CHECK (stage IN ('PICKUP','DELIVERY','PROVIDER_TRANSFER','RETURN'));

CREATE TABLE dispatch_delivery_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id UUID NOT NULL REFERENCES trips(id) ON DELETE RESTRICT,
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    attempt_number INTEGER NOT NULL CHECK (attempt_number > 0),
    state TEXT NOT NULL DEFAULT 'GRACE_PERIOD' CHECK (state IN ('GRACE_PERIOD','RECEIVER_UNAVAILABLE','RECOVERY_SELECTED','RESOLVED','CANCELLED')),
    arrived_at TIMESTAMPTZ NOT NULL,
    grace_expires_at TIMESTAMPTZ NOT NULL,
    failure_reason TEXT NOT NULL DEFAULT '' CHECK (failure_reason IN ('','RECEIVER_NO_ANSWER','RECEIVER_UNREACHABLE','RECEIVER_REFUSED','INVALID_CONTACT','ACCESS_RESTRICTED','LOCATION_UNSAFE','DELIVERY_WINDOW_MISSED','OTHER_REVIEW_REQUIRED')),
    allowed_actions JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(allowed_actions)='array'),
    policy_version TEXT NOT NULL,
    source_version BIGINT NOT NULL CHECK (source_version > 0),
    idempotency_key TEXT NOT NULL UNIQUE,
    request_fingerprint TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (grace_expires_at > arrived_at),
    UNIQUE (trip_id, attempt_number)
);
CREATE UNIQUE INDEX dispatch_delivery_attempt_one_open
    ON dispatch_delivery_attempts(trip_id) WHERE state IN ('GRACE_PERIOD','RECEIVER_UNAVAILABLE','RECOVERY_SELECTED');

CREATE TABLE dispatch_delivery_attempt_operations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_attempt_id UUID NOT NULL REFERENCES dispatch_delivery_attempts(id) ON DELETE RESTRICT,
    operation_type TEXT NOT NULL CHECK (operation_type IN ('START','FAIL')),
    idempotency_key TEXT NOT NULL UNIQUE,
    request_fingerprint TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (delivery_attempt_id, operation_type)
);

CREATE TABLE dispatch_delivery_contact_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_attempt_id UUID NOT NULL REFERENCES dispatch_delivery_attempts(id) ON DELETE RESTRICT,
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    method TEXT NOT NULL CHECK (method IN ('PHONE','EMAIL','OTHER_SERVER_DEFINED_METHOD')),
    outcome TEXT NOT NULL CHECK (outcome IN ('UNREACHABLE','DECLINED','NUMBER_INVALID','CONTACTED_WAITING','CONTACTED_CANNOT_RECEIVE')),
    attempted_at TIMESTAMPTZ NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    request_fingerprint TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX dispatch_delivery_contact_attempt_history
    ON dispatch_delivery_contact_attempts(delivery_attempt_id, attempted_at, id);

CREATE TABLE dispatch_delivery_recovery_actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_attempt_id UUID NOT NULL REFERENCES dispatch_delivery_attempts(id) ON DELETE RESTRICT,
    trip_id UUID NOT NULL REFERENCES trips(id) ON DELETE RESTRICT,
    booking_id UUID NOT NULL REFERENCES dispatch_bookings(id) ON DELETE RESTRICT,
    customer_id UUID NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('RETRY_SAME_DAY','RESCHEDULE','ALTERNATIVE_RECEIVER','RETURN_TO_SENDER','HUB_HOLD','OPERATIONS_REVIEW')),
    state TEXT NOT NULL CHECK (state IN ('REQUESTED','SCHEDULED','READY','AWAITING_CUSTOMER_APPROVAL','AWAITING_FINANCIAL_AUTHORIZATION','RECONCILIATION_REQUIRED','COMPLETED','CANCELLED')),
    scheduled_for TIMESTAMPTZ NULL,
    alternative_receiver_ref TEXT NOT NULL DEFAULT '',
    hub_ref TEXT NOT NULL DEFAULT '',
    financial_operation_ref TEXT NOT NULL DEFAULT '',
    financial_state TEXT NOT NULL DEFAULT 'NOT_REQUIRED' CHECK (financial_state IN ('NOT_REQUIRED','APPROVED','REQUESTED','ACKNOWLEDGED','UNKNOWN','FAILED')),
    policy_version TEXT NOT NULL,
    expected_trip_version BIGINT NOT NULL CHECK (expected_trip_version > 0),
    idempotency_key TEXT NOT NULL UNIQUE,
    request_fingerprint TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX dispatch_delivery_recovery_one_active
    ON dispatch_delivery_recovery_actions(delivery_attempt_id)
    WHERE state NOT IN ('COMPLETED','CANCELLED');

CREATE TABLE dispatch_return_workflows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recovery_action_id UUID NOT NULL UNIQUE REFERENCES dispatch_delivery_recovery_actions(id) ON DELETE RESTRICT,
    original_trip_id UUID NOT NULL REFERENCES trips(id) ON DELETE RESTRICT,
    delivery_group_id UUID NULL REFERENCES dispatch_delivery_groups(id) ON DELETE RESTRICT,
    original_mission_id UUID NULL REFERENCES dispatch_delivery_missions(id) ON DELETE RESTRICT,
    return_mission_id UUID NULL UNIQUE REFERENCES dispatch_delivery_missions(id) ON DELETE RESTRICT,
    return_booking_id UUID NULL UNIQUE REFERENCES dispatch_bookings(id) ON DELETE RESTRICT,
    parcel_ids UUID[] NOT NULL DEFAULT '{}',
    route_reference TEXT NOT NULL,
    route_snapshot JSONB NOT NULL CHECK (jsonb_typeof(route_snapshot)='object'),
    wallet_operation_ref TEXT NOT NULL DEFAULT '',
    wallet_state TEXT NOT NULL DEFAULT 'NOT_REQUIRED' CHECK (wallet_state IN ('NOT_REQUIRED','APPROVED','REQUESTED','ACKNOWLEDGED','UNKNOWN','FAILED')),
    state TEXT NOT NULL DEFAULT 'REQUIRED' CHECK (state IN ('REQUIRED','ROUTED','RETURNING','HANDOVER_REQUIRED','RETURNED','SUPPORT_REQUIRED')),
    policy_version TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE dispatch_hub_hold_operations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recovery_action_id UUID NOT NULL UNIQUE REFERENCES dispatch_delivery_recovery_actions(id) ON DELETE RESTRICT,
    trip_id UUID NOT NULL REFERENCES trips(id) ON DELETE RESTRICT,
    hub_ref TEXT NOT NULL,
    hub_authority_ref TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('APPROVED','EN_ROUTE','CUSTODY_PENDING','HELD','PICKUP_PENDING','RELEASED','CANCELLED')),
    custody_operation_ref TEXT NOT NULL DEFAULT '',
    evidence_ref TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE dispatch_hub_hold_checkpoints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hub_hold_operation_id UUID NOT NULL REFERENCES dispatch_hub_hold_operations(id) ON DELETE RESTRICT,
    trip_id UUID NOT NULL REFERENCES trips(id) ON DELETE RESTRICT,
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    stage TEXT NOT NULL CHECK (stage IN ('ARRIVAL','RELEASE')),
    manifest_ref TEXT NOT NULL,
    parcel_codes TEXT[] NOT NULL CHECK (cardinality(parcel_codes) > 0),
    media_ref TEXT NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    request_fingerprint TEXT NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,
    UNIQUE (hub_hold_operation_id, stage)
);

CREATE TABLE dispatch_delivery_incidents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id UUID NOT NULL REFERENCES trips(id) ON DELETE RESTRICT,
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    reason_code TEXT NOT NULL CHECK (reason_code IN ('BREAKDOWN','VEHICLE_FAILURE','ILLNESS','SAFETY_CONCERN','ACCIDENT','OPERATIONS_DIRECTED_REPLACEMENT','OTHER_REVIEW_REQUIRED')),
    state TEXT NOT NULL DEFAULT 'REPORTED' CHECK (state IN ('REPORTED','REASSIGNING','TRANSFER_PENDING','RESOLVED','EMERGENCY_RECOVERY','CANCELLED')),
    replacement_strategy TEXT NOT NULL DEFAULT 'SEQUENTIAL_NEAREST' CHECK (replacement_strategy IN ('SEQUENTIAL_NEAREST','ORGANIZATION_DISPATCH','ADMIN_ASSISTED')),
    safe_note TEXT NOT NULL DEFAULT '',
    source_version BIGINT NOT NULL CHECK (source_version > 0),
    idempotency_key TEXT NOT NULL UNIQUE,
    request_fingerprint TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX dispatch_delivery_incident_one_active
    ON dispatch_delivery_incidents(trip_id) WHERE state NOT IN ('RESOLVED','CANCELLED');

CREATE TABLE dispatch_emergency_recovery_actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id UUID NOT NULL REFERENCES dispatch_delivery_incidents(id) ON DELETE RESTRICT,
    trip_id UUID NOT NULL REFERENCES trips(id) ON DELETE RESTRICT,
    actor_ref TEXT NOT NULL,
    reason_code TEXT NOT NULL,
    safe_note TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'OPERATIONS_CONTROLLED' CHECK (state IN ('OPERATIONS_CONTROLLED','RESOLVED','CANCELLED')),
    idempotency_key TEXT NOT NULL UNIQUE,
    request_fingerprint TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE dispatch_expected_routes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id UUID NOT NULL REFERENCES trips(id) ON DELETE RESTRICT,
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    version BIGINT NOT NULL CHECK (version > 0),
    status TEXT NOT NULL DEFAULT 'CURRENT' CHECK (status IN ('CURRENT','SUPERSEDED','COMPLETED','CANCELLED')),
    route_reference TEXT NOT NULL,
    route_provider TEXT NOT NULL,
    route_evidence_version TEXT NOT NULL,
    ordered_points JSONB NOT NULL CHECK (jsonb_typeof(ordered_points)='array'),
    distance_meters BIGINT NOT NULL CHECK (distance_meters > 0),
    duration_seconds BIGINT NOT NULL CHECK (duration_seconds > 0),
    corridor_meters INTEGER NOT NULL CHECK (corridor_meters > 0),
    wrong_way_tolerance_degrees INTEGER NOT NULL CHECK (wrong_way_tolerance_degrees BETWEEN 1 AND 180),
    stationary_seconds INTEGER NOT NULL CHECK (stationary_seconds > 0),
    stale_seconds INTEGER NOT NULL CHECK (stale_seconds > 0),
    high_risk BOOLEAN NOT NULL DEFAULT false,
    policy_version TEXT NOT NULL,
    reroute_reason TEXT NOT NULL DEFAULT '',
    superseded_by UUID NULL REFERENCES dispatch_expected_routes(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (trip_id, version)
);
CREATE UNIQUE INDEX dispatch_expected_route_one_current
    ON dispatch_expected_routes(trip_id) WHERE status='CURRENT';

CREATE TABLE dispatch_route_location_fixes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id UUID NOT NULL REFERENCES trips(id) ON DELETE RESTRICT,
    route_id UUID NOT NULL REFERENCES dispatch_expected_routes(id) ON DELETE RESTRICT,
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    latitude DOUBLE PRECISION NOT NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude DOUBLE PRECISION NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    heading DOUBLE PRECISION NOT NULL CHECK (heading BETWEEN 0 AND 360),
    speed DOUBLE PRECISION NOT NULL CHECK (speed >= 0),
    accuracy DOUBLE PRECISION NOT NULL CHECK (accuracy >= 0),
    client_recorded_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX dispatch_route_location_history ON dispatch_route_location_fixes(trip_id, received_at DESC);

CREATE TABLE dispatch_route_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id UUID NOT NULL REFERENCES trips(id) ON DELETE RESTRICT,
    route_id UUID NOT NULL REFERENCES dispatch_expected_routes(id) ON DELETE RESTRICT,
    route_version BIGINT NOT NULL CHECK (route_version > 0),
    alert_type TEXT NOT NULL CHECK (alert_type IN ('ROUTE_DEVIATION','WRONG_WAY_MOVEMENT','EXCESSIVE_STATIONARY','STALE_LOCATION','SUSPICIOUS_LOCATION','POSSIBLE_ROUTE_TAMPERING')),
    state TEXT NOT NULL DEFAULT 'OPEN' CHECK (state IN ('OPEN','ACKNOWLEDGED','RESOLVED','DISMISSED_FALSE_POSITIVE','ESCALATED')),
    severity TEXT NOT NULL CHECK (severity IN ('LOW','MEDIUM','HIGH')),
    policy_version TEXT NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    safe_evidence JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(safe_evidence)='object'),
    dedupe_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX dispatch_route_alert_open ON dispatch_route_alerts(trip_id, created_at DESC) WHERE state='OPEN';

CREATE TABLE dispatch_device_commands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    trip_id UUID NULL REFERENCES trips(id) ON DELETE RESTRICT,
    device_id TEXT NOT NULL,
    device_command_id TEXT NOT NULL,
    command_type TEXT NOT NULL,
    command_payload JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(command_payload)='object'),
    idempotency_key TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    client_timestamp TIMESTAMPTZ NOT NULL,
    last_known_server_version BIGINT NOT NULL CHECK (last_known_server_version >= 0),
    state TEXT NOT NULL CHECK (state IN ('RECEIVED','ACCEPTED','PENDING_EVIDENCE_SYNC','REJECTED','COMPLETED')),
    result_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(result_snapshot)='object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider_id, device_id, device_command_id),
    UNIQUE (provider_id, idempotency_key)
);
CREATE INDEX dispatch_device_command_trip ON dispatch_device_commands(trip_id, created_at DESC) WHERE trip_id IS NOT NULL;

CREATE OR REPLACE FUNCTION dispatch_reject_recovery_history_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'dispatch recovery history is append-only';
END;
$$;
CREATE TRIGGER dispatch_contact_attempt_append_only
    BEFORE UPDATE OR DELETE ON dispatch_delivery_contact_attempts
    FOR EACH ROW EXECUTE FUNCTION dispatch_reject_recovery_history_mutation();
CREATE TRIGGER dispatch_route_fix_append_only
    BEFORE UPDATE OR DELETE ON dispatch_route_location_fixes
    FOR EACH ROW EXECUTE FUNCTION dispatch_reject_recovery_history_mutation();
CREATE TRIGGER dispatch_hub_checkpoint_append_only
    BEFORE UPDATE OR DELETE ON dispatch_hub_hold_checkpoints
    FOR EACH ROW EXECUTE FUNCTION dispatch_reject_recovery_history_mutation();
CREATE TRIGGER dispatch_emergency_recovery_append_only
    BEFORE UPDATE OR DELETE ON dispatch_emergency_recovery_actions
    FOR EACH ROW EXECUTE FUNCTION dispatch_reject_recovery_history_mutation();

CREATE OR REPLACE FUNCTION dispatch_protect_expected_route_history()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'dispatch expected route history is immutable';
    END IF;
    IF NEW.id <> OLD.id OR NEW.trip_id <> OLD.trip_id OR NEW.provider_id <> OLD.provider_id
       OR NEW.version <> OLD.version OR NEW.route_reference <> OLD.route_reference
       OR NEW.route_provider <> OLD.route_provider OR NEW.route_evidence_version <> OLD.route_evidence_version
       OR NEW.ordered_points <> OLD.ordered_points OR NEW.distance_meters <> OLD.distance_meters
       OR NEW.duration_seconds <> OLD.duration_seconds OR NEW.corridor_meters <> OLD.corridor_meters
       OR NEW.wrong_way_tolerance_degrees <> OLD.wrong_way_tolerance_degrees
       OR NEW.stationary_seconds <> OLD.stationary_seconds OR NEW.stale_seconds <> OLD.stale_seconds
       OR NEW.high_risk <> OLD.high_risk OR NEW.policy_version <> OLD.policy_version
       OR NEW.reroute_reason <> OLD.reroute_reason OR NEW.created_at <> OLD.created_at THEN
        RAISE EXCEPTION 'dispatch expected route evidence is immutable';
    END IF;
    IF OLD.status <> NEW.status AND NOT (OLD.status='CURRENT' AND NEW.status IN ('SUPERSEDED','COMPLETED','CANCELLED')) THEN
        RAISE EXCEPTION 'invalid expected route status transition';
    END IF;
    IF OLD.superseded_by IS NOT NULL AND NEW.superseded_by IS DISTINCT FROM OLD.superseded_by THEN
        RAISE EXCEPTION 'expected route successor is immutable';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER dispatch_expected_route_history_guard
    BEFORE UPDATE OR DELETE ON dispatch_expected_routes
    FOR EACH ROW EXECUTE FUNCTION dispatch_protect_expected_route_history();

COMMENT ON TABLE dispatch_delivery_attempts IS 'Receiver arrival/grace and recovery decision projection; canonical Trip remains lifecycle authority.';
COMMENT ON TABLE dispatch_expected_routes IS 'Versioned operational route evidence from the Section 6 route authority; alerts are not misconduct findings.';
COMMENT ON TABLE dispatch_device_commands IS 'Offline replay ledger; client commands remain non-authoritative until server acceptance.';
