-- Dev seed: dispatch riders that can log into the dispatch_provider app and
-- go online immediately (all availability gates pass) — plus 3 riders in
-- deliberately varied states, and a small history of dispatch_bookings/trips
-- so the customer/rider apps have something to show without driving a full
-- booking manually.
--
-- Idempotent: fixed UUIDs + ON CONFLICT DO NOTHING; safe to re-run.
--
-- Login: use the phone numbers below with POST /api/v1/auth/login/start.
-- With DISPATCH_RIDER_DEBUG_OTP=true the OTP is both printed in the SERVICE
-- LOGS ("development dispatch rider login otp ...") AND returned as
-- `debug_otp` in the HTTP response (shown on-screen by the app in debug
-- builds) — never in a production build/backend.
--
-- Riders:
--   +2348077770001  Emeka Dispatch    fully verified, ready to go online (motorcycle, LAG-101-DR)
--   +2348077770002  Bisi Rider        fully verified, ready to go online (dispatch_bike, LAG-102-DR)
--   +2348077770003  Ngozi Onboarding  mid-onboarding: verified_to_go_online=false, no bike yet
--                                     -> go-online fails with GateNotVerified
--   +2348077770004  Tunde Suspended   fully onboarded but verification_status='suspended'
--                                     -> go-online fails with GateAccountSuspended
--   +2348077770005  Chidi Pending     verified provider, bike verification_status='pending'
--                                     -> go-online fails the verified-active-bike gate
--
-- Only +2348077770001 / +2348077770002 belong in cmd/online's seededRiders
-- list — the other three are deliberately NOT go-online-ready, so they need
-- to keep failing that gate to stay useful as test fixtures.
--
-- "Fully verified" for riders 1/2 means BOTH: providers.verification_status
-- ='verified' AND all 5 REQUIRED verification_steps (identity, vehicle,
-- face, guarantor, emergency) are 'approved' (licence is optional and
-- excluded). The app's verification badge / GET /provider/verification/status
-- computes its own overall_status from verification_steps and ignores
-- providers.verification_status entirely, so seeding only the flat column
-- (as an earlier version of this seed did) left the app showing "Unverified"
-- despite the DB looking fully verified.
--
-- Booking/trip history (synthetic customer_id e0000000-0000-4000-8000-000000000001,
-- NOT a real customer-service account — it won't show up if you log into the
-- customer app with a real login, only useful for backend/DB/rider-app testing):
--   2 completed deliveries (one on rider 1, one on rider 2) with a full trip
--     lifecycle, delivery proof, and a review (ratings row).
--   1 cancelled booking (customer cancelled before a rider was matched).
--   1 unmatched booking (no rider found in time).
--   1 in-progress booking on rider 1 (trip status='in_progress') so the
--     "Ongoing" bucket / active-trip screens have something to show.

-- ── Reclaim seed identifiers ─────────────────────────────────────────────────
-- If a stray non-seed row (e.g. a manual test account) already holds one of the
-- seed phone numbers / emails / plate numbers, the unique constraint would make
-- the seed insert silently no-op and later FK inserts fail. Rename such rows'
-- identifiers out of the way instead of deleting them (deletes can hit trip FKs).

UPDATE providers
SET phone = phone || '.old.' || substr(id::text, 1, 8)
WHERE phone IN ('+2348077770001', '+2348077770002', '+2348077770003', '+2348077770004', '+2348077770005')
  AND id NOT IN (
    'd1000000-0000-4000-8000-000000000001', 'd2000000-0000-4000-8000-000000000002',
    'd3000000-0000-4000-8000-000000000003', 'd4000000-0000-4000-8000-000000000004',
    'd5000000-0000-4000-8000-000000000005'
  );

UPDATE providers
SET email = NULL
WHERE email IN (
    'emeka.dispatch@karrygo.dev', 'bisi.rider@karrygo.dev',
    'ngozi.onboarding@karrygo.dev', 'tunde.suspended@karrygo.dev', 'chidi.pending@karrygo.dev'
  )
  AND id NOT IN (
    'd1000000-0000-4000-8000-000000000001', 'd2000000-0000-4000-8000-000000000002',
    'd3000000-0000-4000-8000-000000000003', 'd4000000-0000-4000-8000-000000000004',
    'd5000000-0000-4000-8000-000000000005'
  );

UPDATE dispatch_rider_identities
SET phone_number = phone_number || '.old.' || substr(id::text, 1, 8)
WHERE phone_number IN ('+2348077770001', '+2348077770002', '+2348077770003', '+2348077770004', '+2348077770005')
  AND id NOT IN (
    'd1000000-0000-4000-8000-000000000001', 'd2000000-0000-4000-8000-000000000002',
    'd3000000-0000-4000-8000-000000000003', 'd4000000-0000-4000-8000-000000000004',
    'd5000000-0000-4000-8000-000000000005'
  );

UPDATE dispatch_rider_identities
SET email = NULL
WHERE email IN (
    'emeka.dispatch@karrygo.dev', 'bisi.rider@karrygo.dev',
    'ngozi.onboarding@karrygo.dev', 'tunde.suspended@karrygo.dev', 'chidi.pending@karrygo.dev'
  )
  AND id NOT IN (
    'd1000000-0000-4000-8000-000000000001', 'd2000000-0000-4000-8000-000000000002',
    'd3000000-0000-4000-8000-000000000003', 'd4000000-0000-4000-8000-000000000004',
    'd5000000-0000-4000-8000-000000000005'
  );

UPDATE bikes
SET plate_number = plate_number || '.old.' || substr(id::text, 1, 8)
WHERE plate_number IN ('LAG-101-DR', 'LAG-102-DR', 'LAG-104-DR', 'LAG-105-DR')
  AND id NOT IN (
    'b1000000-0000-4000-8000-000000000001', 'b2000000-0000-4000-8000-000000000002',
    'b4000000-0000-4000-8000-000000000004', 'b5000000-0000-4000-8000-000000000005'
  );

-- ── Rider 1 ──────────────────────────────────────────────────────────────────

INSERT INTO dispatch_rider_identities (id, phone_number, email, status)
VALUES ('d1000000-0000-4000-8000-000000000001', '+2348077770001', 'emeka.dispatch@karrygo.dev', 'active')
ON CONFLICT DO NOTHING;

INSERT INTO providers (
    id, phone, full_name, email, state, city, country, operation_type,
    verification_status, is_active, onboarding_complete
) VALUES (
    'd1000000-0000-4000-8000-000000000001', '+2348077770001', 'Emeka Dispatch',
    'emeka.dispatch@karrygo.dev', 'Lagos', 'Ikeja', 'NG', 'individual',
    'verified', true, true
)
ON CONFLICT DO NOTHING;

INSERT INTO provider_availability (provider_id, status, verified_to_go_online)
VALUES ('d1000000-0000-4000-8000-000000000001', 'offline', true)
ON CONFLICT DO NOTHING;

INSERT INTO bikes (
    id, provider_id, bike_type, brand, model, year, color, plate_number,
    verification_status, is_active, is_primary
) VALUES (
    'b1000000-0000-4000-8000-000000000001', 'd1000000-0000-4000-8000-000000000001',
    'motorcycle', 'Bajaj', 'Boxer 150', 2023, 'Red', 'LAG-101-DR',
    'verified', true, true
)
ON CONFLICT DO NOTHING;

INSERT INTO verification_steps (provider_id, step, is_optional, status, submitted_at, reviewed_at, licence_expiry_date)
VALUES ('d1000000-0000-4000-8000-000000000001', 'licence', true, 'approved', now(), now(), (now() + interval '2 years')::date)
ON CONFLICT DO NOTHING;

-- The go-online gate (checkOnlineGate) only reads providers/provider_availability/bikes,
-- but the app's verification badge (GET /provider/verification/status) computes its own
-- "overall status" from these five REQUIRED steps and ignores providers.verification_status
-- entirely — a provider with verification_status='verified' still shows "Unverified" in the
-- app until identity/vehicle/face/guarantor/emergency are all approved here too.
INSERT INTO verification_steps (provider_id, step, is_optional, is_auto_confirmed, confirm_method, status, submitted_at, reviewed_at) VALUES
    ('d1000000-0000-4000-8000-000000000001', 'identity',   false, false, NULL,    'approved', now(), now()),
    ('d1000000-0000-4000-8000-000000000001', 'vehicle',    false, false, NULL,    'approved', now(), now()),
    ('d1000000-0000-4000-8000-000000000001', 'face',       false, false, NULL,    'approved', now(), now()),
    ('d1000000-0000-4000-8000-000000000001', 'guarantor',  false, true,  'auto',  'approved', now(), now()),
    ('d1000000-0000-4000-8000-000000000001', 'emergency',  false, true,  'auto',  'approved', now(), now())
ON CONFLICT DO NOTHING;

-- ── Rider 2 ──────────────────────────────────────────────────────────────────

INSERT INTO dispatch_rider_identities (id, phone_number, email, status)
VALUES ('d2000000-0000-4000-8000-000000000002', '+2348077770002', 'bisi.rider@karrygo.dev', 'active')
ON CONFLICT DO NOTHING;

INSERT INTO providers (
    id, phone, full_name, email, state, city, country, operation_type,
    verification_status, is_active, onboarding_complete
) VALUES (
    'd2000000-0000-4000-8000-000000000002', '+2348077770002', 'Bisi Rider',
    'bisi.rider@karrygo.dev', 'Lagos', 'Surulere', 'NG', 'individual',
    'verified', true, true
)
ON CONFLICT DO NOTHING;

INSERT INTO provider_availability (provider_id, status, verified_to_go_online)
VALUES ('d2000000-0000-4000-8000-000000000002', 'offline', true)
ON CONFLICT DO NOTHING;

INSERT INTO bikes (
    id, provider_id, bike_type, brand, model, year, color, plate_number,
    verification_status, is_active, is_primary
) VALUES (
    'b2000000-0000-4000-8000-000000000002', 'd2000000-0000-4000-8000-000000000002',
    'dispatch_bike', 'TVS', 'HLX 125', 2024, 'Green', 'LAG-102-DR',
    'verified', true, true
)
ON CONFLICT DO NOTHING;

INSERT INTO verification_steps (provider_id, step, is_optional, status, submitted_at, reviewed_at, licence_expiry_date)
VALUES ('d2000000-0000-4000-8000-000000000002', 'licence', true, 'approved', now(), now(), (now() + interval '2 years')::date)
ON CONFLICT DO NOTHING;

INSERT INTO verification_steps (provider_id, step, is_optional, is_auto_confirmed, confirm_method, status, submitted_at, reviewed_at) VALUES
    ('d2000000-0000-4000-8000-000000000002', 'identity',   false, false, NULL,    'approved', now(), now()),
    ('d2000000-0000-4000-8000-000000000002', 'vehicle',    false, false, NULL,    'approved', now(), now()),
    ('d2000000-0000-4000-8000-000000000002', 'face',       false, false, NULL,    'approved', now(), now()),
    ('d2000000-0000-4000-8000-000000000002', 'guarantor',  false, true,  'auto',  'approved', now(), now()),
    ('d2000000-0000-4000-8000-000000000002', 'emergency',  false, true,  'auto',  'approved', now(), now())
ON CONFLICT DO NOTHING;

-- ── Rider 3 — mid-onboarding, cannot go online yet ──────────────────────────

INSERT INTO dispatch_rider_identities (id, phone_number, email, status)
VALUES ('d3000000-0000-4000-8000-000000000003', '+2348077770003', 'ngozi.onboarding@karrygo.dev', 'active')
ON CONFLICT DO NOTHING;

INSERT INTO providers (
    id, phone, full_name, email, state, city, country, operation_type,
    verification_status, is_active, onboarding_complete
) VALUES (
    'd3000000-0000-4000-8000-000000000003', '+2348077770003', 'Ngozi Onboarding',
    'ngozi.onboarding@karrygo.dev', 'Lagos', 'Ajah', 'NG', 'individual',
    'pending_review', true, false
)
ON CONFLICT DO NOTHING;

-- verified_to_go_online stays false: this is the GateNotVerified fixture.
INSERT INTO provider_availability (provider_id, status, verified_to_go_online)
VALUES ('d3000000-0000-4000-8000-000000000003', 'offline', false)
ON CONFLICT DO NOTHING;

-- No bike yet — still mid-onboarding.

INSERT INTO verification_steps (provider_id, step, status, submitted_at)
VALUES ('d3000000-0000-4000-8000-000000000003', 'licence', 'submitted', now())
ON CONFLICT DO NOTHING;

-- ── Rider 4 — fully onboarded, then suspended ───────────────────────────────

INSERT INTO dispatch_rider_identities (id, phone_number, email, status)
VALUES ('d4000000-0000-4000-8000-000000000004', '+2348077770004', 'tunde.suspended@karrygo.dev', 'active')
ON CONFLICT DO NOTHING;

INSERT INTO providers (
    id, phone, full_name, email, state, city, country, operation_type,
    verification_status, is_active, onboarding_complete
) VALUES (
    'd4000000-0000-4000-8000-000000000004', '+2348077770004', 'Tunde Suspended',
    'tunde.suspended@karrygo.dev', 'Lagos', 'Ojota', 'NG', 'individual',
    'suspended', true, true
)
ON CONFLICT DO NOTHING;

-- Passed the online gate before being suspended: verified_to_go_online is
-- still true, so this row specifically exercises GateAccountSuspended rather
-- than GateNotVerified.
INSERT INTO provider_availability (provider_id, status, verified_to_go_online)
VALUES ('d4000000-0000-4000-8000-000000000004', 'offline', true)
ON CONFLICT DO NOTHING;

INSERT INTO bikes (
    id, provider_id, bike_type, brand, model, year, color, plate_number,
    verification_status, is_active, is_primary
) VALUES (
    'b4000000-0000-4000-8000-000000000004', 'd4000000-0000-4000-8000-000000000004',
    'motorcycle', 'Honda', 'CB 125', 2022, 'Black', 'LAG-104-DR',
    'verified', true, true
)
ON CONFLICT DO NOTHING;

INSERT INTO verification_steps (provider_id, step, status, submitted_at, reviewed_at, licence_expiry_date)
VALUES ('d4000000-0000-4000-8000-000000000004', 'licence', 'approved', now(), now(), (now() + interval '2 years')::date)
ON CONFLICT DO NOTHING;

-- ── Rider 5 — verified provider, unverified bike ────────────────────────────

INSERT INTO dispatch_rider_identities (id, phone_number, email, status)
VALUES ('d5000000-0000-4000-8000-000000000005', '+2348077770005', 'chidi.pending@karrygo.dev', 'active')
ON CONFLICT DO NOTHING;

INSERT INTO providers (
    id, phone, full_name, email, state, city, country, operation_type,
    verification_status, is_active, onboarding_complete
) VALUES (
    'd5000000-0000-4000-8000-000000000005', '+2348077770005', 'Chidi Pending',
    'chidi.pending@karrygo.dev', 'Lagos', 'Gbagada', 'NG', 'individual',
    'verified', true, true
)
ON CONFLICT DO NOTHING;

INSERT INTO provider_availability (provider_id, status, verified_to_go_online)
VALUES ('d5000000-0000-4000-8000-000000000005', 'offline', true)
ON CONFLICT DO NOTHING;

-- verification_status='pending' -> fails the verified-active-bike gate even
-- though the provider itself is fully verified.
INSERT INTO bikes (
    id, provider_id, bike_type, brand, model, year, color, plate_number,
    verification_status, is_active, is_primary
) VALUES (
    'b5000000-0000-4000-8000-000000000005', 'd5000000-0000-4000-8000-000000000005',
    'dispatch_bike', 'TVS', 'Star City', 2023, 'Blue', 'LAG-105-DR',
    'pending', true, true
)
ON CONFLICT DO NOTHING;

INSERT INTO verification_steps (provider_id, step, status, submitted_at, reviewed_at, licence_expiry_date)
VALUES ('d5000000-0000-4000-8000-000000000005', 'licence', 'approved', now(), now(), (now() + interval '2 years')::date)
ON CONFLICT DO NOTHING;

-- ── Booking / trip history ───────────────────────────────────────────────────
-- Synthetic customer_id — not a real customer-service account (no cross-service
-- FK is enforced on this schema; same approach the hauling seed uses).

-- Booking A: completed, on rider 1 (Emeka).

INSERT INTO dispatch_bookings (
    id, customer_id, status,
    pickup_address, pickup_lat, pickup_lng,
    dropoff_address, dropoff_lat, dropoff_lng,
    distance_km, fare_amount, currency,
    payment_method, payment_status, payment_ref,
    receiver_name, receiver_phone,
    package_desc, package_type, package_size, is_fragile, service_tier,
    provider_id, completed_at, created_at
) VALUES (
    'f1000000-0000-4000-8000-000000000001', 'e0000000-0000-4000-8000-000000000001', 'completed',
    'Computer Village, Ikeja, Lagos', 6.6018, 3.3515,
    'Yaba, Lagos', 6.5095, 3.3711,
    10.20, 260000, 'NGN',
    'wallet', 'paid', 'seed-wallet-ref-f1000000',
    'Aisha Bello', '+2348022223001',
    'Sealed envelope of documents', 'documents', 'small', false, 'standard',
    'd1000000-0000-4000-8000-000000000001',
    NOW() - INTERVAL '3 days', NOW() - INTERVAL '3 days' - INTERVAL '40 min'
)
ON CONFLICT DO NOTHING;

INSERT INTO trips (
    id, booking_id, provider_id, customer_id, status,
    pickup_address, pickup_lat, pickup_lng,
    dropoff_address, dropoff_lat, dropoff_lng,
    distance_km, fare_amount, currency,
    receiver_name, receiver_phone,
    package_desc, package_weight, package_type, package_size, is_fragile, service_tier,
    started_at, arrived_at, completed_at, created_at
) VALUES (
    'a1000000-0000-4000-8000-000000000001', 'f1000000-0000-4000-8000-000000000001',
    'd1000000-0000-4000-8000-000000000001', 'e0000000-0000-4000-8000-000000000001', 'completed',
    'Computer Village, Ikeja, Lagos', 6.6018, 3.3515,
    'Yaba, Lagos', 6.5095, 3.3711,
    10.20, 260000, 'NGN',
    'Aisha Bello', '+2348022223001',
    'Sealed envelope of documents', 0.50, 'documents', 'small', false, 'standard',
    NOW() - INTERVAL '3 days' - INTERVAL '25 min', NOW() - INTERVAL '3 days' - INTERVAL '30 min',
    NOW() - INTERVAL '3 days', NOW() - INTERVAL '3 days' - INTERVAL '38 min'
)
ON CONFLICT DO NOTHING;

INSERT INTO trip_state_log (id, trip_id, from_status, to_status, changed_at, changed_by, notes) VALUES
    ('c1000000-0000-4000-8000-000000000001', 'a1000000-0000-4000-8000-000000000001', 'none',            'assigned',        NOW() - INTERVAL '3 days' - INTERVAL '38 min', 'system',   'created_from_request_accepted'),
    ('c1000000-0000-4000-8000-000000000002', 'a1000000-0000-4000-8000-000000000001', 'assigned',        'en_route_pickup', NOW() - INTERVAL '3 days' - INTERVAL '35 min', 'provider', 'provider_en_route_pickup'),
    ('c1000000-0000-4000-8000-000000000003', 'a1000000-0000-4000-8000-000000000001', 'en_route_pickup', 'arrived_pickup',  NOW() - INTERVAL '3 days' - INTERVAL '30 min', 'provider', 'provider_arrived_pickup'),
    ('c1000000-0000-4000-8000-000000000004', 'a1000000-0000-4000-8000-000000000001', 'arrived_pickup',  'in_progress',     NOW() - INTERVAL '3 days' - INTERVAL '25 min', 'provider', 'provider_started_delivery'),
    ('c1000000-0000-4000-8000-000000000005', 'a1000000-0000-4000-8000-000000000001', 'in_progress',     'proof_submitted', NOW() - INTERVAL '3 days' - INTERVAL '5 min',  'provider', 'proof_submitted'),
    ('c1000000-0000-4000-8000-000000000006', 'a1000000-0000-4000-8000-000000000001', 'proof_submitted', 'completed',       NOW() - INTERVAL '3 days',                     'provider', 'provider_completed_delivery')
ON CONFLICT DO NOTHING;

INSERT INTO delivery_proofs (trip_id, photo_ref, signature_ref, receiver_name, receiver_phone, submitted_at, verified, verified_at)
VALUES (
    'a1000000-0000-4000-8000-000000000001',
    'local-private://trips/d1000000-0000-4000-8000-000000000001/g1000000-0000-4000-8000-000000000001/proof/seed-f1-photo_delivery.jpg',
    'local-private://trips/d1000000-0000-4000-8000-000000000001/g1000000-0000-4000-8000-000000000001/proof/seed-f1-signature_delivery.png',
    'Aisha Bello', '+2348022223001',
    NOW() - INTERVAL '3 days' - INTERVAL '5 min', true, NOW() - INTERVAL '3 days'
)
ON CONFLICT DO NOTHING;

INSERT INTO ratings (id, provider_id, booking_id, rated_by_customer_id, score, comment, recommends_provider)
VALUES (
    '91000000-0000-4000-8000-000000000001', 'd1000000-0000-4000-8000-000000000001',
    'f1000000-0000-4000-8000-000000000001', 'e0000000-0000-4000-8000-000000000001',
    5, 'Fast and careful with the package.', true
)
ON CONFLICT (booking_id) DO NOTHING;

-- Booking B: completed, on rider 2 (Bisi).

INSERT INTO dispatch_bookings (
    id, customer_id, status,
    pickup_address, pickup_lat, pickup_lng,
    dropoff_address, dropoff_lat, dropoff_lng,
    distance_km, fare_amount, currency,
    payment_method, payment_status, payment_ref,
    receiver_name, receiver_phone,
    package_desc, package_type, package_size, is_fragile, service_tier,
    provider_id, completed_at, created_at
) VALUES (
    'f2000000-0000-4000-8000-000000000002', 'e0000000-0000-4000-8000-000000000001', 'completed',
    'Surulere, Lagos', 6.5005, 3.3580,
    'Apapa Wharf, Lagos', 6.4489, 3.3619,
    8.50, 255000, 'NGN',
    'wallet', 'paid', 'seed-wallet-ref-f2000000',
    'Chinwe Obi', '+2348022223002',
    'Spare parts in a small box', 'parcel', 'medium', false, 'standard',
    'd2000000-0000-4000-8000-000000000002',
    NOW() - INTERVAL '2 days', NOW() - INTERVAL '2 days' - INTERVAL '35 min'
)
ON CONFLICT DO NOTHING;

INSERT INTO trips (
    id, booking_id, provider_id, customer_id, status,
    pickup_address, pickup_lat, pickup_lng,
    dropoff_address, dropoff_lat, dropoff_lng,
    distance_km, fare_amount, currency,
    receiver_name, receiver_phone,
    package_desc, package_weight, package_type, package_size, is_fragile, service_tier,
    started_at, arrived_at, completed_at, created_at
) VALUES (
    'a2000000-0000-4000-8000-000000000002', 'f2000000-0000-4000-8000-000000000002',
    'd2000000-0000-4000-8000-000000000002', 'e0000000-0000-4000-8000-000000000001', 'completed',
    'Surulere, Lagos', 6.5005, 3.3580,
    'Apapa Wharf, Lagos', 6.4489, 3.3619,
    8.50, 255000, 'NGN',
    'Chinwe Obi', '+2348022223002',
    'Spare parts in a small box', 3.00, 'parcel', 'medium', false, 'standard',
    NOW() - INTERVAL '2 days' - INTERVAL '20 min', NOW() - INTERVAL '2 days' - INTERVAL '25 min',
    NOW() - INTERVAL '2 days', NOW() - INTERVAL '2 days' - INTERVAL '33 min'
)
ON CONFLICT DO NOTHING;

INSERT INTO trip_state_log (id, trip_id, from_status, to_status, changed_at, changed_by, notes) VALUES
    ('c2000000-0000-4000-8000-000000000001', 'a2000000-0000-4000-8000-000000000002', 'none',            'assigned',        NOW() - INTERVAL '2 days' - INTERVAL '33 min', 'system',   'created_from_request_accepted'),
    ('c2000000-0000-4000-8000-000000000002', 'a2000000-0000-4000-8000-000000000002', 'assigned',        'en_route_pickup', NOW() - INTERVAL '2 days' - INTERVAL '30 min', 'provider', 'provider_en_route_pickup'),
    ('c2000000-0000-4000-8000-000000000003', 'a2000000-0000-4000-8000-000000000002', 'en_route_pickup', 'arrived_pickup',  NOW() - INTERVAL '2 days' - INTERVAL '25 min', 'provider', 'provider_arrived_pickup'),
    ('c2000000-0000-4000-8000-000000000004', 'a2000000-0000-4000-8000-000000000002', 'arrived_pickup',  'in_progress',     NOW() - INTERVAL '2 days' - INTERVAL '20 min', 'provider', 'provider_started_delivery'),
    ('c2000000-0000-4000-8000-000000000005', 'a2000000-0000-4000-8000-000000000002', 'in_progress',     'proof_submitted', NOW() - INTERVAL '2 days' - INTERVAL '4 min',  'provider', 'proof_submitted'),
    ('c2000000-0000-4000-8000-000000000006', 'a2000000-0000-4000-8000-000000000002', 'proof_submitted', 'completed',       NOW() - INTERVAL '2 days',                     'provider', 'provider_completed_delivery')
ON CONFLICT DO NOTHING;

INSERT INTO delivery_proofs (trip_id, photo_ref, signature_ref, receiver_name, receiver_phone, submitted_at, verified, verified_at)
VALUES (
    'a2000000-0000-4000-8000-000000000002',
    'local-private://trips/d2000000-0000-4000-8000-000000000002/g2000000-0000-4000-8000-000000000002/proof/seed-f2-photo_delivery.jpg',
    'local-private://trips/d2000000-0000-4000-8000-000000000002/g2000000-0000-4000-8000-000000000002/proof/seed-f2-signature_delivery.png',
    'Chinwe Obi', '+2348022223002',
    NOW() - INTERVAL '2 days' - INTERVAL '4 min', true, NOW() - INTERVAL '2 days'
)
ON CONFLICT DO NOTHING;

INSERT INTO ratings (id, provider_id, booking_id, rated_by_customer_id, score, comment, recommends_provider)
VALUES (
    '92000000-0000-4000-8000-000000000002', 'd2000000-0000-4000-8000-000000000002',
    'f2000000-0000-4000-8000-000000000002', 'e0000000-0000-4000-8000-000000000001',
    4, 'Good delivery, arrived a little late.', true
)
ON CONFLICT (booking_id) DO NOTHING;

-- Reflect the seeded reviews on the providers directly — the app only updates
-- avg_rating inside the live review-submission transaction, so a SQL seed has
-- to set it explicitly (no DB trigger exists).
UPDATE providers SET avg_rating = 5.00, total_trips = 1 WHERE id = 'd1000000-0000-4000-8000-000000000001';
UPDATE providers SET avg_rating = 4.00, total_trips = 1 WHERE id = 'd2000000-0000-4000-8000-000000000002';

-- Booking C: cancelled by the customer before a rider was matched. No trip.

INSERT INTO dispatch_bookings (
    id, customer_id, status,
    pickup_address, pickup_lat, pickup_lng,
    dropoff_address, dropoff_lat, dropoff_lng,
    distance_km, fare_amount, currency,
    payment_method, payment_status, payment_ref,
    receiver_name, receiver_phone,
    package_desc, package_type, package_size, is_fragile, service_tier,
    cancel_reason, cancelled_at, created_at
) VALUES (
    'f3000000-0000-4000-8000-000000000003', 'e0000000-0000-4000-8000-000000000001', 'cancelled',
    'Lekki Phase 1, Lagos', 6.4478, 3.4723,
    'Ajah, Lagos', 6.4669, 3.5710,
    12.00, 300000, 'NGN',
    'wallet', 'refunded', 'seed-wallet-ref-f3000000',
    'Chinedu Okafor', '+2348033334002',
    'Small parcel', 'parcel', 'small', false, 'standard',
    'customer_cancelled', NOW() - INTERVAL '1 day', NOW() - INTERVAL '1 day' - INTERVAL '10 min'
)
ON CONFLICT DO NOTHING;

-- Booking D: no rider found in time (unmatched). No trip.

INSERT INTO dispatch_bookings (
    id, customer_id, status,
    pickup_address, pickup_lat, pickup_lng,
    dropoff_address, dropoff_lat, dropoff_lng,
    distance_km, fare_amount, currency,
    payment_method, payment_status, payment_ref,
    receiver_name, receiver_phone,
    package_desc, package_type, package_size, is_fragile, service_tier,
    cancel_reason, cancelled_at, created_at
) VALUES (
    'f4000000-0000-4000-8000-000000000004', 'e0000000-0000-4000-8000-000000000001', 'unmatched',
    'Ikoyi, Lagos', 6.4541, 3.4348,
    'Victoria Island, Lagos', 6.4281, 3.4219,
    5.00, 170000, 'NGN',
    'wallet', 'refunded', 'seed-wallet-ref-f4000000',
    'Funmi Lawal', '+2348033334003',
    'Envelope', 'documents', 'small', false, 'standard',
    'no_provider_found', NOW() - INTERVAL '12 hours', NOW() - INTERVAL '12 hours' - INTERVAL '5 min'
)
ON CONFLICT DO NOTHING;

-- Booking E: in progress, on rider 1 (Emeka) — trip is mid-delivery, no proof
-- or completion yet, so the "Ongoing" bucket / active-trip screens have data.

INSERT INTO dispatch_bookings (
    id, customer_id, status,
    pickup_address, pickup_lat, pickup_lng,
    dropoff_address, dropoff_lat, dropoff_lng,
    distance_km, fare_amount, currency,
    payment_method, payment_status, payment_ref,
    receiver_name, receiver_phone,
    package_desc, package_type, package_size, is_fragile, service_tier,
    provider_id, created_at
) VALUES (
    'f5000000-0000-4000-8000-000000000005', 'e0000000-0000-4000-8000-000000000001', 'accepted',
    'Ikeja, Lagos', 6.6018, 3.3515,
    'Maryland, Lagos', 6.5731, 3.3654,
    6.00, 190000, 'NGN',
    'wallet', 'held', 'seed-wallet-ref-f5000000',
    'David Umeh', '+2348033334004',
    'Small package', 'parcel', 'small', false, 'standard',
    'd1000000-0000-4000-8000-000000000001', NOW() - INTERVAL '18 min'
)
ON CONFLICT DO NOTHING;

INSERT INTO trips (
    id, booking_id, provider_id, customer_id, status,
    pickup_address, pickup_lat, pickup_lng,
    dropoff_address, dropoff_lat, dropoff_lng,
    distance_km, fare_amount, currency,
    receiver_name, receiver_phone,
    package_desc, package_weight, package_type, package_size, is_fragile, service_tier,
    started_at, arrived_at, created_at
) VALUES (
    'a5000000-0000-4000-8000-000000000005', 'f5000000-0000-4000-8000-000000000005',
    'd1000000-0000-4000-8000-000000000001', 'e0000000-0000-4000-8000-000000000001', 'in_progress',
    'Ikeja, Lagos', 6.6018, 3.3515,
    'Maryland, Lagos', 6.5731, 3.3654,
    6.00, 190000, 'NGN',
    'David Umeh', '+2348033334004',
    'Small package', 1.00, 'parcel', 'small', false, 'standard',
    NOW() - INTERVAL '4 min', NOW() - INTERVAL '8 min', NOW() - INTERVAL '16 min'
)
ON CONFLICT DO NOTHING;

INSERT INTO trip_state_log (id, trip_id, from_status, to_status, changed_at, changed_by, notes) VALUES
    ('c5000000-0000-4000-8000-000000000001', 'a5000000-0000-4000-8000-000000000005', 'none',            'assigned',        NOW() - INTERVAL '16 min', 'system',   'created_from_request_accepted'),
    ('c5000000-0000-4000-8000-000000000002', 'a5000000-0000-4000-8000-000000000005', 'assigned',        'en_route_pickup', NOW() - INTERVAL '13 min', 'provider', 'provider_en_route_pickup'),
    ('c5000000-0000-4000-8000-000000000003', 'a5000000-0000-4000-8000-000000000005', 'en_route_pickup', 'arrived_pickup',  NOW() - INTERVAL '8 min',  'provider', 'provider_arrived_pickup'),
    ('c5000000-0000-4000-8000-000000000004', 'a5000000-0000-4000-8000-000000000005', 'arrived_pickup',  'in_progress',     NOW() - INTERVAL '4 min',  'provider', 'provider_started_delivery')
ON CONFLICT DO NOTHING;
