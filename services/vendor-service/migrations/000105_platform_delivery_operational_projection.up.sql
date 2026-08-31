-- Projection-only copy of authenticated Dispatch operational evidence and
-- lifecycle facts. Dispatch remains canonical; media bytes remain external.
CREATE TABLE commerce_platform_operational_events (
    id text PRIMARY KEY,
    event_id text NOT NULL UNIQUE,
    canonical_mission_ref text NOT NULL,
    operation_ref text NOT NULL,
    mission_slot_ref text NOT NULL,
    event_type text NOT NULL CHECK (event_type IN (
        'commerce.evidence.recorded.v1','commerce.exception.reported.v1',
        'commerce.return.started.v1','commerce.return.completed.v1',
        'commerce.provider_transfer.started.v1','commerce.provider_transfer.completed.v1'
    )),
    source_version bigint NOT NULL CHECK (source_version > 0),
    parcel_refs text[] NOT NULL DEFAULT '{}',
    evidence_ref text NOT NULL DEFAULT '',
    exception_ref text NOT NULL DEFAULT '',
    return_ref text NOT NULL DEFAULT '',
    transfer_ref text NOT NULL DEFAULT '',
    reason_code text NOT NULL DEFAULT '',
    payload_hash text NOT NULL,
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    occurred_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX commerce_platform_operational_events_mission_idx
    ON commerce_platform_operational_events(operation_ref, source_version, occurred_at, id);
CREATE INDEX commerce_platform_operational_events_parcels_idx
    ON commerce_platform_operational_events USING gin(parcel_refs);

COMMENT ON TABLE commerce_platform_operational_events IS
'Append-only Vendor projection of authenticated Dispatch evidence, exceptions, returns and provider transfer. It is not canonical mission authority and contains no media bytes.';
