package section6contract

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

var routes = []struct{ method, path string }{
	{"GET", "/capability-types"}, {"POST", "/businesses"}, {"GET", "/businesses"}, {"GET", "/businesses/{businessId}"}, {"PATCH", "/businesses/{businessId}"}, {"POST", "/businesses/{businessId}/pause"}, {"POST", "/businesses/{businessId}/resume"},
	{"GET", "/businesses/{businessId}/capabilities"}, {"POST", "/businesses/{businessId}/capabilities"}, {"GET", "/businesses/{businessId}/capabilities/{capabilityId}"}, {"POST", "/businesses/{businessId}/capabilities/{capabilityId}/pause"}, {"POST", "/businesses/{businessId}/capabilities/{capabilityId}/resume"},
	{"GET", "/businesses/{businessId}/brands"}, {"POST", "/businesses/{businessId}/brands"}, {"GET", "/businesses/{businessId}/brands/{brandId}"}, {"PATCH", "/businesses/{businessId}/brands/{brandId}"},
	{"GET", "/businesses/{businessId}/branches"}, {"POST", "/businesses/{businessId}/branches"}, {"GET", "/businesses/{businessId}/branches/{branchId}"}, {"PATCH", "/businesses/{businessId}/branches/{branchId}"}, {"POST", "/businesses/{businessId}/branches/{branchId}/pause"}, {"POST", "/businesses/{businessId}/branches/{branchId}/resume"},
	{"GET", "/businesses/{businessId}/branches/{branchId}/hours"}, {"PUT", "/businesses/{businessId}/branches/{branchId}/hours"}, {"GET", "/businesses/{businessId}/branches/{branchId}/service-area"}, {"PUT", "/businesses/{businessId}/branches/{branchId}/service-area"}, {"GET", "/businesses/{businessId}/branches/{branchId}/operations-settings"}, {"PUT", "/businesses/{businessId}/branches/{branchId}/operations-settings"}, {"GET", "/businesses/{businessId}/branches/{branchId}/capabilities"}, {"PUT", "/businesses/{businessId}/branches/{branchId}/capabilities/{capabilityId}"}, {"POST", "/businesses/{businessId}/branches/{branchId}/capabilities/{capabilityId}/pause"}, {"POST", "/businesses/{businessId}/branches/{branchId}/capabilities/{capabilityId}/resume"},
}

type spec struct {
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Schemas map[string]struct {
			Enum []string `json:"enum"`
		} `json:"schemas"`
	} `json:"components"`
}

func load(t *testing.T) spec {
	t.Helper()
	raw, err := os.ReadFile("../../docs/openapi/section6-vendor-business.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var s spec
	if err = json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	return s
}
func TestOpenAPIMatchesEveryRegisteredSection6Route(t *testing.T) {
	s := load(t)
	documented := map[string]bool{}
	for path, methods := range s.Paths {
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
		t.Fatalf("route drift onlySpec=%v onlyCode=%v", onlySpec, onlyCode)
	}
}
func TestEveryMutationRequiresIdempotencyAndPermission(t *testing.T) {
	s := load(t)
	for path, methods := range s.Paths {
		for method, raw := range methods {
			if method == "parameters" {
				continue
			}
			var op struct {
				Permission string `json:"x-permission"`
				Parameters []struct {
					Ref  string `json:"$ref"`
					Name string `json:"name"`
				} `json:"parameters"`
				Responses map[string]any `json:"responses"`
			}
			if err := json.Unmarshal(raw, &op); err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
			if op.Permission == "" {
				t.Errorf("%s %s lacks x-permission", method, path)
			}
			if method != "get" {
				found := false
				for _, p := range op.Parameters {
					if strings.Contains(p.Ref, "IdempotencyKey") || strings.EqualFold(p.Name, "Idempotency-Key") {
						found = true
					}
				}
				if !found {
					t.Errorf("%s %s lacks required Idempotency-Key", method, path)
				}
			}
			success := false
			for code := range op.Responses {
				if strings.HasPrefix(code, "2") {
					success = true
				}
			}
			if !success {
				t.Errorf("%s %s lacks 2xx response", method, path)
			}
		}
	}
}
func TestStateVocabulariesAndCapabilitiesAreExact(t *testing.T) {
	s := load(t)
	want := map[string][]string{"CapabilityType": {"RESTAURANT", "FAST_FOOD", "CLOUD_KITCHEN", "PASTRY_BAKERY", "SHOP_GROCERY", "MARKET", "WHOLESALE"}, "BusinessStatus": {"DRAFT", "PENDING_VERIFICATION", "PENDING_REVIEW", "ACTIVE", "PAUSED", "SUSPENDED", "CLOSED"}, "CapabilityStatus": {"DRAFT", "PENDING_REVIEW", "ACTIVE", "PAUSED", "SUSPENDED", "REJECTED"}, "BrandVerificationState": {"NOT_STARTED", "IN_PROGRESS", "PENDING_REVIEW", "VERIFIED", "REJECTED", "EXPIRED", "SUSPENDED"}, "BranchStatus": {"DRAFT", "ACTIVE", "PAUSED", "SUSPENDED", "CLOSED"}, "BranchCapabilityStatus": {"DRAFT", "ACTIVE", "PAUSED", "SUSPENDED"}}
	for name, values := range want {
		got := s.Components.Schemas[name].Enum
		if strings.Join(got, "|") != strings.Join(values, "|") {
			t.Errorf("%s=%v want %v", name, got, values)
		}
	}
}
