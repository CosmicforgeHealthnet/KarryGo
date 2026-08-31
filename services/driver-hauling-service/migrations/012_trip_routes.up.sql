-- Road-route geometry for the live trip map and the progress bar.
--
-- Two legs are stored per booking because the trip has two distinct phases the
-- customer watches: the driver approaching the pickup, and the cargo moving to
-- the drop-off. Progress is measured along whichever leg is active.
--
-- With no HAULING_ROUTES_API_KEY configured, routes are simply never fetched
-- and consumers fall back to straight-line geometry. The polylines follow the
-- empty-string-not-null convention migration 007 set for proof URLs; the
-- numeric columns stay nullable so "not fetched" is distinguishable from a
-- genuine zero-length route.
ALTER TABLE haulage_bookings
  ADD COLUMN IF NOT EXISTS route_pickup_polyline TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS route_pickup_distance_m BIGINT,
  ADD COLUMN IF NOT EXISTS route_pickup_duration_s BIGINT,
  ADD COLUMN IF NOT EXISTS route_dropoff_polyline TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS route_dropoff_distance_m BIGINT,
  ADD COLUMN IF NOT EXISTS route_dropoff_duration_s BIGINT,
  ADD COLUMN IF NOT EXISTS route_fetched_at TIMESTAMPTZ;
