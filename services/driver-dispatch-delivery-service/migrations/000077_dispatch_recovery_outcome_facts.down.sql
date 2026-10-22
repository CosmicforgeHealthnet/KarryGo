-- Already-recorded facts stay in the outbox: they describe outcomes that
-- happened and may already have been delivered.
DROP TRIGGER IF EXISTS dispatch_delivery_recovery_network_analytics ON dispatch_delivery_recovery_actions;
DROP FUNCTION IF EXISTS dispatch_delivery_recovery_network_analytics();
DROP TRIGGER IF EXISTS dispatch_incident_recovery_network_analytics ON dispatch_delivery_incidents;
DROP FUNCTION IF EXISTS dispatch_incident_recovery_network_analytics();
DROP FUNCTION IF EXISTS dispatch_analytics_recovery_fact(text, text, text, jsonb);
