-- Refuses to discard decision evidence. Rolling back is only possible on a
-- database where no Secure Admin decision, appeal or disclosure was recorded.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM admin_owner_command_decisions) OR EXISTS(SELECT 1 FROM admin_owner_context_history)
    OR EXISTS(SELECT 1 FROM admin_owner_read_audit) OR EXISTS(SELECT 1 FROM admin_owner_context_preparation_audit)
    OR EXISTS(SELECT 1 FROM vendor_case_appeals) THEN
  RAISE EXCEPTION 'Section 18 verification decision evidence cannot be rolled back';
 END IF;
END $$;
DROP TRIGGER IF EXISTS vendor_verification_cases_admin_event ON vendor_verification_cases;
DROP FUNCTION IF EXISTS verification_emit_vendor_case_admin_event();
DROP TABLE IF EXISTS admin_event_outbox;
DROP TABLE IF EXISTS admin_owner_command_decisions;
DROP TABLE IF EXISTS admin_owner_read_audit;
DROP TABLE IF EXISTS admin_owner_context_preparation_audit;
DROP TABLE IF EXISTS admin_owner_read_nonces;
DROP TABLE IF EXISTS admin_owner_policy_revisions;
DROP TABLE IF EXISTS admin_owner_context_history;
DROP TABLE IF EXISTS admin_owner_context_heads;
DROP FUNCTION IF EXISTS reject_admin_owner_command_decision_mutation();
DROP FUNCTION IF EXISTS reject_admin_owner_read_audit_mutation();
DROP FUNCTION IF EXISTS reject_admin_owner_context_history_mutation();
DROP TABLE IF EXISTS verification_admin_staff_grants;
DROP TABLE IF EXISTS vendor_case_appeals;
DROP TABLE IF EXISTS vendor_case_automation_findings;
DROP INDEX IF EXISTS vendor_verification_cases_document_refs_idx;
DROP INDEX IF EXISTS vendor_verification_cases_expiry_due_idx;
DROP INDEX IF EXISTS vendor_verification_cases_automation_due_idx;
ALTER TABLE vendor_verification_cases
 DROP COLUMN IF EXISTS automation_lease_expires_at,
 DROP COLUMN IF EXISTS automation_lease_owner,
 DROP COLUMN IF EXISTS automation_evaluated_version,
 DROP COLUMN IF EXISTS automation_recommendation;
DROP TRIGGER IF EXISTS vendor_verification_case_history_append_only ON vendor_verification_case_history;
ALTER TABLE vendor_verification_case_history DROP CONSTRAINT IF EXISTS vendor_case_history_decider_named;
ALTER TABLE vendor_verification_case_history DROP COLUMN IF EXISTS appeal_ref, DROP COLUMN IF EXISTS decided_by_kind;
DROP FUNCTION IF EXISTS verification_reject_append_only_mutation();
