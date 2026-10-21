-- 008_partner_billing_down.sql
-- Reverts 008: removes the charge-loop additions while preserving the 006
-- Billing & Usage invoice table.

DROP TABLE IF EXISTS partner_invoice_charge_attempts;
DROP TABLE IF EXISTS partner_billing_authorizations;
DROP INDEX IF EXISTS idx_partner_invoices_due_at;

UPDATE partner_invoices SET status = 'failed' WHERE status = 'charge_failed';
UPDATE partner_invoices SET status = 'pending' WHERE status = 'charging';

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_constraint
               WHERE conrelid = 'partner_invoices'::regclass
                 AND conname = 'partner_invoices_status_check') THEN
        ALTER TABLE partner_invoices DROP CONSTRAINT partner_invoices_status_check;
    END IF;

    ALTER TABLE partner_invoices
        ADD CONSTRAINT partner_invoices_status_check
        CHECK (status IN ('pending', 'paid', 'failed'));
END $$;

ALTER TABLE partner_invoices
    DROP COLUMN IF EXISTS partner_email,
    DROP COLUMN IF EXISTS due_at,
    DROP COLUMN IF EXISTS wallet_reference,
    DROP COLUMN IF EXISTS failure_reason,
    DROP COLUMN IF EXISTS updated_at;
