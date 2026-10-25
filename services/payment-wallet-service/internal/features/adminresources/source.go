// Package adminresources exposes the payment-wallet service's fixed, minimized
// administrative read model. Every projection is compiled from owner tables;
// bank coordinates, payment-provider tokens, raw webhook payloads, request
// fingerprints and free-form memos are deliberately absent.
package adminresources

import (
	"context"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminowner"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Owner = "payment-wallet-service"

type SourceReader struct{ *adminowner.SQLSource }

func Source() *SourceReader {
	return &SourceReader{adminowner.NewSQLSource(Owner, map[string]adminowner.SQLSpec{
		"customer-accounts":        {Table: "(SELECT a.*,COALESCE((SELECT sum(CASE WHEN e.side=a.normal_balance THEN e.amount_kobo ELSE -e.amount_kobo END) FROM ledger_entries e WHERE e.account_id=a.id),0) AS balance_minor FROM wallet_accounts a)", StateColumn: "status", VersionColumn: "", ReferenceExpression: "t.id::text", BindingColumns: []string{"owner_type", "owner_id", "account_type", "currency"}, Dimensions: map[string]string{"owner_type": "owner_type", "account_type": "account_type", "currency": "currency"}, Counters: map[string]string{"balance_minor": "balance_minor"}, References: map[string]string{"owner": "owner_id"}},
		"payment-intents":          {Table: "payment_intents", StateColumn: "status", VersionColumn: "", ReferenceExpression: "t.id::text", BindingColumns: []string{"source_service", "source_reference", "customer_id", "provider_id", "provider_type", "currency"}, Dimensions: map[string]string{"source_service": "source_service", "provider_type": "provider_type", "payment_method": "payment_method", "currency": "currency"}, Counters: map[string]string{"amount_minor": "amount_kobo", "platform_fee_minor": "platform_fee_kobo"}, References: map[string]string{"source": "source_reference", "customer": "customer_id", "provider": "provider_id"}},
		"customer-refunds":         {Table: "refunds", StateColumn: "status", VersionColumn: "", ReferenceExpression: "t.id::text", BindingColumns: []string{"payment_intent_id", "currency"}, Dimensions: map[string]string{"currency": "currency"}, Counters: map[string]string{"amount_minor": "amount_kobo"}, References: map[string]string{"payment_intent": "payment_intent_id"}},
		"provider-payouts":         {Table: "withdrawals", StateColumn: "status", VersionColumn: "", ReferenceExpression: "t.id::text", BindingColumns: []string{"provider_type", "provider_id", "currency"}, Dimensions: map[string]string{"provider_type": "provider_type", "currency": "currency"}, Counters: map[string]string{"amount_minor": "amount_kobo"}, References: map[string]string{"provider": "provider_id"}},
		"provider-webhook-events":  {Table: "(SELECT w.id,w.event_type,w.reference,w.received_at,w.processed_at,CASE WHEN w.processed_at IS NOT NULL AND NULLIF(w.processing_error,'') IS NULL THEN 'PROCESSED' WHEN NULLIF(w.processing_error,'') IS NOT NULL THEN 'FAILED' ELSE 'PENDING' END AS status FROM paystack_webhook_events w)", StateColumn: "status", VersionColumn: "", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.received_at", UpdatedAtExpression: "COALESCE(t.processed_at,t.received_at)", BindingColumns: []string{"event_type", "reference"}, Dimensions: map[string]string{"event_type": "event_type"}, References: map[string]string{"provider_reference": "reference"}},
		"commerce-payment-intents": {Table: "commerce_payment_intents", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.payment_intent_id::text", BindingColumns: []string{"payment_intent_id", "parent_commerce_reference", "quote_reference", "quote_version"}, Counters: map[string]string{"quote_version": "quote_version", "final_authorized_minor": "final_authorized_minor", "max_authorized_minor": "max_authorized_minor"}, References: map[string]string{"payment_intent": "payment_intent_id", "parent_order": "parent_commerce_reference", "quote": "quote_reference"}},
		"commerce-allocations":     {Table: "commerce_payment_allocations", StateColumn: "status", VersionColumn: "version", BindingColumns: []string{"payment_intent_id", "allocation_reference", "scope", "vendor_order_id", "vendor_id", "branch_id", "assigned_provider_type", "assigned_provider_id", "currency"}, Dimensions: map[string]string{"scope": "scope", "currency": "currency", "delivery_responsibility": "delivery_responsibility"}, Counters: map[string]string{"customer_payable_minor": "customer_payable_minor", "max_authorized_minor": "max_authorized_minor", "merchandise_gross_minor": "merchandise_gross_minor", "vendor_product_discount_minor": "vendor_product_discount_minor", "platform_product_discount_minor": "platform_product_discount_minor", "platform_commission_minor": "platform_commission_minor", "delivery_customer_minor": "delivery_customer_minor", "vendor_delivery_subsidy_minor": "vendor_delivery_subsidy_minor", "platform_delivery_subsidy_minor": "platform_delivery_subsidy_minor", "other_funding_minor": "other_funding_minor", "vendor_merchandise_payable_minor": "vendor_merchandise_payable_minor", "vendor_delivery_payable_minor": "vendor_delivery_payable_minor", "provider_delivery_payable_minor": "provider_delivery_payable_minor", "platform_revenue_minor": "platform_revenue_minor", "vendor_released_minor": "vendor_released_minor", "delivery_released_minor": "delivery_released_minor", "refunded_minor": "refunded_minor"}, References: map[string]string{"payment_intent": "payment_intent_id", "allocation": "allocation_reference", "vendor_order": "vendor_order_id", "vendor": "vendor_id", "branch": "branch_id", "assigned_provider": "assigned_provider_id"}},
		"commerce-operations":      {Table: "commerce_financial_operations", StateColumn: "status", VersionColumn: "", BindingColumns: []string{"payment_intent_id", "allocation_id", "operation_type", "source_reference", "item_reference"}, Dimensions: map[string]string{"operation_type": "operation_type"}, Counters: map[string]string{"amount_minor": "amount_minor"}, References: map[string]string{"payment_intent": "payment_intent_id", "allocation": "allocation_id", "source": "source_reference", "item": "item_reference", "ledger": "ledger_reference"}},
		"organization-wallets":     {Table: "(SELECT w.*,jsonb_array_length(w.restrictions) AS restriction_count FROM finance_wallets w)", StateColumn: "status", VersionColumn: "version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "currency", "ledger_account_id"}, Dimensions: map[string]string{"currency": "currency"}, Counters: map[string]string{"available_minor": "available_minor", "held_minor": "held_minor", "pending_settlement_minor": "pending_settlement_minor", "restriction_count": "restriction_count"}, References: map[string]string{"organization": "organization_id", "ledger_account": "ledger_account_id"}, TimelineTable: "finance_state_history", TimelineParentColumn: "entity_id", TimelineTypeColumn: "to_state"},
		"provider-accounts":        {Table: "(SELECT a.*,jsonb_array_length(a.restrictions) AS restriction_count FROM finance_earning_accounts a)", StateColumn: "status", VersionColumn: "version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "provider_reference", "provider_id", "organization_membership_id", "provider_service_category", "currency"}, Dimensions: map[string]string{"provider_service": "provider_service", "service_category": "provider_service_category", "currency": "currency"}, Counters: map[string]string{"payable_minor": "payable_minor", "held_minor": "held_minor", "settled_minor": "settled_minor", "restriction_count": "restriction_count"}, References: map[string]string{"organization": "organization_id", "organization_wallet": "organization_wallet_id", "provider": "provider_id", "organization_membership": "organization_membership_id", "ledger_account": "ledger_account_id", "payout_destination": "payout_destination_ref"}, TimelineTable: "finance_state_history", TimelineParentColumn: "entity_id", TimelineTypeColumn: "to_state"},
		"ledger-accounts":          {Table: "finance_ledger_accounts", StateColumn: "status", VersionColumn: "version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "owner_type", "owner_ref", "account_code", "currency"}, Dimensions: map[string]string{"owner_type": "owner_type", "account_code": "account_code", "currency": "currency", "normal_balance": "normal_balance"}, References: map[string]string{"organization": "organization_id", "owner": "owner_ref"}},
		"ledger-balances":          {Table: "(SELECT b.*,a.organization_id,a.owner_ref,a.updated_at AS account_updated_at FROM finance_account_balances b JOIN finance_ledger_accounts a ON a.id=b.ledger_account_id)", StateColumn: "", VersionColumn: "version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, ReferenceExpression: "t.ledger_account_id", CreatedAtExpression: "t.account_updated_at", UpdatedAtExpression: "t.updated_at", BindingColumns: []string{"organization_id", "ledger_account_id", "owner_ref", "currency"}, Dimensions: map[string]string{"currency": "currency"}, Counters: map[string]string{"debit_minor": "debit_minor", "credit_minor": "credit_minor"}, References: map[string]string{"organization": "organization_id", "ledger_account": "ledger_account_id"}},
		"journals":                 {Table: "finance_journals", StateColumn: "status", VersionColumn: "", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, CreatedAtExpression: "t.posted_at", UpdatedAtExpression: "t.posted_at", BindingColumns: []string{"organization_id", "journal_type", "source_service", "source_reference", "reverses_journal_id"}, Dimensions: map[string]string{"journal_type": "journal_type", "currency": "currency", "source_service": "source_service"}, References: map[string]string{"organization": "organization_id", "source": "source_reference", "reverses_journal": "reverses_journal_id"}, TimelineTable: "finance_state_history", TimelineParentColumn: "entity_id", TimelineTypeColumn: "to_state"},
		"journal-lines":            {Table: "(SELECT l.*,j.organization_id,j.posted_at FROM finance_journal_lines l JOIN finance_journals j ON j.id=l.journal_id)", StateColumn: "", VersionColumn: "", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.created_at", BindingColumns: []string{"organization_id", "journal_id", "ledger_account_id", "line_no", "side", "currency"}, Dimensions: map[string]string{"side": "side", "currency": "currency"}, Counters: map[string]string{"amount_minor": "amount_minor", "line_number": "line_no"}, References: map[string]string{"organization": "organization_id", "journal": "journal_id", "ledger_account": "ledger_account_id"}},
		"holds":                    {Table: "finance_holds", StateColumn: "state", VersionColumn: "version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "subject_type", "wallet_id", "earning_account_id", "authority_service", "authoritative_reference"}, Dimensions: map[string]string{"currency": "currency", "reason_code": "reason_code", "expires_at": "expires_at"}, Counters: map[string]string{"amount_minor": "amount_minor"}, References: map[string]string{"organization": "organization_id", "wallet": "wallet_id", "ledger_account": "earning_account_id", "release_journal": "release_journal_id"}, TimelineTable: "finance_state_history", TimelineParentColumn: "entity_id", TimelineTypeColumn: "to_state"},
		"subscriptions":            {Table: "finance_subscriptions", StateColumn: "status", VersionColumn: "version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "plan_id", "currency"}, Dimensions: map[string]string{"currency": "currency", "current_period_start": "current_period_start", "current_period_end": "current_period_end", "grace_expires_at": "grace_expires_at"}, References: map[string]string{"organization": "organization_id", "plan": "plan_id", "latest_invoice": "latest_invoice_id"}, TimelineTable: "finance_state_history", TimelineParentColumn: "entity_id", TimelineTypeColumn: "to_state"},
		"invoices":                 {Table: "finance_invoices", StateColumn: "status", VersionColumn: "version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "subscription_id", "number", "currency"}, Dimensions: map[string]string{"currency": "currency", "issued_at": "issued_at", "due_at": "due_at"}, Counters: map[string]string{"subtotal_minor": "subtotal_minor", "tax_minor": "tax_minor", "total_minor": "total_minor", "amount_paid_minor": "amount_paid_minor"}, References: map[string]string{"organization": "organization_id", "subscription": "subscription_id"}, TimelineTable: "finance_state_history", TimelineParentColumn: "entity_id", TimelineTypeColumn: "to_state"},
		"payments":                 {Table: "finance_payments", StateColumn: "status", VersionColumn: "version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "invoice_id", "currency"}, Dimensions: map[string]string{"currency": "currency", "method": "method"}, Counters: map[string]string{"amount_minor": "amount_minor"}, References: map[string]string{"organization": "organization_id", "invoice": "invoice_id", "journal": "journal_id"}, TimelineTable: "finance_state_history", TimelineParentColumn: "entity_id", TimelineTypeColumn: "to_state"},
		"commission-rules":         {Table: "finance_commission_rules", StateColumn: "status", VersionColumn: "", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, UpdatedAtExpression: "COALESCE(t.effective_to,t.created_at)", BindingColumns: []string{"organization_id", "service_type", "currency", "rule_version"}, Dimensions: map[string]string{"service_type": "service_type", "currency": "currency", "effective_from": "effective_from", "effective_to": "effective_to"}, Counters: map[string]string{"rule_version": "rule_version", "commission_bps": "commission_bps", "fleet_fee_bps": "fleet_fee_bps", "provider_share_bps": "provider_share_bps", "organization_share_bps": "organization_share_bps"}, References: map[string]string{"organization": "organization_id"}},
		"earning-allocations":      {Table: "finance_earning_allocations", StateColumn: "status", VersionColumn: "version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "source_service", "job_reference", "rule_id", "provider_reference", "earning_account_id", "wallet_id", "currency"}, Dimensions: map[string]string{"source_service": "source_service", "currency": "currency"}, Counters: map[string]string{"gross_minor": "gross_minor", "commission_minor": "commission_minor", "fleet_fee_minor": "fleet_fee_minor", "provider_earning_minor": "provider_earning_minor", "organization_earning_minor": "organization_earning_minor", "rule_version": "rule_version"}, References: map[string]string{"organization": "organization_id", "job": "job_reference", "rule": "rule_id", "provider": "provider_reference", "earning_account": "earning_account_id", "wallet": "wallet_id", "journal": "journal_id"}, TimelineTable: "finance_state_history", TimelineParentColumn: "entity_id", TimelineTypeColumn: "to_state"},
		"settlements":              {Table: "finance_settlement_batches", StateColumn: "status", VersionColumn: "version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "beneficiary_type", "beneficiary_reference", "earning_account_id", "wallet_id"}, Dimensions: map[string]string{"beneficiary_type": "beneficiary_type", "currency": "currency"}, Counters: map[string]string{"total_minor": "total_minor", "item_count": "item_count"}, References: map[string]string{"organization": "organization_id", "beneficiary": "beneficiary_reference", "earning_account": "earning_account_id", "wallet": "wallet_id"}, TimelineTable: "finance_state_history", TimelineParentColumn: "entity_id", TimelineTypeColumn: "to_state"},
		"settlement-items":         {Table: "finance_settlement_items", StateColumn: "status", VersionColumn: "version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "batch_id", "earning_allocation_id", "currency"}, Dimensions: map[string]string{"currency": "currency"}, Counters: map[string]string{"amount_minor": "amount_minor", "attempts": "attempts"}, References: map[string]string{"organization": "organization_id", "settlement": "batch_id", "allocation": "earning_allocation_id", "journal": "journal_id"}, TimelineTable: "finance_state_history", TimelineParentColumn: "entity_id", TimelineTypeColumn: "to_state"},
		"refunds":                  {Table: "finance_refunds", StateColumn: "status", VersionColumn: "version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "original_journal_id", "currency"}, Dimensions: map[string]string{"currency": "currency", "reason_code": "reason_code"}, Counters: map[string]string{"amount_minor": "amount_minor", "attempts": "attempts"}, References: map[string]string{"organization": "organization_id", "original_journal": "original_journal_id", "resulting_journal": "journal_id"}, TimelineTable: "finance_state_history", TimelineParentColumn: "entity_id", TimelineTypeColumn: "to_state"},
		"reversals":                {Table: "finance_reversals", StateColumn: "status", VersionColumn: "version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "original_journal_id", "currency"}, Dimensions: map[string]string{"currency": "currency", "reason_code": "reason_code", "safe_error_code": "safe_error_code"}, Counters: map[string]string{"amount_minor": "amount_minor", "attempts": "attempts"}, References: map[string]string{"organization": "organization_id", "original_journal": "original_journal_id", "resulting_journal": "resulting_journal_id"}, TimelineTable: "finance_state_history", TimelineParentColumn: "entity_id", TimelineTypeColumn: "to_state"},
		"provider-allocations":     {Table: "finance_provider_allocations", StateColumn: "status", VersionColumn: "version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "organization_wallet_id", "earning_account_id", "provider_id", "organization_membership_id", "business_reference_type", "business_reference_id"}, Dimensions: map[string]string{"allocation_type": "allocation_type", "business_reference_type": "business_reference_type", "currency": "currency", "reason_code": "reason_code"}, Counters: map[string]string{"amount_minor": "amount_minor"}, References: map[string]string{"organization": "organization_id", "organization_wallet": "organization_wallet_id", "earning_account": "earning_account_id", "provider": "provider_id", "organization_membership": "organization_membership_id", "business_reference": "business_reference_id", "journal": "journal_id", "reverses_allocation": "reverses_allocation_id"}, TimelineTable: "finance_state_history", TimelineParentColumn: "entity_id", TimelineTypeColumn: "to_state"},
		"provider-withdrawals":     {Table: "finance_provider_withdrawals", StateColumn: "status", VersionColumn: "version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "earning_account_id", "organization_membership_id", "provider_id", "currency"}, Dimensions: map[string]string{"currency": "currency", "policy_mode": "policy_mode", "safe_error_code": "safe_error_code"}, Counters: map[string]string{"amount_minor": "amount_minor", "attempts": "attempts"}, References: map[string]string{"organization": "organization_id", "earning_account": "earning_account_id", "organization_membership": "organization_membership_id", "provider": "provider_id", "payout_destination": "destination_reference", "hold_journal": "hold_journal_id", "payout_journal": "payout_journal_id"}, TimelineTable: "finance_state_history", TimelineParentColumn: "entity_id", TimelineTypeColumn: "to_state"},
		"job-payments":             {Table: "finance_job_payments", StateColumn: "status", VersionColumn: "version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "source_service", "dispatch_request_reference", "trip_reference", "booking_reference", "payment_reference"}, Dimensions: map[string]string{"currency": "currency", "refund_state": "refund_state", "reversal_state": "reversal_state", "source_service": "source_service"}, Counters: map[string]string{"amount_minor": "amount_minor"}, References: map[string]string{"organization": "organization_id", "dispatch_request": "dispatch_request_reference", "trip": "trip_reference", "booking": "booking_reference", "payment": "payment_reference", "journal": "journal_id", "operation": "operation_reference"}, TimelineTable: "finance_state_history", TimelineParentColumn: "entity_id", TimelineTypeColumn: "to_state"},
		"reconciliation-runs":      {Table: "finance_reconciliation_runs", StateColumn: "status", VersionColumn: "", CreatedAtExpression: "t.started_at", UpdatedAtExpression: "COALESCE(t.completed_at,t.started_at)", BindingColumns: []string{"scope"}, Dimensions: map[string]string{"scope": "scope"}, Counters: map[string]string{"checked_count": "checked_count", "finding_count": "findings_count"}},
		"reconciliation-findings":  {Table: "finance_reconciliation_findings", StateColumn: "", VersionColumn: "", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, UpdatedAtExpression: "COALESCE(t.resolved_at,t.created_at)", BindingColumns: []string{"run_id", "organization_id", "entity_type", "entity_id", "finding_code"}, Dimensions: map[string]string{"entity_type": "entity_type", "finding_code": "finding_code"}, References: map[string]string{"organization": "organization_id", "run": "run_id", "entity": "entity_id"}},
		"statements":               {Table: "finance_statements", StateColumn: "", VersionColumn: "", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, UpdatedAtExpression: "t.created_at", BindingColumns: []string{"organization_id", "statement_type", "currency", "period_start", "period_end", "content_hash"}, Dimensions: map[string]string{"statement_type": "statement_type", "currency": "currency", "period_start": "period_start", "period_end": "period_end"}, Counters: map[string]string{"line_count": "line_count", "total_debit_minor": "total_debit_minor", "total_credit_minor": "total_credit_minor"}, References: map[string]string{"organization": "organization_id"}},
		"financial-audit":          {Table: "finance_audit_records", StateColumn: "", VersionColumn: "", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, CreatedAtExpression: "t.occurred_at", UpdatedAtExpression: "t.occurred_at", BindingColumns: []string{"organization_id", "action", "resource_type", "resource_id"}, Dimensions: map[string]string{"action": "action", "resource_type": "resource_type"}, References: map[string]string{"organization": "organization_id", "resource": "resource_id"}},
		"financial-cases":          {Table: "wallet_admin_cases", ReferenceExpression: "t.case_ref", StateColumn: "state", VersionColumn: "version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "subject_kind", "subject_id"}, Dimensions: map[string]string{"category": "category", "severity": "severity", "subject_kind": "subject_kind"}, References: map[string]string{"organization": "organization_id", "subject": "subject_id", "owner_operation": "owner_operation_ref"}, TimelineTable: "wallet_admin_case_history", TimelineParentColumn: "case_ref", TimelineTypeColumn: "action"},
	})}
}

var permissions = map[string]string{
	"customer-accounts": "finance.wallet.view", "payment-intents": "finance.transaction.view",
	"customer-refunds": "finance.transaction.view", "provider-payouts": "finance.provider_withdrawal.view",
	"provider-webhook-events": "finance.transaction.view", "commerce-payment-intents": "finance.transaction.view",
	"commerce-allocations": "finance.transaction.view", "commerce-operations": "finance.transaction.view",
	"organization-wallets": "finance.wallet.view", "provider-accounts": "finance.provider_withdrawal.view",
	"ledger-accounts": "finance.transaction.view", "ledger-balances": "finance.transaction.view",
	"journals": "finance.transaction.view", "journal-lines": "finance.transaction.view",
	"holds": "finance.transaction.view", "settlements": "finance.transaction.view",
	"subscriptions": "finance.transaction.view", "invoices": "finance.transaction.view",
	"payments": "finance.transaction.view", "commission-rules": "finance.transaction.view",
	"earning-allocations": "finance.transaction.view", "statements": "finance.transaction.view",
	"financial-audit":  "finance.transaction.view",
	"settlement-items": "finance.transaction.view", "refunds": "finance.transaction.view",
	"reversals": "finance.transaction.view", "provider-allocations": "finance.transaction.view",
	"provider-withdrawals": "finance.provider_withdrawal.view", "job-payments": "finance.job_payment.view",
	"reconciliation-runs": "finance.transaction.view", "reconciliation-findings": "finance.transaction.view",
}

// NativeTarget resolves the organization from the authoritative Wallet row.
// Empty organization is a platform-scoped record and requires a platform role.
func NativeTarget(ctx context.Context, tx pgx.Tx, kind, id string) (string, string, error) {
	if kind == "financial-cases" {
		var organization, subjectKind, subjectID string
		if err := tx.QueryRow(ctx, `SELECT organization_id,subject_kind,subject_id FROM wallet_admin_cases WHERE case_ref=$1`, id).Scan(&organization, &subjectKind, &subjectID); err != nil {
			return "", "", err
		}
		resolved, permission, err := NativeTarget(ctx, tx, subjectKind, subjectID)
		if err != nil || resolved != organization {
			return "", "", pgx.ErrNoRows
		}
		return organization, permission, nil
	}
	spec, ok := Source().Specs[kind]
	permission, permitted := permissions[kind]
	if !ok || !permitted {
		return "", "", pgx.ErrNoRows
	}
	ref := spec.ReferenceExpression
	if ref == "" {
		ref = "t.id::text"
	}
	org := spec.OrganizationColumn
	if org == "" {
		org = "''::text"
	} else {
		org = "COALESCE(t." + org + "::text,'')"
	}
	var organization string
	if err := tx.QueryRow(ctx, `SELECT `+org+` FROM `+spec.Table+` t `+spec.JoinClause+` WHERE `+ref+`=$1`, id).Scan(&organization); err != nil {
		return "", "", err
	}
	return organization, permission, nil
}

func requireStaff(ctx context.Context, tx pgx.Tx, staff, organization, permission string, lock bool) error {
	query := `SELECT g.id FROM finance_role_grants g
 JOIN finance_role_permissions rp ON rp.role_id=g.role_id
 WHERE g.subject_type='USER' AND g.subject_id=$1 AND rp.permission_code=$3
 AND (g.organization_id IS NULL OR g.organization_id=NULLIF($2,'')) ORDER BY g.id LIMIT 1`
	if lock {
		query += " FOR SHARE OF g,rp"
	}
	var id string
	return tx.QueryRow(ctx, query, staff, organization, permission).Scan(&id)
}

func (s *SourceReader) AuthorizeStaff(ctx context.Context, tx pgx.Tx, staff, kind, id string) error {
	organization, permission, err := NativeTarget(ctx, tx, kind, id)
	if err != nil {
		return err
	}
	return requireStaff(ctx, tx, staff, organization, permission, false)
}

func RequireNativeStaffForMutation(ctx context.Context, tx pgx.Tx, staff, organization, permission string) error {
	return requireStaff(ctx, tx, staff, organization, permission, true)
}

func Runtime(ctx context.Context, db *pgxpool.Pool) (*adminowner.Handler, error) {
	p := adminowner.WalletProfile()
	return adminowner.FromEnvironment(ctx, db, Source(), p.Definitions, Owner, p.Prefix, p.Path)
}

func (s *SourceReader) Read(ctx context.Context, tx pgx.Tx, kind, id string, detail bool) (adminowner.Resource, error) {
	if kind == "financial-cases" {
		if _, _, err := NativeTarget(ctx, tx, kind, id); err != nil {
			return adminowner.Resource{}, err
		}
	}
	item, err := s.SQLSource.Read(ctx, tx, kind, id, detail)
	if err != nil {
		return item, err
	}
	item.SnapshotETag = ""
	item.SnapshotETag = admincontext.Fingerprint(item)
	return item, nil
}

func (s *SourceReader) Binding(ctx context.Context, tx pgx.Tx, kind, id string) (admincontext.RecordBinding, error) {
	if kind == "financial-cases" {
		if _, _, err := NativeTarget(ctx, tx, kind, id); err != nil {
			return admincontext.RecordBinding{}, err
		}
	}
	return s.SQLSource.Binding(ctx, tx, kind, id)
}

var _ adminowner.StaffAuthorizer = (*SourceReader)(nil)
