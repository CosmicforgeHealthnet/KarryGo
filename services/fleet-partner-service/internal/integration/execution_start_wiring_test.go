package integration_test

import (
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// TestExecutionSetWiresRealDispatchAdapterWhenConfigured proves the execution
// set the worker is built from uses a REAL dispatch adapter, while sources
// without their own configuration stay fail-closed.
//
// This is the assertion that stops a fail-closed default from quietly becoming
// production behaviour.
func TestExecutionSetWiresRealDispatchAdapterWhenConfigured(t *testing.T) {
	set := platformclients.BuildExecutionSet(platformclients.Options{
		ServiceName: "driver-dispatch-delivery-service",
		BaseURL:     "http://dispatch:8100",
		Secret:      []byte("a-configured-dispatch-secret"),
		Timeout:     5 * time.Second,
		Metrics:     observability.NewMetrics(),
	})

	if !set.AnyConfigured() {
		t.Fatal("a configured dispatch client did not produce a usable execution set")
	}

	dispatch, ok := set.For(platformclients.CategoryDispatch)
	if !ok {
		t.Fatal("no dispatch contract in the execution set")
	}
	if !dispatch.Configured() {
		t.Fatal("the dispatch adapter reports unconfigured despite full configuration")
	}
	if _, isFallback := dispatch.(platformclients.UnimplementedExecution); isFallback {
		t.Fatal("the worker would run the fail-closed adapter for dispatch")
	}

	// Taxi and Hauling were not configured in this call and stay fail-closed.
	for _, category := range []string{
		platformclients.CategoryTaxi, platformclients.CategoryHauling,
	} {
		contract, ok := set.For(category)
		if !ok {
			t.Fatalf("no contract for %s", category)
		}
		if contract.Configured() {
			t.Fatalf("%s reports configured; it must fail closed", category)
		}
		if _, isFallback := contract.(platformclients.UnimplementedExecution); !isFallback {
			t.Fatalf("%s is not the fail-closed adapter", category)
		}
	}
}

func TestExecutionSetWiresRealHaulingAdapterWhenConfigured(t *testing.T) {
	set := platformclients.BuildExecutionSet(platformclients.Options{}, platformclients.Options{
		ServiceName: "fleet-partner-service",
		BaseURL:     "http://hauling:8104",
		Secret:      []byte("a-configured-hauling-secret"),
		Timeout:     5 * time.Second,
		Metrics:     observability.NewMetrics(),
	})
	hauling, ok := set.For(platformclients.CategoryHauling)
	if !ok || !hauling.Configured() {
		t.Fatal("complete Hauling configuration did not produce a live execution adapter")
	}
	if _, isFallback := hauling.(platformclients.UnimplementedExecution); isFallback {
		t.Fatal("Hauling execution is still wired to the fail-closed placeholder")
	}
	dispatch, _ := set.For(platformclients.CategoryDispatch)
	if dispatch.Configured() {
		t.Fatal("configuring Hauling must not implicitly enable Dispatch")
	}
	taxi, _ := set.For(platformclients.CategoryTaxi)
	if taxi.Configured() {
		t.Fatal("Taxi must remain fail closed")
	}
}

// TestExecutionSetFailsClosedWithoutDispatchSecret proves the reverse.
func TestExecutionSetFailsClosedWithoutDispatchSecret(t *testing.T) {
	set := platformclients.BuildExecutionSet(platformclients.Options{
		ServiceName: "driver-dispatch-delivery-service",
		BaseURL:     "http://dispatch:8100",
		// No secret.
		Timeout: 5 * time.Second,
	})

	dispatch, _ := set.For(platformclients.CategoryDispatch)
	if dispatch.Configured() {
		t.Fatal("a missing dispatch secret produced a live adapter")
	}
	if set.AnyConfigured() {
		t.Fatal("an unconfigured set reported usable; the worker would start and only fail")
	}
}
