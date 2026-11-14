DROP TRIGGER IF EXISTS taxi_fleet_marketplace_feed_publish ON taxi_bookings;
DROP TRIGGER IF EXISTS taxi_fleet_marketplace_feed_version ON taxi_bookings;
DROP FUNCTION IF EXISTS taxi_fleet_marketplace_feed_publish();
DROP FUNCTION IF EXISTS taxi_fleet_marketplace_feed_version();
DROP FUNCTION IF EXISTS taxi_fleet_marketplace_feed_event(text, text, boolean, boolean, text);
DROP FUNCTION IF EXISTS taxi_fleet_marketplace_feed_enabled();
