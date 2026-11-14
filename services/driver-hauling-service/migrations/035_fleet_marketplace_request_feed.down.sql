DROP TRIGGER IF EXISTS hauling_fleet_marketplace_feed_publish ON haulage_bookings;
DROP TRIGGER IF EXISTS hauling_fleet_marketplace_feed_version ON haulage_bookings;
DROP FUNCTION IF EXISTS hauling_fleet_marketplace_feed_publish();
DROP FUNCTION IF EXISTS hauling_fleet_marketplace_feed_version();
DROP FUNCTION IF EXISTS hauling_fleet_marketplace_feed_event(text, text, uuid);
DROP FUNCTION IF EXISTS hauling_fleet_marketplace_feed_enabled();
