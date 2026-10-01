-- Reverse of 000001. Order matters: triggers depend on the guard function.
DROP TRIGGER IF EXISTS pricing_audit_log_append_only ON pricing_audit_log;

DROP TABLE IF EXISTS pricing_operators;
DROP TABLE IF EXISTS pricing_outbox;
DROP TABLE IF EXISTS pricing_audit_log;
DROP TABLE IF EXISTS pricing_idempotency_keys;
DROP TABLE IF EXISTS pricing_service_nonces;

DROP FUNCTION IF EXISTS pricing_reject_mutation_purgeable();
DROP FUNCTION IF EXISTS pricing_reject_mutation();
