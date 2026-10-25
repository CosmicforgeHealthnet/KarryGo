package admincommand

// WalletSpec contains only operations backed by Wallet's native financial
// state machines. Monetary requests and approvals are separate actions and
// retain Wallet's independent permission and separation-of-duties checks.
func WalletSpec() Spec {
	subjects := kinds("customer-accounts", "payment-intents", "customer-refunds", "provider-payouts", "provider-webhook-events", "commerce-payment-intents", "commerce-allocations", "commerce-operations", "organization-wallets", "provider-accounts", "ledger-accounts", "ledger-balances", "journals", "journal-lines", "holds", "subscriptions", "invoices", "payments", "commission-rules", "earning-allocations", "settlements", "settlement-items", "refunds", "reversals", "provider-allocations", "provider-withdrawals", "job-payments", "reconciliation-runs", "reconciliation-findings", "statements", "financial-audit")
	return Spec{Owner: "payment-wallet-service", Domain: "wallet", Segment: "payment-wallet", Contract: "wallet.admin-command.v1", Capability: "wallet.admin.command.execute", ReadCapability: "wallet.admin.command.read", ApproveCapability: "wallet.admin.command.approve", Actions: map[string]ActionSpec{
		"RESTRICT_ACCOUNT": {SubjectKinds: kinds("organization-wallets"), RequiresApproval: true, VersionOf: VersionSubject, Params: map[string]ParamSpec{"restriction": enum(true, "NEW_PAYMENTS_BLOCKED", "PAYOUTS_BLOCKED", "MANUAL_REVIEW")}},
		"SUSPEND_ACCOUNT":  {SubjectKinds: kinds("organization-wallets"), RequiresApproval: true, VersionOf: VersionSubject, Params: map[string]ParamSpec{"restriction": enum(true, "FRAUD_REVIEW", "COMPLIANCE_REVIEW", "SECURITY_INCIDENT")}},
		"ACTIVATE_ACCOUNT": {SubjectKinds: kinds("organization-wallets"), RequiresApproval: true, VersionOf: VersionSubject},

		"REQUEST_REFUND":              {SubjectKinds: kinds("journals"), RequiresCase: true, VersionOf: VersionCase, Params: map[string]ParamSpec{"amount_minor": {Kind: ParamUint, Required: true}}},
		"APPROVE_REFUND":              {SubjectKinds: kinds("refunds"), RequiresApproval: true, VersionOf: VersionSubject},
		"REQUEST_REVERSAL":            {SubjectKinds: kinds("journals"), RequiresCase: true, VersionOf: VersionCase, Params: map[string]ParamSpec{"amount_minor": {Kind: ParamUint, Required: true}}},
		"APPROVE_REVERSAL":            {SubjectKinds: kinds("reversals"), RequiresApproval: true, VersionOf: VersionSubject},
		"REJECT_REVERSAL":             {SubjectKinds: kinds("reversals"), RequiresApproval: true, VersionOf: VersionSubject},
		"APPROVE_PROVIDER_WITHDRAWAL": {SubjectKinds: kinds("provider-withdrawals"), RequiresApproval: true, VersionOf: VersionSubject},
		"REJECT_PROVIDER_WITHDRAWAL":  {SubjectKinds: kinds("provider-withdrawals"), RequiresApproval: true, VersionOf: VersionSubject},
		"REQUEST_SETTLEMENT":          {SubjectKinds: kinds("provider-accounts"), RequiresCase: true, VersionOf: VersionCase, Params: map[string]ParamSpec{"expected_account_version": {Kind: ParamUint, Required: true}, "limit": {Kind: ParamUint}}},
		"APPROVE_SETTLEMENT":          {SubjectKinds: kinds("settlements"), RequiresApproval: true, VersionOf: VersionSubject},

		"OPEN_FINANCIAL_INVESTIGATION":     {SubjectKinds: subjects, RequiresCase: true, VersionOf: VersionCase, Params: map[string]ParamSpec{"category": enum(true, "PAYMENT_FAILURE", "REFUND_REVIEW", "REVERSAL_REVIEW", "DISPUTE", "CHARGEBACK", "SETTLEMENT_FAILURE", "PAYOUT_FAILURE", "BALANCE_MISMATCH", "FRAUD_SIGNAL", "RECONCILIATION_FINDING"), "severity": enum(true, "LOW", "MEDIUM", "HIGH", "CRITICAL")}},
		"ESCALATE_FINANCIAL_INVESTIGATION": {SubjectKinds: subjects, RequiresCase: true, VersionOf: VersionCase, Params: map[string]ParamSpec{"destination": enum(true, "FINANCE_OPERATIONS", "FRAUD", "COMPLIANCE", "WALLET_APPROVER", "SOURCE_OWNER")}},
		"RECORD_FINANCIAL_FINDING":         {SubjectKinds: subjects, RequiresCase: true, VersionOf: VersionCase, Params: map[string]ParamSpec{"finding": enum(true, "SUBSTANTIATED", "UNSUBSTANTIATED", "INCONCLUSIVE", "RECONCILED"), "evidence_ref": refReq}},
		"CLOSE_FINANCIAL_INVESTIGATION":    {SubjectKinds: subjects, RequiresCase: true, RequiresApproval: true, VersionOf: VersionCase, Params: map[string]ParamSpec{"closure": enum(true, "RESOLVED", "REFERRED_TO_OWNER", "NO_FURTHER_ACTION")}},
	}}
}
