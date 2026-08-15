// Package section8bcontract holds the Section 8B OpenAPI/route parity and
// contract-vocabulary guardrails. It mirrors the Section 5/6/7/8A contract
// packages: the route list here is the declared production surface, and the
// tests fail if the specification and that surface drift apart in either
// direction.
package section8bcontract

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

// routes is the exact Section 8B production surface: twenty-one vendor-facing
// operations and no internal ones. Section 8B introduces no authority callback
// because it consumes no external authority — every input it acts on already
// belongs to Vendor Service.
var routes = []struct{ method, path string }{
	{"GET", "/crm/segment-fields"},

	{"GET", "/crm/tags"},
	{"POST", "/crm/tags"},
	{"POST", "/crm/tags/bulk-assign"},
	{"POST", "/crm/tags/bulk-remove"},
	{"GET", "/crm/tags/{tagId}"},
	{"PATCH", "/crm/tags/{tagId}"},
	{"POST", "/crm/tags/{tagId}/archive"},

	{"GET", "/crm/segments"},
	{"POST", "/crm/segments"},
	{"POST", "/crm/segments/preview"},
	{"GET", "/crm/segments/{segmentId}"},
	{"PATCH", "/crm/segments/{segmentId}"},
	{"POST", "/crm/segments/{segmentId}/activate"},
	{"POST", "/crm/segments/{segmentId}/archive"},
	{"POST", "/crm/segments/{segmentId}/materialize"},
	{"GET", "/crm/segments/{segmentId}/members"},
	{"POST", "/crm/segments/{segmentId}/members/bulk-add"},
	{"POST", "/crm/segments/{segmentId}/members/bulk-remove"},
	{"POST", "/crm/segments/{segmentId}/exclusions/bulk-add"},
	{"POST", "/crm/segments/{segmentId}/exclusions/bulk-remove"},
}

const specPath = "../../docs/openapi/section8b-crm-tags-segmentation.openapi.json"

type operation struct {
	OperationID string `json:"operationId"`
	Permission  string `json:"x-permission"`
	Parameters  []struct {
		Ref  string `json:"$ref"`
		Name string `json:"name"`
	} `json:"parameters"`
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
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	var s spec
	if err = json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func isMethod(name string) bool {
	switch name {
	case "get", "post", "put", "patch", "delete":
		return true
	}
	return false
}

func TestBidirectionalRouteParity(t *testing.T) {
	s := load(t)
	documented := map[string]bool{}
	for path, methods := range s.Paths {
		for method := range methods {
			if isMethod(method) {
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

// TestRouteCount pins the Section 8B surface so an accidental addition or
// removal is a deliberate decision rather than a silent drift.
func TestRouteCount(t *testing.T) {
	const publicRoutes, internalRoutes = 21, 0
	if len(routes) != publicRoutes+internalRoutes {
		t.Fatalf("Section 8B route count = %d; want %d public + %d internal",
			len(routes), publicRoutes, internalRoutes)
	}
	for _, route := range routes {
		if strings.HasPrefix(route.path, "/internal/") {
			t.Errorf("Section 8B declares an internal route %s; it consumes no external authority and needs none", route.path)
		}
	}
}

// TestEveryOperationDeclaresAPermission keeps an endpoint from shipping with no
// authorization documented — the most likely way an unguarded route reaches
// production unnoticed.
func TestEveryOperationDeclaresAPermission(t *testing.T) {
	s := load(t)
	for path, methods := range s.Paths {
		for method, raw := range methods {
			if !isMethod(method) {
				continue
			}
			var op operation
			if err := json.Unmarshal(raw, &op); err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
			if op.OperationID == "" {
				t.Errorf("%s %s has no operationId", strings.ToUpper(method), path)
			}
			if !strings.HasPrefix(op.Permission, "vendor.crm.") {
				t.Errorf("%s %s declares permission %q; every Section 8B operation must name a vendor.crm.* permission",
					strings.ToUpper(method), path, op.Permission)
			}
		}
	}
}

// TestCommandsRequireIdempotencyKey pins the command convention Sections 6 and 7
// already follow, so a retried bulk tag or segment creation replays instead of
// repeating.
func TestCommandsRequireIdempotencyKey(t *testing.T) {
	s := load(t)
	for path, methods := range s.Paths {
		for method, raw := range methods {
			if method != "post" && method != "patch" {
				continue
			}
			var op operation
			if err := json.Unmarshal(raw, &op); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, parameter := range op.Parameters {
				if strings.HasSuffix(parameter.Ref, "/IdempotencyKey") || parameter.Name == "Idempotency-Key" {
					found = true
				}
			}
			if !found {
				t.Errorf("%s %s is a command but does not declare Idempotency-Key", strings.ToUpper(method), path)
			}
		}
	}
}

// TestNoCampaignOrLaterSectionRoutes is a structural guard against Section 8B
// quietly growing into 8C-8F. Producing an audience and contacting it are
// different authorities, and the split has to be visible in the surface.
func TestNoCampaignOrLaterSectionRoutes(t *testing.T) {
	forbidden := []string{
		"campaign", "send", "message", "notification", "broadcast",
		"interaction", "task", "loyalty", "corporate", "export", "score", "rfm", "churn",
	}
	for _, route := range routes {
		lowered := strings.ToLower(route.path)
		for _, term := range forbidden {
			if strings.Contains(lowered, term) {
				t.Errorf("route %s %s names %q, which belongs to a later CRM section", route.method, route.path, term)
			}
		}
	}
}

// TestContractVocabularies pins the exact enums the rest of the platform, and
// any rule builder, will integrate against.
func TestContractVocabularies(t *testing.T) {
	s := load(t)
	expected := map[string][]string{
		"ScopeType":           {"BUSINESS", "BRANCH"},
		"TagStatus":           {"ACTIVE", "ARCHIVED"},
		"SegmentType":         {"STATIC", "DYNAMIC"},
		"SegmentStatus":       {"DRAFT", "ACTIVE", "ARCHIVED"},
		"EvaluationStatus":    {"NOT_EVALUATED", "QUEUED", "RUNNING", "CURRENT", "STALE", "FAILED"},
		"EvaluationRunStatus": {"QUEUED", "RUNNING", "SUCCESS", "FAILED", "DEAD_LETTER", "SUPERSEDED"},
		"MembershipSource":    {"STATIC", "DYNAMIC"},
		"CountState":          {"EXACT", "AT_LEAST"},
		"FieldType":           {"BOOLEAN", "INTEGER", "MONEY_MINOR", "ENUM", "TIMESTAMP", "TAG_REFERENCE"},
		"FieldSource":         {"PROFILE", "COMMERCIAL", "RELATIONSHIP", "TAGS", "CONSENT"},
		"SegmentOperator": {
			"EQ", "NEQ", "IN", "NOT_IN", "GT", "GTE", "LT", "LTE", "BETWEEN",
			"EXISTS", "NOT_EXISTS", "BEFORE", "AFTER", "WITHIN_LAST", "HAS_TAG", "NOT_HAS_TAG",
		},
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

// TestRuleSchemaIsFullyDocumented refuses the lazy `object: {}` rule shape. A
// builder that cannot read the grammar from the contract will invent one.
func TestRuleSchemaIsFullyDocumented(t *testing.T) {
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"RuleDocument", "RuleNode", "RuleGroup", "RuleCondition", "RuleValue",
		"MoneyValue", "DurationWindow", "TagReferenceValue", "RuleLimits", "SegmentField",
	} {
		body, present := document.Components.Schemas[required]
		if !present {
			t.Errorf("rule schema %s is missing", required)
			continue
		}
		if len(body) < 40 {
			t.Errorf("rule schema %s is a stub: %s", required, body)
		}
	}

	// The group schema must be recursive: a rule language documented as one
	// level deep is not the language the server accepts.
	group, present := document.Components.Schemas["RuleGroup"]
	if !present || !strings.Contains(string(group), "#/components/schemas/RuleNode") {
		t.Fatal("RuleGroup does not recurse into RuleNode; the documented grammar is not the real one")
	}
}

// TestSpecStatesSQLIsUnsupported keeps the single most important property of
// this API — that it is not a query interface — stated in the contract itself,
// not just in the implementation.
func TestSpecStatesSQLIsUnsupported(t *testing.T) {
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, phrase := range []string{"raw SQL", "field key", "bind parameter"} {
		if !strings.Contains(body, phrase) {
			t.Errorf("the specification never mentions %q; the rule-safety contract must be stated, not implied", phrase)
		}
	}
}

// TestErrorCodesAreEnumerated makes the stable machine codes part of the
// contract rather than something a client discovers by triggering failures.
func TestErrorCodesAreEnumerated(t *testing.T) {
	s := load(t)
	schema, present := s.Components.Schemas["SegmentErrorCode"]
	if !present {
		t.Fatal("SegmentErrorCode is missing")
	}
	required := []string{
		"CRM_SEGMENT_RULE_INVALID", "CRM_SEGMENT_FIELD_NOT_SUPPORTED",
		"CRM_SEGMENT_OPERATOR_NOT_SUPPORTED", "CRM_SEGMENT_VALUE_INVALID",
		"CRM_SEGMENT_TOO_COMPLEX", "CRM_SEGMENT_SCOPE_DENIED",
		"CRM_PROFILE_REDACTED", "CRM_PROFILE_MERGED", "CRM_VERSION_CONFLICT",
		"CRM_BULK_OPERATION_TOO_LARGE", "CRM_SEGMENTATION_DISABLED",
	}
	for _, code := range required {
		found := false
		for _, value := range schema.Enum {
			if value == code {
				found = true
			}
		}
		if !found {
			t.Errorf("error code %s is not documented", code)
		}
	}
}
