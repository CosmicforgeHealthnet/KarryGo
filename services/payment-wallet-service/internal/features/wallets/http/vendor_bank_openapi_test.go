package wallethttp

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestVendorBankOpenAPIParityAndPrivacy(t *testing.T) {
	raw, err := os.ReadFile("../../../../docs/openapi/vendor-bank-onboarding.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"/internal/vendor-bank-profiles/resolve":      {"post"},
		"/internal/vendor-bank-profiles/{businessId}": {"put", "get"},
	}
	for path, methods := range want {
		for _, method := range methods {
			if _, ok := document.Paths[path][method]; !ok {
				t.Fatalf("missing %s %s", method, path)
			}
		}
	}
	text := string(raw)
	if !strings.Contains(text, `"writeOnly": true`) || strings.Contains(text, `"account_number":{"type":"string"}`) {
		t.Fatal("bank contract does not mark the full account number write-only")
	}
}
