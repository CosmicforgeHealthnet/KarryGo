-- Quotes: what pricing said, for whom, from which exact policy versions.
--
-- A quote is immutable. It records the versions it was priced with, so it can
-- be re-derived and reconciled against its ORIGINAL rules long after those rules
-- have been replaced. Binding and reconciliation are separate append-only
-- records rather than columns updated in place, so the history of a quote is a
-- sequence of facts, never an edited row.

CREATE TABLE IF NOT EXISTS pricing_quotes (
    id                    text PRIMARY KEY,
    -- Who asked, and their own reference for this pricing attempt. Asking again
    -- with the same reference and class returns this quote rather than a second.
    consumer_service      text NOT NULL CHECK (consumer_service <> ''),
    consumer_ref          text NOT NULL CHECK (consumer_ref <> ''),
    scope                 text NOT NULL CHECK (scope IN ('TAXI', 'DISPATCH', 'HAULAGE')),
    channel               text NOT NULL CHECK (channel <> ''),
    class                 text NOT NULL CHECK (class <> ''),
    -- SHADOW quotes are evidence only; CENTRAL quotes are the price charged.
    mode                  text NOT NULL CHECK (mode IN ('SHADOW', 'CENTRAL')),

    market_id             text NOT NULL REFERENCES pricing_markets (id),
    market_code           text NOT NULL,
    zone_id               text REFERENCES pricing_zones (id),
    zone_version_id       text REFERENCES pricing_zone_versions (id),

    tariff_version_id     text NOT NULL REFERENCES pricing_policy_versions (id),
    commission_version_id text REFERENCES pricing_policy_versions (id),

    -- The normalized request and the full engine result, exactly as priced.
    request               jsonb NOT NULL,
    result                jsonb NOT NULL,
    result_hash           text NOT NULL CHECK (result_hash ~ '^[0-9a-f]{64}$'),
    gross_kobo            bigint NOT NULL CHECK (gross_kobo >= 0),
    provider_kobo         bigint NOT NULL CHECK (provider_kobo >= 0),
    platform_fee_kobo     bigint NOT NULL CHECK (platform_fee_kobo >= 0),
    tax_kobo              bigint NOT NULL DEFAULT 0 CHECK (tax_kobo >= 0),
    currency              char(3) NOT NULL,

    -- What the caller itself charges, sent in shadow mode for comparison.
    legacy_fare_kobo      bigint CHECK (legacy_fare_kobo IS NULL OR legacy_fare_kobo >= 0),

    issued_at             timestamptz NOT NULL DEFAULT now(),
    expires_at            timestamptz NOT NULL,
    CONSTRAINT pricing_quotes_expiry_after_issue CHECK (expires_at > issued_at),
    UNIQUE (consumer_service, consumer_ref, class)
);

CREATE INDEX IF NOT EXISTS pricing_quotes_issued_idx ON pricing_quotes (issued_at DESC);
CREATE INDEX IF NOT EXISTS pricing_quotes_scope_mode_idx ON pricing_quotes (scope, mode, issued_at DESC);

-- Quotes are never edited. The only permitted DELETE is the retention purge of
-- expired, never-bound quotes, which runs under the pricing.allow_purge flag.
DROP TRIGGER IF EXISTS pricing_quotes_append_only ON pricing_quotes;
CREATE TRIGGER pricing_quotes_append_only
    BEFORE UPDATE OR DELETE ON pricing_quotes
    FOR EACH ROW EXECUTE FUNCTION pricing_reject_mutation_purgeable();

-- A quote is bound to at most one booking, once.
CREATE TABLE IF NOT EXISTS pricing_quote_bindings (
    id               text PRIMARY KEY,
    quote_id         text NOT NULL UNIQUE REFERENCES pricing_quotes (id),
    consumer_service text NOT NULL CHECK (consumer_service <> ''),
    booking_ref      text NOT NULL CHECK (booking_ref <> ''),
    bound_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS pricing_quote_bindings_booking_idx
    ON pricing_quote_bindings (consumer_service, booking_ref);

DROP TRIGGER IF EXISTS pricing_quote_bindings_append_only ON pricing_quote_bindings;
CREATE TRIGGER pricing_quote_bindings_append_only
    BEFORE UPDATE OR DELETE ON pricing_quote_bindings
    FOR EACH ROW EXECUTE FUNCTION pricing_reject_mutation();

-- A bound quote is reconciled at most once, from the actual route, priced with
-- the quote's original policy versions.
CREATE TABLE IF NOT EXISTS pricing_quote_reconciliations (
    id                 text PRIMARY KEY,
    quote_id           text NOT NULL UNIQUE REFERENCES pricing_quotes (id),
    request            jsonb NOT NULL,
    result             jsonb NOT NULL,
    result_hash        text NOT NULL CHECK (result_hash ~ '^[0-9a-f]{64}$'),
    gross_kobo         bigint NOT NULL CHECK (gross_kobo >= 0),
    provider_kobo      bigint NOT NULL CHECK (provider_kobo >= 0),
    platform_fee_kobo  bigint NOT NULL CHECK (platform_fee_kobo >= 0),
    legacy_final_kobo  bigint CHECK (legacy_final_kobo IS NULL OR legacy_final_kobo >= 0),
    reconciled_at      timestamptz NOT NULL DEFAULT now()
);

DROP TRIGGER IF EXISTS pricing_quote_reconciliations_append_only ON pricing_quote_reconciliations;
CREATE TRIGGER pricing_quote_reconciliations_append_only
    BEFORE UPDATE OR DELETE ON pricing_quote_reconciliations
    FOR EACH ROW EXECUTE FUNCTION pricing_reject_mutation();
