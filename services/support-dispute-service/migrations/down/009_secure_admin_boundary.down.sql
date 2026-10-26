-- Reverses 009_secure_admin_boundary.sql.
--
-- Kept in migrations/down/ because the runner applies every file directly in
-- migrations/ on every start. Run this by hand, deliberately.
--
-- Immutable Secure Admin evidence (command decisions, context history, read
-- and preparation audit) refuses rollback once it holds any row, exactly like
-- the other owners. Undelivered admin events also refuse rollback: dropping
-- them would silently lose a change the Bridge has not yet seen.
--
-- Not reversible: the 'admin' value added to complainant_type (PostgreSQL
-- cannot drop an enum value). It is harmless when unused.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM admin_owner_command_decisions) OR EXISTS(SELECT 1 FROM admin_owner_context_history)
    OR EXISTS(SELECT 1 FROM admin_owner_read_audit) OR EXISTS(SELECT 1 FROM admin_owner_context_preparation_audit) THEN
  RAISE EXCEPTION 'Secure Admin owner evidence cannot be rolled back';
 END IF;
 IF EXISTS(SELECT 1 FROM admin_event_outbox WHERE status = 'PENDING') THEN
  RAISE EXCEPTION 'Undelivered Secure Admin events cannot be rolled back';
 END IF;
END $$;

DROP TRIGGER IF EXISTS support_admin_case_event_emit ON complaints;
DROP TRIGGER IF EXISTS support_admin_disputes_touch ON disputes;
DROP TRIGGER IF EXISTS support_admin_evidence_touch ON complaint_evidence;
DROP TRIGGER IF EXISTS support_admin_messages_read_touch ON support_chat_messages;
DROP TRIGGER IF EXISTS support_admin_messages_sequence ON support_chat_messages;
DROP TRIGGER IF EXISTS support_admin_disputes_version_update ON disputes;
DROP TRIGGER IF EXISTS support_admin_disputes_version_insert ON disputes;
DROP TRIGGER IF EXISTS support_admin_complaints_version_update ON complaints;
DROP TRIGGER IF EXISTS support_admin_complaints_version_insert ON complaints;
DROP FUNCTION IF EXISTS support_admin_emit_case_event();
DROP FUNCTION IF EXISTS support_admin_case_event(uuid);
DROP FUNCTION IF EXISTS support_admin_message_sequence();
DROP FUNCTION IF EXISTS support_admin_touch_case();
DROP FUNCTION IF EXISTS support_admin_version_insert();
DROP FUNCTION IF EXISTS support_admin_version_update();
DROP FUNCTION IF EXISTS support_admin_rfc3339(timestamptz);
DROP FUNCTION IF EXISTS support_admin_opaque(text);
DROP FUNCTION IF EXISTS support_admin_lacks_trigger(text, text);
DROP FUNCTION IF EXISTS support_admin_lacks_column(text, text);
DROP FUNCTION IF EXISTS support_admin_ddl_if(boolean, text);

DROP TABLE IF EXISTS support_admin_case_emissions;
DROP TABLE IF EXISTS support_admin_reply_deliveries;
DROP TABLE IF EXISTS support_admin_staff_grants;
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

DROP INDEX IF EXISTS uq_support_chat_messages_client_message;
DROP INDEX IF EXISTS uq_support_chat_messages_sequence;
ALTER TABLE support_chat_messages DROP COLUMN IF EXISTS sequence;
ALTER TABLE support_chat_messages DROP COLUMN IF EXISTS client_message_id;

DROP INDEX IF EXISTS uq_disputes_remedy_wallet_operation;
ALTER TABLE disputes DROP CONSTRAINT IF EXISTS disputes_remedy_chk;
ALTER TABLE disputes DROP CONSTRAINT IF EXISTS disputes_admin_version_chk;
ALTER TABLE disputes DROP COLUMN IF EXISTS remedy_recorded_by;
ALTER TABLE disputes DROP COLUMN IF EXISTS remedy_recorded_at;
ALTER TABLE disputes DROP COLUMN IF EXISTS remedy_amount_minor;
ALTER TABLE disputes DROP COLUMN IF EXISTS remedy_wallet_operation_id;
ALTER TABLE disputes DROP COLUMN IF EXISTS remedy_state;
ALTER TABLE disputes DROP COLUMN IF EXISTS admin_version_txid;
ALTER TABLE disputes DROP COLUMN IF EXISTS admin_version;

ALTER TABLE complaints DROP CONSTRAINT IF EXISTS complaints_escalation_level_chk;
ALTER TABLE complaints DROP CONSTRAINT IF EXISTS complaints_message_seq_chk;
ALTER TABLE complaints DROP CONSTRAINT IF EXISTS complaints_admin_version_chk;
ALTER TABLE complaints DROP COLUMN IF EXISTS message_seq;
ALTER TABLE complaints DROP COLUMN IF EXISTS escalation_level;
ALTER TABLE complaints DROP COLUMN IF EXISTS admin_version_txid;
ALTER TABLE complaints DROP COLUMN IF EXISTS admin_version;
