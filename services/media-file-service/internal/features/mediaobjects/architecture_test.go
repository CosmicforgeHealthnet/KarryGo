package mediaobjects_test

// Media holds bytes and says whether a file may be RELIED ON as stated -- is
// it intact, scanned, active, bound where the caller says. It never decides
// what the file PROVES. Whether a photo shows a roadworthy vehicle is
// Verification's decision; whether a damage photo supports a refund is
// Support's and the money owner's. A media service that started answering
// those would be making compliance and financial decisions with no authority
// to make either.

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

func mediaSourceFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
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
	if len(files) < 5 {
		t.Fatalf("found %d files; the walk is looking in the wrong place", len(files))
	}
	return files
}

// TestMediaMakesNoVerificationOrFinancialDecision bans the imports and
// identifiers that would mean Media had started deciding what evidence proves.
func TestMediaMakesNoVerificationOrFinancialDecision(t *testing.T) {
	bannedImports := []string{"logisticscapability", "capabilityfit", "walletclient",
		"commercequote", "dispatchpricing", "managedsettlement"}
	bannedIdentifiers := []string{"certify", "certification", "eligible", "eligibility",
		"approvecapability", "refundamount", "payout"}
	for _, path := range mediaSourceFiles(t) {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			imported, _ := strconv.Unquote(spec.Path.Value)
			for _, banned := range bannedImports {
				if strings.HasSuffix(imported, "/"+banned) {
					t.Errorf("%s imports %s", path, imported)
				}
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.FuncDecl:
				check(t, path, n.Name.Name, bannedIdentifiers)
			case *ast.Field:
				for _, name := range n.Names {
					check(t, path, name.Name, bannedIdentifiers)
				}
			}
			return true
		})
	}
}

func check(t *testing.T, path, name string, banned []string) {
	t.Helper()
	lowered := strings.ToLower(name)
	for _, word := range banned {
		if strings.Contains(lowered, word) {
			t.Errorf("%s declares %s: media reports whether a file can be relied on, "+
				"never what it proves", path, name)
		}
	}
}
