ALTER TABLE haulage_bookings
  DROP COLUMN IF EXISTS route_pickup_polyline,
  DROP COLUMN IF EXISTS route_pickup_distance_m,
  DROP COLUMN IF EXISTS route_pickup_duration_s,
  DROP COLUMN IF EXISTS route_dropoff_polyline,
  DROP COLUMN IF EXISTS route_dropoff_distance_m,
  DROP COLUMN IF EXISTS route_dropoff_duration_s,
  DROP COLUMN IF EXISTS route_fetched_at;
