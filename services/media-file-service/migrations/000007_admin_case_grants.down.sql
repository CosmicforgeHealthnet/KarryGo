-- Grant evidence must be archived under an approved retention procedure.
DO $$ BEGIN IF EXISTS(SELECT 1 FROM media_admin_case_grants) OR EXISTS(SELECT 1 FROM media_admin_case_revocations) OR EXISTS(SELECT 1 FROM media_admin_case_policy_heads) THEN RAISE EXCEPTION 'Cannot roll back populated case grant evidence'; END IF; END $$;
DROP TABLE media_admin_case_policy_heads;
DROP TABLE media_admin_case_revocations;
DROP TABLE media_admin_case_grants;
DROP FUNCTION reject_media_admin_case_mutation();
