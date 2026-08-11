// Package section8acontract holds the Section 8A OpenAPI/route parity and
// contract-vocabulary guardrails. It mirrors the Section 5/6/7 contract
// packages: the route list here is the declared production surface, and the
// tests fail if the specification and that surface drift apart in either
// direction.
package section8acontract

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

// routes is the exact Section 8A production surface: six vendor-facing
// operations plus one root-mounted internal event ingress.
var routes = []struct{ method, path string }{
	{"GET", "/crm/customers"},
	{"GET", "/crm/customers/{profileId}"},
	{"GET", "/crm/customers/{profileId}/consent"},
	{"GET", "/crm/customers/{profileId}/privacy"},
	{"POST", "/crm/customers/{profileId}/archive"},
	{"POST", "/crm/customers/{profileId}/restore"},
	{"POST", "/internal/vendor/v1/crm/events"},
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
	raw, err := os.ReadFile("../../docs/openapi/section8a-crm-foundation.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var s spec
	if err = json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestBidirectionalRouteParity(t *testing.T) {
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
	for _, route := range routes {
		registered[route.method+" "+route.path] = true
	}

	var onlySpec, onlyCode []string
	for key := range documented {
		if !registered[key] {
			onlySpec = append(onlySpec, key)
		}
	}
	for key := range registered {
		if !documented[key] {
			onlyCode = append(onlyCode, key)
		}
	}
	sort.Strings(onlySpec)
	sort.Strings(onlyCode)
	if len(onlySpec) > 0 {
		t.Errorf("documented but not registered: %v", onlySpec)
	}
	if len(onlyCode) > 0 {
		t.Errorf("registered but not documented: %v", onlyCode)
	}
}

// TestRouteCount pins the Section 8A surface so an accidental addition or
// removal is a deliberate decision rather than a silent drift.
func TestRouteCount(t *testing.T) {
	const publicRoutes, internalRoutes = 6, 1
	if len(routes) != publicRoutes+internalRoutes {
		t.Fatalf("Section 8A route count = %d; want %d public + %d internal",
			len(routes), publicRoutes, internalRoutes)
	}
}

// TestNoAuthorityMutationRoutes is a structural guard against the single most
// dangerous mistake Section 8A could make: exposing a vendor-facing way to
// create a customer, set consent, set privacy state or force a merge.
func TestNoAuthorityMutationRoutes(t *testing.T) {
	forbidden := []string{"merge", "consent/set", "privacy/set", "redact", "unredact", "link"}
	for _, route := range routes {
		if route.method == "GET" {
			continue
		}
		if strings.HasPrefix(route.path, "/internal/") {
			continue
		}
		for _, term := range forbidden {
			if strings.Contains(route.path, term) {
				t.Errorf("vendor-facing authority mutation route %s %s", route.method, route.path)
			}
		}
	}
	// The only vendor-facing writes are the archive/restore lifecycle pair.
	writes := 0
	for _, route := range routes {
		if route.method != "GET" && !strings.HasPrefix(route.path, "/internal/") {
			writes++
		}
	}
	if writes != 2 {
		t.Fatalf("vendor-facing write routes = %d; want exactly 2 (archive, restore)", writes)
	}
	// There must be no vendor-facing customer-creation route.
	for _, route := range routes {
		if route.method == "POST" && route.path == "/crm/customers" {
			t.Fatal("a vendor-facing create-customer route exists; relationships may only be created from an authoritative event")
		}
	}
}

// TestInternalRouteIsRootMounted proves the CRM event ingress is not nested
// under the public API base, repeating the Section 7 callback lesson.
func TestInternalRouteIsRootMounted(t *testing.T) {
	found := false
	for _, route := range routes {
		if !strings.HasPrefix(route.path, "/internal/") {
			continue
		}
		found = true
		if route.path != "/internal/vendor/v1/crm/events" {
			t.Errorf("unexpected internal route %s", route.path)
		}
		if strings.Contains(route.path, "/api/v1/vendor") {
			t.Errorf("internal route %s is mounted below the public API base", route.path)
		}
	}
	if !found {
		t.Fatal("no internal CRM event route is declared")
	}
}

// TestContractVocabularies pins the exact enums the rest of the platform will
// integrate against.
func TestContractVocabularies(t *testing.T) {
	s := load(t)
	expected := map[string][]string{
		"SubjectType":     {"CUSTOMER", "GUEST", "EXTERNAL"},
		"ProfileStatus":   {"ACTIVE", "ARCHIVED"},
		"PrivacyState":    {"ACTIVE", "RESTRICTED", "REDACTED", "MERGED"},
		"MergeState":      {"NONE", "LINK_PENDING", "MERGED_SOURCE", "MERGED_TARGET"},
		"ProjectionState": {"NOT_AVAILABLE", "CURRENT", "STALE", "REBUILDING", "ERROR"},
		"ConsentDecision": {"ALLOW", "DENY", "UNKNOWN"},
		"ConsentChannel":  {"EMAIL", "PUSH", "IN_APP", "SMS"},
		"PrivacyAction":   {"RESTRICT", "REDACT", "MARKETING_SUPPRESS", "IDENTITY_MERGE"},
		"OrderFactState":  {"COMPLETED", "CANCELLED", "REFUNDED", "UPDATED"},
	}
	for name, want := range expected {
		schema, present := s.Components.Schemas[name]
		if !present {
			t.Errorf("schema %s is missing", name)
			continue
		}
		got := append([]string(nil), schema.Enum...)
		sort.Strings(got)
		expectedSorted := append([]string(nil), want...)
		sort.Strings(expectedSorted)
		if strings.Join(got, ",") != strings.Join(expectedSorted, ",") {
			t.Errorf("schema %s enum = %v; want %v", name, schema.Enum, want)
		}
	}
}

// TestConsentDecisionKeepsUnknown is called out separately because collapsing
// UNKNOWN into a two-valued consent flag is exactly the change that would
// silently make marketing fail open.
func TestConsentDecisionKeepsUnknown(t *testing.T) {
	s := load(t)
	schema, present := s.Components.Schemas["ConsentDecision"]
	if !present {
		t.Fatal("ConsentDecision schema is missing")
	}
	var hasUnknown bool
	for _, value := range schema.Enum {
		if value == "UNKNOWN" {
			hasUnknown = true
		}
	}
	if !hasUnknown {
		t.Fatal("ConsentDecision no longer models UNKNOWN; missing consent would become indistinguishable from a decision")
	}
}

// TestPrivacyStateFilterExcludesRedacted proves the documented search contract
// cannot be used to pull redacted relationships into an ordinary view.
func TestPrivacyStateFilterExcludesRedacted(t *testing.T) {
	raw, err := os.ReadFile("../../docs/openapi/section8a-crm-foundation.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Components struct {
			Parameters map[string]struct {
				Schema struct {
					Enum []string `json:"enum"`
				} `json:"schema"`
			} `json:"parameters"`
		} `json:"components"`
	}
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	filter, present := document.Components.Parameters["PrivacyStateFilter"]
	if !present {
		t.Fatal("PrivacyStateFilter parameter is missing")
	}
	for _, value := range filter.Schema.Enum {
		if value == "REDACTED" || value == "MERGED" {
			t.Fatalf("privacy_state filter accepts %s, which would expose relationships excluded from ordinary search", value)
		}
	}
}
