-- Phase B11 dev seed. Idempotent: every insert is keyed on a FIXED UUID with
-- ON CONFLICT DO NOTHING, so running this against a database that already
-- has some or all of these rows (including the dev provider used throughout
-- this project's own live-testing sessions, caca4b5a-...) is always a safe
-- no-op for anything that already exists, and creates everything identically
-- on a fresh database. Every UUID literal below uses only 0-9a-f -- the
-- dispatch seeder's first draft once used g/h/i prefixes and Postgres
-- rejected the whole file; documented once already on this platform, must
-- not recur here.

-- ─── Market + operating zone (Lagos) ───────────────────────────────────────
INSERT INTO markets (id, country, name, code) VALUES
  ('a0000000-0000-4000-8000-000000000001', 'NG', 'Lagos', 'NG-LAG')
ON CONFLICT (id) DO NOTHING;

INSERT INTO operating_zones (id, market_id, name, code, center_lat, center_lng, supported_classes, match_radius_km) VALUES
  ('a0000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000001', 'Lagos Mainland', 'NG-LAG-MAIN', 6.52, 3.37, '{economy,comfort,xl}', 25.00)
ON CONFLICT (id) DO NOTHING;

-- ─── Providers spanning every addendum §11.1 compliance dimension ──────────
-- caca4b5a-...: fully clean/ready -- the SAME id used throughout this
-- project's live-testing sessions (see project memory), reused here rather
-- than creating a redundant second "clean" provider.
--
-- accepts_parcels is true ONLY for this one: the column defaults to false, so
-- without an opted-in driver every parcel booking on a fresh database goes
-- unmatched and the feature looks broken rather than merely unstaffed. The
-- other seeded providers keep the default, which is what the matcher's
-- opt-out path is tested against.
INSERT INTO taxi_providers (id, phone, first_name, last_name, account_status, onboarding_status, identity_verification_status, vehicle_verification_status, document_compliance_status, accepts_parcels) VALUES
  ('caca4b5a-7018-4fa8-9d08-c3f14f6d910c', '+2348099990001', 'Ada', 'Okafor', 'active', 'complete', 'verified', 'verified', 'compliant', true)
ON CONFLICT (id) DO NOTHING;

-- account_status = suspended (otherwise fully compliant).
INSERT INTO taxi_providers (id, phone, first_name, last_name, account_status, onboarding_status, identity_verification_status, vehicle_verification_status, document_compliance_status) VALUES
  ('b0000000-0000-4000-8000-000000000002', '+2348099990002', 'Bola', 'Suspended', 'suspended', 'complete', 'verified', 'verified', 'compliant')
ON CONFLICT (id) DO NOTHING;

-- identity_verification_status = failed.
INSERT INTO taxi_providers (id, phone, first_name, last_name, account_status, onboarding_status, identity_verification_status, vehicle_verification_status, document_compliance_status) VALUES
  ('b0000000-0000-4000-8000-000000000003', '+2348099990003', 'Chidi', 'IdentityFailed', 'active', 'complete', 'failed', 'verified', 'compliant')
ON CONFLICT (id) DO NOTHING;

-- vehicle_verification_status = pending.
INSERT INTO taxi_providers (id, phone, first_name, last_name, account_status, onboarding_status, identity_verification_status, vehicle_verification_status, document_compliance_status) VALUES
  ('b0000000-0000-4000-8000-000000000004', '+2348099990004', 'Dupe', 'VehiclePending', 'active', 'complete', 'verified', 'pending', 'compliant')
ON CONFLICT (id) DO NOTHING;

-- document_compliance_status = expired.
INSERT INTO taxi_providers (id, phone, first_name, last_name, account_status, onboarding_status, identity_verification_status, vehicle_verification_status, document_compliance_status) VALUES
  ('b0000000-0000-4000-8000-000000000005', '+2348099990005', 'Emeka', 'DocsExpired', 'active', 'complete', 'verified', 'verified', 'expired')
ON CONFLICT (id) DO NOTHING;

-- One active vehicle per provider -- CanGoAvailable's vehicle-operational
-- dimension must be satisfied for every seeded driver EXCEPT the clean one's
-- own vehicle check, so each failure mode above is isolated to exactly the
-- ONE dimension it names, not compounded by also having no vehicle at all.
INSERT INTO vehicles (id, provider_id, vehicle_class, plate_number, seats, status) VALUES
  ('7e4cc5a5-f9e2-4978-846e-56fdbeb390d9', 'caca4b5a-7018-4fa8-9d08-c3f14f6d910c', 'comfort', 'LAG-500-BB', 4, 'active')
ON CONFLICT (id) DO NOTHING;

INSERT INTO vehicles (id, provider_id, vehicle_class, plate_number, seats, status) VALUES
  ('c0000000-0000-4000-8000-000000000002', 'b0000000-0000-4000-8000-000000000002', 'economy', 'LAG-500-CC', 4, 'active'),
  ('c0000000-0000-4000-8000-000000000003', 'b0000000-0000-4000-8000-000000000003', 'economy', 'LAG-500-DD', 4, 'active'),
  ('c0000000-0000-4000-8000-000000000004', 'b0000000-0000-4000-8000-000000000004', 'economy', 'LAG-500-EE', 4, 'active'),
  ('c0000000-0000-4000-8000-000000000005', 'b0000000-0000-4000-8000-000000000005', 'economy', 'LAG-500-FF', 4, 'active')
ON CONFLICT (id) DO NOTHING;

-- ─── Bookings covering every terminal/non-terminal status the addendum's
-- own B11 test-plan row names, all assigned to the clean provider, all on a
-- fixed SYNTHETIC customer id (not a real customer-service account -- the
-- same convention hauling/dispatch's own dev seeds already use). ──────────

-- Completed: full ladder, settled fare, paid.
INSERT INTO taxi_bookings (
  id, customer_id, provider_id, vehicle_id, market_id, operating_zone_id,
  vehicle_class, status, pickup_verification_status,
  fare_estimate_kobo, fare_final_kobo, pricing_quote_reference, pricing_version,
  distance_km, duration_minutes, actual_distance_km,
  payment_method, payment_status,
  matched_at, accepted_at, arrived_at, started_at, completed_at, matching_started_at
) VALUES (
  'e0000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000001',
  'caca4b5a-7018-4fa8-9d08-c3f14f6d910c', '7e4cc5a5-f9e2-4978-846e-56fdbeb390d9',
  'a0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000002',
  'comfort', 'completed', 'verified',
  246550, 246550, 'seed-quote-completed', 'provisional-dev-v1',
  8.71, 22, 8.9,
  'cash', 'paid',
  now() - interval '2 days' + interval '1 minute', now() - interval '2 days' + interval '2 minutes',
  now() - interval '2 days' + interval '7 minutes', now() - interval '2 days' + interval '9 minutes',
  now() - interval '2 days' + interval '30 minutes', now() - interval '2 days'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO taxi_booking_stops (booking_id, sequence, type, address, lat, lng, status) VALUES
  ('e0000000-0000-4000-8000-000000000001', 1, 'PICKUP', 'Ikeja', 6.5244, 3.3792, 'completed'),
  ('e0000000-0000-4000-8000-000000000001', 2, 'DROPOFF', 'Victoria Island', 6.5300, 3.3850, 'completed')
ON CONFLICT (booking_id, sequence) DO NOTHING;

-- Cancelled: customer cancelled pre-pickup, refunded.
INSERT INTO taxi_bookings (
  id, customer_id, provider_id, vehicle_id, market_id, operating_zone_id,
  vehicle_class, status, pickup_verification_status,
  fare_estimate_kobo, pricing_quote_reference, pricing_version,
  distance_km, duration_minutes,
  payment_method, payment_status,
  cancel_reason, cancelled_by,
  matched_at, accepted_at, cancelled_at, matching_started_at
) VALUES (
  'e0000000-0000-4000-8000-000000000002', 'd0000000-0000-4000-8000-000000000001',
  'caca4b5a-7018-4fa8-9d08-c3f14f6d910c', '7e4cc5a5-f9e2-4978-846e-56fdbeb390d9',
  'a0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000002',
  'comfort', 'cancelled', 'not_required',
  120000, 'seed-quote-cancelled', 'provisional-dev-v1',
  3.2, 10,
  'wallet', 'refunded',
  'changed my mind', 'customer',
  now() - interval '1 day' + interval '1 minute', now() - interval '1 day' + interval '2 minutes',
  now() - interval '1 day' + interval '5 minutes', now() - interval '1 day'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO taxi_booking_stops (booking_id, sequence, type, address, lat, lng, status) VALUES
  ('e0000000-0000-4000-8000-000000000002', 1, 'PICKUP', 'Yaba', 6.5158, 3.3696, 'pending'),
  ('e0000000-0000-4000-8000-000000000002', 2, 'DROPOFF', 'Surulere', 6.4926, 3.3547, 'pending')
ON CONFLICT (booking_id, sequence) DO NOTHING;

-- Unmatched: no driver was available in time.
INSERT INTO taxi_bookings (
  id, customer_id, market_id, operating_zone_id,
  vehicle_class, status, pickup_verification_status,
  fare_estimate_kobo, pricing_quote_reference, pricing_version,
  distance_km, duration_minutes,
  payment_method, payment_status, matching_started_at
) VALUES (
  'e0000000-0000-4000-8000-000000000003', 'd0000000-0000-4000-8000-000000000001',
  'a0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000002',
  'xl', 'unmatched', 'not_required',
  180000, 'seed-quote-unmatched', 'provisional-dev-v1',
  6.4, 18,
  'cash', 'unpaid', now() - interval '3 hours'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO taxi_booking_stops (booking_id, sequence, type, address, lat, lng, status) VALUES
  ('e0000000-0000-4000-8000-000000000003', 1, 'PICKUP', 'Apapa', 6.4432, 3.3596, 'pending'),
  ('e0000000-0000-4000-8000-000000000003', 2, 'DROPOFF', 'Ikeja', 6.5244, 3.3792, 'pending')
ON CONFLICT (booking_id, sequence) DO NOTHING;

-- In progress: pickup already verified, trip underway, nothing settled yet.
--
-- Deliberately assigned to the SUSPENDED provider, NOT the clean one.
-- uq_taxi_bookings_one_active_per_provider is a partial unique index over
-- provider_id for every in-trip status, so parking a permanent in_progress
-- booking on the clean provider makes MarkMatched fail with a 23505 for
-- every future booking -- i.e. the one driver ./cmd/online exists to make
-- matchable could never actually be matched, and every customer booking sat
-- at pending_match until its search window elapsed. The suspended provider
-- can never be offered a ride anyway, so holding the demo trip costs
-- nothing there. A driver-app tester reaches the active-trip screen by
-- accepting a real booking, which now works.
INSERT INTO taxi_bookings (
  id, customer_id, provider_id, vehicle_id, market_id, operating_zone_id,
  vehicle_class, status, pickup_verification_status,
  fare_estimate_kobo, pricing_quote_reference, pricing_version,
  distance_km, duration_minutes,
  payment_method, payment_status,
  matched_at, accepted_at, arrived_at, started_at, matching_started_at
) VALUES (
  'e0000000-0000-4000-8000-000000000004', 'd0000000-0000-4000-8000-000000000001',
  'b0000000-0000-4000-8000-000000000002', 'c0000000-0000-4000-8000-000000000002',
  'a0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000002',
  'economy', 'in_progress', 'verified',
  205000, 'seed-quote-inprogress', 'provisional-dev-v1',
  7.1, 19,
  'cash', 'unpaid',
  now() - interval '20 minutes', now() - interval '19 minutes',
  now() - interval '14 minutes', now() - interval '12 minutes', now() - interval '20 minutes'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO taxi_booking_stops (booking_id, sequence, type, address, lat, lng, status) VALUES
  ('e0000000-0000-4000-8000-000000000004', 1, 'PICKUP', 'Lekki Phase 1', 6.4408, 3.4653, 'completed'),
  ('e0000000-0000-4000-8000-000000000004', 2, 'DROPOFF', 'Victoria Island', 6.5300, 3.3850, 'pending')
ON CONFLICT (booking_id, sequence) DO NOTHING;

-- Scheduled: booked ahead, not yet promoted to matching (phase B9).
INSERT INTO taxi_bookings (
  id, customer_id, market_id, operating_zone_id,
  vehicle_class, status, pickup_verification_status,
  fare_estimate_kobo, pricing_quote_reference, pricing_version,
  distance_km, duration_minutes,
  payment_method, payment_status,
  scheduled_at, dispatch_start_at
) VALUES (
  'e0000000-0000-4000-8000-000000000005', 'd0000000-0000-4000-8000-000000000001',
  'a0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000002',
  'economy', 'scheduled', 'not_required',
  95000, 'seed-quote-scheduled', 'provisional-dev-v1',
  4.0, 14,
  'wallet', 'unpaid',
  now() + interval '1 day', now() + interval '1 day' - interval '10 minutes'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO taxi_booking_stops (booking_id, sequence, type, address, lat, lng, status) VALUES
  ('e0000000-0000-4000-8000-000000000005', 1, 'PICKUP', 'Ikoyi', 6.4550, 3.4340, 'pending'),
  ('e0000000-0000-4000-8000-000000000005', 2, 'DROPOFF', 'Ikeja', 6.5244, 3.3792, 'pending')
ON CONFLICT (booking_id, sequence) DO NOTHING;
