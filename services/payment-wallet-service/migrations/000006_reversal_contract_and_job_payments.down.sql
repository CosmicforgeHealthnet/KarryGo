-- Rollback: remove the durable reversal contract and job payment verifications.

DELETE FROM finance_approval_policies WHERE operation = 'REVERSAL_APPROVAL';

DELETE FROM finance_role_permissions
WHERE permission_code IN ('finance.reversal.request', 'finance.reversal.approve',
                          'finance.job_payment.view');

DELETE FROM finance_permissions
WHERE code IN ('finance.reversal.request', 'finance.reversal.approve',
               'finance.job_payment.view');

DROP INDEX IF EXISTS idx_finance_job_payments_org;
DROP INDEX IF EXISTS idx_finance_job_payments_idempotency;
DROP INDEX IF EXISTS idx_finance_job_payments_payment_reference;
DROP INDEX IF EXISTS idx_finance_job_payments_booking;
DROP INDEX IF EXISTS idx_finance_job_payments_trip;
DROP INDEX IF EXISTS idx_finance_job_payments_dispatch;
DROP TABLE IF EXISTS finance_job_payments;

DROP INDEX IF EXISTS idx_finance_reversals_org;
DROP INDEX IF EXISTS idx_finance_reversals_lease;
DROP INDEX IF EXISTS idx_finance_reversals_due;
DROP INDEX IF EXISTS idx_finance_reversals_original;
DROP INDEX IF EXISTS idx_finance_reversals_resulting_journal;
DROP INDEX IF EXISTS idx_finance_reversals_idempotency;
DROP TABLE IF EXISTS finance_reversals;
