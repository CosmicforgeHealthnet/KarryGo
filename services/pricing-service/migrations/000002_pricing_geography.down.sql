-- Reverse of 000002.
DROP TRIGGER IF EXISTS pricing_zone_versions_append_only ON pricing_zone_versions;

DROP TABLE IF EXISTS pricing_zone_mappings;
DROP TABLE IF EXISTS pricing_zone_versions;
DROP TABLE IF EXISTS pricing_zones;
DROP TABLE IF EXISTS pricing_markets;
DROP TABLE IF EXISTS pricing_states;
DROP TABLE IF EXISTS pricing_countries;
