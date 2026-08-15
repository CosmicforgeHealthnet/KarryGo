// Package section8econtract pins the exact Section 8E public HTTP and OpenAPI
// surface. It is the guard against the documentation and the router drifting
// apart, and — more importantly — against Section 8E quietly growing an
// endpoint, a schema or a state that would let it schedule or deliver a
// message. That boundary is the whole point of the section, and a boundary
// nothing enforces is a boundary that moves.
package section8econtract

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

const specPath = "../../docs/openapi/section8e-crm-campaign-intelligence.openapi.json"

// routes mirrors RegisterCampaignRoutes exactly. The router-side half of this
// parity check lives in internal/features/crm/campaign_http_integration_test.go,
// which asserts the live gin engine registers precisely these ten and nothing
// else.
var routes = []struct{ method, path string }{
	{"GET", "/crm/campaigns"},
	{"POST", "/crm/campaigns"},
	{"GET", "/crm/campaigns/{campaignId}"},
	{"PATCH", "/crm/campaigns/{campaignId}"},
	{"POST", "/crm/campaigns/{campaignId}/archive"},
	{"POST", "/crm/campaigns/{campaignId}/preview"},
	{"POST", "/crm/campaigns/{campaignId}/prepare"},
	{"GET", "/crm/campaigns/{campaignId}/snapshots"},
	{"GET", "/crm/campaigns/{campaignId}/snapshots/{snapshotId}"},
	{"GET", "/crm/campaigns/{campaignId}/snapshots/{snapshotId}/targets"},
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
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Schemas map[string]schema `json:"schemas"`
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

func TestBidirectionalRouteParityAndCount(t *testing.T) {
	document := load(t)
	documented := map[string]bool{}
	for path, methods := range document.Paths {
		for method := range methods {
			if isMethod(method) {
				documented[strings.ToUpper(method)+" "+path] = true
			}
		}
	}
	registered := map[string]bool{}
	for _, route := range routes {
		registered[route.method+" "+route.path] = true
		if strings.HasPrefix(route.path, "/internal/") {
			t.Errorf("Section 8E must not declare an internal route: %s %s", route.method, route.path)
		}
	}
	if len(routes) != 10 || len(documented) != 10 {
		t.Fatalf("route count: registered=%d documented=%d; want 10 public + 0 internal", len(routes), len(documented))
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

// TestNoDeliverySurface is the load-bearing scope guard. Section 8E prepares an
// audience; Section 8F executes. A path, schema or property here that described
// a send, a schedule or a destination would mean the two had merged.
func TestNoDeliverySurface(t *testing.T) {
	document := load(t)
	for path := range document.Paths {
		lowered := strings.ToLower(path)
		for _, forbidden := range []string{"send", "schedule", "deliver", "dispatch", "notification",
			"template", "message", "internal"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("path %s exposes %q; Section 8E publishes no delivery route", path, forbidden)
			}
		}
	}
	for name := range document.Components.Schemas {
		lowered := strings.ToLower(name)
		for _, forbidden := range []string{"sendrequest", "schedulerequest", "delivery", "notification",
			"template", "messagebody"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("schema %s documents delivery machinery that belongs to Section 8F", name)
			}
		}
	}
	// The prepared-audience vocabulary must never claim authority to send.
	raw := rawSpec(t)
	if strings.Contains(raw, "AUTHORIZED_TO_SEND") {
		t.Error("the contract declares AUTHORIZED_TO_SEND; Section 8E cannot authorise a future delivery")
	}
	for _, forbidden := range []string{"\"campaign.sent", "\"notification.sent", "\"sms.sent", "\"email.sent"} {
		if strings.Contains(strings.ToLower(raw), forbidden) {
			t.Errorf("the specification references %q; Section 8E emits no delivery event", forbidden)
		}
	}
}

func TestEveryOperationDeclaresPermissionAndCommandsAreIdempotent(t *testing.T) {
	document := load(t)
	// Preview is a POST because it carries a body, not because it changes
	// state: it writes nothing, so it neither needs nor declares a key.
	readOnlyPosts := map[string]bool{"/crm/campaigns/{campaignId}/preview": true}

	for path, methods := range document.Paths {
		for method, raw := range methods {
			if !isMethod(method) {
				continue
			}
			var op operation
			if err := json.Unmarshal(raw, &op); err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
			if op.OperationID == "" || !strings.HasPrefix(op.Permission, "vendor.crm.campaigns.") {
				t.Errorf("%s %s has operationId=%q permission=%q", strings.ToUpper(method), path, op.OperationID, op.Permission)
			}
			if method != "post" && method != "patch" {
				continue
			}
			found := false
			for _, parameter := range op.Parameters {
				found = found || strings.HasSuffix(parameter.Ref, "/IdempotencyKey") || parameter.Name == "Idempotency-Key"
			}
			if readOnlyPosts[path] {
				if found {
					t.Errorf("%s %s declares Idempotency-Key but persists nothing", strings.ToUpper(method), path)
				}
				continue
			}
			if !found {
				t.Errorf("%s %s does not declare Idempotency-Key", strings.ToUpper(method), path)
			}
		}
	}
}

// TestPermissionSetIsExactlyTheFiveDeclared keeps the IAM surface from growing
// silently: five permissions are seeded in migration 000019 and five are used.
// A sixth — in particular a send permission — would be granted long before the
// guard rails around it existed.
func TestPermissionSetIsExactlyTheFiveDeclared(t *testing.T) {
	document := load(t)
	used := map[string]bool{}
	for _, methods := range document.Paths {
		for method, raw := range methods {
			if !isMethod(method) {
				continue
			}
			var op operation
			if err := json.Unmarshal(raw, &op); err != nil {
				t.Fatal(err)
			}
			used[op.Permission] = true
		}
	}
	want := map[string]bool{
		"vendor.crm.campaigns.read":         true,
		"vendor.crm.campaigns.manage":       true,
		"vendor.crm.campaigns.preview":      true,
		"vendor.crm.campaigns.prepare":      true,
		"vendor.crm.campaigns.targets.read": true,
	}
	if len(used) != len(want) {
		t.Fatalf("permissions used = %v; want exactly %v", used, want)
	}
	for permission := range want {
		if !used[permission] {
			t.Errorf("permission %s is declared by no operation", permission)
		}
	}
	if strings.Contains(rawSpec(t), "vendor.crm.campaigns.send") {
		t.Error("the contract declares a send permission for an authority this section does not hold")
	}
}

func TestControlledVocabularies(t *testing.T) {
	document := load(t)
	expected := map[string][]string{
		"CampaignStatus":           {"DRAFT", "PREPARING", "PREPARED", "PREPARATION_FAILED", "ARCHIVED"},
		"CampaignObjective":        {"GENERAL_MARKETING", "RETENTION", "REACTIVATION", "LOYALTY", "ANNOUNCEMENT"},
		"CampaignScopeType":        {"BUSINESS", "BRANCH"},
		"CampaignChannel":          {"EMAIL", "PUSH", "IN_APP", "SMS"},
		"SnapshotStatus":           {"BUILDING", "CURRENT", "FAILED", "SUPERSEDED"},
		"SnapshotEligibilityState": {"ELIGIBLE_AT_SNAPSHOT", "INELIGIBLE_CONSENT"},
		"ConsentDecision":          {"ALLOW", "DENY", "UNKNOWN"},
		"SuppressionReasonCode": {"PRIVACY_REDACTED", "PRIVACY_RESTRICTED",
			"PRIVACY_MARKETING_SUPPRESSED", "IDENTITY_MERGED"},
		"ConsentReasonCode": {"CONSENT_ALLOWED", "CONSENT_DENIED", "CONSENT_UNKNOWN",
			"CONSENT_STALE", "CONSENT_SOURCE_UNAVAILABLE"},
	}
	for name, want := range expected {
		got := append([]string(nil), document.Components.Schemas[name].Enum...)
		sort.Strings(got)
		sort.Strings(want)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s enum=%v; want %v", name, got, want)
		}
	}
	// The campaign lifecycle must not contain a delivery state.
	lifecycle := strings.Join(document.Components.Schemas["CampaignStatus"].Enum, ",")
	for _, forbidden := range []string{"SCHEDULED", "SENDING", "SENT", "DELIVERED"} {
		if strings.Contains(lifecycle, forbidden) {
			t.Errorf("CampaignStatus contains the delivery state %s", forbidden)
		}
	}
}

// TestSendTimeRecheckIsUnconditional is the dual-time consent contract stated
// as a schema property. Every place the contract reports channel eligibility it
// must also report that the decision will be revalidated before delivery, and
// those flags are pinned to true rather than merely typed as booleans.
func TestSendTimeRecheckIsUnconditional(t *testing.T) {
	document := load(t)
	cases := []struct{ schemaName, property string }{
		{"CampaignTargetChannel", "send_time_recheck_required"},
		{"CampaignChannelProjection", "send_time_recheck_required"},
		{"CampaignTargetPage", "send_time_recheck_required"},
		{"FrequencyPolicy", "send_time_recheck_required"},
		{"QuietHoursPolicy", "send_time_recheck_required"},
		{"CampaignSnapshot", "send_time_consent_recheck_required"},
		{"CampaignSnapshot", "send_time_frequency_recheck_required"},
		{"CampaignSnapshot", "send_time_quiet_hours_recheck_required"},
	}
	for _, testCase := range cases {
		raw, present := document.Components.Schemas[testCase.schemaName].Properties[testCase.property]
		if !present {
			t.Errorf("%s does not declare %s", testCase.schemaName, testCase.property)
			continue
		}
		var field struct {
			Type string `json:"type"`
			Enum []any  `json:"enum"`
		}
		if err := json.Unmarshal(raw, &field); err != nil {
			t.Fatal(err)
		}
		if field.Type != "boolean" || len(field.Enum) != 1 || field.Enum[0] != true {
			t.Errorf("%s.%s must be pinned to true, got type=%q enum=%v",
				testCase.schemaName, testCase.property, field.Type, field.Enum)
		}
	}
	// And the frequency policy can only ever report that it could not be
	// evaluated here.
	raw, present := document.Components.Schemas["FrequencyPolicy"].Properties["evaluation_state"]
	if !present {
		t.Fatal("FrequencyPolicy does not declare evaluation_state")
	}
	var state struct {
		Enum []string `json:"enum"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Enum) != 1 || state.Enum[0] != "NOT_AVAILABLE" {
		t.Errorf("FrequencyPolicy.evaluation_state enum=%v; Section 8E has no delivery history to evaluate against", state.Enum)
	}
}

func TestSchemasErrorsAndVersionContractsAreConcrete(t *testing.T) {
	document := load(t)
	for _, name := range []string{"Campaign", "CampaignDefinition", "CampaignWithDefinition", "CampaignPage",
		"CampaignDefinitionRequest", "ArchiveCampaignRequest", "PreviewCampaignRequest", "CampaignPreview",
		"CampaignSnapshot", "CampaignSnapshotPage", "CampaignTarget", "CampaignTargetChannel",
		"CampaignTargetPage", "ExperimentVariant", "FrequencyPolicy", "QuietHoursPolicy"} {
		if len(document.Components.Schemas[name].Properties) == 0 {
			t.Errorf("schema %s is missing or a stub", name)
		}
	}
	// Optimistic concurrency is documented where it is enforced.
	if !strings.Contains(rawSpec(t), "expected_version") {
		t.Error("the contract does not document expected_version")
	}
	if !strings.Contains(strings.Join(document.Components.Schemas["ArchiveCampaignRequest"].Required, ","), "") {
		t.Error("ArchiveCampaignRequest is malformed")
	}
	// The audience source is a segment reference and only a segment reference.
	required := strings.Join(document.Components.Schemas["CampaignDefinitionRequest"].Required, ",")
	if !strings.Contains(required, "source_segment_ref") {
		t.Error("CampaignDefinitionRequest does not require a source segment")
	}
	if !strings.Contains(required, "channels") {
		t.Error("CampaignDefinitionRequest does not require at least one channel")
	}

	codes := strings.Join(document.Components.Schemas["CampaignErrorCode"].Enum, ",")
	for _, want := range []string{
		"CRM_CAMPAIGN_NOT_FOUND", "CRM_CAMPAIGN_SCOPE_DENIED", "CRM_CAMPAIGN_VERSION_CONFLICT",
		"CRM_CAMPAIGN_INVALID_STATE", "CRM_CAMPAIGN_AUDIENCE_NOT_FOUND", "CRM_CAMPAIGN_AUDIENCE_NOT_CURRENT",
		"CRM_CAMPAIGN_AUDIENCE_SCOPE_INVALID", "CRM_CAMPAIGN_INVALID_CHANNEL", "CRM_CAMPAIGN_INVALID_EXPERIMENT",
		"CRM_CAMPAIGN_INVALID_FREQUENCY_POLICY", "CRM_CAMPAIGN_INVALID_QUIET_HOURS",
		"CRM_CAMPAIGN_PREPARATION_IN_PROGRESS", "CRM_CAMPAIGN_PREPARATION_FAILED",
		"CRM_IDEMPOTENCY_CONFLICT",
	} {
		if !strings.Contains(codes, want) {
			t.Errorf("error code %s is not documented", want)
		}
	}
}

// TestExperimentAllocationIsIntegerBasisPoints guards the modelling decision
// that keeps cohorts reproducible. A percentage or a float anywhere here would
// make a control group silently change size between runs.
func TestExperimentAllocationIsIntegerBasisPoints(t *testing.T) {
	document := load(t)
	raw, present := document.Components.Schemas["ExperimentVariant"].Properties["allocation_bps"]
	if !present {
		t.Fatal("ExperimentVariant does not declare allocation_bps")
	}
	var field struct {
		Type    string `json:"type"`
		Minimum int    `json:"minimum"`
		Maximum int    `json:"maximum"`
	}
	if err := json.Unmarshal(raw, &field); err != nil {
		t.Fatal(err)
	}
	if field.Type != "integer" || field.Minimum != 0 || field.Maximum != 10000 {
		t.Errorf("allocation_bps=%+v; want an integer in [0,10000]", field)
	}
	for name, definition := range document.Components.Schemas {
		for property, propertyRaw := range definition.Properties {
			if !strings.Contains(strings.ToLower(property), "allocation") &&
				!strings.Contains(strings.ToLower(property), "percent") {
				continue
			}
			var typed struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(propertyRaw, &typed); err != nil {
				t.Fatal(err)
			}
			if typed.Type == "number" {
				t.Errorf("%s.%s is a floating-point allocation", name, property)
			}
		}
	}
}

// TestNoPIISurface is the disclosure guard. Section 8E holds relationship
// references and counts; a contact field anywhere in this contract would mean a
// prepared audience had become an exportable contact list.
func TestNoPIISurface(t *testing.T) {
	document := load(t)
	for name, definition := range document.Components.Schemas {
		for property := range definition.Properties {
			lowered := strings.ToLower(property)
			for _, forbidden := range []string{
				"email_address", "phone", "msisdn", "address", "date_of_birth", "password",
				"device_token", "push_token", "destination", "recipient",
			} {
				if strings.Contains(lowered, forbidden) {
					t.Errorf("schema %s exposes %q; campaigns carry references and counts only", name, property)
				}
			}
		}
	}
}

// TestNoLaterSectionSurface keeps Section 8F onwards, and Section 9, out.
func TestNoLaterSectionSurface(t *testing.T) {
	document := load(t)
	for path := range document.Paths {
		lowered := strings.ToLower(path)
		for _, forbidden := range []string{"corporate", "rfm", "churn", "score", "recommendation",
			"export", "order", "checkout", "catalogue"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("path %s enters later-section authority %q", path, forbidden)
			}
		}
	}
	for name := range document.Components.Schemas {
		lowered := strings.ToLower(name)
		for _, forbidden := range []string{"corporate", "rfm", "churn", "recommendation", "voucher", "coupon"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("schema %s enters later-section authority %q", name, forbidden)
			}
		}
	}
}
