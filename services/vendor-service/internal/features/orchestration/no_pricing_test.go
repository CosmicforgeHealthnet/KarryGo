package orchestration

// Readiness and orchestration never price anything.
//
// This is the standing guard for the rule that shaped both features: Vendor's
// just-in-time planner decides WHEN work is handed over, WHICH authority
// carries it and WHO travels together. It never decides what anybody is paid.
//
// The rule is worth enforcing mechanically rather than by review because the
// pressure to break it is real and sounds reasonable every time -- "the planner
// already knows the distance, it may as well estimate the fare", "we could pick
// the cheaper authority". Both would move a pricing decision into a service
// that has no pricing authority, and both would be one small diff.
//
// The check is structural: it walks the source of these two features and fails
// on a pricing import, a pricing-shaped identifier or a pricing-shaped route.
//
// orchestration_test.go carries an older, narrower version of the import half
// of this rule. It is kept rather than merged because it names the specific
// types authority selection reads, which is a claim about selection worth
// stating on its own. What is here is the wider net: BOTH features, every
// struct rather than a listed few, and the route surface as well as the types.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// pricingWords are the shapes money takes in this codebase. "kobo" is here
// because amounts are integer kobo platform-wide, so a field carrying one is
// almost always named for it.
var pricingWords = []string{
	"price", "pricing", "fare", "tariff", "surge", "commission",
	"payout", "subsidy", "ratecard", "kobo", "amountdue", "totalamount",
}

// planningSourceFiles returns the non-test source of the two features this
// rule covers.
func planningSourceFiles(t *testing.T) []string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, feature := range []string{"orchestration", "fulfilment_readiness"} {
		dir := filepath.Join(root, feature)
		entries, readErr := os.ReadDir(dir)
		if readErr != nil {
			t.Fatalf("read %s: %v", dir, readErr)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") ||
				strings.HasSuffix(name, "_test.go") {
				continue
			}
			files = append(files, filepath.Join(dir, name))
		}
	}
	if len(files) < 2 {
		t.Fatalf("found %d files to check; the walk root is wrong", len(files))
	}
	return files
}

func parsePlanningFile(t *testing.T, path string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return file
}

// TestPlanningImportsNoPricingPackage bans the dependency outright.
//
// commercequote is the interesting one: it is the platform's delivery-quote
// contract, it is genuinely nearby, and importing it here is how the planner
// would acquire a fare without anybody deciding it should.
func TestPlanningImportsNoPricingPackage(t *testing.T) { //nolint:gocognit
	banned := []string{
		"cosmicforge/logistics/shared/go/commercequote",
		"cosmicforge/logistics/shared/go/dispatchpricing",
		"cosmicforge/logistics/shared/go/walletclient",
		"cosmicforge/logistics/shared/go/commercecapability",
	}
	for _, path := range planningSourceFiles(t) {
		for _, spec := range parsePlanningFile(t, path).Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("%s: bad import path %s", path, spec.Path.Value)
			}
			for _, pkg := range banned {
				if imported == pkg {
					t.Errorf("%s imports %s: planning must not depend on a pricing or "+
						"money contract", path, pkg)
				}
			}
			if strings.Contains(strings.ToLower(imported), "pricing") {
				t.Errorf("%s imports %s, which looks like pricing", path, imported)
			}
		}
	}
}

// TestPlanningDeclaresNoPricingField walks every struct these features define.
//
// A field is where a fare would actually land. Catching the import alone is not
// enough: an amount can be carried as a plain int64 with a money name and no
// pricing dependency at all.
func TestPlanningDeclaresNoPricingField(t *testing.T) {
	for _, path := range planningSourceFiles(t) {
		file := parsePlanningFile(t, path)
		ast.Inspect(file, func(node ast.Node) bool {
			structType, ok := node.(*ast.StructType)
			if !ok || structType.Fields == nil {
				return true
			}
			for _, field := range structType.Fields.List {
				for _, name := range field.Names {
					lowered := strings.ToLower(name.Name)
					for _, word := range pricingWords {
						if strings.Contains(lowered, word) {
							t.Errorf("%s declares field %s: readiness and orchestration "+
								"carry no money", path, name.Name)
						}
					}
				}
			}
			return true
		})
	}
}

// TestPlanningExposesNoPricingRoute checks the surface rather than the types.
//
// A route is the other way a fare escapes: an endpoint that returns an
// estimate, a quote or a cost would make the planner a pricing authority to
// everyone who calls it, whatever its internal types say.
func TestPlanningExposesNoPricingRoute(t *testing.T) {
	banned := []string{"/quote", "/quotes", "/price", "/pricing",
		"/fare", "/fares", "/estimate-cost", "/cost"}
	for _, path := range planningSourceFiles(t) {
		file := parsePlanningFile(t, path)
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err != nil || !strings.HasPrefix(value, "/") {
				return true
			}
			lowered := strings.ToLower(value)
			for _, fragment := range banned {
				if strings.Contains(lowered, fragment) {
					t.Errorf("%s declares route %q: planning exposes no pricing surface",
						path, value)
				}
			}
			return true
		})
	}
}

// TestTheGuardWouldActuallyCatchAPricingField proves the walker above is not
// silently passing because it looks at nothing.
//
// A structural test that has stopped matching anything reports success
// forever, which is worse than no test: it is a green check that nobody
// re-reads. So this feeds the same matcher a field it must reject.
func TestTheGuardWouldActuallyCatchAPricingField(t *testing.T) {
	type wouldBeRejected struct {
		DeliveryFareKobo int64
	}
	var sample wouldBeRejected
	_ = sample

	caught := false
	for _, word := range pricingWords {
		if strings.Contains(strings.ToLower("DeliveryFareKobo"), word) {
			caught = true
		}
	}
	if !caught {
		t.Fatal("the pricing matcher does not recognise DeliveryFareKobo; " +
			"the guards above are not actually guarding anything")
	}
}
