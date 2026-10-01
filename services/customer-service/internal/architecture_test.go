package internal_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Customer Service is a customer-context authority and a read-only movement
// projection. These guards fail the build if it starts importing execution,
// quoting, mission-creation or pricing contracts, or grows mutating
// movement/provider routes.
var forbiddenImports = []string{
	"cosmicforge/logistics/services/",                 // any other service's packages
	"cosmicforge/logistics/shared/go/commercemission", // creating logistics missions
	"cosmicforge/logistics/shared/go/commercequote",   // quoting / serviceability
	"cosmicforge/logistics/shared/go/dispatchcommerce",
	"cosmicforge/logistics/shared/go/dispatchpricing",
	"cosmicforge/logistics/shared/go/walletclient",
	"cosmicforge/logistics/shared/go/custodyops",
}

func goFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, root := range []string{".", filepath.Join("..", "cmd")} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return files
}

func TestCustomerServiceImportsNoExecutionOrPricingAuthority(t *testing.T) {
	const self = "cosmicforge/logistics/services/customer-service/"
	for _, path := range goFiles(t) {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			imported := strings.Trim(spec.Path.Value, `"`)
			if strings.HasPrefix(imported, self) {
				continue
			}
			for _, forbidden := range forbiddenImports {
				if strings.HasPrefix(imported, forbidden) {
					t.Errorf("%s imports %s: customer-service must not own execution, quoting or pricing", path, imported)
				}
			}
		}
	}
}

func TestCustomerServiceHasNoMutatingMovementOrProviderRoutes(t *testing.T) {
	mutating := regexp.MustCompile(`\.(POST|PUT|PATCH|DELETE)\(\s*"([^"]*)"`)
	for _, path := range goFiles(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range mutating.FindAllStringSubmatch(string(raw), -1) {
			route := strings.ToLower(match[2])
			for _, word := range []string{"movement", "booking", "mission", "provider", "assign", "match", "quote", "fare", "price"} {
				if strings.Contains(route, word) {
					t.Errorf("%s registers %s %q: customer-service must not mutate execution state", path, match[1], match[2])
				}
			}
		}
	}
}

func TestMovementProjectionHasNoMissionPlanningLogic(t *testing.T) {
	forbidden := regexp.MustCompile(`(?i)(delivery_count|mission_count\s*[:=]\s*len|selectProvider|chooseProvider|computeFare|pricePerK)`)
	for _, path := range goFiles(t) {
		if !strings.Contains(filepath.ToSlash(path), "movement_history") {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if loc := forbidden.FindString(string(raw)); loc != "" {
			t.Errorf("%s contains planning/pricing logic %q; mission count is only ever copied from the authority", path, loc)
		}
	}
}
