-- psql -v runtime_role=<operator-created runtime role> -f admin-db-grants.sql
-- Run with the migration owner in the dedicated Admin DB only, after migration.
-- Never use the database owner/superuser as the application's runtime identity.
BEGIN;
GRANT USAGE ON SCHEMA public TO :"runtime_role";
GRANT SELECT ON admin_schema_versions,admin_remote_principals,admin_revocations TO :"runtime_role";
GRANT SELECT,INSERT ON admin_security_claims,admin_delegated_operations TO :"runtime_role";
GRANT SELECT,INSERT ON admin_rate_limits TO :"runtime_role";
GRANT UPDATE(count) ON admin_rate_limits TO :"runtime_role";
GRANT INSERT ON admin_audit_entries TO :"runtime_role";
GRANT SELECT,INSERT,UPDATE ON admin_policy_revisions TO :"runtime_role";
GRANT USAGE ON SEQUENCE admin_audit_entries_id_seq TO :"runtime_role";
GRANT SELECT,INSERT ON admin_customer_review_operations TO :"runtime_role";
GRANT UPDATE(status,receipt,updated_at) ON admin_customer_review_operations TO :"runtime_role";
GRANT INSERT ON admin_customer_review_operation_audit TO :"runtime_role";
GRANT USAGE ON SEQUENCE admin_customer_review_operation_audit_id_seq TO :"runtime_role";
COMMIT;
