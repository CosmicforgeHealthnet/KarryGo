-- 008_partner_billing.sql
-- Partner invoice charging: extend the 006 Billing & Usage invoice table with
-- charge-loop metadata, saved card authorizations, and charge attempts.
-- api-gateway never holds Paystack secret keys: charges are executed by
-- payment-wallet-service via POST /api/v1/payment-wallet/internal/charge-authorization,
-- and results come back through POST /api/v1/internal/callbacks/charge-result.

ALTER TABLE partner_invoices
    ADD COLUMN IF NOT EXISTS partner_email text,
    ADD COLUMN IF NOT EXISTS due_at timestamptz,
    ADD COLUMN IF NOT EXISTS wallet_reference text,
    ADD COLUMN IF NOT EXISTS failure_reason text,
    ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_constraint
               WHERE conrelid = 'partner_invoices'::regclass
                 AND conname = 'partner_invoices_status_check') THEN
        ALTER TABLE partner_invoices DROP CONSTRAINT partner_invoices_status_check;
    END IF;

    ALTER TABLE partner_invoices
        ADD CONSTRAINT partner_invoices_status_check
        CHECK (status IN ('pending', 'charging', 'paid', 'failed', 'charge_failed'));
END $$;

CREATE INDEX IF NOT EXISTS idx_partner_invoices_due_at ON partner_invoices(due_at);

-- Saved Paystack card authorizations for recurring partner charges.
-- authorization_code is never exposed through any API response.
CREATE TABLE IF NOT EXISTS partner_billing_authorizations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id uuid NOT NULL REFERENCES partner_accounts(id) ON DELETE CASCADE,
    email text NOT NULL,
    authorization_code text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_partner_billing_authorizations_partner_id
    ON partner_billing_authorizations(partner_id);
CREATE INDEX IF NOT EXISTS idx_partner_billing_authorizations_status
    ON partner_billing_authorizations(status);

-- One row per charge attempt against an invoice. idempotency_key is globally
-- unique so a crashed/re-run charge loop can never double-charge one attempt.
CREATE TABLE IF NOT EXISTS partner_invoice_charge_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id uuid NOT NULL REFERENCES partner_invoices(id) ON DELETE CASCADE,
    partner_id uuid NOT NULL REFERENCES partner_accounts(id) ON DELETE CASCADE,
    idempotency_key text NOT NULL UNIQUE,
    wallet_reference text,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'success', 'failed')),
    amount_kobo bigint NOT NULL,
    failure_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_partner_invoice_charge_attempts_invoice_id
    ON partner_invoice_charge_attempts(invoice_id);
CREATE INDEX IF NOT EXISTS idx_partner_invoice_charge_attempts_status
    ON partner_invoice_charge_attempts(status);
