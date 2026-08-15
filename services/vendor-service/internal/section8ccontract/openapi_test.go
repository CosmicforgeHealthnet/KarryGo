// Package section8ccontract pins the exact Section 8C public HTTP and OpenAPI surface.
package section8ccontract

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

const specPath = "../../docs/openapi/section8c-crm-interactions-tasks.openapi.json"

var routes = []struct{ method, path string }{
	{"GET", "/crm/interactions"},
	{"POST", "/crm/interactions"},
	{"GET", "/crm/interactions/{interactionId}"},
	{"POST", "/crm/interactions/{interactionId}/correct"},
	{"GET", "/crm/interactions/{interactionId}/revisions"},
	{"GET", "/crm/customers/{profileId}/interactions"},
	{"GET", "/crm/tasks"},
	{"POST", "/crm/tasks"},
	{"GET", "/crm/tasks/{taskId}"},
	{"PATCH", "/crm/tasks/{taskId}"},
	{"POST", "/crm/tasks/{taskId}/start"},
	{"POST", "/crm/tasks/{taskId}/complete"},
	{"POST", "/crm/tasks/{taskId}/cancel"},
	{"POST", "/crm/tasks/{taskId}/reassign"},
	{"GET", "/crm/tasks/{taskId}/history"},
	{"GET", "/crm/customers/{profileId}/tasks"},
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
			t.Errorf("Section 8C must not declare an internal route: %s %s", route.method, route.path)
		}
	}
	if len(routes) != 16 || len(documented) != 16 {
		t.Fatalf("route count: registered=%d documented=%d; want 16 public + 0 internal", len(routes), len(documented))
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

func TestEveryOperationDeclaresPermissionAndCommandsAreIdempotent(t *testing.T) {
	document := load(t)
	for path, methods := range document.Paths {
		for method, raw := range methods {
			if !isMethod(method) {
				continue
			}
			var op operation
			if err := json.Unmarshal(raw, &op); err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
			if op.OperationID == "" || !strings.HasPrefix(op.Permission, "vendor.crm.") {
				t.Errorf("%s %s has operationId=%q permission=%q", strings.ToUpper(method), path, op.OperationID, op.Permission)
			}
			if method != "post" && method != "patch" {
				continue
			}
			found := false
			for _, parameter := range op.Parameters {
				found = found || strings.HasSuffix(parameter.Ref, "/IdempotencyKey") || parameter.Name == "Idempotency-Key"
			}
			if !found {
				t.Errorf("%s %s does not declare Idempotency-Key", strings.ToUpper(method), path)
			}
		}
	}
}

func TestControlledVocabularies(t *testing.T) {
	document := load(t)
	expected := map[string][]string{
		"InteractionType":      {"NOTE", "PHONE_CALL", "IN_PERSON", "EMAIL", "SMS", "IN_APP", "SOCIAL", "SALES_FOLLOW_UP", "SERVICE_FOLLOW_UP", "FEEDBACK", "ACCOUNT_REVIEW", "OTHER"},
		"InteractionDirection": {"INBOUND", "OUTBOUND", "INTERNAL", "NOT_APPLICABLE"},
		"InteractionOutcome":   {"NONE", "SUCCESSFUL", "NO_RESPONSE", "FOLLOW_UP_REQUIRED", "RESOLVED", "UNRESOLVED", "CUSTOMER_DECLINED", "VENDOR_DECLINED", "ESCALATED", "CANCELLED"},
		"VisibilityScope":      {"BUSINESS", "BRANCH"},
		"TaskStatus":           {"OPEN", "IN_PROGRESS", "COMPLETED", "CANCELLED"},
		"TaskPriority":         {"LOW", "NORMAL", "HIGH", "URGENT"},
	}
	for name, want := range expected {
		got := append([]string(nil), document.Components.Schemas[name].Enum...)
		sort.Strings(got)
		sort.Strings(want)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s enum=%v; want %v", name, got, want)
		}
	}
}

func TestSchemasErrorsAndVersionContractsAreConcrete(t *testing.T) {
	document := load(t)
	for _, name := range []string{"Interaction", "InteractionRevision", "Task", "TaskHistory", "CreateInteractionRequest", "CorrectInteractionRequest", "CreateTaskRequest", "UpdateTaskRequest", "VersionCommandRequest", "ReassignTaskRequest"} {
		if len(document.Components.Schemas[name].Properties) == 0 {
			t.Errorf("schema %s is missing or a stub", name)
		}
	}
	for _, name := range []string{"CorrectInteractionRequest", "UpdateTaskRequest", "VersionCommandRequest", "ReassignTaskRequest"} {
		if _, ok := document.Components.Schemas[name].Properties["expected_version"]; !ok {
			t.Errorf("schema %s does not document expected_version", name)
		}
	}
	codes := strings.Join(document.Components.Schemas["ActivityErrorCode"].Enum, ",")
	for _, required := range []string{"CRM_INTERACTION_NOT_FOUND", "CRM_INTERACTION_IMMUTABLE", "CRM_TASK_INVALID_STATE_TRANSITION", "CRM_TASK_ASSIGNEE_INVALID", "CRM_TASK_VERSION_CONFLICT", "CRM_PROFILE_REDACTED", "CRM_PROFILE_MERGED", "CRM_IDEMPOTENCY_CONFLICT"} {
		if !strings.Contains(codes, required) {
			t.Errorf("error code %s is not documented", required)
		}
	}
}

func TestNoLaterSectionOrPIIContractSurface(t *testing.T) {
	for _, route := range routes {
		lowered := strings.ToLower(route.path)
		for _, forbidden := range []string{"campaign", "loyalty", "notification", "export", "catalogue", "order"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("route %s %s enters later-section authority %q", route.method, route.path, forbidden)
			}
		}
	}
	document := load(t)
	for _, name := range []string{"Interaction", "InteractionRevision", "Task", "TaskHistory"} {
		for property := range document.Components.Schemas[name].Properties {
			switch property {
			case "email", "phone", "address", "government_id", "payment_data", "password", "access_token":
				t.Errorf("schema %s exposes forbidden canonical PII property %q", name, property)
			}
		}
	}
}
