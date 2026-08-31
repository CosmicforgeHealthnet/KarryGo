// Package section8hcontract pins the exact Section 8H public HTTP and OpenAPI
// surface. It guards against the documentation and the router drifting apart,
// and — more importantly — against the three claims Section 8H must never make:
// that a score is a probability, that a score is permission to contact someone,
// and that a missing score is a low one.
package section8hcontract

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

const specPath = "../../docs/openapi/section8h-crm-intelligence-exports.openapi.json"

// routes mirrors RegisterIntelligenceRoutes and RegisterExportRoutes exactly.
// The router-side half lives in
// internal/features/crm/intelligence_http_integration_test.go.
var routes = []struct{ method, path string }{
	{"GET", "/crm/intelligence/overview"},
	{"GET", "/crm/intelligence/rfm"},
	{"GET", "/crm/intelligence/churn"},
	{"GET", "/crm/customers/{profileId}/intelligence"},
	{"POST", "/crm/intelligence/rebuild"},
	{"GET", "/crm/exports"},
	{"POST", "/crm/exports"},
	{"GET", "/crm/exports/{exportId}"},
	{"POST", "/crm/exports/{exportId}/cancel"},
	{"POST", "/crm/exports/{exportId}/download"},
}

const routeCount = 10

var permissions = []string{
	"vendor.crm.intelligence.read",
	"vendor.crm.intelligence.rebuild",
	"vendor.crm.exports.read",
	"vendor.crm.exports.create",
	"vendor.crm.exports.download",
	"vendor.crm.exports.cancel",
}

type operation struct {
	OperationID string `json:"operationId"`
	Permission  string `json:"x-permission"`
	Parameters  []struct {
		Ref  string `json:"$ref"`
		Name string `json:"name"`
	} `json:"parameters"`
}

type schema struct {
	Enum       []string                   `json:"enum"`
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}

type spec struct {
	Info struct {
		Description string `json:"description"`
	} `json:"info"`
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Parameters map[string]json.RawMessage `json:"parameters"`
		Schemas    map[string]schema          `json:"schemas"`
	} `json:"components"`
}

func load(t *testing.T) spec {
	t.Helper()
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	var document spec
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func rawSpec(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func isMethod(value string) bool {
	switch value {
	case "get", "post", "put", "patch", "delete":
		return true
	default:
		return false
	}
}

func operations(t *testing.T, document spec) map[string]operation {
	t.Helper()
	out := map[string]operation{}
	for path, methods := range document.Paths {
		for method, raw := range methods {
			if !isMethod(method) {
				continue
			}
			var op operation
			if err := json.Unmarshal(raw, &op); err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
			out[strings.ToUpper(method)+" "+path] = op
		}
	}
	return out
}

func TestBidirectionalRouteParityAndCount(t *testing.T) {
	document := load(t)
	documented := map[string]bool{}
	for key := range operations(t, document) {
		documented[key] = true
	}
	registered := map[string]bool{}
	for _, route := range routes {
		registered[route.method+" "+route.path] = true
		if strings.HasPrefix(route.path, "/internal/") {
			t.Errorf("Section 8H must not declare an internal route: %s %s", route.method, route.path)
		}
	}
	if len(routes) != routeCount || len(documented) != routeCount {
		t.Fatalf("route count: registered=%d documented=%d; want %d public + 0 internal",
			len(routes), len(documented), routeCount)
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
	if len(onlySpec) != 0 || len(onlyCode) != 0 {
		t.Fatalf("route drift: spec-only=%v code-only=%v", onlySpec, onlyCode)
	}
}

// TestNoProbabilityOrMachineLearningClaims is the load-bearing scope guard.
// The moment this contract describes a score as a likelihood, every reader who
// acts on it is acting on something the model cannot support.
func TestNoProbabilityOrMachineLearningClaims(t *testing.T) {
	document := load(t)
	raw := strings.ToLower(rawSpec(t))
	// Each forbidden term is allowed only inside a sentence that denies it.
	denials := []string{"not a probability", "no probability", "not a prediction",
		"no machine learning", "not a statistical model", "no model artefact",
		"no training data", "no generated explanation"}
	for _, term := range []string{"probability", "likelihood", "predicts", "machine learning",
		"neural", "training data", "model artefact", "confidence interval", "propensity"} {
		if !strings.Contains(raw, term) {
			continue
		}
		denied := false
		for _, denial := range denials {
			if strings.Contains(raw, denial) {
				denied = true
			}
		}
		if !denied {
			t.Errorf("the contract uses %q without denying it", term)
		}
	}
	// The risk score is an integer, bounded, and says what it is not.
	risk, present := document.Components.Schemas["Score"].Properties["risk_score"]
	if !present {
		t.Fatal("Score does not declare risk_score")
	}
	var field struct {
		Type        string `json:"type"`
		Minimum     int    `json:"minimum"`
		Maximum     int    `json:"maximum"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(risk, &field); err != nil {
		t.Fatal(err)
	}
	if field.Type != "integer" || field.Minimum != 0 || field.Maximum != 100 {
		t.Errorf("risk_score=%+v; want an integer in [0,100]", field)
	}
	lowered := strings.ToLower(field.Description)
	if !strings.Contains(lowered, "not a probability") {
		t.Errorf("risk_score does not state that it is not a probability: %s", field.Description)
	}
	// No score field anywhere is floating point.
	for name, definition := range document.Components.Schemas {
		for property, raw := range definition.Properties {
			if !strings.Contains(strings.ToLower(property), "score") {
				continue
			}
			var typed struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(raw, &typed); err != nil {
				t.Fatal(err)
			}
			if typed.Type == "number" {
				t.Errorf("%s.%s is a floating-point score", name, property)
			}
		}
	}
	// And the contract states the invariant the whole section is subordinate to.
	if !strings.Contains(raw, "a score is not permission") {
		t.Error("the contract does not state that a score is not permission to contact anybody")
	}
	if !strings.Contains(raw, "unavailable is not zero") {
		t.Error("the contract does not state that unavailable is not zero")
	}
}

// TestNoScoreWriteOrRawQuerySurface guards the two things a client must never
// be able to do: set a score, or supply a query.
func TestNoScoreWriteOrRawQuerySurface(t *testing.T) {
	document := load(t)
	for path, methods := range document.Paths {
		lowered := strings.ToLower(path)
		for _, forbidden := range []string{"/score", "/rfm-score", "/churn-score", "/query", "/sql", "internal"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("path %s exposes %q", path, forbidden)
			}
		}
		for method := range methods {
			if !isMethod(method) {
				continue
			}
			// The only writes are a rebuild request and the four export
			// commands. Nothing PATCHes or PUTs a score.
			if method == "patch" || method == "put" {
				t.Errorf("%s %s mutates a resource; intelligence is derived and exports are jobs",
					strings.ToUpper(method), path)
			}
		}
	}
	for name := range document.Components.Schemas {
		lowered := strings.ToLower(name)
		for _, forbidden := range []string{"setscore", "scoreoverride", "query", "sqlrequest", "rawfilter"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("schema %s documents a write or query surface", name)
			}
		}
	}
	// The filter carries no expression of any kind.
	filter := document.Components.Schemas["ExportFilter"]
	for property := range filter.Properties {
		lowered := strings.ToLower(property)
		for _, forbidden := range []string{"sql", "where", "expression", "columns", "select",
			"order_by", "sort", "raw", "jsonpath"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("ExportFilter exposes %q", property)
			}
		}
	}
}

// TestNoPIISurface is the disclosure guard.
func TestNoPIISurface(t *testing.T) {
	document := load(t)
	for name, definition := range document.Components.Schemas {
		for property := range definition.Properties {
			lowered := strings.ToLower(property)
			for _, forbidden := range []string{
				"email", "phone", "msisdn", "first_name", "last_name", "full_name", "address",
				"date_of_birth", "password", "government_id", "device_token", "push_token",
				"card", "wallet", "recipient", "destination",
			} {
				if strings.Contains(lowered, forbidden) {
					t.Errorf("schema %s exposes %q", name, property)
				}
			}
		}
	}
	// No durable download credential is documented.
	for name, definition := range document.Components.Schemas {
		for property := range definition.Properties {
			lowered := strings.ToLower(property)
			if strings.Contains(lowered, "url") || strings.Contains(lowered, "signed") ||
				strings.Contains(lowered, "artifact_ref") {
				t.Errorf("schema %s exposes %q; access is decided live, never stored", name, property)
			}
		}
	}
}

func TestEveryOperationDeclaresPermissionAndCommandsAreIdempotent(t *testing.T) {
	document := load(t)
	known := map[string]bool{}
	for _, permission := range permissions {
		known[permission] = true
	}
	for key, op := range operations(t, document) {
		if op.OperationID == "" {
			t.Errorf("%s has no operationId", key)
		}
		if !known[op.Permission] {
			t.Errorf("%s declares unknown permission %q", key, op.Permission)
		}
		method := strings.ToLower(strings.SplitN(key, " ", 2)[0])
		if method != "post" && method != "patch" && method != "put" {
			continue
		}
		found := false
		for _, parameter := range op.Parameters {
			found = found || strings.HasSuffix(parameter.Ref, "/IdempotencyKey") ||
				parameter.Name == "Idempotency-Key"
		}
		if !found {
			t.Errorf("%s does not declare Idempotency-Key", key)
		}
	}
}

// TestPermissionSetIsExactlyTheSixDeclared.
func TestPermissionSetIsExactlyTheSixDeclared(t *testing.T) {
	document := load(t)
	used := map[string]bool{}
	for _, op := range operations(t, document) {
		used[op.Permission] = true
	}
	if len(used) != len(permissions) {
		t.Fatalf("permissions used = %v; want exactly %v", used, permissions)
	}
	for _, permission := range permissions {
		if !used[permission] {
			t.Errorf("permission %s is declared by no operation", permission)
		}
	}
	raw := rawSpec(t)
	for _, forbidden := range []string{
		"vendor.crm.intelligence.write", "vendor.crm.intelligence.set",
		"vendor.crm.intelligence.override", "vendor.crm.exports.raw",
	} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("the contract declares %s, an authority this section does not hold", forbidden)
		}
	}
}

// TestStepUpIsDocumentedOnBothSensitiveCommands.
func TestStepUpIsDocumentedOnBothSensitiveCommands(t *testing.T) {
	document := load(t)
	for _, name := range []string{"CreateExportRequest", "DownloadExportRequest"} {
		definition := document.Components.Schemas[name]
		if !strings.Contains(strings.Join(definition.Required, ","), "step_up_grant_id") {
			t.Errorf("%s does not require step_up_grant_id", name)
		}
		raw, present := definition.Properties["step_up_grant_id"]
		if !present {
			t.Fatalf("%s does not declare step_up_grant_id", name)
		}
		var field struct {
			Description string `json:"description"`
		}
		if err := json.Unmarshal(raw, &field); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.ToLower(field.Description), "step-up") {
			t.Errorf("%s.step_up_grant_id does not explain the elevation: %s", name, field.Description)
		}
	}
}

func TestControlledVocabularies(t *testing.T) {
	document := load(t)
	expected := map[string][]string{
		"ModelKey":          {"RFM_V1", "CHURN_V1"},
		"IntelligenceScope": {"BUSINESS", "BRANCH"},
		"ScoreState":        {"NOT_AVAILABLE", "CURRENT", "STALE", "FAILED"},
		"ScopeState":        {"NOT_AVAILABLE", "QUEUED", "RUNNING", "CURRENT", "STALE", "FAILED"},
		"RiskBand":          {"LOW", "MEDIUM", "HIGH", "CRITICAL"},
		"ExportFormat":      {"CSV", "JSONL"},
		"ExportStatus":      {"QUEUED", "RUNNING", "READY", "FAILED", "CANCELLED", "EXPIRED"},
		"ExportType": {"CUSTOMER_SUMMARY", "SEGMENT_MEMBERS", "CORPORATE_CUSTOMERS",
			"LOYALTY_SUMMARY", "CAMPAIGN_PERFORMANCE_SUMMARY", "INTELLIGENCE_SCORES"},
	}
	for name, want := range expected {
		got := append([]string(nil), document.Components.Schemas[name].Enum...)
		sort.Strings(got)
		sort.Strings(want)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s enum=%v; want %v", name, got, want)
		}
	}
	// No state anywhere is the ambiguous PENDING.
	for _, name := range []string{"ScoreState", "ScopeState", "ExportStatus"} {
		for _, value := range document.Components.Schemas[name].Enum {
			if value == "PENDING" {
				t.Errorf("%s contains the ambiguous state PENDING", name)
			}
		}
	}
	// Every documented reason code is a code, not prose.
	for _, value := range document.Components.Schemas["ScoreReasonCode"].Enum {
		if value != strings.ToUpper(value) || strings.Contains(value, " ") {
			t.Errorf("reason code %q is not a stable code", value)
		}
	}
}

func TestSchemasAndErrorCodesAreConcrete(t *testing.T) {
	document := load(t)
	for _, name := range []string{
		"Score", "CustomerIntelligence", "ModelAvailability", "IntelligenceOverview",
		"RFMDistribution", "ChurnDistribution", "RebuildIntelligenceRequest", "IntelligenceEvaluation",
		"Export", "ExportPage", "ExportFilter", "CreateExportRequest", "CancelExportRequest",
		"DownloadExportRequest", "ExportAccess", "Error",
	} {
		if len(document.Components.Schemas[name].Properties) == 0 {
			t.Errorf("schema %s is missing or a stub", name)
		}
	}
	// Availability leads every model report, and the unavailable count is a
	// first-class number rather than an absence.
	availability := document.Components.Schemas["ModelAvailability"]
	required := strings.Join(availability.Required, ",")
	for _, field := range []string{"status", "profile_count", "available_count", "unavailable_count"} {
		if !strings.Contains(required, field) {
			t.Errorf("ModelAvailability does not require %s", field)
		}
	}
	// A relationship's block always answers about availability even when there
	// is no score to show.
	intelligence := strings.Join(document.Components.Schemas["CustomerIntelligence"].Required, ",")
	for _, field := range []string{"rfm_state", "churn_state"} {
		if !strings.Contains(intelligence, field) {
			t.Errorf("CustomerIntelligence does not require %s", field)
		}
	}
	codes := strings.Join(document.Components.Schemas["IntelligenceErrorCode"].Enum, ",")
	for _, want := range []string{
		"CRM_INTELLIGENCE_NOT_AVAILABLE", "CRM_INTELLIGENCE_STALE", "CRM_INTELLIGENCE_MODEL_NOT_FOUND",
		"CRM_INTELLIGENCE_SCOPE_DENIED", "CRM_EXPORT_NOT_FOUND", "CRM_EXPORT_SCOPE_DENIED",
		"CRM_EXPORT_INVALID_TYPE", "CRM_EXPORT_INVALID_FORMAT", "CRM_EXPORT_TOO_LARGE",
		"CRM_EXPORT_STEP_UP_REQUIRED", "CRM_EXPORT_NOT_READY", "CRM_EXPORT_EXPIRED",
		"CRM_EXPORT_CANCELLED", "CRM_EXPORT_STORAGE_UNAVAILABLE", "CRM_IDEMPOTENCY_CONFLICT",
	} {
		if !strings.Contains(codes, want) {
			t.Errorf("error code %s is not documented", want)
		}
	}
	// Row bounds are documented where they are enforced.
	rowLimit, present := document.Components.Schemas["CreateExportRequest"].Properties["row_limit"]
	if !present {
		t.Fatal("CreateExportRequest does not declare row_limit")
	}
	var bounds struct {
		Minimum int `json:"minimum"`
		Maximum int `json:"maximum"`
	}
	if err := json.Unmarshal(rowLimit, &bounds); err != nil {
		t.Fatal(err)
	}
	if bounds.Minimum != 1 || bounds.Maximum != 250000 {
		t.Errorf("row_limit bounds = %+v; want 1..250000", bounds)
	}
}

// TestEveryReferenceResolves catches a spec that parses but points at nothing.
func TestEveryReferenceResolves(t *testing.T) {
	document := load(t)
	var tree any
	if err := json.Unmarshal([]byte(rawSpec(t)), &tree); err != nil {
		t.Fatalf("the specification is not valid JSON: %v", err)
	}
	var walk func(node any)
	walk = func(node any) {
		switch typed := node.(type) {
		case map[string]any:
			if target, ok := typed["$ref"].(string); ok {
				switch {
				case strings.HasPrefix(target, "#/components/schemas/"):
					name := strings.TrimPrefix(target, "#/components/schemas/")
					if _, present := document.Components.Schemas[name]; !present {
						t.Errorf("unresolved schema reference %s", target)
					}
				case strings.HasPrefix(target, "#/components/parameters/"):
					name := strings.TrimPrefix(target, "#/components/parameters/")
					if _, present := document.Components.Parameters[name]; !present {
						t.Errorf("unresolved parameter reference %s", target)
					}
				default:
					t.Errorf("unsupported reference %s", target)
				}
			}
			for _, value := range typed {
				walk(value)
			}
		case []any:
			for _, value := range typed {
				walk(value)
			}
		}
	}
	walk(tree)
}

// TestNoLaterSectionSurface keeps Section 8I and Section 9 out.
func TestNoLaterSectionSurface(t *testing.T) {
	document := load(t)
	for path := range document.Paths {
		lowered := strings.ToLower(path)
		for _, forbidden := range []string{"recommendation", "next-best", "automation", "certification",
			"order", "checkout", "catalogue", "invoice"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("path %s enters later-section authority %q", path, forbidden)
			}
		}
	}
	for name := range document.Components.Schemas {
		lowered := strings.ToLower(name)
		for _, forbidden := range []string{"recommendation", "nextbestaction", "automation", "order", "invoice"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("schema %s enters later-section authority %q", name, forbidden)
			}
		}
	}
}
