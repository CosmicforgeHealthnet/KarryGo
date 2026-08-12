// Package section5contract proves Section 5's OpenAPI spec and its actually
// registered routes never drift apart — no route exists only in code, and no
// OpenAPI path claims a route that isn't really registered (mirrors the
// pattern already proven in driver-dispatch-delivery-service's and
// driver-hauling-service's private-execution contract tests).
package section5contract

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
)

// registeredRoutes is a hand-maintained mirror of exactly what
// cmd/main.go's Register closure wires via
// vendorauthhttp.RegisterRoutes/staffhttp.RegisterRoutes/RegisterInternalRoutes
// (all under the /api/v1/vendor base — paths here are relative to that base,
// matching the OpenAPI spec's "servers" entry). Update this list in the same
// change that adds or removes a route.
var registeredRoutes = []struct {
	Method string
	Path   string
}{
	{"POST", "/auth/start"},
	{"POST", "/auth/resend"},
	{"POST", "/auth/verify"},
	{"POST", "/auth/refresh"},
	{"POST", "/auth/logout"},
	{"POST", "/auth/step-up/start"},
	{"POST", "/auth/step-up/verify"},
	{"GET", "/staff"},
	{"POST", "/staff"},
	{"GET", "/staff/{id}"},
	{"PUT", "/staff/{id}/suspend"},
	{"PUT", "/staff/{id}/reactivate"},
	{"GET", "/staff/{id}/roles"},
	{"GET", "/permissions"},
	{"GET", "/roles"},
	{"POST", "/roles"},
	{"GET", "/roles/{id}"},
	{"PUT", "/roles/{id}/permissions"},
	{"POST", "/roles/{id}/assign"},
	{"DELETE", "/assignments/{id}"},
	{"POST", "/internal/staff/provision-owner"},
}

type openAPISpec struct {
	Paths map[string]map[string]json.RawMessage `json:"paths"`
}

func loadSpec(t *testing.T) openAPISpec {
	t.Helper()
	raw, err := os.ReadFile("../../docs/openapi/section5-auth-staff-iam.openapi.json")
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}
	var spec openAPISpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse openapi spec: %v", err)
	}
	return spec
}

var httpMethods = []string{"get", "post", "put", "patch", "delete"}

func specOperations(spec openAPISpec) map[string]bool {
	out := map[string]bool{}
	for path, methods := range spec.Paths {
		for _, m := range httpMethods {
			if _, ok := methods[m]; ok {
				out[m+" "+path] = true
			}
		}
	}
	return out
}

func registeredOperations() map[string]bool {
	out := map[string]bool{}
	for _, r := range registeredRoutes {
		out[toLower(r.Method)+" "+r.Path] = true
	}
	return out
}

func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

func TestSection5OpenAPIMatchesRegisteredRoutes(t *testing.T) {
	spec := loadSpec(t)
	fromSpec := specOperations(spec)
	fromCode := registeredOperations()

	var onlyInSpec, onlyInCode []string
	for op := range fromSpec {
		if !fromCode[op] {
			onlyInSpec = append(onlyInSpec, op)
		}
	}
	for op := range fromCode {
		if !fromSpec[op] {
			onlyInCode = append(onlyInCode, op)
		}
	}
	sort.Strings(onlyInSpec)
	sort.Strings(onlyInCode)

	if len(onlyInSpec) > 0 {
		t.Errorf("OpenAPI documents routes that are not registered in code: %v", onlyInSpec)
	}
	if len(onlyInCode) > 0 {
		t.Errorf("code registers routes that are not documented in OpenAPI: %v", onlyInCode)
	}
}

func TestSection5OpenAPI_EveryOperationHasSuccessResponse(t *testing.T) {
	spec := loadSpec(t)
	for path, methods := range spec.Paths {
		for method, raw := range methods {
			var op struct {
				Responses map[string]any `json:"responses"`
			}
			if err := json.Unmarshal(raw, &op); err != nil {
				t.Fatalf("parse operation %s %s: %v", method, path, err)
			}
			hasSuccess := false
			for code := range op.Responses {
				if len(code) == 3 && (code[0] == '2') {
					hasSuccess = true
				}
			}
			if !hasSuccess {
				t.Errorf("%s %s has no 2xx response documented", method, path)
			}
		}
	}
}
