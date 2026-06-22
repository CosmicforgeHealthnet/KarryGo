-- support-dispute-service migrations
-- Covers all four service types: customer (taxi, dispatch, hauling bookings) and provider complaints.

CREATE TYPE service_type AS ENUM (
    'taxi',
    'dispatch',
    'hauling',
    'platform'
);

CREATE TYPE complaint_status AS ENUM (
    'open',
    'under_review',
    'awaiting_evidence',
    'resolved',
    'closed',
    'escalated'
);

CREATE TYPE complainant_type AS ENUM (
    'customer',
    'taxi_provider',
    'dispatch_provider',
    'hauling_provider'
);

CREATE TYPE dispute_outcome AS ENUM (
    'pending',
    'favour_complainant',
    'favour_respondent',
    'split',
    'dismissed'
);

-- complaints: the primary ticket raised by any party
CREATE TABLE IF NOT EXISTS complaints (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    complainant_type    complainant_type NOT NULL,
    complainant_id      TEXT NOT NULL,
    service_type        service_type NOT NULL,
    booking_reference   TEXT,                        -- optional: ties complaint to a booking
    subject             TEXT NOT NULL,
    description         TEXT NOT NULL,
    status              complaint_status NOT NULL DEFAULT 'open',
    assigned_to         TEXT,                        -- admin user id
    resolution_note     TEXT,
    resolved_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_complaints_complainant  ON complaints (complainant_type, complainant_id);
CREATE INDEX idx_complaints_booking      ON complaints (booking_reference) WHERE booking_reference IS NOT NULL;
CREATE INDEX idx_complaints_status       ON complaints (status);
CREATE INDEX idx_complaints_service_type ON complaints (service_type);

-- evidence: files or text attached to a complaint
CREATE TABLE IF NOT EXISTS complaint_evidence (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    complaint_id   UUID NOT NULL REFERENCES complaints(id) ON DELETE CASCADE,
    uploader_type  complainant_type NOT NULL,
    uploader_id    TEXT NOT NULL,
    media_asset_id TEXT,
    media_url      TEXT,
    note           TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_evidence_complaint ON complaint_evidence (complaint_id);

-- disputes: a formal escalation from a complaint, involving two parties
CREATE TABLE IF NOT EXISTS disputes (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    complaint_id      UUID NOT NULL REFERENCES complaints(id),
    service_type      service_type NOT NULL,
    booking_reference TEXT,
    respondent_type   complainant_type NOT NULL,
    respondent_id     TEXT NOT NULL,
    outcome           dispute_outcome NOT NULL DEFAULT 'pending',
    adjudicator_id    TEXT,              -- admin who resolved
    adjudication_note TEXT,
    resolved_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_disputes_complaint     ON disputes (complaint_id);
CREATE INDEX idx_disputes_respondent    ON disputes (respondent_type, respondent_id);
CREATE INDEX idx_disputes_outcome       ON disputes (outcome);
CREATE INDEX idx_disputes_service_type  ON disputes (service_type);

-- complaint_events: audit trail of status changes and actions
CREATE TABLE IF NOT EXISTS complaint_events (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    complaint_id UUID NOT NULL REFERENCES complaints(id) ON DELETE CASCADE,
    actor_type   TEXT NOT NULL,
    actor_id     TEXT NOT NULL,
    event_type   TEXT NOT NULL,   -- e.g. "status_changed", "evidence_added", "assigned"
    payload      JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_complaint_events_complaint ON complaint_events (complaint_id);
