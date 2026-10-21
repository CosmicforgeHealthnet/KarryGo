-- Staging test data: customers who book jobs for the test drivers.
-- Safe to re-run: fixed ids, and any existing row (by id, phone or email) is
-- left as it is. Sign in with the number and the staging test code.
--
--   0800 000 1001  Tolu Tester   (has trip history and wallet money)
--   0800 000 1002  Bimpe Tester  (fresh account, wallet money only)
INSERT INTO customers (id, phone, email, first_name, last_name, onboarding_status, status) VALUES
  ('5eed0c00-0000-4000-8000-000000000001', '+2348000001001', 'tolu.tester@karrygo.dev',  'Tolu',  'Tester', 'complete', 'active'),
  ('5eed0c00-0000-4000-8000-000000000002', '+2348000001002', 'bimpe.tester@karrygo.dev', 'Bimpe', 'Tester', 'complete', 'active')
ON CONFLICT DO NOTHING;
