-- Adoption mappings are evidence; rollback refuses populated state, and the
-- original closed binding list cannot be restored over case-bound objects.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM media_legacy_adoptions) THEN
  RAISE EXCEPTION 'Cannot roll back populated legacy adoption evidence';
 END IF;
 IF EXISTS(SELECT 1 FROM media_objects WHERE binding_type NOT IN ('', 'BOOKING', 'MISSION', 'ORDER', 'ASSET', 'PARCEL', 'COMPLAINT', 'RECOVERY_CASE', 'VERIFICATION_CASE'))
 OR EXISTS(SELECT 1 FROM media_object_secondary_bindings WHERE binding_type NOT IN ('BOOKING', 'MISSION', 'ORDER', 'ASSET', 'PARCEL', 'COMPLAINT', 'RECOVERY_CASE', 'VERIFICATION_CASE')) THEN
  RAISE EXCEPTION 'Cannot roll back while objects carry case-registry binding types';
 END IF;
END $$;
DROP TABLE media_legacy_adoptions;
DROP FUNCTION reject_media_legacy_adoption_mutation();
ALTER TABLE media_objects DROP CONSTRAINT chk_media_object_case_binding_private;
ALTER TABLE media_object_secondary_bindings DROP CONSTRAINT media_object_secondary_bindings_binding_type_check;
ALTER TABLE media_object_secondary_bindings ADD CONSTRAINT media_object_secondary_bindings_binding_type_check
    CHECK (binding_type IN ('BOOKING', 'MISSION', 'ORDER', 'ASSET', 'PARCEL', 'COMPLAINT', 'RECOVERY_CASE', 'VERIFICATION_CASE'));
ALTER TABLE media_objects DROP CONSTRAINT media_objects_binding_type_check;
ALTER TABLE media_objects ADD CONSTRAINT media_objects_binding_type_check
    CHECK (binding_type IN ('', 'BOOKING', 'MISSION', 'ORDER', 'ASSET', 'PARCEL', 'COMPLAINT', 'RECOVERY_CASE', 'VERIFICATION_CASE'));
