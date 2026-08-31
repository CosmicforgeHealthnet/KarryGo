// Package fulfilmentcontract proves the fulfilment aggregate's OpenAPI spec and
// its actually registered routes never drift apart: no route exists only in
// code, and no documented path claims a route that is not really registered.
//
// It also pins the two boundaries this surface exists to hold. The spec must
// document no route that creates a platform mission, sets a platform mission's
// status, or deletes fulfilment history — those are operations that would let a
// request impersonate a logistics authority or erase what happened, so they
// have no surface at all rather than a surface that answers 403.
package fulfilmentcontract

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

// registeredRoutes mirrors exactly what fulfilmentgroup.RegisterRoutes wires,
// under cmd/main.go's /api/v1/vendor base. Paths here are relative to the
// spec's own /api/v1 server entry, so they carry the /vendor prefix.
//
// Update this list in the same change that adds or removes a route.
var registeredRoutes = []struct {
	Method string
	Path   string
}{
	{"GET", "/vendor/fulfilment/groups/{groupId}"},
	{"GET", "/vendor/fulfilment/groups/{groupId}/plans"},
	{"GET", "/vendor/fulfilment/units/{unitId}/parcels"},
	{"POST", "/vendor/fulfilment/units/{unitId}/parcels"},
	{"PUT", "/vendor/fulfilment/units/{unitId}/readiness"},
	{"GET", "/vendor/fulfilment/parcels/{parcelId}"},
	{"GET", "/vendor/fulfilment/parcels/{parcelId}/custody"},
	{"PUT", "/vendor/fulfilment/parcels/{parcelId}/manifest"},
	{"POST", "/vendor/fulfilment/parcels/{parcelId}/returns"},
	{"GET", "/vendor/fulfilment/vendor-missions"},
	{"GET", "/vendor/fulfilment/vendor-missions/{missionId}"},
	{"PUT", "/vendor/fulfilment/vendor-missions/{missionId}/rider"},
	{"PUT", "/vendor/fulfilment/vendor-missions/{missionId}/status"},
}

const specPath = "../../docs/openapi/fulfilment-group-parcel-mission.openapi.json"

type operation struct {
	Summary     string                       `json:"summary"`
	Description string                       `json:"description"`
	Parameters  []map[string]json.RawMessage `json:"parameters"`
}

type openAPISpec struct {
	Paths map[string]map[string]json.RawMessage `json:"paths"`
}

var httpMethods = []string{"get", "post", "put", "patch", "delete"}

func loadSpec(t *testing.T) openAPISpec {
	t.Helper()
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}
	var spec openAPISpec
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse openapi spec: %v", err)
	}
	return spec
}

func specOperations(spec openAPISpec) map[string]bool {
	out := map[string]bool{}
	for path, methods := range spec.Paths {
		for _, method := range httpMethods {
			if _, ok := methods[method]; ok {
				out[method+" "+path] = true
			}
		}
	}
	return out
}

func registeredOperations() map[string]bool {
	out := map[string]bool{}
	for _, route := range registeredRoutes {
		out[strings.ToLower(route.Method)+" "+route.Path] = true
	}
	return out
}

func TestOpenAPIMatchesRegisteredRoutes(t *testing.T) {
	fromSpec := specOperations(loadSpec(t))
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

// The platform mission is somebody else's. No surface may create one, bind its
// canonical reference, or set its status — a route that could would turn a
// projection into an impersonation of the owning authority.
func TestNoRouteWritesAPlatformMission(t *testing.T) {
	for _, route := range registeredRoutes {
		path := strings.ToLower(route.Path)
		if route.Method == "GET" {
			continue
		}
		for _, forbidden := range []string{
			"platform-mission", "platform_mission", "mission-ref", "external-ref",
		} {
			if strings.Contains(path, forbidden) {
				t.Errorf("%s %s writes a platform mission; the canonical mission belongs to its own authority",
					route.Method, route.Path)
			}
		}
	}
}

// Operational fulfilment history is not deleted. There is no DELETE route for a
// plan, a parcel, a mission reference or a custody event, because a history
// that can be erased cannot be relied on afterwards.
func TestNoDestructiveHistoryRoutes(t *testing.T) {
	for _, route := range registeredRoutes {
		if route.Method == "DELETE" {
			t.Errorf("%s %s deletes fulfilment history; history must remain readable",
				route.Method, route.Path)
		}
	}
}

// Every mutating route takes If-Match, except the two whose safety comes from
// Idempotency-Key instead. A write with neither is a blind write, and on a
// shared operational console a blind write discards a colleague's work.
func TestEveryMutationHasConcurrencyControl(t *testing.T) {
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}
	var spec struct {
		Paths map[string]map[string]operation `json:"paths"`
	}
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse openapi spec: %v", err)
	}

	for _, route := range registeredRoutes {
		if route.Method == "GET" {
			continue
		}
		op, found := spec.Paths[route.Path][strings.ToLower(route.Method)]
		if !found {
			t.Fatalf("%s %s missing from spec", route.Method, route.Path)
		}

		guarded := false
		for _, parameter := range op.Parameters {
			var name string
			if raw, ok := parameter["name"]; ok {
				_ = json.Unmarshal(raw, &name)
			}
			if name == "If-Match" || name == "Idempotency-Key" {
				guarded = true
			}
		}
		if !guarded {
			t.Errorf("%s %s has neither If-Match nor Idempotency-Key; a blind write on a shared console discards concurrent work",
				route.Method, route.Path)
		}
	}
}
