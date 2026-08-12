DELETE FROM fleet_partner_role_permissions
WHERE permission_code IN ('integrations.read','integrations.manage','webhooks.manage');
DELETE FROM fleet_partner_permissions
WHERE code IN ('integrations.read','integrations.manage','webhooks.manage');

DROP TABLE IF EXISTS fleet_partner_webhook_fanout_events;
DROP TABLE IF EXISTS fleet_partner_integration_commands;
DROP INDEX IF EXISTS idx_fleet_partner_webhook_delivery_event;
DROP INDEX IF EXISTS idx_fleet_partner_webhook_delivery_subscription;
DROP INDEX IF EXISTS idx_fleet_partner_webhook_delivery_claim;
DROP TABLE IF EXISTS fleet_partner_webhook_deliveries;
DROP INDEX IF EXISTS idx_fleet_partner_webhooks_event_types;
DROP INDEX IF EXISTS idx_fleet_partner_webhooks_org_status;
DROP TABLE IF EXISTS fleet_partner_webhook_subscriptions;
DROP INDEX IF EXISTS idx_fleet_partner_api_clients_expiry;
DROP INDEX IF EXISTS idx_fleet_partner_api_clients_org_status;
DROP TABLE IF EXISTS fleet_partner_organization_api_clients;
