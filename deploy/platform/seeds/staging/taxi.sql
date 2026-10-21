-- Staging test data: taxi drivers, cars, ride history and reviews.
-- Safe to re-run: fixed ids, ON CONFLICT DO NOTHING. Sign in on the taxi app
-- with the number and the staging test code.
--
--   0800 000 3001  Ngozi Taxi     approved (comfort car), 6 completed + 1 cancelled ride, reviews
--   0800 000 3002  Segun Rides    approved (economy car), 2 completed rides
--   0800 000 3003  Halima Newdriver awaiting approval (identity, documents and car in review)
--
-- Rides were booked by the test customer Tolu Tester (0800 000 1001). Uses the
-- staging Lagos market and zone.

INSERT INTO taxi_providers (
  id, phone, email, first_name, last_name, account_status, onboarding_status,
  identity_verification_status, vehicle_verification_status, vehicle_operational_status, document_compliance_status,
  rating, total_trips, location_state, location_city, service_type, language,
  driver_license_number, license_expiry_date,
  guarantor_name, guarantor_phone, emergency_contact_name, emergency_contact_phone, emergency_contact_relationship,
  accepts_parcels
) VALUES
  ('5eed0b00-0000-4000-8000-000000000001', '+2348000003001', 'ngozi.taxi@karrygo.dev', 'Ngozi', 'Taxi', 'active', 'complete',
   'verified', 'verified', 'active', 'compliant', 4.85, 6, 'Lagos', 'Ikeja', 'taxi', 'English',
   'LAG55667DD01', '2029-08-31', 'Obi Nnamdi', '+2348000009011', 'Ada Nnamdi', '+2348000009012', 'Sister', true),
  ('5eed0b00-0000-4000-8000-000000000002', '+2348000003002', 'segun.rides@karrygo.dev', 'Segun', 'Rides', 'active', 'complete',
   'verified', 'verified', 'active', 'compliant', 4.50, 2, 'Lagos', 'Yaba', 'taxi', 'English',
   'LAG77889EE02', '2028-03-31', 'Bayo Ojo', '+2348000009013', 'Funmi Ojo', '+2348000009014', 'Wife', false),
  ('5eed0b00-0000-4000-8000-000000000003', '+2348000003003', 'halima.newdriver@karrygo.dev', 'Halima', 'Newdriver', 'active', 'complete',
   'pending', 'pending', 'active', 'pending', 0, 0, 'Lagos', 'Surulere', 'taxi', 'English',
   'LAG99001FF03', '2031-01-31', 'Sani Bello', '+2348000009015', 'Zainab Bello', '+2348000009016', 'Mother', false)
ON CONFLICT DO NOTHING;

INSERT INTO vehicles (id, provider_id, vehicle_class, plate_number, seats, year, make, model, color, status) VALUES
  ('5eed0b10-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000001', 'comfort', 'STG-301-CMF', 4, 2020, 'Toyota', 'Camry', 'Black', 'active'),
  ('5eed0b10-0000-4000-8000-000000000002', '5eed0b00-0000-4000-8000-000000000002', 'economy', 'STG-302-ECO', 4, 2018, 'Toyota', 'Corolla', 'Silver', 'active'),
  ('5eed0b10-0000-4000-8000-000000000003', '5eed0b00-0000-4000-8000-000000000003', 'economy', 'STG-303-ECO', 4, 2019, 'Honda', 'Civic', 'Blue', 'active')
ON CONFLICT DO NOTHING;

INSERT INTO taxi_bookings (
  id, customer_id, provider_id, vehicle_id, market_id, operating_zone_id, vehicle_class, ride_purpose, passenger_count,
  fare_estimate_kobo, fare_final_kobo, pricing_quote_reference, pricing_version, distance_km, duration_minutes, actual_distance_km,
  payment_method, payment_status, status, pickup_verification_status,
  created_at, matching_started_at, matched_at, accepted_at, arrived_at, started_at, completed_at,
  cancelled_at, cancelled_by, cancel_reason
) VALUES
  ('5eed0b20-0000-4000-8000-000000000001', '5eed0c00-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000001', '5eed0b10-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000002', 'comfort', 'passenger', 1,
   685000, 685000, 'staging-seed-1', 'staging-seed', 21.3, 42, 21.3,
   'wallet', 'paid', 'completed', 'verified',
   now() - interval '1 days', now() - interval '1 days', now() - interval '1 days' + interval '1 minutes', now() - interval '1 days' + interval '2 minutes', now() - interval '1 days' + interval '8 minutes', now() - interval '1 days' + interval '10 minutes', now() - interval '1 days' + interval '52 minutes', NULL, NULL, NULL),
  ('5eed0b20-0000-4000-8000-000000000002', '5eed0c00-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000001', '5eed0b10-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000002', 'comfort', 'passenger', 1,
   940000, 940000, 'staging-seed-2', 'staging-seed', 28.9, 55, 28.9,
   'wallet', 'paid', 'completed', 'verified',
   now() - interval '3 days', now() - interval '3 days', now() - interval '3 days' + interval '1 minutes', now() - interval '3 days' + interval '2 minutes', now() - interval '3 days' + interval '8 minutes', now() - interval '3 days' + interval '10 minutes', now() - interval '3 days' + interval '65 minutes', NULL, NULL, NULL),
  ('5eed0b20-0000-4000-8000-000000000003', '5eed0c00-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000001', '5eed0b10-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000002', 'comfort', 'passenger', 1,
   215000, 215000, 'staging-seed-3', 'staging-seed', 4.8, 14, 4.8,
   'wallet', 'paid', 'completed', 'verified',
   now() - interval '4 days', now() - interval '4 days', now() - interval '4 days' + interval '1 minutes', now() - interval '4 days' + interval '2 minutes', now() - interval '4 days' + interval '8 minutes', now() - interval '4 days' + interval '10 minutes', now() - interval '4 days' + interval '24 minutes', NULL, NULL, NULL),
  ('5eed0b20-0000-4000-8000-000000000004', '5eed0c00-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000001', '5eed0b10-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000002', 'comfort', 'passenger', 1,
   530000, 530000, 'staging-seed-4', 'staging-seed', 15.2, 33, 15.2,
   'wallet', 'paid', 'completed', 'verified',
   now() - interval '6 days', now() - interval '6 days', now() - interval '6 days' + interval '1 minutes', now() - interval '6 days' + interval '2 minutes', now() - interval '6 days' + interval '8 minutes', now() - interval '6 days' + interval '10 minutes', now() - interval '6 days' + interval '43 minutes', NULL, NULL, NULL),
  ('5eed0b20-0000-4000-8000-000000000005', '5eed0c00-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000001', '5eed0b10-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000002', 'comfort', 'passenger', 1,
   690000, 690000, 'staging-seed-5', 'staging-seed', 21.0, 47, 21.0,
   'wallet', 'paid', 'completed', 'verified',
   now() - interval '10 days', now() - interval '10 days', now() - interval '10 days' + interval '1 minutes', now() - interval '10 days' + interval '2 minutes', now() - interval '10 days' + interval '8 minutes', now() - interval '10 days' + interval '10 minutes', now() - interval '10 days' + interval '57 minutes', NULL, NULL, NULL),
  ('5eed0b20-0000-4000-8000-000000000006', '5eed0c00-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000001', '5eed0b10-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000002', 'comfort', 'passenger', 1,
   610000, 610000, 'staging-seed-6', 'staging-seed', 18.4, 39, 18.4,
   'wallet', 'paid', 'completed', 'verified',
   now() - interval '13 days', now() - interval '13 days', now() - interval '13 days' + interval '1 minutes', now() - interval '13 days' + interval '2 minutes', now() - interval '13 days' + interval '8 minutes', now() - interval '13 days' + interval '10 minutes', now() - interval '13 days' + interval '49 minutes', NULL, NULL, NULL),
  ('5eed0b20-0000-4000-8000-000000000007', '5eed0c00-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000001', '5eed0b10-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000002', 'comfort', 'passenger', 1,
   480000, NULL, 'staging-seed-7', 'staging-seed', 14.7, 30, NULL,
   'wallet', 'unpaid', 'cancelled', 'not_required',
   now() - interval '2 days', now() - interval '2 days', now() - interval '2 days' + interval '1 minutes', now() - interval '2 days' + interval '2 minutes', NULL, NULL, NULL, now() - interval '2 days' + interval '6 minutes', 'customer', 'Plans changed'),
  ('5eed0b20-0000-4000-8000-000000000008', '5eed0c00-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000002', '5eed0b10-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000002', 'economy', 'passenger', 1,
   295000, 295000, 'staging-seed-8', 'staging-seed', 11.6, 26, 11.6,
   'wallet', 'paid', 'completed', 'verified',
   now() - interval '2 days', now() - interval '2 days', now() - interval '2 days' + interval '1 minutes', now() - interval '2 days' + interval '2 minutes', now() - interval '2 days' + interval '8 minutes', now() - interval '2 days' + interval '10 minutes', now() - interval '2 days' + interval '36 minutes', NULL, NULL, NULL),
  ('5eed0b20-0000-4000-8000-000000000009', '5eed0c00-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000002', '5eed0b10-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000002', 'economy', 'passenger', 1,
   230000, 230000, 'staging-seed-9', 'staging-seed', 8.3, 20, 8.3,
   'wallet', 'paid', 'completed', 'verified',
   now() - interval '5 days', now() - interval '5 days', now() - interval '5 days' + interval '1 minutes', now() - interval '5 days' + interval '2 minutes', now() - interval '5 days' + interval '8 minutes', now() - interval '5 days' + interval '10 minutes', now() - interval '5 days' + interval '30 minutes', NULL, NULL, NULL)
ON CONFLICT DO NOTHING;

INSERT INTO taxi_booking_stops (booking_id, sequence, type, address, lat, lng, status) VALUES
  ('5eed0b20-0000-4000-8000-000000000001', 1, 'PICKUP', 'Ikeja City Mall, Obafemi Awolowo Way', 6.6143, 3.3576, 'completed'),
  ('5eed0b20-0000-4000-8000-000000000001', 2, 'DROPOFF', 'Eko Hotel, Victoria Island', 6.4262, 3.4295, 'completed'),
  ('5eed0b20-0000-4000-8000-000000000002', 1, 'PICKUP', 'Murtala Muhammed Airport, Ikeja', 6.5774, 3.3215, 'completed'),
  ('5eed0b20-0000-4000-8000-000000000002', 2, 'DROPOFF', 'Lekki Phase 1 Gate', 6.4474, 3.4724, 'completed'),
  ('5eed0b20-0000-4000-8000-000000000003', 1, 'PICKUP', 'Tejuosho Market, Yaba', 6.5103, 3.3726, 'completed'),
  ('5eed0b20-0000-4000-8000-000000000003', 2, 'DROPOFF', 'National Stadium, Surulere', 6.4993, 3.3639, 'completed'),
  ('5eed0b20-0000-4000-8000-000000000004', 1, 'PICKUP', 'Maryland Mall', 6.5715, 3.3676, 'completed'),
  ('5eed0b20-0000-4000-8000-000000000004', 2, 'DROPOFF', 'Falomo Roundabout, Ikoyi', 6.4429, 3.4363, 'completed'),
  ('5eed0b20-0000-4000-8000-000000000005', 1, 'PICKUP', 'Eko Hotel, Victoria Island', 6.4262, 3.4295, 'completed'),
  ('5eed0b20-0000-4000-8000-000000000005', 2, 'DROPOFF', 'Ikeja City Mall, Obafemi Awolowo Way', 6.6143, 3.3576, 'completed'),
  ('5eed0b20-0000-4000-8000-000000000006', 1, 'PICKUP', 'Lekki Phase 1 Gate', 6.4474, 3.4724, 'completed'),
  ('5eed0b20-0000-4000-8000-000000000006', 2, 'DROPOFF', 'Tejuosho Market, Yaba', 6.5103, 3.3726, 'completed'),
  ('5eed0b20-0000-4000-8000-000000000007', 1, 'PICKUP', 'National Stadium, Surulere', 6.4993, 3.3639, 'pending'),
  ('5eed0b20-0000-4000-8000-000000000007', 2, 'DROPOFF', 'Murtala Muhammed Airport, Ikeja', 6.5774, 3.3215, 'pending'),
  ('5eed0b20-0000-4000-8000-000000000008', 1, 'PICKUP', 'Tejuosho Market, Yaba', 6.5103, 3.3726, 'completed'),
  ('5eed0b20-0000-4000-8000-000000000008', 2, 'DROPOFF', 'Ikeja City Mall, Obafemi Awolowo Way', 6.6143, 3.3576, 'completed'),
  ('5eed0b20-0000-4000-8000-000000000009', 1, 'PICKUP', 'Maryland Mall', 6.5715, 3.3676, 'completed'),
  ('5eed0b20-0000-4000-8000-000000000009', 2, 'DROPOFF', 'Tejuosho Market, Yaba', 6.5103, 3.3726, 'completed')
ON CONFLICT (booking_id, sequence) DO NOTHING;

-- Both directions, so each side can already read the other's review.
INSERT INTO booking_reviews (id, booking_id, direction, customer_id, provider_id, rating, review_text, recommends_driver) VALUES
  ('5eed0b30-0000-4000-8000-000000000001', '5eed0b20-0000-4000-8000-000000000001', 'customer_to_provider', '5eed0c00-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000001', 5, 'Smooth ride and very polite.', true),
  ('5eed0b31-0000-4000-8000-000000000001', '5eed0b20-0000-4000-8000-000000000001', 'provider_to_customer', '5eed0c00-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000001', 5, '', NULL),
  ('5eed0b30-0000-4000-8000-000000000002', '5eed0b20-0000-4000-8000-000000000002', 'customer_to_provider', '5eed0c00-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000001', 5, 'Helped with my luggage. Great driver!', true),
  ('5eed0b31-0000-4000-8000-000000000002', '5eed0b20-0000-4000-8000-000000000002', 'provider_to_customer', '5eed0c00-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000001', 5, '', NULL),
  ('5eed0b30-0000-4000-8000-000000000003', '5eed0b20-0000-4000-8000-000000000003', 'customer_to_provider', '5eed0c00-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000001', 4, 'Good ride.', true),
  ('5eed0b31-0000-4000-8000-000000000003', '5eed0b20-0000-4000-8000-000000000003', 'provider_to_customer', '5eed0c00-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000001', 5, '', NULL),
  ('5eed0b30-0000-4000-8000-000000000004', '5eed0b20-0000-4000-8000-000000000004', 'customer_to_provider', '5eed0c00-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000001', 5, '', true),
  ('5eed0b31-0000-4000-8000-000000000004', '5eed0b20-0000-4000-8000-000000000004', 'provider_to_customer', '5eed0c00-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000001', 4, '', NULL),
  ('5eed0b30-0000-4000-8000-000000000008', '5eed0b20-0000-4000-8000-000000000008', 'customer_to_provider', '5eed0c00-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000002', 4, 'On time.', true),
  ('5eed0b31-0000-4000-8000-000000000008', '5eed0b20-0000-4000-8000-000000000008', 'provider_to_customer', '5eed0c00-0000-4000-8000-000000000001', '5eed0b00-0000-4000-8000-000000000002', 5, '', NULL)
ON CONFLICT DO NOTHING;
