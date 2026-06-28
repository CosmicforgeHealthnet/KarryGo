-- ─────────────────────────────────────────────────────────────────────────────
-- Driver-hauling dev seed
--
-- Goal: log into the truck provider app fast and receive bookings on every front.
--
-- Seeds 6 fully-onboarded, Lagos-based providers (3 phone logins, 3 email logins).
-- Each provider owns one ACTIVE truck of every bookable type
-- (van, flatbed, refrigerated, container, tipper) at high capacity, so whichever
-- account you log into matches ANY Lagos booking of any type / weight.
--
-- Each provider also has one COMPLETED trip (with an audit trail) so the
-- Earnings / Trips / Transaction screens show data.
--
-- Idempotent: fixed UUIDs + ON CONFLICT DO NOTHING — safe to re-run.
--
-- To receive a live booking after seeding:
--   1. Run the service with HAULING_OTP_DEBUG=true (already in .env) so the OTP
--      comes back in the API response.
--   2. Log in with a seeded phone/email below.
--   3. Tap "Go Online" on the home screen (sets your real GPS + heartbeat in Redis;
--      the seed deliberately does NOT touch Redis).
--   4. Create a booking from the customer app with a Lagos pickup.
--
-- Phone logins: +2348011111001 Emeka Okonkwo · +2348011111002 Biodun Adeyemi
--               +2348011111003 Chidi Eze
-- Email logins: tunde@karrygo.dev Tunde Bakare · ngozi@karrygo.dev Ngozi Eze
--               samuel@karrygo.dev Samuel Adeniyi
-- ─────────────────────────────────────────────────────────────────────────────

-- ── Providers ────────────────────────────────────────────────────────────────
-- onboarding_status='complete' → the Flutter app routes straight to Home/Go-Online
-- (skips the onboarding flow). status='active' → eligible for matching.

-- Reclaim seed identities: an earlier OTP-login test may have created an
-- incomplete row holding one of the seed phones/emails (with a different id),
-- which would collide with the UNIQUE(phone)/UNIQUE(email) constraints below.
-- Remove ONLY such stray rows that are pure login artifacts — i.e. they hold a
-- seeded identifier AND own no trucks and no bookings. Rows with real work
-- attached are never touched. (Cascade removes only their orphan login sessions.)
DELETE FROM truck_providers p
WHERE (
        p.phone IN ('+2348011111001', '+2348011111002', '+2348011111003')
     OR p.email IN ('tunde@karrygo.dev', 'ngozi@karrygo.dev', 'samuel@karrygo.dev')
      )
  AND NOT EXISTS (SELECT 1 FROM trucks t           WHERE t.provider_id = p.id)
  AND NOT EXISTS (SELECT 1 FROM haulage_bookings b  WHERE b.provider_id = p.id)
  AND p.id NOT IN (
        'a0000000-0000-4000-8000-000000000001',
        'a0000000-0000-4000-8000-000000000002',
        'a0000000-0000-4000-8000-000000000003',
        'a0000000-0000-4000-8000-000000000004',
        'a0000000-0000-4000-8000-000000000005',
        'a0000000-0000-4000-8000-000000000006'
      );

INSERT INTO truck_providers (
    id, phone, email, first_name, last_name,
    status, onboarding_status, rating, total_trips,
    location_state, location_city, operation_mode, service_type, language,
    driver_license_number, license_expiry_year, license_expiry_date
) VALUES
-- NOTE: the unused identifier is NULL (not '') because truck_providers.phone and
-- .email each carry a UNIQUE constraint; multiple ''s would collide, multiple
-- NULLs do not.
    ('a0000000-0000-4000-8000-000000000001', '+2348011111001', NULL, 'Emeka',  'Okonkwo',  'active', 'complete', 4.90, 37, 'Lagos', 'Ikeja',   'fleet_owner',     'hauling', 'English', 'AAA12345AA1', '2030', '2030-04-12'),
    ('a0000000-0000-4000-8000-000000000002', '+2348011111002', NULL, 'Biodun', 'Adeyemi',  'active', 'complete', 4.80, 29, 'Lagos', 'Surulere','owner_operator',  'hauling', 'English', 'BBB12345BB2', '2029', '2029-09-03'),
    ('a0000000-0000-4000-8000-000000000003', '+2348011111003', NULL, 'Chidi',  'Eze',      'active', 'complete', 5.00, 51, 'Lagos', 'Lekki',   'fleet_owner',     'hauling', 'English', 'CCC12345CC3', '2031', '2031-01-22'),
    ('a0000000-0000-4000-8000-000000000004', NULL, 'tunde@karrygo.dev',  'Tunde',  'Bakare',   'active', 'complete', 4.70, 18, 'Lagos', 'Yaba',    'owner_operator',  'hauling', 'English', 'DDD12345DD4', '2028', '2028-06-30'),
    ('a0000000-0000-4000-8000-000000000005', NULL, 'ngozi@karrygo.dev',  'Ngozi',  'Eze',      'active', 'complete', 4.95, 44, 'Lagos', 'Ikoyi',   'fleet_owner',     'hauling', 'English', 'EEE12345EE5', '2030', '2030-11-15'),
    ('a0000000-0000-4000-8000-000000000006', NULL, 'samuel@karrygo.dev', 'Samuel', 'Adeniyi',  'active', 'complete', 4.85, 26, 'Lagos', 'Apapa',   'owner_operator',  'hauling', 'English', 'FFF12345FF6', '2029', '2029-02-08')
ON CONFLICT (id) DO NOTHING;

-- ── Trucks ───────────────────────────────────────────────────────────────────
-- One ACTIVE truck of every bookable backend type per provider, high capacity so
-- cargo weight never excludes a match. Deterministic plate numbers keep re-runs
-- idempotent. Truck-id scheme: t<provider#>0000-...-<typecode> where typecode is
-- 1=van 2=flatbed 3=refrigerated 4=container 5=tipper.

INSERT INTO trucks (id, provider_id, truck_type, capacity_kg, plate_number, year, make, model, color, status) VALUES
    -- Provider 1 — Emeka Okonkwo
    ('b1000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001', 'van',          3000,  'KRG-101-VAN', 2021, 'Toyota',      'Dyna',       'White',  'active'),
    ('b1000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000001', 'flatbed',      12000, 'KRG-101-FLB', 2020, 'MAN',         'TGS',        'Blue',   'active'),
    ('b1000000-0000-4000-8000-000000000003', 'a0000000-0000-4000-8000-000000000001', 'refrigerated', 6000,  'KRG-101-RFR', 2022, 'Isuzu',       'Forward',    'White',  'active'),
    ('b1000000-0000-4000-8000-000000000004', 'a0000000-0000-4000-8000-000000000001', 'container',    30000, 'KRG-101-CTR', 2019, 'Mercedes',    'Actros',     'Silver', 'active'),
    ('b1000000-0000-4000-8000-000000000005', 'a0000000-0000-4000-8000-000000000001', 'tipper',       18000, 'KRG-101-TPR', 2021, 'Howo',        'A7',         'Yellow', 'active'),
    -- Provider 2 — Biodun Adeyemi
    ('b2000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000002', 'van',          3000,  'KRG-102-VAN', 2022, 'Toyota',      'HiAce',      'White',  'active'),
    ('b2000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000002', 'flatbed',      14000, 'KRG-102-FLB', 2019, 'Scania',      'R450',       'Red',    'active'),
    ('b2000000-0000-4000-8000-000000000003', 'a0000000-0000-4000-8000-000000000002', 'refrigerated', 7000,  'KRG-102-RFR', 2021, 'Mitsubishi',  'Canter',     'White',  'active'),
    ('b2000000-0000-4000-8000-000000000004', 'a0000000-0000-4000-8000-000000000002', 'container',    28000, 'KRG-102-CTR', 2020, 'MAN',         'TGX',        'Blue',   'active'),
    ('b2000000-0000-4000-8000-000000000005', 'a0000000-0000-4000-8000-000000000002', 'tipper',       16000, 'KRG-102-TPR', 2022, 'Sinotruk',    'Hohan',      'Orange', 'active'),
    -- Provider 3 — Chidi Eze
    ('b3000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000003', 'van',          3500,  'KRG-103-VAN', 2023, 'Ford',        'Transit',    'Grey',   'active'),
    ('b3000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000003', 'flatbed',      13000, 'KRG-103-FLB', 2021, 'Volvo',       'FH',         'White',  'active'),
    ('b3000000-0000-4000-8000-000000000003', 'a0000000-0000-4000-8000-000000000003', 'refrigerated', 6500,  'KRG-103-RFR', 2022, 'Isuzu',       'NPR',        'White',  'active'),
    ('b3000000-0000-4000-8000-000000000004', 'a0000000-0000-4000-8000-000000000003', 'container',    30000, 'KRG-103-CTR', 2020, 'DAF',         'XF',         'Blue',   'active'),
    ('b3000000-0000-4000-8000-000000000005', 'a0000000-0000-4000-8000-000000000003', 'tipper',       20000, 'KRG-103-TPR', 2021, 'Howo',        'A7',         'Green',  'active'),
    -- Provider 4 — Tunde Bakare
    ('b4000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000004', 'van',          2800,  'KRG-104-VAN', 2020, 'Nissan',      'NV350',      'White',  'active'),
    ('b4000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000004', 'flatbed',      11000, 'KRG-104-FLB', 2019, 'MAN',         'TGM',        'Blue',   'active'),
    ('b4000000-0000-4000-8000-000000000003', 'a0000000-0000-4000-8000-000000000004', 'refrigerated', 5500,  'KRG-104-RFR', 2021, 'Mitsubishi',  'Fuso',       'White',  'active'),
    ('b4000000-0000-4000-8000-000000000004', 'a0000000-0000-4000-8000-000000000004', 'container',    26000, 'KRG-104-CTR', 2018, 'Mercedes',    'Axor',       'Silver', 'active'),
    ('b4000000-0000-4000-8000-000000000005', 'a0000000-0000-4000-8000-000000000004', 'tipper',       15000, 'KRG-104-TPR', 2020, 'Sinotruk',    'Howo',       'Yellow', 'active'),
    -- Provider 5 — Ngozi Eze
    ('b5000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000005', 'van',          3200,  'KRG-105-VAN', 2022, 'Toyota',      'Dyna',       'White',  'active'),
    ('b5000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000005', 'flatbed',      15000, 'KRG-105-FLB', 2021, 'Scania',      'P360',       'White',  'active'),
    ('b5000000-0000-4000-8000-000000000003', 'a0000000-0000-4000-8000-000000000005', 'refrigerated', 7500,  'KRG-105-RFR', 2023, 'Isuzu',       'Forward',    'White',  'active'),
    ('b5000000-0000-4000-8000-000000000004', 'a0000000-0000-4000-8000-000000000005', 'container',    29000, 'KRG-105-CTR', 2020, 'Volvo',       'FM',         'Blue',   'active'),
    ('b5000000-0000-4000-8000-000000000005', 'a0000000-0000-4000-8000-000000000005', 'tipper',       19000, 'KRG-105-TPR', 2022, 'Howo',        'A7',         'Red',    'active'),
    -- Provider 6 — Samuel Adeniyi
    ('b6000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000006', 'van',          3000,  'KRG-106-VAN', 2021, 'Ford',        'Transit',    'White',  'active'),
    ('b6000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000006', 'flatbed',      12500, 'KRG-106-FLB', 2020, 'MAN',         'TGS',        'Grey',   'active'),
    ('b6000000-0000-4000-8000-000000000003', 'a0000000-0000-4000-8000-000000000006', 'refrigerated', 6000,  'KRG-106-RFR', 2022, 'Mitsubishi',  'Canter',     'White',  'active'),
    ('b6000000-0000-4000-8000-000000000004', 'a0000000-0000-4000-8000-000000000006', 'container',    27000, 'KRG-106-CTR', 2019, 'DAF',         'CF',         'Blue',   'active'),
    ('b6000000-0000-4000-8000-000000000005', 'a0000000-0000-4000-8000-000000000006', 'tipper',       17000, 'KRG-106-TPR', 2021, 'Sinotruk',    'Hohan',      'Orange', 'active')
ON CONFLICT (id) DO NOTHING;

-- ── Completed trips (history) ────────────────────────────────────────────────
-- One completed booking per provider so Earnings/Trips/Transaction screens have
-- data. Fares follow the v1 formula (base ₦5,000 + ₦250/km + 10% weight surcharge
-- over 500 kg + ₦2,000/helper) stored in kobo. payment_method='cash',
-- payment_status='paid'. completed_at staggered across recent days.
-- customer_id values are synthetic (no cross-service FK in this schema).

INSERT INTO haulage_bookings (
    id, customer_id, provider_id, truck_id,
    pickup_address, pickup_lat, pickup_lng,
    dropoff_address, dropoff_lat, dropoff_lng,
    cargo_type, preferred_truck_type, cargo_weight_kg, cargo_description,
    requires_helpers, helper_count,
    distance_km, fare_estimate_kobo, fare_final_kobo,
    status, payment_method, payment_status,
    matched_at, accepted_at, picked_up_at, delivered_at, completed_at, created_at
) VALUES
    -- P1: 12 km, 800 kg flatbed, 1 helper → 5000 + 3000 + 10%*5000(500) + 2000 = 10,300 → wait recompute below
    ('c1000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-0000000c0001', 'a0000000-0000-4000-8000-000000000001', 'b1000000-0000-4000-8000-000000000002',
     'Computer Village, Ikeja, Lagos', 6.5959, 3.3421,
     'Victoria Island, Lagos', 6.4281, 3.4219,
     'general_goods', 'flatbed', 800, 'Office furniture',
     TRUE, 1,
     12.00, 1080000, 1080000,
     'completed', 'cash', 'paid',
     NOW() - INTERVAL '6 days' - INTERVAL '40 min', NOW() - INTERVAL '6 days' - INTERVAL '38 min', NOW() - INTERVAL '6 days' - INTERVAL '25 min', NOW() - INTERVAL '6 days' - INTERVAL '5 min', NOW() - INTERVAL '6 days', NOW() - INTERVAL '6 days' - INTERVAL '45 min'),
    -- P2: 9 km container, 400 kg, 0 helpers → 5000 + 2250 = 7,250
    ('c2000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-0000000c0002', 'a0000000-0000-4000-8000-000000000002', 'b2000000-0000-4000-8000-000000000004',
     'Surulere, Lagos', 6.5009, 3.3556,
     'Apapa Wharf, Lagos', 6.4489, 3.3619,
     'general_goods', 'container', 400, 'Sealed cartons',
     FALSE, 0,
     9.00, 725000, 725000,
     'completed', 'cash', 'paid',
     NOW() - INTERVAL '5 days' - INTERVAL '35 min', NOW() - INTERVAL '5 days' - INTERVAL '33 min', NOW() - INTERVAL '5 days' - INTERVAL '22 min', NOW() - INTERVAL '5 days' - INTERVAL '4 min', NOW() - INTERVAL '5 days', NOW() - INTERVAL '5 days' - INTERVAL '40 min'),
    -- P3: 15 km tipper, 1200 kg, 2 helpers → 5000 + 3750 + 10%*8750 + 4000 = 13,625
    ('c3000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-0000000c0003', 'a0000000-0000-4000-8000-000000000003', 'b3000000-0000-4000-8000-000000000005',
     'Lekki Phase 1, Lagos', 6.4478, 3.4723,
     'Ajah, Lagos', 6.4669, 3.5710,
     'construction_material', 'tipper', 1200, 'Granite chippings',
     TRUE, 2,
     15.00, 1362500, 1362500,
     'completed', 'cash', 'paid',
     NOW() - INTERVAL '4 days' - INTERVAL '50 min', NOW() - INTERVAL '4 days' - INTERVAL '47 min', NOW() - INTERVAL '4 days' - INTERVAL '30 min', NOW() - INTERVAL '4 days' - INTERVAL '6 min', NOW() - INTERVAL '4 days', NOW() - INTERVAL '4 days' - INTERVAL '55 min'),
    -- P4: 7 km van, 300 kg, 0 helpers → 5000 + 1750 = 6,750
    ('c4000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-0000000c0004', 'a0000000-0000-4000-8000-000000000004', 'b4000000-0000-4000-8000-000000000001',
     'Yaba, Lagos', 6.5095, 3.3711,
     'Ebute Metta, Lagos', 6.4895, 3.3815,
     'general_goods', 'van', 300, 'Electronics boxes',
     FALSE, 0,
     7.00, 675000, 675000,
     'completed', 'cash', 'paid',
     NOW() - INTERVAL '3 days' - INTERVAL '30 min', NOW() - INTERVAL '3 days' - INTERVAL '28 min', NOW() - INTERVAL '3 days' - INTERVAL '18 min', NOW() - INTERVAL '3 days' - INTERVAL '3 min', NOW() - INTERVAL '3 days', NOW() - INTERVAL '3 days' - INTERVAL '34 min'),
    -- P5: 11 km refrigerated, 600 kg, 1 helper → 5000 + 2750 + 10%*7750 + 2000 = 10,525
    ('c5000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-0000000c0005', 'a0000000-0000-4000-8000-000000000005', 'b5000000-0000-4000-8000-000000000003',
     'Ikoyi, Lagos', 6.4541, 3.4348,
     'Oniru, Victoria Island, Lagos', 6.4316, 3.4509,
     'perishable_goods', 'refrigerated', 600, 'Frozen seafood',
     TRUE, 1,
     11.00, 1052500, 1052500,
     'completed', 'cash', 'paid',
     NOW() - INTERVAL '2 days' - INTERVAL '38 min', NOW() - INTERVAL '2 days' - INTERVAL '36 min', NOW() - INTERVAL '2 days' - INTERVAL '24 min', NOW() - INTERVAL '2 days' - INTERVAL '5 min', NOW() - INTERVAL '2 days', NOW() - INTERVAL '2 days' - INTERVAL '42 min'),
    -- P6: 10 km container, 500 kg, 0 helpers → 5000 + 2500 = 7,500
    ('c6000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-0000000c0006', 'a0000000-0000-4000-8000-000000000006', 'b6000000-0000-4000-8000-000000000004',
     'Apapa, Lagos', 6.4489, 3.3619,
     'Ojota, Lagos', 6.5778, 3.3837,
     'general_goods', 'container', 500, 'Bagged commodities',
     FALSE, 0,
     10.00, 750000, 750000,
     'completed', 'cash', 'paid',
     NOW() - INTERVAL '1 day' - INTERVAL '33 min', NOW() - INTERVAL '1 day' - INTERVAL '31 min', NOW() - INTERVAL '1 day' - INTERVAL '20 min', NOW() - INTERVAL '1 day' - INTERVAL '4 min', NOW() - INTERVAL '1 day', NOW() - INTERVAL '1 day' - INTERVAL '38 min')
ON CONFLICT (id) DO NOTHING;

-- ── Booking audit trail ──────────────────────────────────────────────────────
-- Minimal lifecycle trail per completed booking (created → accepted → picked_up →
-- delivered → completed). actor_id references the provider where relevant.

INSERT INTO booking_events (id, booking_id, event_type, actor_type, actor_id, created_at) VALUES
    -- P1
    ('e1000000-0000-4000-8000-000000000001', 'c1000000-0000-4000-8000-000000000001', 'created',   'customer', 'd0000000-0000-4000-8000-0000000c0001', NOW() - INTERVAL '6 days' - INTERVAL '45 min'),
    ('e1000000-0000-4000-8000-000000000002', 'c1000000-0000-4000-8000-000000000001', 'accepted',  'provider', 'a0000000-0000-4000-8000-000000000001', NOW() - INTERVAL '6 days' - INTERVAL '38 min'),
    ('e1000000-0000-4000-8000-000000000003', 'c1000000-0000-4000-8000-000000000001', 'picked_up', 'provider', 'a0000000-0000-4000-8000-000000000001', NOW() - INTERVAL '6 days' - INTERVAL '25 min'),
    ('e1000000-0000-4000-8000-000000000004', 'c1000000-0000-4000-8000-000000000001', 'delivered', 'provider', 'a0000000-0000-4000-8000-000000000001', NOW() - INTERVAL '6 days' - INTERVAL '5 min'),
    ('e1000000-0000-4000-8000-000000000005', 'c1000000-0000-4000-8000-000000000001', 'completed', 'system',   'system',                               NOW() - INTERVAL '6 days'),
    -- P2
    ('e2000000-0000-4000-8000-000000000001', 'c2000000-0000-4000-8000-000000000001', 'created',   'customer', 'd0000000-0000-4000-8000-0000000c0002', NOW() - INTERVAL '5 days' - INTERVAL '40 min'),
    ('e2000000-0000-4000-8000-000000000002', 'c2000000-0000-4000-8000-000000000001', 'accepted',  'provider', 'a0000000-0000-4000-8000-000000000002', NOW() - INTERVAL '5 days' - INTERVAL '33 min'),
    ('e2000000-0000-4000-8000-000000000003', 'c2000000-0000-4000-8000-000000000001', 'picked_up', 'provider', 'a0000000-0000-4000-8000-000000000002', NOW() - INTERVAL '5 days' - INTERVAL '22 min'),
    ('e2000000-0000-4000-8000-000000000004', 'c2000000-0000-4000-8000-000000000001', 'delivered', 'provider', 'a0000000-0000-4000-8000-000000000002', NOW() - INTERVAL '5 days' - INTERVAL '4 min'),
    ('e2000000-0000-4000-8000-000000000005', 'c2000000-0000-4000-8000-000000000001', 'completed', 'system',   'system',                               NOW() - INTERVAL '5 days'),
    -- P3
    ('e3000000-0000-4000-8000-000000000001', 'c3000000-0000-4000-8000-000000000001', 'created',   'customer', 'd0000000-0000-4000-8000-0000000c0003', NOW() - INTERVAL '4 days' - INTERVAL '55 min'),
    ('e3000000-0000-4000-8000-000000000002', 'c3000000-0000-4000-8000-000000000001', 'accepted',  'provider', 'a0000000-0000-4000-8000-000000000003', NOW() - INTERVAL '4 days' - INTERVAL '47 min'),
    ('e3000000-0000-4000-8000-000000000003', 'c3000000-0000-4000-8000-000000000001', 'picked_up', 'provider', 'a0000000-0000-4000-8000-000000000003', NOW() - INTERVAL '4 days' - INTERVAL '30 min'),
    ('e3000000-0000-4000-8000-000000000004', 'c3000000-0000-4000-8000-000000000001', 'delivered', 'provider', 'a0000000-0000-4000-8000-000000000003', NOW() - INTERVAL '4 days' - INTERVAL '6 min'),
    ('e3000000-0000-4000-8000-000000000005', 'c3000000-0000-4000-8000-000000000001', 'completed', 'system',   'system',                               NOW() - INTERVAL '4 days'),
    -- P4
    ('e4000000-0000-4000-8000-000000000001', 'c4000000-0000-4000-8000-000000000001', 'created',   'customer', 'd0000000-0000-4000-8000-0000000c0004', NOW() - INTERVAL '3 days' - INTERVAL '34 min'),
    ('e4000000-0000-4000-8000-000000000002', 'c4000000-0000-4000-8000-000000000001', 'accepted',  'provider', 'a0000000-0000-4000-8000-000000000004', NOW() - INTERVAL '3 days' - INTERVAL '28 min'),
    ('e4000000-0000-4000-8000-000000000003', 'c4000000-0000-4000-8000-000000000001', 'picked_up', 'provider', 'a0000000-0000-4000-8000-000000000004', NOW() - INTERVAL '3 days' - INTERVAL '18 min'),
    ('e4000000-0000-4000-8000-000000000004', 'c4000000-0000-4000-8000-000000000001', 'delivered', 'provider', 'a0000000-0000-4000-8000-000000000004', NOW() - INTERVAL '3 days' - INTERVAL '3 min'),
    ('e4000000-0000-4000-8000-000000000005', 'c4000000-0000-4000-8000-000000000001', 'completed', 'system',   'system',                               NOW() - INTERVAL '3 days'),
    -- P5
    ('e5000000-0000-4000-8000-000000000001', 'c5000000-0000-4000-8000-000000000001', 'created',   'customer', 'd0000000-0000-4000-8000-0000000c0005', NOW() - INTERVAL '2 days' - INTERVAL '42 min'),
    ('e5000000-0000-4000-8000-000000000002', 'c5000000-0000-4000-8000-000000000001', 'accepted',  'provider', 'a0000000-0000-4000-8000-000000000005', NOW() - INTERVAL '2 days' - INTERVAL '36 min'),
    ('e5000000-0000-4000-8000-000000000003', 'c5000000-0000-4000-8000-000000000001', 'picked_up', 'provider', 'a0000000-0000-4000-8000-000000000005', NOW() - INTERVAL '2 days' - INTERVAL '24 min'),
    ('e5000000-0000-4000-8000-000000000004', 'c5000000-0000-4000-8000-000000000001', 'delivered', 'provider', 'a0000000-0000-4000-8000-000000000005', NOW() - INTERVAL '2 days' - INTERVAL '5 min'),
    ('e5000000-0000-4000-8000-000000000005', 'c5000000-0000-4000-8000-000000000001', 'completed', 'system',   'system',                               NOW() - INTERVAL '2 days'),
    -- P6
    ('e6000000-0000-4000-8000-000000000001', 'c6000000-0000-4000-8000-000000000001', 'created',   'customer', 'd0000000-0000-4000-8000-0000000c0006', NOW() - INTERVAL '1 day' - INTERVAL '38 min'),
    ('e6000000-0000-4000-8000-000000000002', 'c6000000-0000-4000-8000-000000000001', 'accepted',  'provider', 'a0000000-0000-4000-8000-000000000006', NOW() - INTERVAL '1 day' - INTERVAL '31 min'),
    ('e6000000-0000-4000-8000-000000000003', 'c6000000-0000-4000-8000-000000000001', 'picked_up', 'provider', 'a0000000-0000-4000-8000-000000000006', NOW() - INTERVAL '1 day' - INTERVAL '20 min'),
    ('e6000000-0000-4000-8000-000000000004', 'c6000000-0000-4000-8000-000000000001', 'delivered', 'provider', 'a0000000-0000-4000-8000-000000000006', NOW() - INTERVAL '1 day' - INTERVAL '4 min'),
    ('e6000000-0000-4000-8000-000000000005', 'c6000000-0000-4000-8000-000000000001', 'completed', 'system',   'system',                               NOW() - INTERVAL '1 day')
ON CONFLICT (id) DO NOTHING;
