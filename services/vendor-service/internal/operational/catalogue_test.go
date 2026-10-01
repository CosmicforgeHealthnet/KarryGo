package operational

import "testing"

func TestRequiredSection46FlagsAreUniqueSafeAndComplete(t *testing.T) {
	want := []string{"VENDOR_SERVICE_ENABLED", "VENDOR_ONBOARDING_WRITES_ENABLED", "VENDOR_CRM_ENABLED", "VENDOR_INVENTORY_ENABLED", "VENDOR_PURCHASING_ENABLED", "VENDOR_RECIPES_ENABLED", "COMMERCE_FOOD_ENABLED", "COMMERCE_SHOP_ENABLED", "COMMERCE_MULTI_VENDOR_ENABLED", "COMMERCE_VARIABLE_WEIGHT_ENABLED", "COMMERCE_RFQ_ENABLED", "VENDOR_OWN_DELIVERY_ENABLED", "COMMERCE_FULFILMENT_WORKERS_ENABLED", "COMMERCE_GLOBAL_KILL_SWITCH"}
	seen := map[string]Flag{}
	for _, flag := range RequiredFlags() {
		if _, duplicate := seen[flag.Name]; duplicate {
			t.Fatalf("duplicate flag %s", flag.Name)
		}
		if flag.Default != "false" {
			t.Fatalf("%s unsafe default %q", flag.Name, flag.Default)
		}
		seen[flag.Name] = flag
	}
	for _, name := range want {
		if _, ok := seen[name]; !ok {
			t.Errorf("missing %s", name)
		}
	}
	guarantee := seen["ORDER_GUARANTEE_ENABLED"]
	if !guarantee.FutureDisabled || guarantee.Equivalent != "no runtime flag by design" {
		t.Fatal("Order Guarantee must remain future-disabled with no runtime switch")
	}
}

func TestDependencyClassificationPreservesAuthorities(t *testing.T) {
	seen := map[string]Dependency{}
	for _, dependency := range Dependencies() {
		seen[dependency.Name] = dependency
	}
	if seen["PostgreSQL"].Requirement != "HARD" {
		t.Fatal("PostgreSQL must be hard")
	}
	if seen["Redis"].Requirement != "OPTIONAL" {
		t.Fatal("Redis must not become authoritative")
	}
	if seen["OTP/QR"].FailureState != "BLOCKED; never insecure fallback" {
		t.Fatal("secure handover must fail closed")
	}
}
