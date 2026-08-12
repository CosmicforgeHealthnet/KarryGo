package section7contract

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

var routes = []struct{ method, path string }{
	{"GET", "/onboarding"}, {"GET", "/onboarding/progress"}, {"GET", "/onboarding/checklist"}, {"GET", "/onboarding/blockers"}, {"GET", "/onboarding/activation-readiness"}, {"POST", "/onboarding/submit"}, {"POST", "/onboarding/request-activation"},
	{"GET", "/onboarding/representative"}, {"PUT", "/onboarding/representative"}, {"GET", "/onboarding/documents"}, {"POST", "/onboarding/documents"}, {"PATCH", "/onboarding/documents/{documentId}"}, {"DELETE", "/onboarding/documents/{documentId}"},
	{"GET", "/onboarding/verifications"}, {"GET", "/onboarding/verifications/{dimension}"}, {"POST", "/onboarding/verifications/{dimension}/submit"}, {"POST", "/onboarding/verifications/{dimension}/resubmit"}, {"GET", "/onboarding/bank-profile"}, {"PUT", "/onboarding/bank-profile"},
	{"POST", "/internal/vendor/v1/verification-events"},
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
	raw, e := os.ReadFile("../../docs/openapi/section7-onboarding-verification.openapi.json")
	if e != nil {
		t.Fatal(e)
	}
	var s spec
	if e = json.Unmarshal(raw, &s); e != nil {
		t.Fatal(e)
	}
	return s
}
func TestBidirectionalRouteParity(t *testing.T) {
	s := load(t)
	documented := map[string]bool{}
	for p, methods := range s.Paths {
		for m := range methods {
			if strings.Contains("get post put patch delete", m) {
				documented[strings.ToUpper(m)+" "+p] = true
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
func TestExactVerificationVocabularies(t *testing.T) {
	s := load(t)
	want := map[string][]string{"VerificationDimension": {"IDENTITY", "BUSINESS", "BANK", "LOCATION", "BRAND", "FOOD_KITCHEN"}, "VerificationStatus": {"NOT_STARTED", "IN_PROGRESS", "PENDING_REVIEW", "VERIFIED", "REJECTED", "EXPIRED", "SUSPENDED"}}
	for name, expected := range want {
		actual := s.Components.Schemas[name].Enum
		if strings.Join(actual, "|") != strings.Join(expected, "|") {
			t.Fatalf("%s=%v want %v", name, actual, expected)
		}
	}
}

func TestInternalCallbackUsesRootLevelServer(t *testing.T) {
	s := load(t)
	raw := s.Paths["/internal/vendor/v1/verification-events"]["post"]
	var op struct {
		Servers []struct {
			URL string `json:"url"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(raw, &op); err != nil {
		t.Fatal(err)
	}
	if len(op.Servers) != 1 || op.Servers[0].URL != "/" {
		t.Fatalf("internal callback servers=%v; want root-level server /", op.Servers)
	}
}
func TestMutationsDeclareSecurityAndIdempotency(t *testing.T) {
	s := load(t)
	for path, methods := range s.Paths {
		for method, raw := range methods {
			if method == "parameters" || method == "get" {
				continue
			}
			var op struct {
				Permission string `json:"x-permission"`
				Parameters []struct {
					Ref  string `json:"$ref"`
					Name string `json:"name"`
				} `json:"parameters"`
				Replay string `json:"x-replay-protection"`
			}
			if e := json.Unmarshal(raw, &op); e != nil {
				t.Fatal(e)
			}
			if op.Permission == "" {
				t.Errorf("%s %s missing permission", method, path)
			}
			if strings.HasPrefix(path, "/internal/") {
				if op.Replay == "" {
					t.Errorf("%s missing replay protection", path)
				}
				continue
			}
			found := false
			for _, p := range op.Parameters {
				if strings.Contains(p.Ref, "IdempotencyKey") || strings.EqualFold(p.Name, "Idempotency-Key") {
					found = true
				}
			}
			if !found {
				t.Errorf("%s %s missing Idempotency-Key", method, path)
			}
		}
	}
}

func TestBankReplacementDeclaresOptimisticVersionAndWriteOnlySecrets(t *testing.T) {
	raw, e := os.ReadFile("../../docs/openapi/section7-onboarding-verification.openapi.json")
	if e != nil {
		t.Fatal(e)
	}
	text := string(raw)
	for _, required := range []string{`"expected_version"`, `"writeOnly": true`} {
		if !strings.Contains(text, required) {
			t.Fatalf("bank replacement contract missing %s", required)
		}
	}
}
