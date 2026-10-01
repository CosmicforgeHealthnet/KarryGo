// Package operational is the deterministic, secret-free Section 46-48
// operational contract. It describes configuration and readiness without
// pretending that repository code proves deployment infrastructure.
package operational

type Flag struct {
	Name, Equivalent, Default, Dependency, FailureMode string
	FutureDisabled                                     bool
}

func RequiredFlags() []Flag {
	return []Flag{
		{"VENDOR_SERVICE_ENABLED", "", "false", "PostgreSQL", "routes remain unregistered or writes refused", false},
		{"VENDOR_ONBOARDING_WRITES_ENABLED", "", "false", "VENDOR_SERVICE_ENABLED; Verification for external submission", "onboarding reads remain available", false},
		{"VENDOR_CRM_ENABLED", "", "false", "VENDOR_SERVICE_ENABLED", "CRM routes absent", false},
		{"VENDOR_INVENTORY_ENABLED", "", "false", "VENDOR_SERVICE_ENABLED", "inventory mutations unavailable", false},
		{"VENDOR_PURCHASING_ENABLED", "", "false", "VENDOR_SERVICE_ENABLED", "purchasing unavailable", false},
		{"VENDOR_RECIPES_ENABLED", "", "false", "VENDOR_SERVICE_ENABLED", "recipe routes absent", false},
		{"COMMERCE_FOOD_ENABLED", "", "false", "VENDOR_SERVICE_ENABLED", "Food routes absent", false},
		{"COMMERCE_SHOP_ENABLED", "", "false", "VENDOR_SERVICE_ENABLED", "Shop routes absent", false},
		{"COMMERCE_MULTI_VENDOR_ENABLED", "", "false", "checkout chain", "multi-vendor planning unavailable", false},
		{"COMMERCE_VARIABLE_WEIGHT_ENABLED", "", "false", "checkout and inventory", "variable-weight checkout unavailable", false},
		{"COMMERCE_RFQ_ENABLED", "COMMERCE_BULK_RFQ_ENABLED", "false", "order and payment chain", "RFQ routes/workers absent", false},
		{"VENDOR_OWN_DELIVERY_ENABLED", "", "false", "secure handover when required", "vendor-owned delivery unavailable", false},
		{"COMMERCE_FULFILMENT_WORKERS_ENABLED", "", "false", "COMMERCE_FULFILMENT_PLANNING_ENABLED", "no new fulfilment worker claims", false},
		{"COMMERCE_GLOBAL_KILL_SWITCH", "", "false", "none", "new commerce mutations and all new worker claims stop; reads and durable state remain", false},
		{"ORDER_GUARANTEE_ENABLED", "no runtime flag by design", "false", "future authority contract", "no route, worker, event, or Wallet compensation", true},
	}
}

type Dependency struct {
	Name, Requirement, FailureState, Authority string
}

func Dependencies() []Dependency {
	return []Dependency{
		{"PostgreSQL", "HARD", "BLOCKED", "Vendor authoritative store"},
		{"Redis", "OPTIONAL", "DEGRADED", "non-authoritative cache/coordination"},
		{"Wallet", "PATH_HARD", "BLOCKED for paid commerce", "Payment Wallet"},
		{"Dispatch", "PATH_HARD", "BLOCKED for selected Dispatch path", "Dispatch"},
		{"Hauling", "PATH_HARD", "BLOCKED for selected Hauling path", "Hauling"},
		{"Taxi Goods", "DISABLED_UNTIL_ENABLED", "DISABLED/BLOCKED when enabled and unhealthy", "Taxi"},
		{"Verification", "PATH_HARD", "BLOCKED for activation/reverification", "Verification"},
		{"Notification", "OPTIONAL", "DEGRADED", "Notification"},
		{"Analytics", "OPTIONAL", "DEGRADED", "projection only"},
		{"OTP/QR", "PATH_HARD", "BLOCKED; never insecure fallback", "existing secure handover authority"},
	}
}
