-- Road-route geometry for the live delivery map and the progress bar.
--
-- Two legs are stored per booking because the customer watches two distinct
-- phases: the rider approaching the pickup, then the package moving to the
-- drop-off. Progress is measured along whichever leg is active.
--
-- With no DISPATCH_ROUTES_API_KEY configured, routes are never fetched and
-- consumers fall back to straight-line geometry. Polylines default to the
-- empty string so they scan as plain strings; the numeric columns stay
-- nullable so "not fetched" stays distinguishable from a zero-length route.
ALTER TABLE dispatch_bookings
  ADD COLUMN IF NOT EXISTS route_pickup_polyline TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS route_pickup_distance_m BIGINT,
  ADD COLUMN IF NOT EXISTS route_pickup_duration_s BIGINT,
  ADD COLUMN IF NOT EXISTS route_dropoff_polyline TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS route_dropoff_distance_m BIGINT,
  ADD COLUMN IF NOT EXISTS route_dropoff_duration_s BIGINT,
  ADD COLUMN IF NOT EXISTS route_fetched_at TIMESTAMPTZ;
