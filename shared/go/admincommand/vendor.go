package admincommand

// VendorSpec permits only named native operations; no state or quantity setter.
func VendorSpec() Spec {
	subjects := kinds("merchants", "branches", "food-items", "shop-listings", "stock-balances", "production-jobs", "vendor-orders", "production-exceptions", "capacity-reservations")
	return Spec{Owner: "vendor-service", Domain: "vendor", Contract: "vendor.admin-command.v1", Capability: "vendor.admin.command.execute", ReadCapability: "vendor.admin.command.read", ApproveCapability: "vendor.admin.command.approve", Actions: map[string]ActionSpec{
		"PROCESS_APPROVED_SUBSTITUTION": {SubjectKinds: kinds("substitutions"), RequiresApproval: true, VersionOf: VersionSubject},
		"RESCHEDULE_CAPACITY":           {SubjectKinds: kinds("capacity-reservations"), RequiresApproval: true, VersionOf: VersionSubject, Params: map[string]ParamSpec{"window_start_unix": {Kind: ParamUint, Required: true}, "window_end_unix": {Kind: ParamUint, Required: true}}},
		"BLOCK_PRODUCTION":              {SubjectKinds: kinds("production-jobs"), VersionOf: VersionSubject},
		"RESUME_PRODUCTION":             {SubjectKinds: kinds("production-jobs"), VersionOf: VersionSubject, RequiresApproval: true},
		"REPORT_PRODUCTION_DELAY":       {SubjectKinds: kinds("production-jobs"), VersionOf: VersionSubject, Params: map[string]ParamSpec{"reason": enum(true, "STAFF_SHORTAGE", "EQUIPMENT_FAILURE", "INGREDIENT_SHORTAGE", "QUALITY_REWORK", "CAPACITY_CONFLICT", "LOGISTICS_CONSTRAINT", "OTHER"), "window_start_unix": {Kind: ParamUint, Required: true}, "window_end_unix": {Kind: ParamUint, Required: true}}},

		"PAUSE_MERCHANT":         {SubjectKinds: kinds("merchants"), RequiresApproval: true, VersionOf: VersionSubject},
		"REOPEN_MERCHANT":        {SubjectKinds: kinds("merchants"), RequiresApproval: true, VersionOf: VersionSubject},
		"PAUSE_BRANCH":           {SubjectKinds: kinds("branches"), RequiresApproval: true, VersionOf: VersionSubject},
		"REOPEN_BRANCH":          {SubjectKinds: kinds("branches"), RequiresApproval: true, VersionOf: VersionSubject},
		"TAKE_DOWN_LISTING":      {SubjectKinds: kinds("food-items", "shop-listings"), VersionOf: VersionSubject},
		"RESTORE_LISTING":        {SubjectKinds: kinds("food-items", "shop-listings"), RequiresApproval: true, VersionOf: VersionSubject},
		"OPEN_INVESTIGATION":     {SubjectKinds: subjects, RequiresCase: true, VersionOf: VersionCase, Params: map[string]ParamSpec{"category": enum(true, "MERCHANT_RESTRICTION_REVIEW", "BRANCH_ACTIVATION_REVIEW", "PRODUCT_MODERATION", "INVENTORY_DISCREPANCY", "STOCK_EXCEPTION", "PRODUCTION_INCIDENT", "KITCHEN_DELAY", "CAPACITY_EXCEPTION", "FULFILMENT_FAILURE", "DELIVERY_INTERRUPTION", "RETURN_REFUND_REVIEW"), "severity": enum(true, "LOW", "MEDIUM", "HIGH", "CRITICAL")}},
		"ESCALATE_INVESTIGATION": {SubjectKinds: subjects, RequiresCase: true, VersionOf: VersionCase, Params: map[string]ParamSpec{"destination": enum(true, "VENDOR_OPERATIONS", "VERIFICATION", "WALLET", "LOGISTICS_OWNER", "SUPPORT")}},
		"RECORD_FINDING":         {SubjectKinds: subjects, RequiresCase: true, VersionOf: VersionCase, Params: map[string]ParamSpec{"finding": enum(true, "SUBSTANTIATED", "UNSUBSTANTIATED", "INCONCLUSIVE"), "evidence_ref": refReq}},
		"RECORD_SELLER_APPEAL":   {SubjectKinds: subjects, RequiresCase: true, VersionOf: VersionCase, Params: map[string]ParamSpec{"appeal_reference": refReq}},
		"DECIDE_SELLER_APPEAL":   {SubjectKinds: subjects, RequiresCase: true, RequiresApproval: true, VersionOf: VersionCase, Params: map[string]ParamSpec{"decision": enum(true, "UPHELD", "OVERTURNED", "REFERRED_TO_OWNER"), "evidence_ref": refReq}},
		"CLOSE_INVESTIGATION":    {SubjectKinds: subjects, RequiresCase: true, RequiresApproval: true, VersionOf: VersionCase, Params: map[string]ParamSpec{"closure": enum(true, "RESOLVED", "REFERRED_TO_OWNER", "NO_FURTHER_ACTION")}},
	}}
}
