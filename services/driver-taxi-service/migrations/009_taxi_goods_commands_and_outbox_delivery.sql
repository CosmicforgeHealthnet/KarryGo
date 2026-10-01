-- Section 29 closure: explicit binding commands and recoverable outbound
-- delivery. This is additive because migrations 007/008 may already be live.

ALTER TABLE taxi_outbox
    ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT '';

ALTER TABLE taxi_goods_handover_challenges
    ADD COLUMN IF NOT EXISTS consumed_operation_ref TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_taxi_goods_handover_consumed_operation
    ON taxi_goods_handover_challenges(mission_id, stage, consumed_operation_ref)
    WHERE consumed_operation_ref <> '';

ALTER TABLE taxi_outbox
    DROP CONSTRAINT IF EXISTS taxi_outbox_attempts_nonnegative;
ALTER TABLE taxi_outbox
    ADD CONSTRAINT taxi_outbox_attempts_nonnegative CHECK (attempts >= 0);

CREATE INDEX IF NOT EXISTS idx_taxi_outbox_lease_recovery
    ON taxi_outbox(lease_until)
    WHERE delivered_at IS NULL AND dead_lettered_at IS NULL;

CREATE TABLE taxi_goods_binding_operations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mission_id UUID NOT NULL REFERENCES taxi_goods_missions(id) ON DELETE RESTRICT,
    operation_ref TEXT NOT NULL UNIQUE,
    operation_type TEXT NOT NULL
        CHECK (operation_type IN ('PROVIDER_REASSIGNMENT','RECEIVER_CHANGE')),
    request_fingerprint TEXT NOT NULL CHECK (length(request_fingerprint) = 64),
    previous_provider_id UUID REFERENCES taxi_providers(id),
    new_provider_id UUID REFERENCES taxi_providers(id),
    previous_receiver_ref TEXT NOT NULL DEFAULT '',
    new_receiver_ref TEXT NOT NULL DEFAULT '',
    resulting_mission_version BIGINT NOT NULL CHECK (resulting_mission_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_taxi_goods_binding_operations_mission
    ON taxi_goods_binding_operations(mission_id, created_at, id);
