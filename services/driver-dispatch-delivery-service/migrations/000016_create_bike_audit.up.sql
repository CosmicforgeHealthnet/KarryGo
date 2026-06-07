CREATE TABLE bike_audit (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    bike_id      UUID        NOT NULL REFERENCES bikes(id) ON DELETE CASCADE,
    provider_id  UUID        NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    action       TEXT        NOT NULL,
    from_status  TEXT        NOT NULL,
    to_status    TEXT        NOT NULL,
    performed_by UUID        NULL,
    notes        TEXT        NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT bike_audit_action_check
        CHECK (action IN ('registered', 'docs_uploaded', 'approved', 'rejected', 'suspended', 'resubmitted', 'updated')),

    CONSTRAINT bike_audit_from_status_check
        CHECK (from_status IN ('unverified', 'pending', 'verified', 'rejected', 'suspended')),

    CONSTRAINT bike_audit_to_status_check
        CHECK (to_status IN ('unverified', 'pending', 'verified', 'rejected', 'suspended'))
);

CREATE INDEX idx_bike_audit_bike     ON bike_audit (bike_id);
CREATE INDEX idx_bike_audit_provider ON bike_audit (provider_id);
