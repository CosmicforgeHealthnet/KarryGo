-- Versioned, explainable Commerce delivery ETA projections and scheduled
-- notification markers. Route, mission, production, money and transport
-- authority remain in their existing owning services.
CREATE TABLE commerce_delivery_eta_versions (
    id text PRIMARY KEY,
    parent_order_id text NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    fulfilment_group_id text REFERENCES commerce_fulfilment_groups(id) ON DELETE RESTRICT,
    mission_slot_ref text NOT NULL CHECK (length(btrim(mission_slot_ref)) > 0),
    plan_version bigint NOT NULL DEFAULT 0 CHECK (plan_version >= 0),
    provider_ref text NOT NULL DEFAULT '',
    eta_version bigint NOT NULL CHECK (eta_version > 0),
    status text NOT NULL CHECK (status IN ('ACTIVE','ACTUAL','INVALIDATED','RETURNING')),
    earliest_at timestamptz,
    latest_at timestamptz,
    actual_at timestamptz,
    confidence text NOT NULL CHECK (confidence IN ('LOW','MEDIUM','HIGH')),
    policy_version text NOT NULL CHECK (length(btrim(policy_version)) > 0),
    trigger_code text NOT NULL CHECK (trigger_code IN (
        'MISSION_CREATED','VENDOR_DELAY','VENDOR_EARLY_READY','PROVIDER_ASSIGNED','PROVIDER_ARRIVED',
        'PROVIDER_REASSIGNED','FULFILMENT_REPLAN','STOP_ADDED','STOP_REMOVED',
        'BREAKDOWN','ROUTE_DEVIATION','PICKUP_COMPLETED','STOP_COMPLETED',
        'OPERATIONAL_EXCEPTION','DELIVERED','CANCELLED','RETURNING','TIMING_UPDATED'
    )),
    promised_latest_at timestamptz,
    delay_state text NOT NULL CHECK (delay_state IN ('ON_TIME','AT_RISK','DELAYED','RECOVERED','NOT_APPLICABLE')),
    components jsonb NOT NULL CHECK (jsonb_typeof(components) = 'array'),
    source_snapshot jsonb NOT NULL CHECK (jsonb_typeof(source_snapshot) = 'object'),
    input_fingerprint text NOT NULL CHECK (length(input_fingerprint) = 64),
    source_event_id text NOT NULL CHECK (length(btrim(source_event_id)) > 0),
    source_authority text NOT NULL CHECK (source_authority IN ('DISPATCH','HAULING','PRODUCTION','VENDOR')),
    source_version bigint NOT NULL CHECK (source_version > 0),
    source_occurred_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(parent_order_id, mission_slot_ref, eta_version),
    UNIQUE(parent_order_id, mission_slot_ref, input_fingerprint),
    UNIQUE(parent_order_id, mission_slot_ref, source_event_id)
);

ALTER TABLE commerce_delivery_eta_versions ADD CONSTRAINT commerce_delivery_eta_window_chk CHECK (
    (status = 'ACTIVE' AND earliest_at IS NOT NULL AND latest_at IS NOT NULL AND latest_at >= earliest_at AND actual_at IS NULL)
    OR (status = 'ACTUAL' AND actual_at IS NOT NULL AND earliest_at = actual_at AND latest_at = actual_at)
    OR (status IN ('INVALIDATED','RETURNING') AND earliest_at IS NULL AND latest_at IS NULL AND actual_at IS NULL)
);

CREATE INDEX commerce_delivery_eta_history_idx
    ON commerce_delivery_eta_versions(parent_order_id, mission_slot_ref, eta_version DESC);
CREATE INDEX commerce_delivery_eta_delay_idx
    ON commerce_delivery_eta_versions(delay_state, created_at DESC)
    WHERE delay_state IN ('AT_RISK','DELAYED');

CREATE TABLE commerce_delivery_eta_current (
    parent_order_id text NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    mission_slot_ref text NOT NULL,
    eta_id text NOT NULL UNIQUE REFERENCES commerce_delivery_eta_versions(id) ON DELETE RESTRICT,
    eta_version bigint NOT NULL CHECK (eta_version > 0),
    plan_version bigint NOT NULL DEFAULT 0 CHECK (plan_version >= 0),
    provider_ref text NOT NULL DEFAULT '',
    source_authority text NOT NULL CHECK (source_authority IN ('DISPATCH','HAULING','PRODUCTION','VENDOR')),
    source_version bigint NOT NULL CHECK (source_version > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(parent_order_id, mission_slot_ref),
    FOREIGN KEY(parent_order_id, mission_slot_ref, eta_version)
        REFERENCES commerce_delivery_eta_versions(parent_order_id, mission_slot_ref, eta_version)
        ON DELETE RESTRICT
);

CREATE TABLE commerce_scheduled_notification_markers (
    scheduled_order_id text NOT NULL REFERENCES commerce_scheduled_orders(id) ON DELETE RESTRICT,
    notification_type text NOT NULL CHECK (notification_type IN ('PREPARATION_CUTOFF')),
    cutoff_at timestamptz NOT NULL,
    source_version bigint NOT NULL CHECK (source_version > 0),
    event_id text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(scheduled_order_id, notification_type)
);

CREATE OR REPLACE FUNCTION reject_commerce_eta_history_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'commerce delivery ETA history is immutable';
END;
$$;

CREATE TRIGGER commerce_delivery_eta_versions_immutable
BEFORE UPDATE OR DELETE ON commerce_delivery_eta_versions
FOR EACH ROW EXECUTE FUNCTION reject_commerce_eta_history_mutation();

COMMENT ON TABLE commerce_delivery_eta_versions IS
'Immutable customer-visible delivery estimate history. Components retain provenance; this is not a route or mission authority.';
COMMENT ON TABLE commerce_delivery_eta_current IS
'Single concurrency-fenced pointer to the latest valid ETA per order mission.';
