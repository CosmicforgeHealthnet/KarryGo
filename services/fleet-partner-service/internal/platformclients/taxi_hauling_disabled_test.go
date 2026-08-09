package platformclients

import (
	"context"
	"errors"
	"testing"
)

// WP11: taxi and hauling must stay disabled behind fail-closed adapters until
// their authoritative services actually expose the marketplace contracts.
//
// The existing wiring test proves BuildExecutionSet hands back the fail-closed
// execution adapter for both categories. These tests cover what it does not:
// the PROVIDER contract set, and whether every individual method actually
// refuses. Configured() returning false is a claim about an adapter; it is not
// the same as each method declining to answer, and it is the methods that get
// called.

// fullyConfiguredOptions is a complete, valid configuration.
//
// The point of configuring everything is that taxi and hauling must fail
// closed even here — their disablement must not depend on someone forgetting
// to set a secret.
func fullyConfiguredOptions() Options {
	return Options{
		ServiceName: "fleet-partner-service",
		BaseURL:     "https://dispatch.internal.example",
		Secret:      []byte("configured-dispatch-secret"),
	}
}

// TestTaxiAndHaulingExecutionMethodsAllFailClosed calls every method on the
// execution contract and asserts each one refuses.
func TestTaxiAndHaulingExecutionMethodsAllFailClosed(t *testing.T) {
	set := BuildExecutionSet(fullyConfiguredOptions())
	ctx := context.Background()
	meta := CallMeta{IdempotencyKey: "wp11-probe"}

	for _, category := range []string{CategoryTaxi, CategoryHauling} {
		contract, ok := set.For(category)
		if !ok {
			t.Fatalf("no contract for %s", category)
		}
		if contract.Configured() {
			t.Fatalf("%s reports configured under full configuration", category)
		}

		// Each entry is one way a caller could try to make taxi or hauling do
		// real work. Every one must return an error.
		probes := map[string]func() error{
			"ReserveForOrganization": func() error {
				_, err := contract.ReserveForOrganization(ctx, meta, ReserveRequest{})
				return err
			},
			"ReadReservation": func() error {
				_, err := contract.ReadReservation(ctx, meta, "req", "idem")
				return err
			},
			"ReleaseReservation": func() error {
				return contract.ReleaseReservation(ctx, meta, "req", "org", "reason", "idem")
			},
			"ValidateProviderEligibility": func() error {
				_, err := contract.ValidateProviderEligibility(ctx, meta, "req", "prv", "veh")
				return err
			},
			"ConfirmProviderAndCreateExecution": func() error {
				_, err := contract.ConfirmProviderAndCreateExecution(ctx, meta, ConfirmProviderRequest{})
				return err
			},
			"ReadExecutionOutcome": func() error {
				_, err := contract.ReadExecutionOutcome(ctx, meta, "req", "idem")
				return err
			},
			"ReadRequest": func() error {
				_, err := contract.ReadRequest(ctx, meta, "req")
				return err
			},
			"ReadTrip": func() error {
				_, err := contract.ReadTrip(ctx, meta, "trip")
				return err
			},
			"CancelExecution": func() error {
				return contract.CancelExecution(ctx, meta, "trip", "reason", "idem")
			},
		}

		for name, probe := range probes {
			if err := probe(); err == nil {
				t.Fatalf("%s.%s succeeded; a disabled integration must refuse, "+
					"not return a usable zero value", category, name)
			}
		}
	}
}

// TestTaxiAndHaulingProviderMethodsAllFailClosed does the same for the
// provider contract, which the execution-set test never touches.
//
// This is the set that would create provider identities and organization
// associations. A silent zero value here would let fleet believe it had linked
// a taxi or hauling provider that no authoritative service has ever heard of.
func TestTaxiAndHaulingProviderMethodsAllFailClosed(t *testing.T) {
	set := NewUnconfiguredContracts().Providers
	ctx := context.Background()
	meta := CallMeta{IdempotencyKey: "wp11-provider-probe"}

	for _, category := range []string{CategoryTaxi, CategoryHauling} {
		contract, ok := set[category]
		if !ok {
			t.Fatalf("no provider contract for %s", category)
		}
		if contract.Configured() {
			t.Fatalf("%s provider contract reports configured", category)
		}
		if _, isFallback := contract.(UnconfiguredProvider); !isFallback {
			t.Fatalf("%s provider contract is not the fail-closed adapter: %T",
				category, contract)
		}

		probes := map[string]func() error{
			"Resolve": func() error {
				_, err := contract.Resolve(ctx, meta, ProviderResolveRequest{})
				return err
			},
			"ProvisionSponsored": func() error {
				_, err := contract.ProvisionSponsored(ctx, meta, ProviderProvisionRequest{})
				return err
			},
			"ReadProvider": func() error {
				_, err := contract.ReadProvider(ctx, meta, "prv")
				return err
			},
			"CreateAssociation": func() error {
				_, err := contract.CreateAssociation(ctx, meta, ProviderAssociationRequest{})
				return err
			},
			"ReadAssociation": func() error {
				_, err := contract.ReadAssociation(ctx, meta, "asc")
				return err
			},
			"RevokeOrganizationEligibility": func() error {
				return contract.RevokeOrganizationEligibility(ctx, meta, "prv", "org", "reason")
			},
		}

		for name, probe := range probes {
			if err := probe(); err == nil {
				t.Fatalf("%s provider.%s succeeded; a disabled integration must "+
					"refuse rather than fabricate a provider identity", category, name)
			}
		}
	}
}

// TestDisabledIntegrationsReportUnknownNotFailure pins how a refusal is
// classified, which decides what callers do with it.
//
// A disabled integration is an UNKNOWN outcome, not a definite negative.
// Reporting "not eligible" would be a factual claim about a provider that no
// service has been asked about — a caller could reasonably act on that, and
// acting on an invented negative is exactly what fail-closed is meant to stop.
func TestDisabledIntegrationsReportUnknownNotFailure(t *testing.T) {
	set := NewUnconfiguredContracts().Providers
	contract := set[CategoryTaxi]

	_, err := contract.ReadProvider(context.Background(), CallMeta{}, "prv")
	if err == nil {
		t.Fatal("taxi ReadProvider succeeded")
	}

	var contractErr *Error
	if !errors.As(err, &contractErr) {
		t.Fatalf("error type = %T, want *platformclients.Error carrying an outcome", err)
	}
	if contractErr.Outcome != OutcomeUnknown {
		t.Fatalf("outcome = %q, want %q — a disabled integration knows nothing "+
			"about the provider and must not imply otherwise",
			contractErr.Outcome, OutcomeUnknown)
	}
}
