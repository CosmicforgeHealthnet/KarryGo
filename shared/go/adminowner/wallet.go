package adminowner

// WalletProfile is the fixed, minimized payment-wallet owner contract. It
// exposes financial state and immutable references, never bank coordinates,
// payment-provider tokens, raw webhook payloads or unrestricted metadata.
func WalletProfile() Profile {
	contract := "wallet.admin-resources.v1"
	def := func(kind, capability string, dimensions, counters, references []string) Definition {
		return Definition{Kind: kind, Capability: capability, SourceContract: contract, Dimensions: dimensions, Counters: counters, References: references}
	}
	return Profile{Owner: "payment-wallet-service", Domain: "wallet", Prefix: "PAYMENT_WALLET", Path: "/internal/payment-wallet/v1/admin", Definitions: map[string]Definition{
		"customer-accounts":        def("customer-accounts", "wallet.customer_accounts.read", []string{"owner_type", "account_type", "currency"}, []string{"balance_minor"}, []string{"owner"}),
		"payment-intents":          def("payment-intents", "wallet.payment_intents.read", []string{"source_service", "provider_type", "payment_method", "currency"}, []string{"amount_minor", "platform_fee_minor"}, []string{"source", "customer", "provider"}),
		"customer-refunds":         def("customer-refunds", "wallet.refunds.read", []string{"currency"}, []string{"amount_minor"}, []string{"payment_intent"}),
		"provider-payouts":         def("provider-payouts", "wallet.provider_withdrawals.read", []string{"provider_type", "currency"}, []string{"amount_minor"}, []string{"provider"}),
		"provider-webhook-events":  def("provider-webhook-events", "wallet.reconciliation.read", []string{"event_type"}, nil, []string{"provider_reference"}),
		"commerce-payment-intents": def("commerce-payment-intents", "wallet.commerce_payments.read", nil, []string{"quote_version", "final_authorized_minor", "max_authorized_minor"}, []string{"payment_intent", "parent_order", "quote"}),
		"commerce-allocations":     def("commerce-allocations", "wallet.commerce_allocations.read", []string{"scope", "currency", "delivery_responsibility"}, []string{"customer_payable_minor", "max_authorized_minor", "merchandise_gross_minor", "vendor_product_discount_minor", "platform_product_discount_minor", "platform_commission_minor", "delivery_customer_minor", "vendor_delivery_subsidy_minor", "platform_delivery_subsidy_minor", "other_funding_minor", "vendor_merchandise_payable_minor", "vendor_delivery_payable_minor", "provider_delivery_payable_minor", "platform_revenue_minor", "vendor_released_minor", "delivery_released_minor", "refunded_minor"}, []string{"payment_intent", "allocation", "vendor_order", "vendor", "branch", "assigned_provider"}),
		"commerce-operations":      def("commerce-operations", "wallet.commerce_operations.read", []string{"operation_type"}, []string{"amount_minor"}, []string{"payment_intent", "allocation", "source", "item", "ledger"}),
		"organization-wallets":     def("organization-wallets", "wallet.accounts.read", []string{"currency"}, []string{"available_minor", "held_minor", "pending_settlement_minor", "restriction_count"}, []string{"organization", "ledger_account"}),
		"provider-accounts":        def("provider-accounts", "wallet.provider_accounts.read", []string{"provider_service", "service_category", "currency"}, []string{"payable_minor", "held_minor", "settled_minor", "restriction_count"}, []string{"organization", "organization_wallet", "provider", "organization_membership", "ledger_account", "payout_destination"}),
		"ledger-accounts":          def("ledger-accounts", "wallet.ledger_accounts.read", []string{"owner_type", "account_code", "currency", "normal_balance"}, nil, []string{"organization", "owner"}),
		"ledger-balances":          def("ledger-balances", "wallet.ledger_balances.read", []string{"currency"}, []string{"debit_minor", "credit_minor"}, []string{"organization", "ledger_account"}),
		"journals":                 def("journals", "wallet.transactions.read", []string{"journal_type", "currency", "source_service"}, nil, []string{"organization", "source", "reverses_journal"}),
		"journal-lines":            def("journal-lines", "wallet.transactions.read", []string{"side", "currency"}, []string{"amount_minor", "line_number"}, []string{"organization", "journal", "ledger_account"}),
		"holds":                    def("holds", "wallet.holds.read", []string{"currency", "reason_code", "expires_at"}, []string{"amount_minor"}, []string{"organization", "wallet", "ledger_account", "release_journal"}),
		"subscriptions":            def("subscriptions", "wallet.subscriptions.read", []string{"currency", "current_period_start", "current_period_end", "grace_expires_at"}, nil, []string{"organization", "plan", "latest_invoice"}),
		"invoices":                 def("invoices", "wallet.invoices.read", []string{"currency", "issued_at", "due_at"}, []string{"subtotal_minor", "tax_minor", "total_minor", "amount_paid_minor"}, []string{"organization", "subscription"}),
		"payments":                 def("payments", "wallet.payments.read", []string{"currency", "method"}, []string{"amount_minor"}, []string{"organization", "invoice", "journal"}),
		"commission-rules":         def("commission-rules", "wallet.commissions.read", []string{"service_type", "currency", "effective_from", "effective_to"}, []string{"rule_version", "commission_bps", "fleet_fee_bps", "provider_share_bps", "organization_share_bps"}, []string{"organization"}),
		"earning-allocations":      def("earning-allocations", "wallet.allocations.read", []string{"source_service", "currency"}, []string{"gross_minor", "commission_minor", "fleet_fee_minor", "provider_earning_minor", "organization_earning_minor", "rule_version"}, []string{"organization", "job", "rule", "provider", "earning_account", "wallet", "journal"}),
		"settlements":              def("settlements", "wallet.settlements.read", []string{"beneficiary_type", "currency"}, []string{"total_minor", "item_count"}, []string{"organization", "beneficiary", "earning_account", "wallet"}),
		"settlement-items":         def("settlement-items", "wallet.settlements.read", []string{"currency"}, []string{"amount_minor", "attempts"}, []string{"organization", "settlement", "allocation", "journal"}),
		"refunds":                  def("refunds", "wallet.refunds.read", []string{"currency", "reason_code"}, []string{"amount_minor", "attempts"}, []string{"organization", "original_journal", "resulting_journal"}),
		"reversals":                def("reversals", "wallet.reversals.read", []string{"currency", "reason_code", "safe_error_code"}, []string{"amount_minor", "attempts"}, []string{"organization", "original_journal", "resulting_journal"}),
		"provider-allocations":     def("provider-allocations", "wallet.provider_allocations.read", []string{"allocation_type", "business_reference_type", "currency", "reason_code"}, []string{"amount_minor"}, []string{"organization", "organization_wallet", "earning_account", "provider", "organization_membership", "business_reference", "journal", "reverses_allocation"}),
		"provider-withdrawals":     def("provider-withdrawals", "wallet.provider_withdrawals.read", []string{"currency", "policy_mode", "safe_error_code"}, []string{"amount_minor", "attempts"}, []string{"organization", "earning_account", "organization_membership", "provider", "payout_destination", "hold_journal", "payout_journal"}),
		"job-payments":             def("job-payments", "wallet.job_payments.read", []string{"currency", "refund_state", "reversal_state", "source_service"}, []string{"amount_minor"}, []string{"organization", "dispatch_request", "trip", "booking", "payment", "journal", "operation"}),
		"reconciliation-runs":      def("reconciliation-runs", "wallet.reconciliation.read", []string{"scope"}, []string{"checked_count", "finding_count"}, nil),
		"reconciliation-findings":  def("reconciliation-findings", "wallet.reconciliation.read", []string{"entity_type", "finding_code"}, nil, []string{"organization", "run", "entity"}),
		"statements":               def("statements", "wallet.statements.read", []string{"statement_type", "currency", "period_start", "period_end"}, []string{"line_count", "total_debit_minor", "total_credit_minor"}, []string{"organization"}),
		"financial-audit":          def("financial-audit", "wallet.audit.read", []string{"action", "resource_type"}, nil, []string{"organization", "resource"}),
		"financial-cases":          {Kind: "financial-cases", Capability: "wallet.cases.read", SourceContract: "wallet.admin-case.v1", Dimensions: []string{"category", "severity", "subject_kind"}, References: []string{"organization", "subject", "owner_operation"}},
	}}
}
