package internal

// Support's authority ends at the case.
//
// It owns complaints, disputes, SOS, chat, evidence references and its own
// audit. It does NOT own a trip, a delivery, a haulage booking, a
// verification decision or a ledger. These guards read Support's source and
// fail on the shapes that would mean it had started deciding any of them.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func supportSources(t *testing.T) []string {
	t.Helper()
	var files []string
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 10 {
		t.Fatalf("found %d files; the walk is looking in the wrong place", len(files))
	}
	return files
}

// operationalTables are tables other services own. Support's SQL may not name
// them at all -- not to read, and certainly not to write.
var operationalTables = []string{
	"taxi_bookings", "taxi_goods_missions", "taxi_recovery_cases",
	"trips", "dispatch_bookings", "dispatch_delivery_missions", "commerce_missions",
	"haulage_bookings", "hauling_recovery_cases",
	"asset_capability_decisions", "service_tier_certifications", "asset_capability_cases",
	"ledger_entries", "payment_intents", "wallet_accounts",
}

// TestSupportNeverTouchesOperationalTables proves Support keeps no second copy
// of operational truth and cannot mutate one directly.
func TestSupportNeverTouchesOperationalTables(t *testing.T) {
	for _, path := range supportSources(t) {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		lowered := strings.ToLower(string(content))
		for _, table := range operationalTables {
			pattern := regexp.MustCompile(`\b(from|into|update|join)\s+` + table + `\b`)
			if pattern.MatchString(lowered) {
				t.Errorf("%s reads or writes %s, which another service owns", path, table)
			}
		}
	}
}

// TestSupportHoldsNoMoneyOrVerificationAuthority bans the imports that would
// let Support move money or decide a certification.
func TestSupportHoldsNoMoneyOrVerificationAuthority(t *testing.T) {
	banned := []string{"shared/go/walletclient", "shared/go/managedsettlement",
		"shared/go/commercequote", "shared/go/dispatchpricing", "shared/go/capabilityfit"}
	for _, path := range supportSources(t) {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			imported, _ := strconv.Unquote(spec.Path.Value)
			for _, pkg := range banned {
				if strings.HasSuffix(imported, pkg) {
					t.Errorf("%s imports %s: Support adjudicates, owners execute", path, imported)
				}
			}
		}
	}
}

// TestSupportCallsNoOwnerMutationPath checks outbound URLs. The only owner
// operation Support may invoke is a recovery START, which the owner validates
// and may refuse. A path that sets a status, cancels, reassigns, completes or
// decides a certification would be Support mutating another service directly.
func TestSupportCallsNoOwnerMutationPath(t *testing.T) {
	forbidden := []string{"/status", "/cancel", "/reassign", "/complete", "/assign",
		"/decisions", "/certif", "/settle", "/refund", "/payout", "/select-replacement"}
	for _, path := range supportSources(t) {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err != nil || !strings.Contains(value, "/internal/") {
				return true
			}
			for _, fragment := range forbidden {
				if strings.Contains(strings.ToLower(value), fragment) {
					t.Errorf("%s calls owner path %q: Support may not mutate another service",
						path, value)
				}
			}
			return true
		})
	}
}
