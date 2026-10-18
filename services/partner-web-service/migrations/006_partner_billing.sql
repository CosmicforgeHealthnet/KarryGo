-- 006_partner_billing.sql
-- Billing & Usage: this is a brand-new feature, not a field addition - no
-- billing/invoicing concept existed anywhere in the backend before this
-- (payment-wallet-service only has customer/provider wallet ledgers, no
-- partner billing).
--
-- partner_payment_methods.authorization_code is a Paystack-issued token, never
-- a raw card number - the frontend tokenizes the card via Paystack
-- Inline/Popup and only sends the resulting token here.

CREATE TABLE IF NOT EXISTS partner_invoices (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id uuid NOT NULL REFERENCES partner_accounts(id) ON DELETE CASCADE,
    invoice_number text NOT NULL,
    period_start timestamptz NOT NULL,
    period_end timestamptz NOT NULL,
    delivery_count int NOT NULL DEFAULT 0,
    rate_per_delivery_kobo bigint NOT NULL,
    amount_kobo bigint NOT NULL,
    currency text NOT NULL DEFAULT 'NGN',
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'paid', 'failed')),
    paid_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (partner_id, period_start)
);

CREATE INDEX IF NOT EXISTS idx_partner_invoices_partner_id ON partner_invoices(partner_id);
CREATE INDEX IF NOT EXISTS idx_partner_invoices_status ON partner_invoices(status);

CREATE TABLE IF NOT EXISTS partner_payment_methods (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id uuid NOT NULL REFERENCES partner_accounts(id) ON DELETE CASCADE,
    provider text NOT NULL DEFAULT 'paystack',
    authorization_code text NOT NULL,
    card_brand text,
    last4 text,
    exp_month text,
    exp_year text,
    is_default boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_partner_payment_methods_partner_id ON partner_payment_methods(partner_id);