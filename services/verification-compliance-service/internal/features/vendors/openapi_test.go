package vendors

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestVendorCaseOpenAPIParityAndSecurity(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/openapi/vendor-cases.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"/internal/vendor-cases/cases":                                       "post",
		"/internal/vendor-cases/cases/by-client-reference/{clientReference}": "get",
		"/internal/vendor-cases/cases/{caseRef}":                             "get",
		"/internal/vendor-cases/cases/{caseRef}/decisions":                   "post",
	}
	if len(document.Paths) != len(want) {
		t.Fatalf("documented paths=%d want %d", len(document.Paths), len(want))
	}
	for path, method := range want {
		op, ok := document.Paths[path][method]
		if !ok || !strings.Contains(string(op), "ServiceHMACv2") {
			t.Fatalf("missing secured %s %s", method, path)
		}
	}
}
