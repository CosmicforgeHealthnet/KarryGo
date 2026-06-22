-- Hauling service dev seed
-- Safe to re-run: all inserts use ON CONFLICT DO NOTHING.
-- Run from the service root: go run ./cmd/seed
--
-- Seed phone numbers (use these to log in via OTP in dev/debug mode):
--   +2348011111001  Emeka Okonkwo   (2 trucks: flatbed, container)
--   +2348011111002  Biodun Adeyemi  (2 trucks: tipper, van)
--   +2348011111003  Chidi Eze       (2 trucks: refrigerated, flatbed)
--
-- Seed emails (log in via OTP in dev/debug mode — fully onboarded, online-ready):
--   tunde@karrygo.dev   Tunde Bakare    (2 trucks: flatbed, container)
--   ngozi@karrygo.dev   Ngozi Eze       (2 trucks: tipper, van)
--   samuel@karrygo.dev  Samuel Adeniyi  (2 trucks: refrigerated, flatbed)

-- ─── Truck providers ──────────────────────────────────────────────────────────

INSERT INTO truck_providers (
    id, phone,
    first_name, last_name,
    status, onboarding_status,
    rating, total_trips,
    created_at, updated_at
) VALUES
(
    '11111111-1111-1111-1111-111111111001',
    '+2348011111001',
    'Emeka', 'Okonkwo',
    'active', 'complete',
    4.80, 47,
    NOW() - INTERVAL '120 days', NOW() - INTERVAL '2 hours'
),
(
    '11111111-1111-1111-1111-111111111002',
    '+2348011111002',
    'Biodun', 'Adeyemi',
    'active', 'complete',
    4.65, 31,
    NOW() - INTERVAL '90 days', NOW() - INTERVAL '5 hours'
),
(
    '11111111-1111-1111-1111-111111111003',
    '+2348011111003',
    'Chidi', 'Eze',
    'active', 'complete',
    4.90, 63,
    NOW() - INTERVAL '180 days', NOW() - INTERVAL '1 day'
)
ON CONFLICT DO NOTHING;

-- ─── Email truck providers (fully onboarded, online-ready) ────────────────────
-- These accounts log in by EMAIL via OTP (HAULING_OTP_DEBUG=true).
-- onboarding_status='complete' + status='active' + active trucks below = can go
-- online and receive bookings immediately. All 12 onboarding columns populated
-- so the provider profile screens show complete data.

INSERT INTO truck_providers (
    id, email,
    first_name, last_name,
    status, onboarding_status,
    rating, total_trips,
    location_state, location_city,
    operation_mode, service_type,
    gov_id_url, driver_license_url, vehicle_reg_url,
    guarantor_name, guarantor_phone,
    emergency_contact_name, emergency_contact_phone, emergency_contact_relationship,
    created_at, updated_at
) VALUES
(
    '11111111-1111-1111-1111-1111111110a1',
    'tunde@karrygo.dev',
    'Tunde', 'Bakare',
    'active', 'complete',
    4.85, 52,
    'Lagos', 'Ikeja',
    'fleet_owner', 'haulage',
    'https://files.karrygo.dev/dev/gov-id-tunde.jpg',
    'https://files.karrygo.dev/dev/license-tunde.jpg',
    'https://files.karrygo.dev/dev/vehicle-reg-tunde.jpg',
    'Funke Bakare', '+2348022220a1',
    'Funke Bakare', '+2348022220a1', 'Spouse',
    NOW() - INTERVAL '130 days', NOW() - INTERVAL '3 hours'
),
(
    '11111111-1111-1111-1111-1111111110a2',
    'ngozi@karrygo.dev',
    'Ngozi', 'Eze',
    'active', 'complete',
    4.70, 38,
    'Lagos', 'Surulere',
    'owner_operator', 'haulage',
    'https://files.karrygo.dev/dev/gov-id-ngozi.jpg',
    'https://files.karrygo.dev/dev/license-ngozi.jpg',
    'https://files.karrygo.dev/dev/vehicle-reg-ngozi.jpg',
    'Chinedu Eze', '+2348022220a2',
    'Chinedu Eze', '+2348022220a2', 'Sibling',
    NOW() - INTERVAL '100 days', NOW() - INTERVAL '6 hours'
),
(
    '11111111-1111-1111-1111-1111111110a3',
    'samuel@karrygo.dev',
    'Samuel', 'Adeniyi',
    'active', 'complete',
    4.95, 71,
    'Lagos', 'Lekki',
    'fleet_owner', 'haulage',
    'https://files.karrygo.dev/dev/gov-id-samuel.jpg',
    'https://files.karrygo.dev/dev/license-samuel.jpg',
    'https://files.karrygo.dev/dev/vehicle-reg-samuel.jpg',
    'Bola Adeniyi', '+2348022220a3',
    'Bola Adeniyi', '+2348022220a3', 'Parent',
    NOW() - INTERVAL '200 days', NOW() - INTERVAL '1 day'
)
ON CONFLICT DO NOTHING;

-- ─── Trucks ───────────────────────────────────────────────────────────────────
-- Valid truck types: flatbed | container | tipper | van | refrigerated

INSERT INTO trucks (
    id, provider_id,
    truck_type, capacity_kg, plate_number,
    year, make, model, color,
    status, created_at, updated_at
) VALUES
-- Emeka's trucks
(
    '22222222-2222-2222-2222-222222222001',
    '11111111-1111-1111-1111-111111111001',
    'flatbed', 10000, 'KTU-001-AB',
    2020, 'Isuzu', 'NPR', 'White',
    'active', NOW() - INTERVAL '115 days', NOW()
),
(
    '22222222-2222-2222-2222-222222222002',
    '11111111-1111-1111-1111-111111111001',
    'container', 20000, 'KTU-002-CD',
    2019, 'MAN', 'TGS', 'Blue',
    'active', NOW() - INTERVAL '115 days', NOW()
),
-- Biodun's trucks
(
    '22222222-2222-2222-2222-222222222003',
    '11111111-1111-1111-1111-111111111002',
    'tipper', 8000, 'KTU-003-EF',
    2021, 'DAF', 'CF', 'Yellow',
    'active', NOW() - INTERVAL '85 days', NOW()
),
(
    '22222222-2222-2222-2222-222222222004',
    '11111111-1111-1111-1111-111111111002',
    'van', 2000, 'LSD-001-GH',
    2022, 'Toyota', 'Hiace', 'White',
    'active', NOW() - INTERVAL '85 days', NOW()
),
-- Chidi's trucks
(
    '22222222-2222-2222-2222-222222222005',
    '11111111-1111-1111-1111-111111111003',
    'refrigerated', 5000, 'LSD-002-IJ',
    2020, 'Nissan', 'UD Trucks', 'Silver',
    'active', NOW() - INTERVAL '175 days', NOW()
),
(
    '22222222-2222-2222-2222-222222222006',
    '11111111-1111-1111-1111-111111111003',
    'flatbed', 12000, 'LSD-003-KL',
    2018, 'Mercedes-Benz', 'Actros', 'Red',
    'active', NOW() - INTERVAL '175 days', NOW()
),
-- Tunde's trucks (email provider)
(
    '22222222-2222-2222-2222-2222222220a1',
    '11111111-1111-1111-1111-1111111110a1',
    'flatbed', 11000, 'IKJ-010-AA',
    2021, 'Isuzu', 'FVR', 'White',
    'active', NOW() - INTERVAL '125 days', NOW()
),
(
    '22222222-2222-2222-2222-2222222220a2',
    '11111111-1111-1111-1111-1111111110a1',
    'container', 22000, 'IKJ-011-BB',
    2020, 'Scania', 'R450', 'Blue',
    'active', NOW() - INTERVAL '125 days', NOW()
),
-- Ngozi's trucks (email provider)
(
    '22222222-2222-2222-2222-2222222220a3',
    '11111111-1111-1111-1111-1111111110a2',
    'tipper', 9000, 'SUR-010-CC',
    2022, 'DAF', 'CF', 'Orange',
    'active', NOW() - INTERVAL '95 days', NOW()
),
(
    '22222222-2222-2222-2222-2222222220a4',
    '11111111-1111-1111-1111-1111111110a2',
    'van', 2500, 'SUR-011-DD',
    2023, 'Mercedes-Benz', 'Sprinter', 'White',
    'active', NOW() - INTERVAL '95 days', NOW()
),
-- Samuel's trucks (email provider)
(
    '22222222-2222-2222-2222-2222222220a5',
    '11111111-1111-1111-1111-1111111110a3',
    'refrigerated', 6000, 'LEK-010-EE',
    2021, 'Volvo', 'FM', 'Silver',
    'active', NOW() - INTERVAL '195 days', NOW()
),
(
    '22222222-2222-2222-2222-2222222220a6',
    '11111111-1111-1111-1111-1111111110a3',
    'flatbed', 13000, 'LEK-011-FF',
    2019, 'MAN', 'TGX', 'Black',
    'active', NOW() - INTERVAL '195 days', NOW()
)
ON CONFLICT DO NOTHING;

-- ─── Completed bookings ───────────────────────────────────────────────────────
-- Seed customer UUIDs are cross-service references (no FK constraint).
-- Fares calculated with the v1 formula:
--   base ₦5,000 + ₦250/km + 10% weight surcharge if >500 kg + ₦2,000/helper

INSERT INTO haulage_bookings (
    id,
    customer_id, provider_id, truck_id,
    pickup_address,  pickup_lat,  pickup_lng,
    dropoff_address, dropoff_lat, dropoff_lng,
    cargo_type, cargo_weight_kg, cargo_description,
    requires_helpers, helper_count,
    distance_km, fare_estimate_kobo, fare_final_kobo,
    status,
    matched_at, accepted_at, picked_up_at, delivered_at, completed_at,
    created_at, updated_at
) VALUES

-- Booking 1: furniture move, Surulere → Victoria Island
--   distance 14.5 km, weight 350 kg (no surcharge), 2 helpers
--   fare = 500,000 + 362,500 + 0 + 400,000 = 1,262,500 kobo
(
    '33333333-3333-3333-3333-333333333001',
    '44444444-4444-4444-4444-444444444001',
    '11111111-1111-1111-1111-111111111001',
    '22222222-2222-2222-2222-222222222001',
    '14 Bode Thomas Street, Surulere, Lagos', 6.5058, 3.3566,
    '3 Ozumba Mbadiwe Avenue, Victoria Island, Lagos', 6.4281, 3.4219,
    'furniture', 350, 'Office furniture relocation — desks, chairs, filing cabinets',
    true, 2,
    14.5, 1262500, 1262500,
    'completed',
    NOW() - INTERVAL '10 days' + INTERVAL '2 minutes',
    NOW() - INTERVAL '10 days' + INTERVAL '10 minutes',
    NOW() - INTERVAL '10 days' + INTERVAL '90 minutes',
    NOW() - INTERVAL '10 days' + INTERVAL '180 minutes',
    NOW() - INTERVAL '10 days' + INTERVAL '210 minutes',
    NOW() - INTERVAL '10 days',
    NOW() - INTERVAL '10 days' + INTERVAL '210 minutes'
),

-- Booking 2: construction materials, Ikeja → Lekki
--   distance 28.2 km, weight 800 kg (surcharge applies), 0 helpers
--   fare = 500,000 + 705,000 + 120,500 + 0 = 1,325,500 kobo
(
    '33333333-3333-3333-3333-333333333002',
    '44444444-4444-4444-4444-444444444002',
    '11111111-1111-1111-1111-111111111002',
    '22222222-2222-2222-2222-222222222003',
    '5 Obafemi Awolowo Way, Ikeja, Lagos', 6.6018, 3.3515,
    '22 Admiralty Way, Lekki Phase 1, Lagos', 6.4306, 3.5342,
    'construction', 800, 'Sand and gravel for site foundation',
    false, 0,
    28.2, 1325500, 1325500,
    'completed',
    NOW() - INTERVAL '5 days' + INTERVAL '3 minutes',
    NOW() - INTERVAL '5 days' + INTERVAL '12 minutes',
    NOW() - INTERVAL '5 days' + INTERVAL '75 minutes',
    NOW() - INTERVAL '5 days' + INTERVAL '200 minutes',
    NOW() - INTERVAL '5 days' + INTERVAL '230 minutes',
    NOW() - INTERVAL '5 days',
    NOW() - INTERVAL '5 days' + INTERVAL '230 minutes'
),

-- Booking 3: frozen goods, Apapa → Ogba
--   distance 22.8 km, weight 280 kg (no surcharge), 0 helpers
--   fare = 500,000 + 570,000 + 0 + 0 = 1,070,000 kobo
(
    '33333333-3333-3333-3333-333333333003',
    '44444444-4444-4444-4444-444444444003',
    '11111111-1111-1111-1111-111111111003',
    '22222222-2222-2222-2222-222222222005',
    '45 Warehouse Road, Apapa, Lagos', 6.4474, 3.3653,
    '2 Ogba-Aguda Road, Ogba, Lagos', 6.6170, 3.3347,
    'food', 280, 'Frozen fish and poultry — temperature controlled cargo',
    false, 0,
    22.8, 1070000, 1070000,
    'completed',
    NOW() - INTERVAL '2 days' + INTERVAL '1 minute',
    NOW() - INTERVAL '2 days' + INTERVAL '8 minutes',
    NOW() - INTERVAL '2 days' + INTERVAL '55 minutes',
    NOW() - INTERVAL '2 days' + INTERVAL '160 minutes',
    NOW() - INTERVAL '2 days' + INTERVAL '190 minutes',
    NOW() - INTERVAL '2 days',
    NOW() - INTERVAL '2 days' + INTERVAL '190 minutes'
),

-- Booking 4 (Tunde): general goods, Ikeja → Yaba
--   distance 11.2 km, weight 420 kg (no surcharge), 1 helper
--   fare = 500,000 + 280,000 + 0 + 200,000 = 980,000 kobo
(
    '33333333-3333-3333-3333-3333333330a1',
    '44444444-4444-4444-4444-4444444440a1',
    '11111111-1111-1111-1111-1111111110a1',
    '22222222-2222-2222-2222-2222222220a1',
    '12 Allen Avenue, Ikeja, Lagos', 6.6010, 3.3490,
    '7 Herbert Macaulay Way, Yaba, Lagos', 6.5095, 3.3711,
    'general', 420, 'Retail stock — cartons of packaged goods',
    true, 1,
    11.2, 980000, 980000,
    'completed',
    NOW() - INTERVAL '8 days' + INTERVAL '2 minutes',
    NOW() - INTERVAL '8 days' + INTERVAL '9 minutes',
    NOW() - INTERVAL '8 days' + INTERVAL '70 minutes',
    NOW() - INTERVAL '8 days' + INTERVAL '150 minutes',
    NOW() - INTERVAL '8 days' + INTERVAL '180 minutes',
    NOW() - INTERVAL '8 days',
    NOW() - INTERVAL '8 days' + INTERVAL '180 minutes'
),

-- Booking 5 (Ngozi): construction, Surulere → Ajah
--   distance 31.6 km, weight 950 kg (surcharge applies), 0 helpers
--   fare = 500,000 + 790,000 + 142,500 + 0 = 1,432,500 kobo
(
    '33333333-3333-3333-3333-3333333330a2',
    '44444444-4444-4444-4444-4444444440a2',
    '11111111-1111-1111-1111-1111111110a2',
    '22222222-2222-2222-2222-2222222220a3',
    '9 Adeniran Ogunsanya Street, Surulere, Lagos', 6.4969, 3.3543,
    '15 Addo Road, Ajah, Lagos', 6.4664, 3.5676,
    'construction', 950, 'Cement blocks and reinforcement rods',
    false, 0,
    31.6, 1432500, 1432500,
    'completed',
    NOW() - INTERVAL '4 days' + INTERVAL '3 minutes',
    NOW() - INTERVAL '4 days' + INTERVAL '11 minutes',
    NOW() - INTERVAL '4 days' + INTERVAL '80 minutes',
    NOW() - INTERVAL '4 days' + INTERVAL '210 minutes',
    NOW() - INTERVAL '4 days' + INTERVAL '240 minutes',
    NOW() - INTERVAL '4 days',
    NOW() - INTERVAL '4 days' + INTERVAL '240 minutes'
),

-- Booking 6 (Samuel): frozen goods, Lekki → Festac
--   distance 35.4 km, weight 300 kg (no surcharge), 0 helpers
--   fare = 500,000 + 885,000 + 0 + 0 = 1,385,000 kobo
(
    '33333333-3333-3333-3333-3333333330a3',
    '44444444-4444-4444-4444-4444444440a3',
    '11111111-1111-1111-1111-1111111110a3',
    '22222222-2222-2222-2222-2222222220a5',
    '18 Admiralty Way, Lekki Phase 1, Lagos', 6.4378, 3.4720,
    '4th Avenue, Festac Town, Lagos', 6.4655, 3.2837,
    'food', 300, 'Frozen seafood — temperature controlled cargo',
    false, 0,
    35.4, 1385000, 1385000,
    'completed',
    NOW() - INTERVAL '1 day' + INTERVAL '2 minutes',
    NOW() - INTERVAL '1 day' + INTERVAL '7 minutes',
    NOW() - INTERVAL '1 day' + INTERVAL '60 minutes',
    NOW() - INTERVAL '1 day' + INTERVAL '175 minutes',
    NOW() - INTERVAL '1 day' + INTERVAL '205 minutes',
    NOW() - INTERVAL '1 day',
    NOW() - INTERVAL '1 day' + INTERVAL '205 minutes'
)
ON CONFLICT DO NOTHING;

-- ─── Booking events (audit trail for seeded bookings) ──────────────────────────

INSERT INTO booking_events (
    id, booking_id, event_type, actor_type, actor_id, metadata, created_at
) VALUES
('55555555-5555-5555-5555-555555555101', '33333333-3333-3333-3333-333333333001', 'created',   'customer', '44444444-4444-4444-4444-444444444001', '{}', NOW() - INTERVAL '10 days'),
('55555555-5555-5555-5555-555555555102', '33333333-3333-3333-3333-333333333001', 'matched',   'system',   'matcher',                             '{}', NOW() - INTERVAL '10 days' + INTERVAL '2 minutes'),
('55555555-5555-5555-5555-555555555103', '33333333-3333-3333-3333-333333333001', 'accepted',  'provider', '11111111-1111-1111-1111-111111111001', '{}', NOW() - INTERVAL '10 days' + INTERVAL '10 minutes'),
('55555555-5555-5555-5555-555555555104', '33333333-3333-3333-3333-333333333001', 'picked_up', 'provider', '11111111-1111-1111-1111-111111111001', '{}', NOW() - INTERVAL '10 days' + INTERVAL '90 minutes'),
('55555555-5555-5555-5555-555555555105', '33333333-3333-3333-3333-333333333001', 'delivered', 'provider', '11111111-1111-1111-1111-111111111001', '{}', NOW() - INTERVAL '10 days' + INTERVAL '180 minutes'),
('55555555-5555-5555-5555-555555555106', '33333333-3333-3333-3333-333333333001', 'completed', 'system',   'auto-complete',                        '{}', NOW() - INTERVAL '10 days' + INTERVAL '210 minutes'),

('55555555-5555-5555-5555-555555555201', '33333333-3333-3333-3333-333333333002', 'created',   'customer', '44444444-4444-4444-4444-444444444002', '{}', NOW() - INTERVAL '5 days'),
('55555555-5555-5555-5555-555555555202', '33333333-3333-3333-3333-333333333002', 'matched',   'system',   'matcher',                             '{}', NOW() - INTERVAL '5 days' + INTERVAL '3 minutes'),
('55555555-5555-5555-5555-555555555203', '33333333-3333-3333-3333-333333333002', 'accepted',  'provider', '11111111-1111-1111-1111-111111111002', '{}', NOW() - INTERVAL '5 days' + INTERVAL '12 minutes'),
('55555555-5555-5555-5555-555555555204', '33333333-3333-3333-3333-333333333002', 'picked_up', 'provider', '11111111-1111-1111-1111-111111111002', '{}', NOW() - INTERVAL '5 days' + INTERVAL '75 minutes'),
('55555555-5555-5555-5555-555555555205', '33333333-3333-3333-3333-333333333002', 'delivered', 'provider', '11111111-1111-1111-1111-111111111002', '{}', NOW() - INTERVAL '5 days' + INTERVAL '200 minutes'),
('55555555-5555-5555-5555-555555555206', '33333333-3333-3333-3333-333333333002', 'completed', 'system',   'auto-complete',                        '{}', NOW() - INTERVAL '5 days' + INTERVAL '230 minutes'),

('55555555-5555-5555-5555-555555555301', '33333333-3333-3333-3333-333333333003', 'created',   'customer', '44444444-4444-4444-4444-444444444003', '{}', NOW() - INTERVAL '2 days'),
('55555555-5555-5555-5555-555555555302', '33333333-3333-3333-3333-333333333003', 'matched',   'system',   'matcher',                             '{}', NOW() - INTERVAL '2 days' + INTERVAL '1 minute'),
('55555555-5555-5555-5555-555555555303', '33333333-3333-3333-3333-333333333003', 'accepted',  'provider', '11111111-1111-1111-1111-111111111003', '{}', NOW() - INTERVAL '2 days' + INTERVAL '8 minutes'),
('55555555-5555-5555-5555-555555555304', '33333333-3333-3333-3333-333333333003', 'picked_up', 'provider', '11111111-1111-1111-1111-111111111003', '{}', NOW() - INTERVAL '2 days' + INTERVAL '55 minutes'),
('55555555-5555-5555-5555-555555555305', '33333333-3333-3333-3333-333333333003', 'delivered', 'provider', '11111111-1111-1111-1111-111111111003', '{}', NOW() - INTERVAL '2 days' + INTERVAL '160 minutes'),
('55555555-5555-5555-5555-555555555306', '33333333-3333-3333-3333-333333333003', 'completed', 'system',   'auto-complete',                        '{}', NOW() - INTERVAL '2 days' + INTERVAL '190 minutes'),

-- Tunde's booking
('55555555-5555-5555-5555-5555555550a1', '33333333-3333-3333-3333-3333333330a1', 'created',   'customer', '44444444-4444-4444-4444-4444444440a1', '{}', NOW() - INTERVAL '8 days'),
('55555555-5555-5555-5555-5555555550a2', '33333333-3333-3333-3333-3333333330a1', 'matched',   'system',   'matcher',                             '{}', NOW() - INTERVAL '8 days' + INTERVAL '2 minutes'),
('55555555-5555-5555-5555-5555555550a3', '33333333-3333-3333-3333-3333333330a1', 'accepted',  'provider', '11111111-1111-1111-1111-1111111110a1', '{}', NOW() - INTERVAL '8 days' + INTERVAL '9 minutes'),
('55555555-5555-5555-5555-5555555550a4', '33333333-3333-3333-3333-3333333330a1', 'picked_up', 'provider', '11111111-1111-1111-1111-1111111110a1', '{}', NOW() - INTERVAL '8 days' + INTERVAL '70 minutes'),
('55555555-5555-5555-5555-5555555550a5', '33333333-3333-3333-3333-3333333330a1', 'delivered', 'provider', '11111111-1111-1111-1111-1111111110a1', '{}', NOW() - INTERVAL '8 days' + INTERVAL '150 minutes'),
('55555555-5555-5555-5555-5555555550a6', '33333333-3333-3333-3333-3333333330a1', 'completed', 'system',   'auto-complete',                        '{}', NOW() - INTERVAL '8 days' + INTERVAL '180 minutes'),

-- Ngozi's booking
('55555555-5555-5555-5555-5555555550b1', '33333333-3333-3333-3333-3333333330a2', 'created',   'customer', '44444444-4444-4444-4444-4444444440a2', '{}', NOW() - INTERVAL '4 days'),
('55555555-5555-5555-5555-5555555550b2', '33333333-3333-3333-3333-3333333330a2', 'matched',   'system',   'matcher',                             '{}', NOW() - INTERVAL '4 days' + INTERVAL '3 minutes'),
('55555555-5555-5555-5555-5555555550b3', '33333333-3333-3333-3333-3333333330a2', 'accepted',  'provider', '11111111-1111-1111-1111-1111111110a2', '{}', NOW() - INTERVAL '4 days' + INTERVAL '11 minutes'),
('55555555-5555-5555-5555-5555555550b4', '33333333-3333-3333-3333-3333333330a2', 'picked_up', 'provider', '11111111-1111-1111-1111-1111111110a2', '{}', NOW() - INTERVAL '4 days' + INTERVAL '80 minutes'),
('55555555-5555-5555-5555-5555555550b5', '33333333-3333-3333-3333-3333333330a2', 'delivered', 'provider', '11111111-1111-1111-1111-1111111110a2', '{}', NOW() - INTERVAL '4 days' + INTERVAL '210 minutes'),
('55555555-5555-5555-5555-5555555550b6', '33333333-3333-3333-3333-3333333330a2', 'completed', 'system',   'auto-complete',                        '{}', NOW() - INTERVAL '4 days' + INTERVAL '240 minutes'),

-- Samuel's booking
('55555555-5555-5555-5555-5555555550c1', '33333333-3333-3333-3333-3333333330a3', 'created',   'customer', '44444444-4444-4444-4444-4444444440a3', '{}', NOW() - INTERVAL '1 day'),
('55555555-5555-5555-5555-5555555550c2', '33333333-3333-3333-3333-3333333330a3', 'matched',   'system',   'matcher',                             '{}', NOW() - INTERVAL '1 day' + INTERVAL '2 minutes'),
('55555555-5555-5555-5555-5555555550c3', '33333333-3333-3333-3333-3333333330a3', 'accepted',  'provider', '11111111-1111-1111-1111-1111111110a3', '{}', NOW() - INTERVAL '1 day' + INTERVAL '7 minutes'),
('55555555-5555-5555-5555-5555555550c4', '33333333-3333-3333-3333-3333333330a3', 'picked_up', 'provider', '11111111-1111-1111-1111-1111111110a3', '{}', NOW() - INTERVAL '1 day' + INTERVAL '60 minutes'),
('55555555-5555-5555-5555-5555555550c5', '33333333-3333-3333-3333-3333333330a3', 'delivered', 'provider', '11111111-1111-1111-1111-1111111110a3', '{}', NOW() - INTERVAL '1 day' + INTERVAL '175 minutes'),
('55555555-5555-5555-5555-5555555550c6', '33333333-3333-3333-3333-3333333330a3', 'completed', 'system',   'auto-complete',                        '{}', NOW() - INTERVAL '1 day' + INTERVAL '205 minutes')
ON CONFLICT DO NOTHING;
