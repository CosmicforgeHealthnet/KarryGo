CREATE TABLE customer_ratings_by_provider (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id     UUID        NOT NULL UNIQUE REFERENCES trips(id) ON DELETE RESTRICT,
    provider_id UUID        NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    customer_id UUID        NOT NULL,
    score       SMALLINT    NOT NULL,
    comment     TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_customer_ratings_score CHECK (score >= 1 AND score <= 5)
);

CREATE INDEX idx_customer_ratings_trip_id     ON customer_ratings_by_provider(trip_id);
CREATE INDEX idx_customer_ratings_provider_id ON customer_ratings_by_provider(provider_id);
CREATE INDEX idx_customer_ratings_customer_id ON customer_ratings_by_provider(customer_id);
