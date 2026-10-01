-- Reverses 000005.
--
-- Secondary bindings are dropped with their table; the immutability trigger
-- goes first so the drop is not itself refused. The sensitive-private CHECKs
-- are dropped before the columns they sit beside, and nothing here touches a
-- column or constraint 000005 did not add, so every earlier purpose and rule
-- keeps exactly the meaning it had.
DROP INDEX IF EXISTS idx_media_secondary_bindings_target;
DROP INDEX IF EXISTS idx_media_objects_binding;

DROP TRIGGER IF EXISTS media_secondary_bindings_no_update ON media_object_secondary_bindings;
DROP FUNCTION IF EXISTS media_secondary_bindings_immutable();
DROP TABLE IF EXISTS media_object_secondary_bindings;

ALTER TABLE media_upload_authorizations DROP CONSTRAINT IF EXISTS chk_upload_authorization_sensitive_private;

ALTER TABLE media_objects DROP CONSTRAINT IF EXISTS chk_media_object_sensitive_private;
ALTER TABLE media_objects DROP CONSTRAINT IF EXISTS chk_media_object_revocation;
ALTER TABLE media_objects DROP CONSTRAINT IF EXISTS chk_media_object_binding_pair;
ALTER TABLE media_objects DROP COLUMN IF EXISTS revocation_reason;
ALTER TABLE media_objects DROP COLUMN IF EXISTS revoked_at;
ALTER TABLE media_objects DROP COLUMN IF EXISTS evidence_state;
ALTER TABLE media_objects DROP COLUMN IF EXISTS binding_id;
ALTER TABLE media_objects DROP COLUMN IF EXISTS binding_type;
