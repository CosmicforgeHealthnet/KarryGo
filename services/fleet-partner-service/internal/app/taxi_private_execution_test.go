package app

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	privatemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Section 10's Taxi private execution. Fleet asks Taxi to create a corporate
// trip and records the reference it gets back; it never writes taxi state.

func taxiConfig(enabled bool) config.Config {
	cfg := config.Config{
		PrivateTaxiExecutionEnabled: enabled,
		HTTPClientTimeout:           0,
		Clients: map[string]config.ClientConfig{
			"driver-taxi-service": {BaseURL: "https://taxi.internal", Secret: []byte("taxi-secret")},
			"driver-hauling-service": {BaseURL: "https://hauling.internal",
				Secret: []byte("hauling-secret")},
		},
		DispatchServiceURL:    "https://dispatch.internal",
		DispatchServiceSecret: []byte("dispatch-secret"),
	}
	return cfg
}

func taxiOperation() privatemodels.ExecutionOperation {
	return privatemodels.ExecutionOperation{
		ID: "op-1", OrganizationID: "org-1", ExecutionService: "TAXI",
		IdempotencyKey: "idem-1",
	}
}

// TestTaxiPrivateExecutionIsDisabledByDefault is mandatory test 8.
//
// The contract existing is not the same as a deployment having certified it.
// With the flag off every call must refuse, and refuse with a reason an
// operator can act on rather than a generic misconfiguration.
func TestTaxiPrivateExecutionIsDisabledByDefault(t *testing.T) {
	adapter := BuildPrivateExecutionAdapter(taxiConfig(false), nil)
	ctx, scope, operation := context.Background(), tenant.Context{OrganizationID: "org-1"}, taxiOperation()

	probes := map[string]func() error{
		"Submit": func() error {
			_, err := adapter.Submit(ctx, scope, operation)
			return err
		},
		"ReadBack": func() error {
			_, err := adapter.ReadBack(ctx, scope, operation)
			return err
		},
		"Cancel": func() error {
			_, err := adapter.Cancel(ctx, scope, operation, "reason")
			return err
		},
		"ReadCancellation": func() error {
			_, err := adapter.ReadCancellation(ctx, scope, operation)
			return err
		},
	}
	for name, probe := range probes {
		err := probe()
		if err == nil {
			t.Fatalf("%s succeeded with the flag off", name)
		}
		if !errors.Is(err, ErrTaxiPrivateExecutionDisabled) {
			t.Fatalf("%s = %v, want the disabled reason rather than a generic failure", name, err)
		}
	}
}

// TestTaxiPrivateExecutionIsReachableWhenEnabled is mandatory test 9's
// composition half.
//
// With the flag on and the client configured, the adapter resolves a Taxi
// client instead of refusing. It does not make a network call here: what is
// under test is that the authority is no longer hardcoded off, which is what
// made every previous Taxi private order unexecutable.
func TestTaxiPrivateExecutionIsReachableWhenEnabled(t *testing.T) {
	adapter := BuildPrivateExecutionAdapter(taxiConfig(true), nil).(*privateExecutionAdapter)

	client, err := adapter.client("TAXI")
	if err != nil {
		t.Fatalf("taxi client with the flag on = %v, want a usable client", err)
	}
	if client == nil || !client.Configured() {
		t.Fatal("the taxi client resolved but reports unconfigured")
	}

	// Dispatch and Haulage are unaffected by the Taxi flag.
	for _, authority := range []string{"DISPATCH", "HAULING"} {
		if _, err = adapter.client(authority); err != nil {
			t.Fatalf("%s client = %v, want it unaffected by the taxi flag", authority, err)
		}
	}
}

// TestKillSwitchOverridesTheTaxiRolloutFlag proves the incident control works
// without anyone having to remember which rollout flag also has to move.
func TestKillSwitchOverridesTheTaxiRolloutFlag(t *testing.T) {
	for _, killed := range []config.Config{
		func() config.Config { c := taxiConfig(true); c.IntegrationsKillSwitch = true; return c }(),
		func() config.Config { c := taxiConfig(true); c.PrivateOperationsKillSwitch = true; return c }(),
	} {
		adapter := BuildPrivateExecutionAdapter(killed, nil).(*privateExecutionAdapter)
		if _, err := adapter.client("TAXI"); !errors.Is(err, ErrTaxiPrivateExecutionDisabled) {
			t.Fatalf("taxi reachable under a kill switch: %v", err)
		}
	}
}

// TestEachAuthorityKeepsItsOwnPath guards the three private-execution paths.
//
// They differ because each service mounts its internal group under its own API
// base. A shared path would 404 against two of the three, and the outbox would
// retry that for hours before dead-lettering it as if Taxi were down.
func TestEachAuthorityKeepsItsOwnPath(t *testing.T) {
	paths := map[string]string{
		"dispatch": dispatchPrivateExecutionPath,
		"taxi":     taxiPrivateExecutionPath,
		"hauling":  haulingPrivateExecutionPath,
	}
	seen := map[string]string{}
	for name, path := range paths {
		if !strings.HasSuffix(path, "/private-executions") {
			t.Fatalf("%s path %q does not name the private execution surface", name, path)
		}
		if other, clash := seen[path]; clash {
			t.Fatalf("%s and %s share the path %q", name, other, path)
		}
		seen[path] = name
	}
	if taxiPrivateExecutionPath != "/api/v1/taxi/internal/private-executions" {
		t.Fatalf("taxi path = %q; it must match where driver-taxi-service mounts the surface",
			taxiPrivateExecutionPath)
	}
}

// TestFleetOwnsNoTaxiCanonicalState is the architecture guard.
//
// Fleet projects a taxi lifecycle; it must never hold one. The two ways that
// could go wrong are importing Taxi's own packages, and writing Taxi's tables
// directly. Both are checked by reading this service's source rather than by
// trusting review, because the mistake is one line and looks harmless.
func TestFleetOwnsNoTaxiCanonicalState(t *testing.T) {
	root := filepath.Join("..", "..", "internal")
	// Tables that belong to driver-taxi-service. Fleet may name a taxi
	// reference; it may not write taxi state.
	forbiddenTables := []string{
		"taxi_bookings", "taxi_providers", "taxi_booking_stops",
		"taxi_fleet_execution_reservations", "taxi_private_execution_mappings",
	}
	fileSet := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, parseErr := parser.ParseFile(fileSet, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, spec := range parsed.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			if strings.Contains(importPath, "taxi-service/internal") {
				t.Errorf("%s imports %s: Fleet must not reach into Taxi's internals",
					path, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// The table check reads raw source, because a table name is a string and
	// the parser above would not see it.
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, readErr := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
		if readErr != nil {
			return readErr
		}
		ast.Inspect(source, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			lowered := strings.ToLower(literal.Value)
			for _, table := range forbiddenTables {
				if strings.Contains(lowered, table) {
					t.Errorf("%s contains a reference to %s: taxi state is driver-taxi-service's to write",
						path, table)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
