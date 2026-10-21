CREATE TABLE taxi_admin_read_nonces (
  nonce_hash text PRIMARY KEY,
  expires_at timestamptz NOT NULL
);
CREATE INDEX taxi_admin_read_nonces_expiry ON taxi_admin_read_nonces(expires_at);
CREATE TABLE taxi_admin_read_audit (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  staff_ref text NOT NULL,
  market_code text NOT NULL,
  zone_code text NOT NULL,
  organization_ref text NOT NULL,
  purpose_code text NOT NULL,
  target_ref text NOT NULL,
  recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE FUNCTION reject_taxi_admin_read_audit_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Taxi admin read audit is immutable'; END $$;
CREATE TRIGGER taxi_admin_read_audit_immutable
 BEFORE UPDATE OR DELETE OR TRUNCATE ON taxi_admin_read_audit
 FOR EACH STATEMENT EXECUTE FUNCTION reject_taxi_admin_read_audit_mutation();
