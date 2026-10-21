-- Private authorization evidence, never included in an HQ release. Old cached
-- releases are deliberately not backfilled with invented membership proof.
CREATE TABLE IF NOT EXISTS analytics_admin_disclosure_authorizations (
 release_id text PRIMARY KEY REFERENCES analytics_admin_disclosures(release_id),
 authorization_payload jsonb NOT NULL CHECK(jsonb_typeof(authorization_payload)='object' AND octet_length(authorization_payload::text)<=65536),
 recorded_at timestamptz NOT NULL DEFAULT now()
);
DROP TRIGGER IF EXISTS analytics_admin_disclosure_authorizations_immutable ON analytics_admin_disclosure_authorizations;
CREATE TRIGGER analytics_admin_disclosure_authorizations_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
 ON analytics_admin_disclosure_authorizations FOR EACH STATEMENT EXECUTE FUNCTION reject_analytics_admin_evidence_mutation();
REVOKE ALL ON analytics_admin_disclosure_authorizations FROM PUBLIC;
