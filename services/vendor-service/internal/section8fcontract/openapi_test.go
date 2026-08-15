package section8fcontract

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

var routes = []struct{ method, path string }{
	{"POST", "/api/v1/vendor/crm/campaigns/{campaignId}/runs"}, {"GET", "/api/v1/vendor/crm/campaigns/{campaignId}/runs"},
	{"GET", "/api/v1/vendor/crm/campaigns/{campaignId}/runs/{runId}"}, {"POST", "/api/v1/vendor/crm/campaigns/{campaignId}/runs/{runId}/cancel"},
	{"POST", "/api/v1/vendor/crm/campaigns/{campaignId}/runs/{runId}/pause"}, {"POST", "/api/v1/vendor/crm/campaigns/{campaignId}/runs/{runId}/resume"},
	{"GET", "/api/v1/vendor/crm/campaigns/{campaignId}/runs/{runId}/deliveries"}, {"GET", "/api/v1/vendor/crm/campaigns/{campaignId}/runs/{runId}/deliveries/{deliveryId}"},
	{"GET", "/api/v1/vendor/crm/campaigns/{campaignId}/runs/{runId}/deliveries/{deliveryId}/history"}, {"POST", "/internal/vendor/v1/crm/campaign-delivery-events"},
}

func load(t *testing.T) (map[string]map[string]json.RawMessage, []byte) {
	t.Helper()
	raw, err := os.ReadFile("../../docs/openapi/section8f-campaign-execution.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	return spec.Paths, raw
}

func TestBidirectionalRouteParity(t *testing.T) {
	paths, _ := load(t)
	documented := map[string]bool{}
	for path, methods := range paths {
		for method := range methods {
			if strings.Contains("get post put patch delete", method) {
				documented[strings.ToUpper(method)+" "+path] = true
			}
		}
	}
	registered := map[string]bool{}
	for _, r := range routes {
		registered[r.method+" "+r.path] = true
	}
	var onlySpec, onlyCode []string
	for k := range documented {
		if !registered[k] {
			onlySpec = append(onlySpec, k)
		}
	}
	for k := range registered {
		if !documented[k] {
			onlyCode = append(onlyCode, k)
		}
	}
	sort.Strings(onlySpec)
	sort.Strings(onlyCode)
	if len(onlySpec) > 0 || len(onlyCode) > 0 {
		t.Fatalf("spec-only=%v code-only=%v", onlySpec, onlyCode)
	}
}
func TestRouteCountAndInternalMount(t *testing.T) {
	if len(routes) != 10 {
		t.Fatalf("route count=%d want 9 public + 1 internal", len(routes))
	}
	if routes[9].path != "/internal/vendor/v1/crm/campaign-delivery-events" || strings.Contains(routes[9].path, "/api/v1/vendor") {
		t.Fatalf("internal route is not root mounted: %s", routes[9].path)
	}
}
func TestContractContainsNoDestinationPII(t *testing.T) {
	_, raw := load(t)
	lower := strings.ToLower(string(raw))
	for _, term := range []string{`"email":`, `"phone":`, `"device_token":`, `"destination_address":`} {
		if strings.Contains(lower, term) {
			t.Errorf("OpenAPI exposes forbidden destination field %s", term)
		}
	}
}
