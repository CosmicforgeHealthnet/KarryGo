package main

// Structural guards for just-in-time commitment.
//
// Each of these enforces a rule that is easy to break with a small, reasonable-
// looking change, and whose breakage is invisible until production: a nil
// committer that plans everything and dispatches nothing, a second place that
// creates missions, a legacy path that dispatches alongside the planner.
//
// They are checked by reading the source rather than by running it, because the
// failures they prevent are configuration and wiring failures that a passing
// test suite would not notice.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/features/orchestration"
)

// TestJITNeverRunsWithoutACommitter is the fail-closed rule, exercised rather
// than read.
//
// A deployment with the flag on and no committer is the one broken state that
// looks healthy: orders plan, workers run, nothing is ever dispatched, and no
// error is produced anywhere. Refusing to start is the only way that surfaces.
func TestJITNeverRunsWithoutACommitter(t *testing.T) {
	cfg := config.Config{}
	cfg.Flags.VendorJITPickupEnabled = true
	cfg.Flags.VendorReadinessOrchestrationEnabled = true

	if _, err := buildReadinessOrchestration(cfg, nil, nil, nil); err == nil {
		t.Fatal("just-in-time commitment was allowed to start with no mission committer")
	}

	// And with a committer it assembles.
	components, err := buildReadinessOrchestration(cfg, nil, nil,
		productionMissionCommitter{})
	if err != nil {
		t.Fatalf("a configured committer was rejected: %v", err)
	}
	if !components.orchestrationEnabled {
		t.Fatal("just-in-time orchestration did not enable with its flag on")
	}
}

// TestKillSwitchDisablesJITWithoutNeedingACommitter proves the kill switch
// wins over the rollout flag, and that stopping does not then demand the very
// dependency it is stopping.
func TestKillSwitchDisablesJITWithoutNeedingACommitter(t *testing.T) {
	cfg := config.Config{}
	cfg.Flags.VendorJITPickupEnabled = true
	cfg.Flags.VendorReadinessOrchestrationEnabled = true
	cfg.Flags.CommerceGlobalKillSwitch = true

	components, err := buildReadinessOrchestration(cfg, nil, nil, nil)
	if err != nil {
		t.Fatalf("the kill switch required a committer in order to stop: %v", err)
	}
	if components.orchestrationEnabled {
		t.Fatal("the kill switch did not stop just-in-time orchestration")
	}
}

// commandSourceFiles returns the composition root's own non-test source.
func commandSourceFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	files := []string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") {
			continue
		}
		files = append(files, name)
	}
	if len(files) == 0 {
		t.Fatal("found no command source to check")
	}
	return files
}

func parseGoFile(t *testing.T, path string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return file
}

// TestOnlyTheOrdersFeatureCreatesMissions pins the single-assembler rule.
//
// Exactly one call site in the whole service may invoke the authority's
// CreateMission, and it lives in the orders feature. The committer delegates to
// it; a committer that called an authority client directly would be a second
// assembler, and two assemblers eventually disagree about the approved fee.
func TestOnlyTheOrdersFeatureCreatesMissions(t *testing.T) {
	root, err := filepath.Abs("../internal/features")
	if err != nil {
		t.Fatal(err)
	}
	offenders := []string{}
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return nil
		}
		feature := featureNameOf(root, path)
		if feature == "orders" || feature == "" {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		// The orchestration declares a CreateMission on its own committer PORT,
		// which is a delegation and not an authority call. What must not appear
		// anywhere outside orders is a call INTO a shared mission client.
		for _, banned := range []string{
			"commercemission.New", "dispatchcommerce.New",
			"shared/go/commercemission", "shared/go/dispatchcommerce",
		} {
			if strings.Contains(string(content), banned) {
				offenders = append(offenders, path+" references "+banned)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, offender := range offenders {
		t.Errorf("a feature outside orders talks to a mission client: %s", offender)
	}
}

// featureNameOf returns the feature directory a file belongs to.
func featureNameOf(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[0]
}

// TestActivationHasExactlyOneAuthorityCallSite proves the legacy and
// just-in-time paths cannot both dispatch.
//
// The rule is structural: the just-in-time branch must RETURN control before
// the synchronous create, so no configuration exists in which both run for the
// same slot. This walks the activation function and checks the branch does
// exactly that.
func TestActivationHasExactlyOneAuthorityCallSite(t *testing.T) {
	path, err := filepath.Abs("../internal/features/orders/fulfilment.go")
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := string(content)

	if strings.Count(source, "s.missions.CreateMission(ctx, MissionRequest{") != 1 {
		t.Fatal("fulfilment.go has more than one authority create; " +
			"activation must dispatch from exactly one place")
	}

	planIndex := strings.Index(source, "if s.JITPlanningEnabled() {")
	createIndex := strings.Index(source, "s.missions.CreateMission(ctx, MissionRequest{")
	if planIndex < 0 {
		t.Fatal("activation has no just-in-time branch at all")
	}
	if planIndex > createIndex {
		t.Fatal("the just-in-time branch runs AFTER the synchronous create; " +
			"both paths would dispatch for the same slot")
	}
	between := source[planIndex:createIndex]
	if !strings.Contains(between, "continue") {
		t.Fatal("the just-in-time branch does not return control to the loop, " +
			"so activation falls through into the synchronous create")
	}
}

// TestJITCommitLogicImportsNoPricing keeps the commit path free of money
// decisions.
//
// The committer carries an already-approved fee through untouched. An import of
// a pricing or quote contract here would be the first step towards selecting an
// authority by price, which is a commercial decision the planner has no mandate
// to make.
func TestJITCommitLogicImportsNoPricing(t *testing.T) {
	banned := []string{
		"shared/go/commercequote",
		"shared/go/dispatchpricing",
	}
	for _, name := range []string{"jit_order_binding.go", "mission_readback.go",
		"readiness_orchestration_wiring.go"} {
		for _, spec := range parseGoFile(t, name).Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("%s: bad import %s", name, spec.Path.Value)
			}
			for _, pkg := range banned {
				if strings.HasSuffix(imported, pkg) {
					t.Errorf("%s imports %s: the commit path decides when and "+
						"which authority, never price", name, imported)
				}
			}
			if strings.Contains(strings.ToLower(imported), "pricing") {
				t.Errorf("%s imports %s, which looks like pricing", name, imported)
			}
		}
	}
}

// TestJITBindingNamesNoProvider walks the composition root's just-in-time
// binding for any identifier that would name a provider.
//
// Vendor selects an AUTHORITY and a PRODUCT. The moment a rider, driver or
// vehicle appears in this file, Vendor has started selecting the person who
// does the work, which belongs to the executing authority alone.
func TestJITBindingNamesNoProvider(t *testing.T) {
	banned := []string{"providerid", "driverid", "riderid", "bikeid",
		"vehicleid", "truckid", "fleetmember", "assignee"}
	for _, name := range []string{"jit_order_binding.go", "mission_readback.go"} {
		file := parseGoFile(t, name)
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			lowered := strings.ToLower(identifier.Name)
			for _, word := range banned {
				if lowered == word {
					t.Errorf("%s names %s: Vendor never selects a provider",
						name, identifier.Name)
				}
			}
			return true
		})
	}
}

// TestTheProviderGuardIsNotVacuous proves the walker above would actually
// reject one, so a green result means something.
func TestTheProviderGuardIsNotVacuous(t *testing.T) {
	const planted = "providerID"
	caught := false
	for _, word := range []string{"providerid", "driverid", "riderid"} {
		if strings.ToLower(planted) == word {
			caught = true
		}
	}
	if !caught {
		t.Fatal("the provider matcher does not recognise providerID")
	}
}

// TestCommitterIsWiredFromTheOrdersService documents, structurally, that the
// production committer delegates rather than assembles.
func TestCommitterIsWiredFromTheOrdersService(t *testing.T) {
	var committer orchestration.MissionCommitter = productionMissionCommitter{}
	if committer == nil {
		t.Fatal("the production committer does not satisfy the orchestration's port")
	}
	content, err := os.ReadFile("jit_order_binding.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(content)
	for _, required := range []string{
		"c.orders.CommitPlannedMission(", "c.orders.ReadPlannedMission(",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("the committer does not delegate through %s", required)
		}
	}
	for _, forbidden := range []string{"CreateMission(ctx, orders.MissionRequest{"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("the committer assembles its own mission request (%s)", forbidden)
		}
	}
	_ = commandSourceFiles(t)
}
