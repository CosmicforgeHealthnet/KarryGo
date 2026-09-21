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

	cfg.Flags.VendorReadinessOrchestrationEnabled = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("readiness plus JIT was rejected: %v", err)
	}
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
