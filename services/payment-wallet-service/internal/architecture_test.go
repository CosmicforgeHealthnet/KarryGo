package internal

// Section 17 guards.

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Wallet moves and records money; it never prices a job.
func TestWalletImportsNoPricing(t *testing.T) {
	banned := []string{"shared/go/dispatchpricing", "shared/go/commercequote", "internal/pkg/pricing"}
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			imported, _ := strconv.Unquote(spec.Path.Value)
			for _, pkg := range banned {
				if strings.HasSuffix(imported, pkg) {
					t.Errorf("%s imports %s", path, imported)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A rebind moves the beneficiary only. Its UPDATE may not name any column
// that describes what is held or whose job it is.
func TestTheRebindCannotAlterWhatIsHeld(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("features", "wallets", "repositories", "execution_rebind.go"))
	if err != nil {
		t.Fatal(err)
	}
	updates := regexp.MustCompile(`(?is)UPDATE\s+payment_intents\s+SET(.*?)WHERE`).FindAllStringSubmatch(string(raw), -1)
	if len(updates) == 0 {
		t.Fatal("rebind UPDATE not found")
	}
	for _, update := range updates {
		for _, column := range []string{"amount_kobo", "currency", "platform_fee_kobo", "source_reference",
			"source_service", "customer_id", "status", "settlement_mode"} {
			if regexp.MustCompile(`\b` + column + `\s*=`).MatchString(update[1]) {
				t.Errorf("the rebind UPDATE sets %s", column)
			}
		}
	}
}
