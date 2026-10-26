-- Section 17: customer marketing consent authority.
--
-- One row per customer per marketing channel. Consent is only ever captured
-- from the customer (or withdrawn by them or by an authorised privacy
-- fulfilment); it is never defaulted, inferred or backfilled. Absence of a row
-- means "no consent recorded", which is never permission to send.
--
-- market is the market code the customer's app declared at opt-in. NULL means
-- the market is unknown; a market-targeted campaign never matches it.
--
-- This service re-applies every migration on each MIGRATION=true boot, so
-- every statement is idempotent.
CREATE TABLE IF NOT EXISTS customer_marketing_consents (
 customer_id uuid NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
 channel text NOT NULL CHECK (channel IN ('in_app','push','email')),
 status text NOT NULL CHECK (status IN ('OPTED_IN','WITHDRAWN')),
 market text CHECK (market IS NULL OR market ~ '^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,63}$'),
 version bigint NOT NULL CHECK (version >= 1),
 captured_at timestamptz NOT NULL,
 withdrawn_at timestamptz,
 capture_source text NOT NULL CHECK (capture_source IN ('CUSTOMER_APP','ADMIN_PRIVACY_WITHDRAWAL')),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (customer_id, channel),
 CHECK ((status = 'WITHDRAWN') = (withdrawn_at IS NOT NULL))
);
-- Audience paging: opted-in customers of one channel/market in id order.
CREATE INDEX IF NOT EXISTS customer_marketing_consents_audience
 ON customer_marketing_consents (channel, market, customer_id) WHERE status = 'OPTED_IN';

CREATE TABLE IF NOT EXISTS customer_marketing_consent_history (
 id bigserial PRIMARY KEY,
 customer_id uuid NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
 channel text NOT NULL CHECK (channel IN ('in_app','push','email')),
 prior_status text CHECK (prior_status IS NULL OR prior_status IN ('OPTED_IN','WITHDRAWN')),
 status text NOT NULL CHECK (status IN ('OPTED_IN','WITHDRAWN')),
 market text,
 recorded_version bigint NOT NULL CHECK (recorded_version >= 1),
 capture_source text NOT NULL CHECK (capture_source IN ('CUSTOMER_APP','ADMIN_PRIVACY_WITHDRAWAL')),
 decision_ref text,
 recorded_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (customer_id, channel, recorded_version)
);
CREATE INDEX IF NOT EXISTS customer_marketing_consent_history_customer
 ON customer_marketing_consent_history (customer_id, recorded_at DESC);

CREATE OR REPLACE FUNCTION reject_customer_marketing_consent_history_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Marketing consent history is append-only'; END $$;
DROP TRIGGER IF EXISTS customer_marketing_consent_history_immutable ON customer_marketing_consent_history;
CREATE TRIGGER customer_marketing_consent_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON customer_marketing_consent_history FOR EACH STATEMENT EXECUTE FUNCTION reject_customer_marketing_consent_history_mutation();
