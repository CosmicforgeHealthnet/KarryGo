-- Staging test data: truck drivers, trucks, trip history and reviews.
-- Safe to re-run: fixed ids, ON CONFLICT DO NOTHING. Sign in on the truck app
-- with the number and the staging test code.
--
--   0800 000 2001  Musa Haulage   approved, 2 trucks, 6 completed + 1 cancelled job, reviews
--   0800 000 2002  Kemi Carriers  approved, 1 truck, 2 completed jobs
--   0800 000 2003  Femi Newdriver awaiting approval (documents in review, truck not active)
--
-- Jobs were booked by the test customer Tolu Tester (0800 000 1001).

INSERT INTO truck_providers (
  id, phone, email, first_name, last_name, status, onboarding_status, rating, total_trips,
  location_state, location_city, operation_mode, service_type, language,
  driver_license_number, license_expiry_year, license_expiry_date,
  guarantor_name, guarantor_phone, emergency_contact_name, emergency_contact_phone, emergency_contact_relationship
) VALUES
  ('5eed0a00-0000-4000-8000-000000000001', '+2348000002001', 'musa.haulage@karrygo.dev', 'Musa', 'Haulage', 'active', 'complete', 4.80, 6,
   'Lagos', 'Ikeja', 'owner_operator', 'hauling', 'English', 'LAG24681AA01', '2029', '2029-06-30',
   'Aliyu Musa', '+2348000009001', 'Hauwa Musa', '+2348000009002', 'Wife'),
  ('5eed0a00-0000-4000-8000-000000000002', '+2348000002002', 'kemi.carriers@karrygo.dev', 'Kemi', 'Carriers', 'active', 'complete', 4.50, 2,
   'Lagos', 'Yaba', 'owner_operator', 'hauling', 'English', 'LAG13579BB02', '2028', '2028-11-15',
   'Dele Adewale', '+2348000009003', 'Tope Adewale', '+2348000009004', 'Brother'),
  ('5eed0a00-0000-4000-8000-000000000003', '+2348000002003', 'femi.newdriver@karrygo.dev', 'Femi', 'Newdriver', 'active', 'pending_verification', 0, 0,
   'Lagos', 'Surulere', 'owner_operator', 'hauling', 'English', 'LAG11223CC03', '2030', '2030-02-28',
   'Kunle Ade', '+2348000009005', 'Yemi Ade', '+2348000009006', 'Mother')
ON CONFLICT DO NOTHING;

INSERT INTO trucks (id, provider_id, truck_type, capacity_kg, plate_number, year, make, model, color, status) VALUES
  ('5eed0a10-0000-4000-8000-000000000001', '5eed0a00-0000-4000-8000-000000000001', 'flatbed', 12000, 'STG-201-FLB', 2020, 'MAN', 'TGS', 'Blue', 'active'),
  ('5eed0a10-0000-4000-8000-000000000002', '5eed0a00-0000-4000-8000-000000000001', 'van', 3000, 'STG-201-VAN', 2022, 'Toyota', 'Dyna', 'White', 'active'),
  ('5eed0a10-0000-4000-8000-000000000003', '5eed0a00-0000-4000-8000-000000000002', 'van', 2500, 'STG-202-VAN', 2021, 'Nissan', 'NV350', 'Silver', 'active'),
  ('5eed0a10-0000-4000-8000-000000000004', '5eed0a00-0000-4000-8000-000000000003', 'tipper', 15000, 'STG-203-TPR', 2019, 'Howo', 'A7', 'Yellow', 'inactive')
ON CONFLICT DO NOTHING;

INSERT INTO haulage_bookings (
  id, customer_id, provider_id, truck_id,
  pickup_address, pickup_lat, pickup_lng, dropoff_address, dropoff_lat, dropoff_lng,
  cargo_type, cargo_weight_kg, cargo_description, package_content, package_size, distance_km,
  fare_estimate_kobo, fare_final_kobo, status, payment_method, payment_status,
  receiver_name, receiver_phone, receiver_customer_id,
  created_at, matched_at, accepted_at, picked_up_at, delivered_at, completed_at,
  cancelled_at, cancelled_by, cancel_reason
) VALUES
  ('5eed0a20-0000-4000-8000-000000000001', '5eed0c00-0000-4000-8000-000000000001', '5eed0a00-0000-4000-8000-000000000001', '5eed0a10-0000-4000-8000-000000000001',
   'Wharf Road, Apapa', 6.4489, 3.3594, 'Allen Avenue, Ikeja', 6.6018, 3.3515,
   'general', 4200, 'Building materials', 'Building materials', 'large', 19.4,
   2450000, 2450000, 'completed', 'wallet', 'paid',
   'Chinedu Okafor', '+2348000009001', '5eed0c00-0000-4000-8000-000000000001',
   now() - interval '2 days', now() - interval '2 days' + interval '1 minutes', now() - interval '2 days' + interval '2 minutes', now() - interval '2 days' + interval '30 minutes', now() - interval '2 days' + interval '75 minutes', now() - interval '2 days' + interval '110 minutes',
   NULL, NULL, NULL),
  ('5eed0a20-0000-4000-8000-000000000002', '5eed0c00-0000-4000-8000-000000000001', '5eed0a00-0000-4000-8000-000000000001', '5eed0a10-0000-4000-8000-000000000002',
   'Herbert Macaulay Way, Yaba', 6.5095, 3.3711, 'Admiralty Way, Lekki Phase 1', 6.4474, 3.4724,
   'general', 650, 'Office furniture', 'Office furniture', 'medium', 17.2,
   1580000, 1580000, 'completed', 'wallet', 'paid',
   'Amaka Obi', '+2348000009002', '5eed0c00-0000-4000-8000-000000000001',
   now() - interval '4 days', now() - interval '4 days' + interval '1 minutes', now() - interval '4 days' + interval '2 minutes', now() - interval '4 days' + interval '30 minutes', now() - interval '4 days' + interval '75 minutes', now() - interval '4 days' + interval '110 minutes',
   NULL, NULL, NULL),
  ('5eed0a20-0000-4000-8000-000000000003', '5eed0c00-0000-4000-8000-000000000001', '5eed0a00-0000-4000-8000-000000000001', '5eed0a10-0000-4000-8000-000000000001',
   'Oshodi Market, Oshodi', 6.5568, 3.3488, 'Ikorodu Road, Ikorodu', 6.6194, 3.5105,
   'general', 5000, 'Cement bags', 'Cement bags', 'large', 22.8,
   2730000, 2730000, 'completed', 'wallet', 'paid',
   'Sule Bello', '+2348000009003', '5eed0c00-0000-4000-8000-000000000001',
   now() - interval '7 days', now() - interval '7 days' + interval '1 minutes', now() - interval '7 days' + interval '2 minutes', now() - interval '7 days' + interval '30 minutes', now() - interval '7 days' + interval '75 minutes', now() - interval '7 days' + interval '110 minutes',
   NULL, NULL, NULL),
  ('5eed0a20-0000-4000-8000-000000000004', '5eed0c00-0000-4000-8000-000000000001', '5eed0a00-0000-4000-8000-000000000001', '5eed0a10-0000-4000-8000-000000000002',
   'Adeniran Ogunsanya, Surulere', 6.4969, 3.3532, 'Ahmadu Bello Way, Victoria Island', 6.4281, 3.4219,
   'general', 400, 'Household items', 'Household items', 'medium', 9.6,
   980000, 980000, 'completed', 'wallet', 'paid',
   'Funke Adeyemi', '+2348000009004', '5eed0c00-0000-4000-8000-000000000001',
   now() - interval '9 days', now() - interval '9 days' + interval '1 minutes', now() - interval '9 days' + interval '2 minutes', now() - interval '9 days' + interval '30 minutes', now() - interval '9 days' + interval '75 minutes', now() - interval '9 days' + interval '110 minutes',
   NULL, NULL, NULL),
  ('5eed0a20-0000-4000-8000-000000000005', '5eed0c00-0000-4000-8000-000000000001', '5eed0a00-0000-4000-8000-000000000001', '5eed0a10-0000-4000-8000-000000000002',
   'Allen Avenue, Ikeja', 6.6018, 3.3515, 'Herbert Macaulay Way, Yaba', 6.5095, 3.3711,
   'general', 300, 'Electronics', 'Electronics', 'small', 13.1,
   1240000, 1240000, 'completed', 'wallet', 'paid',
   'Ibrahim Musa', '+2348000009005', '5eed0c00-0000-4000-8000-000000000001',
   now() - interval '12 days', now() - interval '12 days' + interval '1 minutes', now() - interval '12 days' + interval '2 minutes', now() - interval '12 days' + interval '30 minutes', now() - interval '12 days' + interval '75 minutes', now() - interval '12 days' + interval '110 minutes',
   NULL, NULL, NULL),
  ('5eed0a20-0000-4000-8000-000000000006', '5eed0c00-0000-4000-8000-000000000001', '5eed0a00-0000-4000-8000-000000000001', '5eed0a10-0000-4000-8000-000000000001',
   'Wharf Road, Apapa', 6.4489, 3.3594, 'Ajah Roundabout, Ajah', 6.4698, 3.5852,
   'general', 6000, 'Steel rods', 'Steel rods', 'large', 31.5,
   3690000, 3690000, 'completed', 'wallet', 'paid',
   'Grace Eze', '+2348000009006', '5eed0c00-0000-4000-8000-000000000001',
   now() - interval '16 days', now() - interval '16 days' + interval '1 minutes', now() - interval '16 days' + interval '2 minutes', now() - interval '16 days' + interval '30 minutes', now() - interval '16 days' + interval '75 minutes', now() - interval '16 days' + interval '110 minutes',
   NULL, NULL, NULL),
  ('5eed0a20-0000-4000-8000-000000000007', '5eed0c00-0000-4000-8000-000000000001', '5eed0a00-0000-4000-8000-000000000001', '5eed0a10-0000-4000-8000-000000000002',
   'Admiralty Way, Lekki Phase 1', 6.4474, 3.4724, 'Adeniran Ogunsanya, Surulere', 6.4969, 3.3532,
   'general', 250, 'Clothing stock', 'Clothing stock', 'small', 14.9,
   1320000, NULL, 'cancelled', 'wallet', 'unpaid',
   'Tunde Bakare', '+2348000009007', '5eed0c00-0000-4000-8000-000000000001',
   now() - interval '5 days', now() - interval '5 days' + interval '1 minutes', now() - interval '5 days' + interval '2 minutes', NULL, NULL, NULL,
   now() - interval '5 days' + interval '9 minutes', 'customer', 'Customer changed plans'),
  ('5eed0a20-0000-4000-8000-000000000008', '5eed0c00-0000-4000-8000-000000000001', '5eed0a00-0000-4000-8000-000000000002', '5eed0a10-0000-4000-8000-000000000003',
   'Herbert Macaulay Way, Yaba', 6.5095, 3.3711, 'Adeniran Ogunsanya, Surulere', 6.4969, 3.3532,
   'general', 180, 'Fridge and cooker', 'Fridge and cooker', 'small', 6.1,
   790000, 790000, 'completed', 'wallet', 'paid',
   'Ngozi Udeh', '+2348000009008', '5eed0c00-0000-4000-8000-000000000001',
   now() - interval '3 days', now() - interval '3 days' + interval '1 minutes', now() - interval '3 days' + interval '2 minutes', now() - interval '3 days' + interval '30 minutes', now() - interval '3 days' + interval '75 minutes', now() - interval '3 days' + interval '110 minutes',
   NULL, NULL, NULL),
  ('5eed0a20-0000-4000-8000-000000000009', '5eed0c00-0000-4000-8000-000000000001', '5eed0a00-0000-4000-8000-000000000002', '5eed0a10-0000-4000-8000-000000000003',
   'Ahmadu Bello Way, Victoria Island', 6.4281, 3.4219, 'Admiralty Way, Lekki Phase 1', 6.4474, 3.4724,
   'general', 350, 'Event chairs', 'Event chairs', 'medium', 8.7,
   940000, 940000, 'completed', 'wallet', 'paid',
   'Bola Ahmed', '+2348000009009', '5eed0c00-0000-4000-8000-000000000001',
   now() - interval '8 days', now() - interval '8 days' + interval '1 minutes', now() - interval '8 days' + interval '2 minutes', now() - interval '8 days' + interval '30 minutes', now() - interval '8 days' + interval '75 minutes', now() - interval '8 days' + interval '110 minutes',
   NULL, NULL, NULL)
ON CONFLICT DO NOTHING;

INSERT INTO booking_reviews (id, booking_id, customer_id, provider_id, rating, review_text, recommends_driver) VALUES
  ('5eed0a30-0000-4000-8000-000000000001', '5eed0a20-0000-4000-8000-000000000001', '5eed0c00-0000-4000-8000-000000000001', '5eed0a00-0000-4000-8000-000000000001', 5, 'Careful with the materials and on time. Thank you!', true),
  ('5eed0a30-0000-4000-8000-000000000002', '5eed0a20-0000-4000-8000-000000000002', '5eed0c00-0000-4000-8000-000000000001', '5eed0a00-0000-4000-8000-000000000001', 5, 'Very professional, furniture arrived without a scratch.', true),
  ('5eed0a30-0000-4000-8000-000000000003', '5eed0a20-0000-4000-8000-000000000003', '5eed0c00-0000-4000-8000-000000000001', '5eed0a00-0000-4000-8000-000000000001', 4, 'Good job, a bit late at pickup.', true),
  ('5eed0a30-0000-4000-8000-000000000005', '5eed0a20-0000-4000-8000-000000000005', '5eed0c00-0000-4000-8000-000000000001', '5eed0a00-0000-4000-8000-000000000001', 5, '', true),
  ('5eed0a30-0000-4000-8000-000000000008', '5eed0a20-0000-4000-8000-000000000008', '5eed0c00-0000-4000-8000-000000000001', '5eed0a00-0000-4000-8000-000000000002', 4, 'Friendly driver.', true)
ON CONFLICT DO NOTHING;
