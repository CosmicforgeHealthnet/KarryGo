-- 001_partner_schema_down.sql
-- Rollback for 001_partner_schema.sql.
-- Drop order respects foreign keys (children before parents).

DROP TABLE IF EXISTS partner_webhook_deliveries;
DROP TABLE IF EXISTS partner_webhooks;
DROP TABLE IF EXISTS partner_idempotency_keys;
DROP TABLE IF EXISTS partner_bookings;
DROP TABLE IF EXISTS partner_api_keys;
DROP TABLE IF EXISTS partner_accounts;
