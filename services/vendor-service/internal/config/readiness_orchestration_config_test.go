package config

import (
	"strings"
	"testing"
)

// Section 11's rollout stages depend on each other in one direction. Each
// refusal below prevents a combination that would look enabled and behave
// wrongly rather than failing loudly.

// section11Config is a configuration that is valid apart from the Section 11
// flags, so each test fails for exactly the reason it is about.
func section11Config() Config {
	cfg := withAuthSecrets(Load())
	cfg.Flags.VendorServiceEnabled = true
	// Just-in-time commitment defers the dispatch that happens during Section
	// 17E order activation, so orders (and the quote lock they are cut from)
	// are genuine prerequisites rather than test scaffolding. They are set
	// here so a test about readiness and orchestration fails on its own rule
	// rather than on that one.
	cfg.Flags.CommerceFoodEnabled = true
	// Vendor-owned delivery satisfies fulfilment planning's own prerequisite
	// WITHOUT supplying a platform authority, so the tests below that assert
	// "there is nobody to ask" still have nobody to ask.
	cfg.Flags.VendorOwnDeliveryEnabled = true
	cfg.Flags.CommerceCheckoutEnabled = true
	cfg.Flags.CommerceFulfilmentPlanningEnabled = true
	cfg.Flags.CommerceQuoteLockEnabled = true
	cfg.Flags.CommerceOrdersEnabled = true
	return cfg
}

// withDispatch supplies the credentials an enabled authority integration
// requires, so a test about Section 11's own rules does not fail on an
// unrelated one.
func withDispatch(cfg Config) Config {
	cfg.Flags.DispatchCommerceIntegrationEnabled = true
	cfg.DispatchURL = "https://dispatch.internal"
	cfg.DispatchSecret = "dispatch-secret"
	return cfg
}

func expectProblem(t *testing.T, cfg Config, fragment string) {
	t.Helper()
	err := cfg.Validate()
	if err == nil {
		t.Fatalf("configuration was accepted; expected a refusal mentioning %q", fragment)
	}
	if !strings.Contains(err.Error(), fragment) {
		t.Fatalf("error = %v, want it to mention %q", err, fragment)
	}
}

// TestJITRequiresReadiness pins the first dependency.
//
// Deferring commitment against no readiness prediction is not just-in-time; it
// is late. The order would sit until some other trigger fired, and the symptom
// would be slow deliveries rather than a configuration error.
func TestJITRequiresReadiness(t *testing.T) {
	cfg := section11Config()
	cfg.Flags.VendorJITPickupEnabled = true
	expectProblem(t, cfg, "VENDOR_READINESS_ORCHESTRATION_ENABLED must be true")

	// Readiness alone is not enough any more: just-in-time commitment now
	// actually commits, so it also needs an authority it can commit TO.
	cfg.Flags.VendorReadinessOrchestrationEnabled = true
	expectProblem(t, cfg, "requires at least one configured logistics authority")

	withAuthority := withDispatch(cfg)
	if err := withAuthority.Validate(); err != nil {
		t.Fatalf("readiness plus JIT plus one authority was rejected: %v", err)
	}
}

// TestJITCannotRunWithoutSomewhereToCommit is the fail-closed rule.
//
// A deployment that defers every mission to an authority it cannot reach plans
// every order and dispatches none. Nothing errors, nothing retries, and the
// orchestrations simply queue at COMMIT_DUE -- which is why this has to be
// refused at startup rather than discovered from a delivery backlog.
func TestJITCannotRunWithoutSomewhereToCommit(t *testing.T) {
	// Enabled but uncredentialled is the interesting case: the flag alone
	// looks configured.
	cfg := section11Config()
	cfg.Flags.VendorReadinessOrchestrationEnabled = true
	cfg.Flags.VendorJITPickupEnabled = true
	cfg.Flags.DispatchCommerceIntegrationEnabled = true
	expectProblem(t, cfg, "requires at least one configured logistics authority")
}

// TestJITRequiresCommerceOrders pins the prerequisite that makes the whole
// integration meaningful: just-in-time commitment replaces the dispatch that
// happens during order activation, so with orders off there is nothing to
// defer.
func TestJITRequiresCommerceOrders(t *testing.T) {
	cfg := withDispatch(section11Config())
	cfg.Flags.CommerceOrdersEnabled = false
	cfg.Flags.VendorReadinessOrchestrationEnabled = true
	cfg.Flags.VendorJITPickupEnabled = true
	expectProblem(t, cfg, "COMMERCE_ORDERS_ENABLED must be true")
}

// TestAutomaticSelectionRequiresJIT pins the second.
//
// Choosing an authority at order time uses supply that will have changed by the
// time anyone collects. The selection would be defensible and stale.
func TestAutomaticSelectionRequiresJIT(t *testing.T) {
	cfg := section11Config()
	cfg.Flags.VendorReadinessOrchestrationEnabled = true
	cfg.Flags.VendorAutoAuthoritySelectionEnabled = true
	cfg = withDispatch(cfg)
	expectProblem(t, cfg, "VENDOR_JIT_PICKUP_ENABLED must be true")
}

// TestAutomaticSelectionNeedsSomebodyToAsk is the rule that catches a
// deployment which enabled the decision and none of its inputs.
//
// With no authority integration there is nothing to choose between. Every order
// would fail selection, and it would look like a planning bug rather than a
// missing switch.
func TestAutomaticSelectionNeedsSomebodyToAsk(t *testing.T) {
	cfg := section11Config()
	cfg.Flags.VendorReadinessOrchestrationEnabled = true
	cfg.Flags.VendorJITPickupEnabled = true
	cfg.Flags.VendorAutoAuthoritySelectionEnabled = true
	expectProblem(t, cfg, "requires at least one of")

	// Any single authority is enough to make the decision meaningful.
	candidate := withDispatch(section11Config())
	candidate.Flags.VendorReadinessOrchestrationEnabled = true
	candidate.Flags.VendorJITPickupEnabled = true
	candidate.Flags.VendorAutoAuthoritySelectionEnabled = true
	if err := candidate.Validate(); err != nil {
		t.Fatalf("a single configured authority was rejected: %v", err)
	}
}

// TestSection11DefaultsToOff keeps the rollout gate honest: the code existing
// is not a deployment having certified it.
func TestSection11DefaultsToOff(t *testing.T) {
	cfg := Load()
	if cfg.Flags.VendorReadinessOrchestrationEnabled ||
		cfg.Flags.VendorJITPickupEnabled ||
		cfg.Flags.VendorAutoAuthoritySelectionEnabled {
		t.Fatal("Section 11 flags must default to off")
	}
}

// TestReadinessAloneIsAValidStage proves the observe-then-trust stage is
// reachable: readiness can be made authoritative without changing fulfilment.
func TestReadinessAloneIsAValidStage(t *testing.T) {
	cfg := section11Config()
	cfg.Flags.VendorReadinessOrchestrationEnabled = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("readiness on its own was rejected: %v", err)
	}
}
