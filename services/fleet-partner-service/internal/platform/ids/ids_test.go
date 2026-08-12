package ids

import (
	"strings"
	"testing"
)

func TestNewGeneratesPrefixedOpaqueIDs(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := New(PrefixOrganization)
		if !strings.HasPrefix(id, "org_") {
			t.Fatalf("missing prefix: %s", id)
		}
		if err := Validate(PrefixOrganization, id); err != nil {
			t.Fatalf("generated id invalid: %v", err)
		}
		if seen[id] {
			t.Fatalf("duplicate id generated: %s", id)
		}
		seen[id] = true
	}
}

func TestValidateRejectsMalformedAndWrongPrefix(t *testing.T) {
	branchID := New(PrefixBranch)
	if err := Validate(PrefixOrganization, branchID); err == nil {
		t.Fatal("wrong prefix should be rejected")
	}
	for _, bad := range []string{"", "org_", "org_123", "1; DROP TABLE", "org_" + strings.Repeat("z", 32)} {
		if err := Validate(PrefixOrganization, bad); err == nil {
			t.Fatalf("malformed id %q should be rejected", bad)
		}
	}
}

func TestNoSequentialStructure(t *testing.T) {
	a := New(PrefixJob)
	b := New(PrefixJob)
	if a == b {
		t.Fatal("ids must be unique")
	}
	if !IsWellFormed(a) || !IsWellFormed(b) {
		t.Fatal("ids must be well formed")
	}
}
