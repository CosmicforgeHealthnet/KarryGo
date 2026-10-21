-- Staging test data: wallet balances for the test drivers and customers.
-- Each credit is one balanced ledger transaction against the system Paystack
-- receivable, the same shape as a real top-up. Safe to re-run: accounts are
-- created only if missing, and a transaction (unique reference) is posted
-- once; its entries are written only when the transaction itself is new.
--
--   Musa Haulage  N107,695.00   Kemi Carriers  N14,705.00   (truck)
--   Ngozi Taxi     N31,195.00   Segun Rides     N4,462.50   (taxi)
--   Tolu Tester    N50,000.00   Bimpe Tester   N25,000.00   (customers)

INSERT INTO wallet_accounts (id, owner_type, owner_id, account_type, currency, normal_balance, status)
VALUES (gen_random_uuid(), 'system', 'paystack', 'paystack_receivable', 'NGN', 'debit', 'active')
ON CONFLICT (owner_type, owner_id, account_type, currency) DO NOTHING;

INSERT INTO wallet_accounts (id, owner_type, owner_id, account_type, currency, normal_balance, status)
VALUES (gen_random_uuid(), 'provider', 'truck_provider:5eed0a00-0000-4000-8000-000000000001', 'provider_payable', 'NGN', 'credit', 'active')
ON CONFLICT (owner_type, owner_id, account_type, currency) DO NOTHING;

WITH t AS (
  INSERT INTO ledger_transactions (id, reference, transaction_type, status, source_service, source_reference, idempotency_key, metadata)
  VALUES ('5eed0d10-0000-4000-8000-000000000001', 'staging-seed-credit-1', 'staging_seed_credit', 'posted',
          'staging-seed', 'truck_provider:5eed0a00-0000-4000-8000-000000000001', 'staging-seed-credit-1', '{"seed": "staging"}'::jsonb)
  ON CONFLICT DO NOTHING
  RETURNING id
)
INSERT INTO ledger_entries (transaction_id, account_id, side, amount_kobo, currency, memo)
SELECT t.id, a.id, x.side, 10769500, 'NGN', 'Earnings from 6 completed jobs (staging seed)'
FROM t
CROSS JOIN (VALUES ('debit', 'system', 'paystack', 'paystack_receivable'),
                   ('credit', 'provider', 'truck_provider:5eed0a00-0000-4000-8000-000000000001', 'provider_payable')) AS x(side, owner_type, owner_id, account_type)
JOIN wallet_accounts a ON a.owner_type = x.owner_type AND a.owner_id = x.owner_id
                      AND a.account_type = x.account_type AND a.currency = 'NGN';

INSERT INTO wallet_accounts (id, owner_type, owner_id, account_type, currency, normal_balance, status)
VALUES (gen_random_uuid(), 'provider', 'truck_provider:5eed0a00-0000-4000-8000-000000000002', 'provider_payable', 'NGN', 'credit', 'active')
ON CONFLICT (owner_type, owner_id, account_type, currency) DO NOTHING;

WITH t AS (
  INSERT INTO ledger_transactions (id, reference, transaction_type, status, source_service, source_reference, idempotency_key, metadata)
  VALUES ('5eed0d10-0000-4000-8000-000000000002', 'staging-seed-credit-2', 'staging_seed_credit', 'posted',
          'staging-seed', 'truck_provider:5eed0a00-0000-4000-8000-000000000002', 'staging-seed-credit-2', '{"seed": "staging"}'::jsonb)
  ON CONFLICT DO NOTHING
  RETURNING id
)
INSERT INTO ledger_entries (transaction_id, account_id, side, amount_kobo, currency, memo)
SELECT t.id, a.id, x.side, 1470500, 'NGN', 'Earnings from 2 completed jobs (staging seed)'
FROM t
CROSS JOIN (VALUES ('debit', 'system', 'paystack', 'paystack_receivable'),
                   ('credit', 'provider', 'truck_provider:5eed0a00-0000-4000-8000-000000000002', 'provider_payable')) AS x(side, owner_type, owner_id, account_type)
JOIN wallet_accounts a ON a.owner_type = x.owner_type AND a.owner_id = x.owner_id
                      AND a.account_type = x.account_type AND a.currency = 'NGN';

INSERT INTO wallet_accounts (id, owner_type, owner_id, account_type, currency, normal_balance, status)
VALUES (gen_random_uuid(), 'provider', 'taxi:5eed0b00-0000-4000-8000-000000000001', 'provider_payable', 'NGN', 'credit', 'active')
ON CONFLICT (owner_type, owner_id, account_type, currency) DO NOTHING;

WITH t AS (
  INSERT INTO ledger_transactions (id, reference, transaction_type, status, source_service, source_reference, idempotency_key, metadata)
  VALUES ('5eed0d10-0000-4000-8000-000000000003', 'staging-seed-credit-3', 'staging_seed_credit', 'posted',
          'staging-seed', 'taxi:5eed0b00-0000-4000-8000-000000000001', 'staging-seed-credit-3', '{"seed": "staging"}'::jsonb)
  ON CONFLICT DO NOTHING
  RETURNING id
)
INSERT INTO ledger_entries (transaction_id, account_id, side, amount_kobo, currency, memo)
SELECT t.id, a.id, x.side, 3119500, 'NGN', 'Earnings from 6 completed rides (staging seed)'
FROM t
CROSS JOIN (VALUES ('debit', 'system', 'paystack', 'paystack_receivable'),
                   ('credit', 'provider', 'taxi:5eed0b00-0000-4000-8000-000000000001', 'provider_payable')) AS x(side, owner_type, owner_id, account_type)
JOIN wallet_accounts a ON a.owner_type = x.owner_type AND a.owner_id = x.owner_id
                      AND a.account_type = x.account_type AND a.currency = 'NGN';

INSERT INTO wallet_accounts (id, owner_type, owner_id, account_type, currency, normal_balance, status)
VALUES (gen_random_uuid(), 'provider', 'taxi:5eed0b00-0000-4000-8000-000000000002', 'provider_payable', 'NGN', 'credit', 'active')
ON CONFLICT (owner_type, owner_id, account_type, currency) DO NOTHING;

WITH t AS (
  INSERT INTO ledger_transactions (id, reference, transaction_type, status, source_service, source_reference, idempotency_key, metadata)
  VALUES ('5eed0d10-0000-4000-8000-000000000004', 'staging-seed-credit-4', 'staging_seed_credit', 'posted',
          'staging-seed', 'taxi:5eed0b00-0000-4000-8000-000000000002', 'staging-seed-credit-4', '{"seed": "staging"}'::jsonb)
  ON CONFLICT DO NOTHING
  RETURNING id
)
INSERT INTO ledger_entries (transaction_id, account_id, side, amount_kobo, currency, memo)
SELECT t.id, a.id, x.side, 446250, 'NGN', 'Earnings from 2 completed rides (staging seed)'
FROM t
CROSS JOIN (VALUES ('debit', 'system', 'paystack', 'paystack_receivable'),
                   ('credit', 'provider', 'taxi:5eed0b00-0000-4000-8000-000000000002', 'provider_payable')) AS x(side, owner_type, owner_id, account_type)
JOIN wallet_accounts a ON a.owner_type = x.owner_type AND a.owner_id = x.owner_id
                      AND a.account_type = x.account_type AND a.currency = 'NGN';

INSERT INTO wallet_accounts (id, owner_type, owner_id, account_type, currency, normal_balance, status)
VALUES (gen_random_uuid(), 'customer', '5eed0c00-0000-4000-8000-000000000001', 'customer_available', 'NGN', 'credit', 'active')
ON CONFLICT (owner_type, owner_id, account_type, currency) DO NOTHING;

WITH t AS (
  INSERT INTO ledger_transactions (id, reference, transaction_type, status, source_service, source_reference, idempotency_key, metadata)
  VALUES ('5eed0d10-0000-4000-8000-000000000005', 'staging-seed-credit-5', 'staging_seed_credit', 'posted',
          'staging-seed', '5eed0c00-0000-4000-8000-000000000001', 'staging-seed-credit-5', '{"seed": "staging"}'::jsonb)
  ON CONFLICT DO NOTHING
  RETURNING id
)
INSERT INTO ledger_entries (transaction_id, account_id, side, amount_kobo, currency, memo)
SELECT t.id, a.id, x.side, 5000000, 'NGN', 'Wallet top-up (staging seed)'
FROM t
CROSS JOIN (VALUES ('debit', 'system', 'paystack', 'paystack_receivable'),
                   ('credit', 'customer', '5eed0c00-0000-4000-8000-000000000001', 'customer_available')) AS x(side, owner_type, owner_id, account_type)
JOIN wallet_accounts a ON a.owner_type = x.owner_type AND a.owner_id = x.owner_id
                      AND a.account_type = x.account_type AND a.currency = 'NGN';

INSERT INTO wallet_accounts (id, owner_type, owner_id, account_type, currency, normal_balance, status)
VALUES (gen_random_uuid(), 'customer', '5eed0c00-0000-4000-8000-000000000002', 'customer_available', 'NGN', 'credit', 'active')
ON CONFLICT (owner_type, owner_id, account_type, currency) DO NOTHING;

WITH t AS (
  INSERT INTO ledger_transactions (id, reference, transaction_type, status, source_service, source_reference, idempotency_key, metadata)
  VALUES ('5eed0d10-0000-4000-8000-000000000006', 'staging-seed-credit-6', 'staging_seed_credit', 'posted',
          'staging-seed', '5eed0c00-0000-4000-8000-000000000002', 'staging-seed-credit-6', '{"seed": "staging"}'::jsonb)
  ON CONFLICT DO NOTHING
  RETURNING id
)
INSERT INTO ledger_entries (transaction_id, account_id, side, amount_kobo, currency, memo)
SELECT t.id, a.id, x.side, 2500000, 'NGN', 'Wallet top-up (staging seed)'
FROM t
CROSS JOIN (VALUES ('debit', 'system', 'paystack', 'paystack_receivable'),
                   ('credit', 'customer', '5eed0c00-0000-4000-8000-000000000002', 'customer_available')) AS x(side, owner_type, owner_id, account_type)
JOIN wallet_accounts a ON a.owner_type = x.owner_type AND a.owner_id = x.owner_id
                      AND a.account_type = x.account_type AND a.currency = 'NGN';
