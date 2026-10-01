package providercompat_test

import (
	"testing"

	"cosmicforge/logistics/shared/go/providercompat"
)

// VIS-017/018. The registry that decides what a provider may see is reused
// here, so a field classified once is protected on every surface.
func TestConcealedFieldsAreFoundInAPayload(t *testing.T) {
	leaked := providercompat.ConcealedFinancialFields(map[string]any{
		"booking_id":               "book_1",
		"own_earning_kobo":         1500,
		"organization_share_minor": 7500,
		"platform_share_minor":     1000,
	})
	if len(leaked) != 2 {
		t.Fatalf("found %v, want the organization and platform fields", leaked)
	}
}

// A bag whose top level is clean and whose nested object carries the
// organization's share is exactly the shape that passes a shallow check.
func TestConcealedFieldsAreFoundWhenNested(t *testing.T) {
	leaked := providercompat.ConcealedFinancialFields(map[string]any{
		"booking_id": "book_1",
		"settlement": map[string]any{
			"provider_share_minor":     1500,
			"organization_share_minor": 7500,
		},
	})
	if len(leaked) != 1 || leaked[0] != "settlement.organization_share_minor" {
		t.Fatalf("found %v, want the nested organization share", leaked)
	}
}

func TestConcealedFieldsAreFoundInsideLists(t *testing.T) {
	leaked := providercompat.ConcealedFinancialFields(map[string]any{
		"lines": []any{
			map[string]any{"own_earning_kobo": 1500},
			map[string]any{"platform_share_minor": 1000},
		},
	})
	if len(leaked) != 1 {
		t.Fatalf("found %v, want the platform share inside the list", leaked)
	}
}

// A provider's own figures are not concealed -- redacting everything would be
// obeyed by nobody.
func TestAProvidersOwnFiguresAreNotConcealed(t *testing.T) {
	leaked := providercompat.ConcealedFinancialFields(map[string]any{
		"own_earning_kobo": 1500, "tips_kobo": 200, "fare_kobo": 10000,
	})
	if len(leaked) != 0 {
		t.Fatalf("a provider's own figures were treated as concealed: %v", leaked)
	}
}

// Redaction marks rather than deletes, so a reader knows a field was present
// and withheld rather than never set.
func TestRedactionMarksRatherThanDeletes(t *testing.T) {
	redacted := providercompat.RedactConcealedFinancialFields(map[string]any{
		"booking_id":               "book_1",
		"organization_share_minor": 7500,
		"settlement":               map[string]any{"platform_share_minor": 1000},
	})
	if redacted["organization_share_minor"] != providercompat.RedactedPlaceholder {
		t.Fatalf("organization share = %v, want the marker", redacted["organization_share_minor"])
	}
	nested, _ := redacted["settlement"].(map[string]any)
	if nested["platform_share_minor"] != providercompat.RedactedPlaceholder {
		t.Fatalf("nested platform share = %v, want the marker", nested["platform_share_minor"])
	}
	if redacted["booking_id"] != "book_1" {
		t.Fatal("redaction removed an operationally useful field")
	}
}

// The original must be untouched, or a caller that logs a redacted view and
// then acts on the payload is silently working with blanks.
func TestRedactionDoesNotMutateTheOriginal(t *testing.T) {
	original := map[string]any{"organization_share_minor": 7500}
	providercompat.RedactConcealedFinancialFields(original)
	if original["organization_share_minor"] != 7500 {
		t.Fatalf("the original was mutated to %v", original["organization_share_minor"])
	}
}

// VIS-018. Credentials never reach a log, whatever else the line carries.
func TestSecretsAreRedactedFromLogFields(t *testing.T) {
	for _, field := range []string{
		"authorization", "Authorization", "X-Service-Signature", "service_secret",
		"token", "access_token", "refresh_token", "password", "pin", "otp",
		"api_key", "hmac", "cookie",
	} {
		if !providercompat.IsSecretField(field) {
			t.Errorf("%q is not treated as a secret", field)
		}
	}
	redacted := providercompat.RedactLogFields(map[string]any{
		"Authorization":            "Bearer abc.def.ghi",
		"request_id":               "req_1",
		"organization_share_minor": 7500,
		"own_earning_kobo":         1500,
	})
	if redacted["Authorization"] != providercompat.RedactedPlaceholder {
		t.Fatalf("the authorization header survived redaction: %v", redacted["Authorization"])
	}
	if redacted["organization_share_minor"] != providercompat.RedactedPlaceholder {
		t.Fatalf("the organization share survived redaction: %v", redacted["organization_share_minor"])
	}
	// An operator still needs the line to be worth keeping.
	if redacted["request_id"] != "req_1" || redacted["own_earning_kobo"] != 1500 {
		t.Fatalf("redaction destroyed operator usefulness: %+v", redacted)
	}
}

func TestOrdinaryFieldsAreNotMistakenForSecrets(t *testing.T) {
	for _, field := range []string{"booking_id", "status", "provider_id", "request_id", "currency"} {
		if providercompat.IsSecretField(field) {
			t.Errorf("%q was treated as a secret", field)
		}
	}
}
