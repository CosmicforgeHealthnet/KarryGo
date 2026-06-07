CREATE TABLE cancellations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id UUID NOT NULL UNIQUE REFERENCES trips(id) ON DELETE CASCADE,
    cancelled_by TEXT NOT NULL,
    reason_code TEXT NOT NULL,
    reason_text TEXT NULL,
    penalty_applied BOOLEAN NOT NULL DEFAULT false,
    cancelled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_cancelled_by CHECK (cancelled_by IN (
        'provider',
        'customer',
        'system',
        'admin'
    ))
);

CREATE UNIQUE INDEX idx_cancel_trip ON cancellations (trip_id);
CREATE INDEX idx_cancelled_by ON cancellations (cancelled_by);
CREATE INDEX idx_cancellations_cancelled_at ON cancellations (cancelled_at DESC);
