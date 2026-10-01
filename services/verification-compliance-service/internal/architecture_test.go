package internal

// Ownership boundaries this service must not cross, enforced by reading its own
// source rather than by review.
//
// Verification decides whether an asset is CERTIFIED for a service tier. It
// does not decide whether that asset is available, where it is, or who should
// be sent it. Those are three different authorities, and the failure mode when
// they blur is specific and bad: a compliance decision that also takes a
// vehicle off the road, or a certification service that can dispatch.
//
// These are compile-time-ish guards. They parse every non-test Go file under
// this service and fail on the imports, identifiers and routes that would mean
// the boundary had moved.

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

// serviceSourceFiles returns every non-test .go file this service owns.
func serviceSourceFiles(t *testing.T) []string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			if info.Name() == "migrations" || info.Name() == "docs" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("found no source files to check; the walk root is wrong")
	}
	return files
}

func parseFile(t *testing.T, path string) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return fset, file
}

// TestVerificationImportsNoOperationalOrPricingPackage pins what this service
// is allowed to depend on.
//
// The execution services' internal packages are unreachable anyway (Go forbids
// it across modules), so the interesting bans are the SHARED ones: a matching,
// mission, quote or pricing package imported here would mean a compliance
// service had acquired the ability to dispatch or to price.
func TestVerificationImportsNoOperationalOrPricingPackage(t *testing.T) {
	banned := map[string]string{
		"commercemission":     "creating missions is an execution authority's job",
		"commercequote":       "quoting is pricing, and pricing is not this service's",
		"dispatchpricing":     "pricing is not this service's",
		"logisticscapability": "operational capability is the execution services' own",
		"supplypreview":       "asking who is available is matching, not certification",
		"fleetproviderwork":   "offering work to a provider is dispatch",
		"providercompat":      "provider work surfaces belong to the execution services",
		"walletclient":        "certification never moves money",
	}
	for _, path := range serviceSourceFiles(t) {
		_, file := parseFile(t, path)
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("%s: bad import path %s", path, spec.Path.Value)
			}
			for pkg, why := range banned {
				if imported == "cosmicforge/logistics/shared/go/"+pkg {
					t.Errorf("%s imports %s: %s", path, imported, why)
				}
			}
			if strings.Contains(imported, "/internal/features/") &&
				!strings.Contains(imported, "verification-compliance-service") {
				t.Errorf("%s imports another service's internal package %s", path, imported)
			}
		}
	}
}

// TestVerificationOwnsNoOperationalVehicleProfile proves this service does not
// model the operational vehicle.
//
// It holds documents and inspections ABOUT an asset, and a certification FOR a
// service tier. It must not hold the asset's operational profile -- its
// availability, its position, its current assignment, its odometer. Those live
// with the service that operates it, and duplicating them here would create a
// second answer to "is this vehicle working right now" that nobody updates.
func TestVerificationOwnsNoOperationalVehicleProfile(t *testing.T) {
	// Field names that would mean an operational profile had been absorbed.
	// Capability, certification, document and inspection vocabulary is
	// deliberately absent from this list: that is exactly what this service
	// does own.
	banned := []string{
		"isonline", "isavailable", "availabilitystatus", "currentlatitude",
		"currentlongitude", "lastknownlocation", "currenttripid", "currentbookingid",
		"activeassignment", "odometer", "fuellevel", "heartbeatat", "onlinesince",
	}
	for _, path := range serviceSourceFiles(t) {
		_, file := parseFile(t, path)
		ast.Inspect(file, func(node ast.Node) bool {
			structType, ok := node.(*ast.StructType)
			if !ok || structType.Fields == nil {
				return true
			}
			for _, field := range structType.Fields.List {
				for _, name := range field.Names {
					lowered := strings.ToLower(name.Name)
					for _, word := range banned {
						if lowered == word {
							t.Errorf("%s declares field %s: that is operational vehicle state, "+
								"which belongs to the service that operates the asset", path, name.Name)
						}
					}
				}
			}
			return true
		})
	}
}

// TestVerificationCannotInvokeTheMatcher pins the outbound surface.
//
// Every path this service is allowed to call belongs to a certification
// ingress. A matching, offer, assignment, booking or mission path appearing in
// a string literal here would mean a compliance decision could put a vehicle on
// a job -- which is the single worst way for these two authorities to blur.
func TestVerificationCannotInvokeTheMatcher(t *testing.T) {
	banned := []string{
		"/match", "/matching", "/offers", "/assignments", "/dispatch/",
		"/bookings", "/missions", "/trips", "/quote", "/pricing", "/fares",
	}
	for _, path := range serviceSourceFiles(t) {
		_, file := parseFile(t, path)
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
					t.Errorf("%s contains outbound path %q: verification must not reach "+
						"a matching, assignment or pricing surface", path, value)
				}
			}
			return true
		})
	}
}

// TestCallbacksOnlyTargetEventIngresses states the positive half of the rule:
// every path this service calls is an INTERNAL EVENT INGRESS on the receiving
// service, and nothing else.
//
// That is what keeps each operational service the owner of its own projection.
// Verification announces a decision; the receiving service decides what that
// decision means for its own assets. A callback that wrote into somebody's
// asset table, or read their availability, would have taken that ownership
// away -- and it would look exactly like a path that is not an event ingress.
func TestCallbacksOnlyTargetEventIngresses(t *testing.T) {
	seen := 0
	for _, path := range serviceSourceFiles(t) {
		if !strings.HasSuffix(path, "callbacks.go") {
			continue
		}
		_, file := parseFile(t, path)
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			// A bare separator is string surgery (trimming a trailing slash),
			// not a destination.
			if err != nil || !strings.HasPrefix(value, "/") || value == "/" {
				return true
			}
			seen++
			if !strings.Contains(value, "/internal/") || !strings.HasSuffix(value, "-events") {
				t.Errorf("%s targets %q, which is not an internal event ingress", path, value)
			}
			return true
		})
	}
	if seen == 0 {
		t.Fatal("found no callback paths to check; this test has stopped proving anything")
	}
}
