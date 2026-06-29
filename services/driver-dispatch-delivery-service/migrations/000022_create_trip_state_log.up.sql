CREATE TABLE trip_state_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id UUID NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    from_status TEXT NOT NULL,
    to_status TEXT NOT NULL,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by TEXT NOT NULL,
    notes TEXT NULL,
    CONSTRAINT chk_trip_state_log_from_status CHECK (from_status IN (
        'none',
        'assigned',
        'en_route_pickup',
        'arrived_pickup',
        'in_progress',
        'proof_submitted',
        'completed',
        'cancelled',
        'failed'
    )),
    CONSTRAINT chk_trip_state_log_to_status CHECK (to_status IN (
        'assigned',
        'en_route_pickup',
        'arrived_pickup',
        'in_progress',
        'proof_submitted',
        'completed',
        'cancelled',
        'failed'
    )),
    CONSTRAINT chk_trip_state_log_changed_by CHECK (changed_by IN (
        'provider',
        'customer',
        'system',
        'admin'
    ))
);

CREATE INDEX idx_state_log_trip ON trip_state_log (trip_id);
CREATE INDEX idx_state_log_trip_changed_at ON trip_state_log (trip_id, changed_at DESC);
