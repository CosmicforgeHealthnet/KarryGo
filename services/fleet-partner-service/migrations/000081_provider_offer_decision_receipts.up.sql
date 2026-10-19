-- Immutable source-owned evidence for exact provider offer decisions. This is
-- historical read-back, not execution authority or a Media access grant.
CREATE TABLE fleet_partner_provider_offer_decision_receipts (
 id text PRIMARY KEY,organization_id text NOT NULL,offer_id text NOT NULL,recipient_id text NOT NULL,
 membership_id text NOT NULL,provider_id text NOT NULL,source_service text NOT NULL CHECK(source_service IN ('DISPATCH','TAXI','HAULING')),
 client_response_id text NOT NULL CHECK(length(client_response_id) BETWEEN 1 AND 128),
 decision text NOT NULL CHECK(decision IN ('ACCEPTED','DECLINED')),
 intent_digest text NOT NULL CHECK(intent_digest ~ '^[0-9a-f]{64}$'),recorded_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(recipient_id),UNIQUE(offer_id,provider_id,source_service,client_response_id)
);
CREATE FUNCTION reject_provider_offer_receipt_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Provider offer decision receipts are append-only'; END $$;
CREATE TRIGGER fleet_partner_provider_offer_receipts_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
ON fleet_partner_provider_offer_decision_receipts FOR EACH STATEMENT EXECUTE FUNCTION reject_provider_offer_receipt_mutation();
