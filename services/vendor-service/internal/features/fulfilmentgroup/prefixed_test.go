package fulfilmentgroup

import (
	"strings"
	"testing"
)

// The qualified column lists must stay valid SQL: a literal inside a
// COALESCE is an argument, not a column.
func TestPrefixedQualifiesOnlyColumns(t *testing.T) {
	got := prefixed("id, COALESCE(replaces_id, ''), version", "r")
	if got != "r.id, COALESCE(r.replaces_id, ''), r.version" {
		t.Fatalf("prefixed = %q", got)
	}
	for _, columns := range []string{missionRefColumns, parcelColumns} {
		if strings.Contains(prefixed(columns, "x"), "x.''") {
			t.Fatalf("a literal was qualified: %s", prefixed(columns, "x"))
		}
	}
}
