-- Section 16: network evidence -- what a file is evidence FOR, and whether it
-- still counts.
--
-- Two facts are added and nothing existing changes meaning.
--
-- BINDING is not ownership. owner_type/owner_reference already say who a file
-- belongs to -- the provider who took the photo. A damage photo is also
-- evidence ABOUT something else: the mission it was taken on, the parcel it
-- shows, the recovery case it supports. Folding that into owner_reference
-- would make one column answer two different questions, and the first caller
-- to rely on the wrong one would accept a photo of the wrong truck as proof
-- for the right mission. So binding is its own pair of columns.
--
-- EVIDENCE STATE is not deletion. deletion_state tracks whether the bytes
-- still exist. A file can be withdrawn as evidence -- superseded, disputed,
-- uploaded against the wrong job -- while its bytes must be kept precisely
-- because somebody may later need to see what was withdrawn. REVOKED is that
-- state, and it is deliberately separate from DELETED.
--
-- Every existing row keeps its meaning: an unbound file has an empty binding,
-- and every existing file is ACTIVE evidence exactly as it was before.

ALTER TABLE media_objects
    ADD COLUMN binding_type text NOT NULL DEFAULT ''
        CHECK (binding_type IN ('', 'BOOKING', 'MISSION', 'ORDER', 'ASSET', 'PARCEL',
                                'COMPLAINT', 'RECOVERY_CASE', 'VERIFICATION_CASE'));
ALTER TABLE media_objects
    ADD COLUMN binding_id text NOT NULL DEFAULT '';
ALTER TABLE media_objects
    ADD COLUMN evidence_state text NOT NULL DEFAULT 'ACTIVE'
        CHECK (evidence_state IN ('ACTIVE', 'REVOKED'));
ALTER TABLE media_objects
    ADD COLUMN revoked_at timestamptz;
ALTER TABLE media_objects
    ADD COLUMN revocation_reason text NOT NULL DEFAULT '';

-- A binding is a pair or nothing. A type with no id binds to everything of
-- that type; an id with no type binds to an identifier nobody can interpret.
ALTER TABLE media_objects
    ADD CONSTRAINT chk_media_object_binding_pair CHECK (
        (binding_type = '') = (binding_id = ''));

-- Revocation is recorded or it did not happen.
ALTER TABLE media_objects
    ADD CONSTRAINT chk_media_object_revocation CHECK (
        (evidence_state = 'REVOKED') = (revoked_at IS NOT NULL));

-- Sensitive evidence can never be public.
--
-- The rule is written here as well as in the use case for the same reason
-- 000004 wrote the DATA_EXPORT rule here: a CHECK survives whatever endpoint is
-- added next. A photo of a damaged parcel, a vehicle inspection or a safety
-- incident reachable by URL is a privacy failure that no later fix recalls.
ALTER TABLE media_objects
    ADD CONSTRAINT chk_media_object_sensitive_private CHECK (
        visibility = 'PRIVATE'
        OR upload_purpose NOT IN (
            'VEHICLE_INSPECTION', 'CAPABILITY_EVIDENCE', 'CARGO_CONDITION',
            'DAMAGE_EVIDENCE', 'RECOVERY_TRANSFER', 'SAFETY_INCIDENT'));

-- The binding is fixed when the upload is AUTHORISED, not when it completes:
-- the object row is created inside the authorisation itself, so it carries the
-- binding from the grant and no later request can name a different job. That
-- is why the grant table needs no binding columns of its own -- only the
-- privacy rule, which must hold before a single byte is accepted.
ALTER TABLE media_upload_authorizations
    ADD CONSTRAINT chk_upload_authorization_sensitive_private CHECK (
        visibility = 'PRIVATE'
        OR upload_purpose NOT IN (
            'VEHICLE_INSPECTION', 'CAPABILITY_EVIDENCE', 'CARGO_CONDITION',
            'DAMAGE_EVIDENCE', 'RECOVERY_TRANSFER', 'SAFETY_INCIDENT'));

-- Secondary bindings: one photo can show a parcel AND the asset carrying it
-- AND the mission it happened on.
--
-- A table rather than an array, so each binding can be looked up by what it
-- binds to -- "every piece of evidence for this mission" is the question a
-- support agent actually asks, and an array would make it a scan.
CREATE TABLE IF NOT EXISTS media_object_secondary_bindings (
    media_object_id text NOT NULL REFERENCES media_objects(id) ON DELETE CASCADE,
    binding_type    text NOT NULL
                    CHECK (binding_type IN ('BOOKING', 'MISSION', 'ORDER', 'ASSET', 'PARCEL',
                                            'COMPLAINT', 'RECOVERY_CASE', 'VERIFICATION_CASE')),
    binding_id      text NOT NULL CHECK (binding_id <> ''),
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (media_object_id, binding_type, binding_id)
);

-- Secondary bindings are declared once, at authorisation. Rewriting one later
-- would let a file quietly become evidence for a job it was never taken on.
CREATE OR REPLACE FUNCTION media_secondary_bindings_immutable() RETURNS trigger
LANGUAGE plpgsql AS $immutable$
BEGIN
    RAISE EXCEPTION 'media evidence bindings are immutable once declared'
        USING ERRCODE = 'P0001';
END $immutable$;
DROP TRIGGER IF EXISTS media_secondary_bindings_no_update ON media_object_secondary_bindings;
CREATE TRIGGER media_secondary_bindings_no_update
    BEFORE UPDATE ON media_object_secondary_bindings
    FOR EACH ROW EXECUTE FUNCTION media_secondary_bindings_immutable();

CREATE INDEX IF NOT EXISTS idx_media_objects_binding
    ON media_objects (binding_type, binding_id)
    WHERE binding_type <> '';
CREATE INDEX IF NOT EXISTS idx_media_secondary_bindings_target
    ON media_object_secondary_bindings (binding_type, binding_id);
