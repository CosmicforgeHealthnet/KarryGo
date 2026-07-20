CREATE TABLE IF NOT EXISTS provider_transaction_pins (
    id uuid PRIMARY KEY,
    provider_type text NOT NULL,
    provider_id text NOT NULL,
    pin_hash text NOT NULL,
    failed_attempts int NOT NULL DEFAULT 0,
    locked_until timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_type, provider_id)
);
