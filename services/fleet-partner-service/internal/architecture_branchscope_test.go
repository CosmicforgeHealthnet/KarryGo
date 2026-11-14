package internal_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// nullUnsafeBranchScope matches a branch-scope guard written as
// cardinality($n::text[])=0 without COALESCE. An organization-wide caller has
// a nil PermittedBranchIDs slice, which pgx sends as NULL; cardinality(NULL)
// is NULL, so the guard silently hides every row that has a branch.
var nullUnsafeBranchScope = regexp.MustCompile(`cardinality\(\$[0-9]+(::text\[\])?\)\s*=\s*0`)

// TestBranchScopeGuardsAreNullSafe keeps every "no branch restriction" guard
// written as COALESCE(cardinality($n::text[]),0)=0.
func TestBranchScopeGuardsAreNullSafe(t *testing.T) {
	for _, path := range collectGoFiles(t, filepath.Join(serviceRoot, "internal"), false) {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, loc := range nullUnsafeBranchScope.FindAllIndex(src, -1) {
			if loc[0] >= len("COALESCE(") && string(src[loc[0]-len("COALESCE("):loc[0]]) == "COALESCE(" {
				continue
			}
			t.Errorf("%s: NULL-unsafe branch scope guard %q; use COALESCE(cardinality($n::text[]),0)=0", path, src[loc[0]:loc[1]])
		}
	}
}
