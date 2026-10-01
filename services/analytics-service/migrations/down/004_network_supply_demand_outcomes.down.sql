-- Reverses 004. Kept outside migrations/ because the runner applies every
-- .sql file there; run deliberately.
DROP TABLE IF EXISTS mission_outcomes_daily;
DROP INDEX IF EXISTS idx_demand_buckets_slot;
DROP TABLE IF EXISTS demand_buckets;
DROP INDEX IF EXISTS idx_supply_product_snapshots_zone;
DROP TABLE IF EXISTS supply_product_snapshots;
