package routing

// Section 18: the edge routes; it never decides.

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestTheEdgeCarriesNoDomainLogic(t *testing.T) {
	banned := []string{"internal/pkg/pricing", "internal/features/partner", "shared/go/walletclient",
		"shared/go/dispatchpricing", "shared/go/commercequote", "shared/go/managedsettlement", "jackc/pgx"}
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
				if strings.Contains(imported, pkg) {
					t.Errorf("%s imports %s: the edge routes, the owning service decides", path, imported)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
