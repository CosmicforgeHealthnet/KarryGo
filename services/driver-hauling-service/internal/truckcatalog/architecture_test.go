package truckcatalog_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serviceRoot walks up from this package to the service directory.
func serviceRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve service root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("service root %s has no go.mod: %v", root, err)
	}
	return root
}

// The duplicated taxonomy must not come back.
//
// Before this package the service carried ValidTruckTypes and
// ValidPreferredTruckTypes as two independent 13-entry maps, each asking the
// next author in a comment to keep it in sync with the other. This test is why
// that can no longer silently happen: reintroducing either name, or any other
// package-level truck-type list, fails the build instead of waiting for a
// booking to match nobody.
func TestNoDuplicateTruckTypeListsOutsideCatalogue(t *testing.T) {
	root := serviceRoot(t)
	catalogueDir := filepath.Join(root, "internal", "truckcatalog")

	bannedNames := map[string]string{
		"ValidTruckTypes":          "the provider-side copy of the taxonomy",
		"ValidPreferredTruckTypes": "the booking-side copy of the taxonomy",
		"TruckTypes":               "an ad-hoc truck-type list",
		"AllowedTruckTypes":        "an ad-hoc truck-type list",
		"SupportedTruckTypes":      "an ad-hoc truck-type list",
		"ValidBodyFamilies":        "a second body-family list",
	}

	var offences []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "vendor" {
				return filepath.SkipDir
			}
			// The catalogue itself is allowed to hold the one list.
			if path == catalogueDir {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fileSet := token.NewFileSet()
		parsed, parseErr := parser.ParseFile(fileSet, path, nil, 0)
		if parseErr != nil {
			return nil // a file that does not parse is the compiler's problem
		}
		rel, _ := filepath.Rel(root, path)
		for _, decl := range parsed.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || (gen.Tok != token.VAR && gen.Tok != token.CONST) {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, name := range value.Names {
					if why, banned := bannedNames[name.Name]; banned {
						offences = append(offences,
							rel+" declares "+name.Name+" ("+why+"); use internal/truckcatalog instead")
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk service: %v", err)
	}
	if len(offences) > 0 {
		t.Fatalf("truck taxonomy duplicated outside the catalogue:\n  %s", strings.Join(offences, "\n  "))
	}
}

// Booking validation, provider registration and matching must all route
// through the catalogue. If a package that decides truck-type questions stops
// importing it, it has started deciding them on its own.
func TestTruckTypeDecidersImportTheCatalogue(t *testing.T) {
	root := serviceRoot(t)
	const catalogueImport = "cosmicforge/logistics/services/hauling-service/internal/truckcatalog"

	mustImport := map[string]string{
		filepath.Join("internal", "features", "booking", "usecases"):          "booking validation and matching",
		filepath.Join("internal", "features", "provider_profile", "usecases"): "provider truck registration",
	}

	for dir, why := range mustImport {
		full := filepath.Join(root, dir)
		entries, err := os.ReadDir(full)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		found := false
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(full, entry.Name()))
			if err != nil {
				t.Fatalf("read %s: %v", entry.Name(), err)
			}
			if strings.Contains(string(raw), catalogueImport) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s (%s) no longer imports the truck catalogue", dir, why)
		}
	}
}
