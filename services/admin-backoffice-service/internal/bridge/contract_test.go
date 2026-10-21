package bridge

import (
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/operations"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"encoding/json"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// CI validates these actual serialized handler responses against independent
// JSON Schemas, rather than validating examples alone.
func TestContractWireSamples(t *testing.T) {
	f := newFixture(t)
	scope := security.Scope{Market: "NG", Organization: "org-1"}
	f.repo.operation = operations.Operation{ID: uuid.NewString(), Action: "test.command", Target: operations.Target{Type: "provider", ID: "ref", ExpectedVersion: 7}, Scope: scope, Status: "REJECTED", CreatedAt: time.Now().UTC(), AsOf: time.Now().UTC()}
	for _, sample := range []struct {
		name, method, path string
		body               []byte
		status             int
	}{
		{"discovery", "GET", "/capabilities" + scopeQuery, nil, 200},
		{"health", "GET", "/health/aggregate" + scopeQuery, nil, 200},
		{"operation", "GET", "/operations/" + f.repo.operation.ID + scopeQuery, nil, 200},
		{"denied", "GET", "/capabilities?market=IT&organization=org-1&purpose=CASE-123", nil, 403},
		{"blocked", "GET", "/finance/summary" + scopeQuery, nil, 503},
	} {
		w := invoke(f.b, f.request(t, sample.method, sample.path, sample.body))
		if w.Code != sample.status {
			t.Fatalf("%s = %d: %s", sample.name, w.Code, w.Body.String())
		}
		var envelope map[string]any
		if json.Unmarshal(w.Body.Bytes(), &envelope) != nil || envelope["contract_version"] != "1.0" {
			t.Fatal("invalid wire envelope")
		}
		if output := os.Getenv("ADMIN_BRIDGE_CONTRACT_CAPTURE_DIR"); output != "" {
			if err := os.MkdirAll(output, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(output, sample.name+".json"), w.Body.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
