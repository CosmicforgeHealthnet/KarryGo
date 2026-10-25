package adminresources

import (
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/adminowner"
)

func TestWalletSourceDictionaryIsFixedCompleteAndMinimized(t *testing.T) {
	profile := adminowner.WalletProfile()
	source := Source()
	if len(source.Specs) != len(profile.Definitions) {
		t.Fatalf("source/profile mismatch: %d != %d", len(source.Specs), len(profile.Definitions))
	}
	for kind := range profile.Definitions {
		if _, ok := source.Specs[kind]; !ok {
			t.Errorf("missing fixed source projection %q", kind)
		}
	}
	forbidden := []string{"account_number", "recipient_code", "access_code", "authorization_url", "customer_email", "payload", "request_fingerprint", "network_meta", "previous_summary", "new_summary", "last_error"}
	for kind, spec := range source.Specs {
		parts := []string{spec.BindingExpression}
		parts = append(parts, spec.BindingColumns...)
		for _, mapping := range []map[string]string{spec.Dimensions, spec.Counters, spec.References} {
			for key, column := range mapping {
				parts = append(parts, key, column)
			}
		}
		projection := strings.ToLower(strings.Join(parts, " "))
		for _, secret := range forbidden {
			if strings.Contains(projection, secret) {
				t.Errorf("%s exposes forbidden financial field %s", kind, secret)
			}
		}
	}
}
