DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM admin_owner_command_decisions) OR EXISTS(SELECT 1 FROM admin_owner_context_history) OR EXISTS(SELECT 1 FROM admin_owner_read_audit) OR EXISTS(SELECT 1 FROM admin_owner_context_preparation_audit) THEN
  RAISE EXCEPTION 'Secure Admin owner evidence cannot be rolled back';
 END IF;
END $$;
DROP TABLE IF EXISTS admin_owner_command_decisions;
DROP TABLE IF EXISTS admin_owner_read_audit;
DROP TABLE IF EXISTS admin_owner_context_preparation_audit;
DROP TABLE IF EXISTS admin_owner_read_nonces;
DROP TABLE IF EXISTS admin_owner_policy_revisions;
DROP TABLE IF EXISTS admin_owner_context_history;
DROP TABLE IF EXISTS admin_owner_context_heads;
DROP TABLE IF EXISTS referral_admin_staff_grants;
ALTER TABLE fraud_flags DROP COLUMN IF EXISTS version;
ALTER TABLE reward_ledger DROP COLUMN IF EXISTS version;
ALTER TABLE referrals DROP COLUMN IF EXISTS version;
ALTER TABLE referral_persons DROP COLUMN IF EXISTS version;
DROP FUNCTION IF EXISTS reject_admin_owner_command_decision_mutation();
DROP FUNCTION IF EXISTS reject_admin_owner_read_audit_mutation();
DROP FUNCTION IF EXISTS reject_admin_owner_context_history_mutation();
