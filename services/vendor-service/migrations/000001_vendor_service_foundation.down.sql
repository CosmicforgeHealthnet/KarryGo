DROP TABLE IF EXISTS vendor_service_identity;
-- pgcrypto is left installed: other migrations may depend on it and Postgres
-- extension drops are not scoped per-migration in this runner.
