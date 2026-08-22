package inventory

import (
	"testing"
)

// Section 14I fail-closed certification for the internal caller allowlist.
//
// Section 14H proves a non-allowlisted signed caller is refused, but it never
// poses the deployment case that matters most: an environment that has not
// configured the allowlist at all. cmd/main.go builds the caller list with
// strings.Split on a configuration value, and strings.Split("", ",") returns
// a one-element slice containing the empty string rather than an empty slice.
// If that empty string were ever admitted to the allowlist, an unset
// configuration would grant Inventory mutation authority to a caller whose
// service name failed to resolve. This test states that it does not.

func TestSection14IEmptyAllowlistGrantsNobody(t *testing.T) {
	for _, configured := range []struct {
		name    string
		callers []string
	}{
		{"unset configuration", splitLikeMain("")},
		{"whitespace-only configuration", splitLikeMain("   ")},
		{"empty slice", nil},
		{"comma-only configuration", splitLikeMain(",")},
		{"trailing separators", splitLikeMain(",,  ,")},
	} {
		t.Run(configured.name, func(t *testing.T) {
			allowed := callerMap(configured.callers)

			if len(allowed) != 0 {
				t.Fatalf("an unconfigured allowlist admitted %d caller(s): %v",
					len(allowed), allowed)
			}

			// The empty service name is the one an unauthenticated or
			// unresolved caller would carry.
			if allowed[""] {
				t.Fatal("the empty service name was admitted to the Inventory allowlist")
			}
		})
	}
}

// TestSection14IAllowlistAdmitsOnlyConfiguredCallers is the positive half, so
// the fail-closed test above cannot pass merely because the allowlist is
// broken in both directions.
func TestSection14IAllowlistAdmitsOnlyConfiguredCallers(t *testing.T) {
	allowed := callerMap(splitLikeMain("order-service, checkout-service ,"))

	if len(allowed) != 2 {
		t.Fatalf("allowlist size = %d, want 2: %v", len(allowed), allowed)
	}
	for _, caller := range []string{"order-service", "checkout-service"} {
		if !allowed[caller] {
			t.Errorf("%s was configured but is not allowed", caller)
		}
	}
	for _, caller := range []string{"", " ", "driver-dispatch-delivery-service"} {
		if allowed[caller] {
			t.Errorf("%q was never configured but is allowed", caller)
		}
	}
}

// splitLikeMain reproduces exactly how cmd/main.go turns the configured value
// into a caller slice, so this test exercises the real production shape rather
// than a tidier one.
func splitLikeMain(configured string) []string {
	out := []string{}
	current := ""
	for _, character := range configured {
		if character == ',' {
			out = append(out, current)
			current = ""
			continue
		}
		current += string(character)
	}
	return append(out, current)
}
