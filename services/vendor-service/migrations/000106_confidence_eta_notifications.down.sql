DROP TRIGGER IF EXISTS commerce_delivery_eta_versions_immutable ON commerce_delivery_eta_versions;
DROP FUNCTION IF EXISTS reject_commerce_eta_history_mutation();
DROP TABLE IF EXISTS commerce_scheduled_notification_markers;
DROP TABLE IF EXISTS commerce_delivery_eta_current;
DROP TABLE IF EXISTS commerce_delivery_eta_versions;
