package vendorownedhandover

import (
	"encoding/json"
	"os"
	"testing"
)

func TestOpenAPIContainsEveryVendorOwnedAuthorityRoute(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/openapi/vendor-owned-secure-handover.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{"/missions": "post", "/missions/{missionRef}/handover/{stage}": "get", "/missions/{missionRef}/handover/{stage}/confirm": "post", "/missions/{missionRef}/handover/{stage}/resend": "post", "/missions/{missionRef}/reassignments": "post", "/missions/{missionRef}/receiver-changes": "post", "/missions/{missionRef}/cancellations": "post"}
	if len(spec.Paths) != len(expected) {
		t.Fatalf("paths=%d want=%d", len(spec.Paths), len(expected))
	}
	for path, method := range expected {
		if spec.Paths[path] == nil || spec.Paths[path][method] == nil {
			t.Fatalf("missing %s %s", method, path)
		}
	}
}
