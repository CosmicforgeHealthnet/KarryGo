DROP TRIGGER IF EXISTS execution_provider_rebind_audit_no_mutation ON execution_provider_rebind_audit;
DROP FUNCTION IF EXISTS execution_provider_rebind_audit_append_only();
DROP TABLE IF EXISTS execution_provider_rebind_audit;
ALTER TABLE payment_intents DROP COLUMN IF EXISTS execution_binding_version;
