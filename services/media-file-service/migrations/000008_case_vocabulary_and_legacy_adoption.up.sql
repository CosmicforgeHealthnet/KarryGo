-- Section 10: case binding vocabulary from the compiled source case registry
-- and owner-attested legacy asset adoption.
--
-- 1. Binding types. 000005 wrote a closed list of eight binding types. Source
--    owners now enroll further case kinds (shared/go/admincase.Specs) with
--    their own binding type (e.g. INVESTIGATION_CASE). The application keeps
--    the authoritative closed set (Media's own list plus the compiled case
--    registry); the database keeps a strict shape guard, and any binding type
--    outside the original list must be PRIVATE.
ALTER TABLE media_objects DROP CONSTRAINT IF EXISTS media_objects_binding_type_check;
ALTER TABLE media_objects ADD CONSTRAINT media_objects_binding_type_check
    CHECK (binding_type = '' OR binding_type ~ '^[A-Z][A-Z0-9_]{1,63}$');
ALTER TABLE media_object_secondary_bindings DROP CONSTRAINT IF EXISTS media_object_secondary_bindings_binding_type_check;
ALTER TABLE media_object_secondary_bindings ADD CONSTRAINT media_object_secondary_bindings_binding_type_check
    CHECK (binding_type ~ '^[A-Z][A-Z0-9_]{1,63}$');
ALTER TABLE media_objects ADD CONSTRAINT chk_media_object_case_binding_private CHECK (
    visibility = 'PRIVATE'
    OR binding_type IN ('', 'BOOKING', 'MISSION', 'ORDER', 'ASSET', 'PARCEL',
                        'COMPLAINT', 'RECOVERY_CASE', 'VERIFICATION_CASE'));

-- 2. Legacy adoption: one immutable mapping per legacy asset. The source owner
--    that uploaded the legacy asset attested its case/subject/purpose; the
--    modern object was built from the same verified bytes and checksum.
CREATE TABLE IF NOT EXISTS media_legacy_adoptions (
    legacy_asset_id        uuid PRIMARY KEY REFERENCES media_assets (id),
    media_object_id        text NOT NULL UNIQUE REFERENCES media_objects (id),
    owner_service          text NOT NULL,
    legacy_owner_service   text NOT NULL,
    legacy_owner_reference text NOT NULL,
    legacy_purpose         text NOT NULL,
    organization_id        text NOT NULL,
    case_kind              text NOT NULL,
    case_reference         text NOT NULL,
    subject_type           text NOT NULL,
    subject_reference      text NOT NULL,
    purpose                text NOT NULL,
    checksum_sha256        text NOT NULL CHECK (checksum_sha256 ~ '^[0-9a-f]{64}$'),
    attestation_digest     text NOT NULL CHECK (attestation_digest ~ '^[0-9a-f]{64}$'),
    created_at             timestamptz NOT NULL DEFAULT now()
);
CREATE OR REPLACE FUNCTION reject_media_legacy_adoption_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Media legacy adoption mapping is immutable'; END $$;
CREATE TRIGGER media_legacy_adoption_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON media_legacy_adoptions
    FOR EACH STATEMENT EXECUTE FUNCTION reject_media_legacy_adoption_mutation();
