CREATE TABLE bikes (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id         UUID        NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    bike_type           TEXT        NOT NULL,
    brand               TEXT        NOT NULL,
    model               TEXT        NOT NULL,
    year                SMALLINT    NOT NULL,
    color               TEXT        NOT NULL,
    plate_number        TEXT        NOT NULL UNIQUE,
    engine_cc           SMALLINT    NULL,
    chassis_number      TEXT        NULL,
    verification_status TEXT        NOT NULL DEFAULT 'unverified',
    is_active           BOOL        NOT NULL DEFAULT true,
    is_primary          BOOL        NOT NULL DEFAULT false,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT bikes_bike_type_check
        CHECK (bike_type IN ('motorcycle', 'dispatch_bike')),

    CONSTRAINT bikes_verification_status_check
        CHECK (verification_status IN ('unverified', 'pending', 'verified', 'rejected', 'suspended'))
);

CREATE INDEX idx_bikes_provider ON bikes (provider_id);
CREATE INDEX idx_bikes_status   ON bikes (verification_status);
