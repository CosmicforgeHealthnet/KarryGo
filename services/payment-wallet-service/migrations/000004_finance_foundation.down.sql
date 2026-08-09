-- Rollback of the WP7 financial foundation.
--
-- Drop order follows the reverse of the dependency graph. The original
-- consumer ledger tables are not referenced here and are left untouched.

DROP TRIGGER IF EXISTS finance_audit_append_only ON finance_audit_records;
DROP TRIGGER IF EXISTS finance_journal_lines_append_only ON finance_journal_lines;
DROP TRIGGER IF EXISTS finance_journals_append_only ON finance_journals;
DROP TRIGGER IF EXISTS finance_journals_balanced ON finance_journals;

DROP TABLE IF EXISTS finance_approval_policies;
DROP TABLE IF EXISTS finance_role_grants;
DROP TABLE IF EXISTS finance_role_permissions;
DROP TABLE IF EXISTS finance_roles;
DROP TABLE IF EXISTS finance_permissions;

DROP TABLE IF EXISTS finance_outbox;
DROP TABLE IF EXISTS finance_service_request_nonces;
DROP TABLE IF EXISTS finance_worker_leases;
DROP TABLE IF EXISTS finance_idempotency_keys;
DROP TABLE IF EXISTS finance_audit_records;
DROP TABLE IF EXISTS finance_state_history;

DROP TABLE IF EXISTS finance_statements;
DROP TABLE IF EXISTS finance_reconciliation_findings;
DROP TABLE IF EXISTS finance_reconciliation_runs;

DROP TABLE IF EXISTS finance_refunds;
DROP TABLE IF EXISTS finance_settlement_items;
DROP TABLE IF EXISTS finance_settlement_batches;
DROP TABLE IF EXISTS finance_holds;
DROP TABLE IF EXISTS finance_earning_allocations;
DROP TABLE IF EXISTS finance_commission_rules;

DROP TABLE IF EXISTS finance_payments;
DROP TABLE IF EXISTS finance_invoice_lines;
DROP TABLE IF EXISTS finance_invoices;
DROP TABLE IF EXISTS finance_subscriptions;
DROP TABLE IF EXISTS finance_subscription_plans;

DROP TABLE IF EXISTS finance_earning_accounts;
DROP TABLE IF EXISTS finance_wallets;
DROP TABLE IF EXISTS finance_account_balances;
DROP TABLE IF EXISTS finance_journal_lines;
DROP TABLE IF EXISTS finance_journals;
DROP TABLE IF EXISTS finance_ledger_accounts;

DROP FUNCTION IF EXISTS finance_assert_journal_balanced();
DROP FUNCTION IF EXISTS finance_block_mutation();
