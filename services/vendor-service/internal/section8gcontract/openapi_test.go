// Package section8gcontract pins the exact Section 8G public HTTP and OpenAPI
// surface. It is the guard against the documentation and the router drifting
// apart, and — more importantly — against Section 8G quietly growing an
// endpoint, a schema or a state that would make it a corporate ordering,
// invoicing, credit or contract surface. That boundary is the point of the
// section, and a boundary nothing enforces is a boundary that moves.
package section8gcontract

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

const specPath = "../../docs/openapi/section8g-crm-corporate-customers.openapi.json"

// routes mirrors RegisterCorporateRoutes exactly. The router-side half of this
// parity check lives in internal/features/crm/corporate_http_integration_test.go,
// which asserts the live gin engine registers precisely these forty-one and
// nothing else.
var routes = []struct{ method, path string }{
	{"GET", "/crm/corporate-customers"},
	{"POST", "/crm/corporate-customers"},
	{"GET", "/crm/corporate-customers/{corporateId}"},
	{"PATCH", "/crm/corporate-customers/{corporateId}"},
	{"POST", "/crm/corporate-customers/{corporateId}/activate"},
	{"POST", "/crm/corporate-customers/{corporateId}/hold"},
	{"POST", "/crm/corporate-customers/{corporateId}/deactivate"},
	{"POST", "/crm/corporate-customers/{corporateId}/archive"},

	{"GET", "/crm/corporate-customers/{corporateId}/branches"},
	{"PUT", "/crm/corporate-customers/{corporateId}/branches"},

	{"GET", "/crm/corporate-customers/{corporateId}/contacts"},
	{"POST", "/crm/corporate-customers/{corporateId}/contacts"},
	{"GET", "/crm/corporate-customers/{corporateId}/contacts/{contactId}"},
	{"PATCH", "/crm/corporate-customers/{corporateId}/contacts/{contactId}"},
	{"POST", "/crm/corporate-customers/{corporateId}/contacts/{contactId}/archive"},

	{"GET", "/crm/corporate-customers/{corporateId}/memberships"},
	{"POST", "/crm/corporate-customers/{corporateId}/memberships"},
	{"PATCH", "/crm/corporate-customers/{corporateId}/memberships/{membershipId}"},
	{"POST", "/crm/corporate-customers/{corporateId}/memberships/{membershipId}/archive"},

	{"GET", "/crm/corporate-customers/{corporateId}/locations"},
	{"POST", "/crm/corporate-customers/{corporateId}/locations"},
	{"GET", "/crm/corporate-customers/{corporateId}/locations/{locationId}"},
	{"PATCH", "/crm/corporate-customers/{corporateId}/locations/{locationId}"},
	{"POST", "/crm/corporate-customers/{corporateId}/locations/{locationId}/archive"},

	{"GET", "/crm/corporate-customers/{corporateId}/cost-centers"},
	{"POST", "/crm/corporate-customers/{corporateId}/cost-centers"},
	{"GET", "/crm/corporate-customers/{corporateId}/cost-centers/{costCenterId}"},
	{"PATCH", "/crm/corporate-customers/{corporateId}/cost-centers/{costCenterId}"},
	{"POST", "/crm/corporate-customers/{corporateId}/cost-centers/{costCenterId}/archive"},

	{"GET", "/crm/corporate-customers/{corporateId}/interactions"},
	{"POST", "/crm/corporate-customers/{corporateId}/interactions"},
	{"GET", "/crm/corporate-customers/{corporateId}/interactions/{interactionId}"},
	{"POST", "/crm/corporate-customers/{corporateId}/interactions/{interactionId}/correct"},

	{"GET", "/crm/corporate-customers/{corporateId}/tasks"},
	{"POST", "/crm/corporate-customers/{corporateId}/tasks"},
	{"GET", "/crm/corporate-customers/{corporateId}/tasks/{taskId}"},
	{"PATCH", "/crm/corporate-customers/{corporateId}/tasks/{taskId}"},
	{"POST", "/crm/corporate-customers/{corporateId}/tasks/{taskId}/start"},
	{"POST", "/crm/corporate-customers/{corporateId}/tasks/{taskId}/complete"},
	{"POST", "/crm/corporate-customers/{corporateId}/tasks/{taskId}/cancel"},
	{"POST", "/crm/corporate-customers/{corporateId}/tasks/{taskId}/reassign"},
}

const routeCount = 41

var corporatePermissions = []string{
	"vendor.crm.corporate.read",
	"vendor.crm.corporate.manage",
	"vendor.crm.corporate.contacts.manage",
	"vendor.crm.corporate.locations.manage",
	"vendor.crm.corporate.cost_centers.manage",
}

// activityPermissions are Section 8C's, reused rather than reissued under a
// corporate name. A corporate copy of them would be a second place for the same
// question to be answered differently.
var activityPermissions = map[string]bool{
	"vendor.crm.interactions.read":   true,
	"vendor.crm.interactions.create": true,
	"vendor.crm.interactions.manage": true,
	"vendor.crm.tasks.read":          true,
	"vendor.crm.tasks.create":        true,
	"vendor.crm.tasks.manage":        true,
}

type operation struct {
	OperationID          string `json:"operationId"`
	Permission           string `json:"x-permission"`
	AdditionalPermission string `json:"x-additional-permission"`
	Parameters           []struct {
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
			t.Errorf("Section 8G must not declare an internal route: %s %s", route.method, route.path)
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

// TestNoCommerceOrFinanceSurface is the load-bearing scope guard. Section 8G
// records who the vendor deals with. A path or schema describing an order, an
// invoice, credit, a budget, a contract or a quotation would mean corporate
// commerce had arrived without any of the controls it needs.
func TestNoCommerceOrFinanceSurface(t *testing.T) {
	document := load(t)
	forbiddenInPaths := []string{"order", "invoice", "credit", "budget", "contract", "quotation",
		"quote", "wallet", "payment", "checkout", "catalogue", "catalog", "entitlement",
		"approval", "merge", "export", "internal"}
	for path := range document.Paths {
		lowered := strings.ToLower(path)
		for _, forbidden := range forbiddenInPaths {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("path %s exposes %q; Section 8G publishes no commerce, finance or approval route", path, forbidden)
			}
		}
	}
	for name := range document.Components.Schemas {
		lowered := strings.ToLower(name)
		for _, forbidden := range []string{"order", "invoice", "credit", "budget", "contract",
			"quotation", "wallet", "payment", "voucher", "discount", "pricing", "entitlement",
			"rfm", "churn", "score", "export"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("schema %s documents %q, which belongs to a later section or another service", name, forbidden)
			}
		}
	}
	// Nor may a property anywhere carry money or a commercial document.
	for name, definition := range document.Components.Schemas {
		for property := range definition.Properties {
			lowered := strings.ToLower(property)
			for _, forbidden := range []string{"amount", "balance", "currency", "kobo", "minor_units",
				"credit_limit", "budget", "invoice", "order_ref", "price", "discount", "tax"} {
				if strings.Contains(lowered, forbidden) {
					t.Errorf("schema %s exposes %q; Section 8G holds no money", name, property)
				}
			}
		}
	}
	raw := rawSpec(t)
	for _, forbidden := range []string{"\"corporate.order", "\"corporate.invoice", "\"corporate.credit"} {
		if strings.Contains(strings.ToLower(raw), forbidden) {
			t.Errorf("the specification references %q; Section 8G emits no commerce event", forbidden)
		}
	}
}

// TestNoPIISurface is the disclosure guard. A corporate contact is a reference
// to a Section 8A customer relationship plus a non-identifying label. A contact
// field here would turn an account page into an exportable address book.
func TestNoPIISurface(t *testing.T) {
	document := load(t)
	for name, definition := range document.Components.Schemas {
		for property := range definition.Properties {
			lowered := strings.ToLower(property)
			for _, forbidden := range []string{
				"email", "phone", "msisdn", "first_name", "last_name", "full_name",
				"date_of_birth", "password", "government_id", "device_token", "push_token",
			} {
				if strings.Contains(lowered, forbidden) {
					t.Errorf("schema %s exposes %q; corporate contacts carry references and labels only", name, property)
				}
			}
		}
	}
	// The organization's own address is the customer's business address on a
	// site record, which is deliberately the only address in this section.
	addressed := map[string]bool{"CorporateLocation": true, "CorporateLocationRequest": true}
	for name, definition := range document.Components.Schemas {
		if addressed[name] {
			continue
		}
		for property := range definition.Properties {
			if strings.Contains(strings.ToLower(property), "address") {
				t.Errorf("schema %s exposes %q outside a corporate site record", name, property)
			}
		}
	}
}

func TestEveryOperationDeclaresPermissionAndCommandsAreIdempotent(t *testing.T) {
	document := load(t)
	for key, op := range operations(t, document) {
		if op.OperationID == "" {
			t.Errorf("%s has no operationId", key)
		}
		known := activityPermissions[op.Permission]
		for _, permission := range corporatePermissions {
			known = known || op.Permission == permission
		}
		if !known {
			t.Errorf("%s declares unknown permission %q", key, op.Permission)
		}
		// Corporate activity reuses the Section 8C engine, so it must also
		// require corporate read: an activity grant alone cannot reveal which
		// organizations this vendor works with.
		if activityPermissions[op.Permission] && op.AdditionalPermission != "vendor.crm.corporate.read" {
			t.Errorf("%s reuses a Section 8C permission without also requiring vendor.crm.corporate.read", key)
		}
		method := strings.ToLower(strings.SplitN(key, " ", 2)[0])
		if method != "post" && method != "patch" && method != "put" {
			continue
		}
		found := false
		for _, parameter := range op.Parameters {
			found = found || strings.HasSuffix(parameter.Ref, "/IdempotencyKey") || parameter.Name == "Idempotency-Key"
		}
		if !found {
			t.Errorf("%s does not declare Idempotency-Key", key)
		}
	}
}

// TestPermissionSetIsExactlyTheFiveDeclared keeps the IAM surface from growing
// silently: five permissions are seeded in migration 000023 and five are used.
func TestPermissionSetIsExactlyTheFiveDeclared(t *testing.T) {
	document := load(t)
	used := map[string]bool{}
	for _, op := range operations(t, document) {
		if !activityPermissions[op.Permission] {
			used[op.Permission] = true
		}
	}
	if len(used) != len(corporatePermissions) {
		t.Fatalf("corporate permissions used = %v; want exactly %v", used, corporatePermissions)
	}
	for _, permission := range corporatePermissions {
		if !used[permission] {
			t.Errorf("permission %s is declared by no operation", permission)
		}
	}
	raw := rawSpec(t)
	for _, forbidden := range []string{
		"vendor.crm.corporate.orders", "vendor.crm.corporate.invoices", "vendor.crm.corporate.credit",
		"vendor.crm.corporate.contracts", "vendor.crm.corporate.approve",
	} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("the contract declares %s, an authority this section does not hold", forbidden)
		}
	}
}

func TestControlledVocabularies(t *testing.T) {
	document := load(t)
	expected := map[string][]string{
		"CorporateStatus":         {"PROSPECT", "ACTIVE", "ON_HOLD", "INACTIVE", "ARCHIVED"},
		"OrganizationType":        {"COMPANY", "INSTITUTION", "SCHOOL", "HOSPITAL", "HOTEL", "RESTAURANT", "RETAILER", "NGO", "ASSOCIATION", "GOVERNMENT", "OTHER"},
		"CorporateScopeMode":      {"BUSINESS", "BRANCH_SET"},
		"CorporateChildStatus":    {"ACTIVE", "INACTIVE", "ARCHIVED"},
		"CorporateLocationType":   {"HEADQUARTERS", "OFFICE", "BRANCH", "STORE", "WAREHOUSE", "SITE", "CAMPUS", "OTHER"},
		"CorporateContactRole":    {"PRIMARY_CONTACT", "PROCUREMENT", "OPERATIONS", "FINANCE", "MANAGEMENT", "ADMINISTRATION", "TECHNICAL", "OTHER"},
		"CorporateMembershipRole": {"MEMBER", "MANAGER", "APPROVER", "REQUESTER", "PROCUREMENT", "FINANCE", "OPERATIONS", "OTHER"},
	}
	for name, want := range expected {
		got := append([]string(nil), document.Components.Schemas[name].Enum...)
		sort.Strings(got)
		sort.Strings(want)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s enum=%v; want %v", name, got, want)
		}
	}
	// The relationship lifecycle must not contain a commercial state.
	lifecycle := strings.Join(document.Components.Schemas["CorporateStatus"].Enum, ",")
	for _, forbidden := range []string{"ORDERED", "INVOICED", "OVERDUE", "IN_ARREARS", "SUSPENDED_FOR_CREDIT"} {
		if strings.Contains(lifecycle, forbidden) {
			t.Errorf("CorporateStatus contains the commercial state %s", forbidden)
		}
	}
}

func TestSchemasErrorsAndVersionContractsAreConcrete(t *testing.T) {
	document := load(t)
	for _, name := range []string{
		"CorporateCustomer", "CorporateCustomerDetail", "CorporateCustomerPage", "CorporateCustomerRequest",
		"CorporateSummary", "CorporateTransitionRequest", "CorporateBranchScopes", "CorporateBranchScopeRequest",
		"CorporateContact", "CorporateContactPage", "CorporateContactRequest",
		"CorporateMembership", "CorporateMembershipPage", "CorporateMembershipRequest",
		"CorporateLocation", "CorporateLocationPage", "CorporateLocationRequest",
		"CorporateCostCenter", "CorporateCostCenterPage", "CorporateCostCenterRequest",
		"CorporateInteraction", "CorporateInteractionPage", "CorporateInteractionRequest",
		"CorporateTask", "CorporateTaskPage", "CorporateTaskRequest", "CorporateTaskTransitionRequest",
		"Error",
	} {
		if len(document.Components.Schemas[name].Properties) == 0 {
			t.Errorf("schema %s is missing or a stub", name)
		}
	}
	// Optimistic concurrency is documented where it is enforced.
	raw := rawSpec(t)
	if !strings.Contains(raw, "expected_version") {
		t.Error("the contract does not document expected_version")
	}
	for _, name := range []string{"CorporateTransitionRequest", "CorporateBranchScopeRequest", "CorporateTaskTransitionRequest"} {
		if !strings.Contains(strings.Join(document.Components.Schemas[name].Required, ","), "expected_version") {
			t.Errorf("%s does not require expected_version", name)
		}
	}
	// A contact is created from a customer relationship reference, never from
	// contact details.
	required := strings.Join(document.Components.Schemas["CorporateContactRequest"].Required, ",")
	if !strings.Contains(required, "customer_profile_ref") {
		t.Error("CorporateContactRequest does not require a customer relationship reference")
	}
	if !strings.Contains(strings.Join(document.Components.Schemas["CorporateMembershipRequest"].Required, ","), "contact_ref") {
		t.Error("CorporateMembershipRequest does not require a contact of the same account")
	}

	codes := strings.Join(document.Components.Schemas["CorporateErrorCode"].Enum, ",")
	for _, want := range []string{
		"CRM_CORPORATE_DISABLED", "CRM_CORPORATE_CUSTOMER_NOT_FOUND", "CRM_CORPORATE_CUSTOMER_SCOPE_DENIED",
		"CRM_CORPORATE_CUSTOMER_VERSION_CONFLICT", "CRM_CORPORATE_CUSTOMER_INVALID_STATE",
		"CRM_CORPORATE_CUSTOMER_DUPLICATE", "CRM_CORPORATE_CUSTOMER_ARCHIVED",
		"CRM_CORPORATE_ACCOUNT_MANAGER_INVALID", "CRM_CORPORATE_NESTED_SCOPE_INVALID",
		"CRM_CORPORATE_CONTACT_NOT_FOUND", "CRM_CORPORATE_CONTACT_DUPLICATE",
		"CRM_CORPORATE_CONTACT_INVALID_PROFILE", "CRM_CORPORATE_CONTACT_REDACTED",
		"CRM_CORPORATE_CONTACT_MERGED", "CRM_CORPORATE_MEMBERSHIP_NOT_FOUND",
		"CRM_CORPORATE_LOCATION_NOT_FOUND", "CRM_CORPORATE_COST_CENTER_NOT_FOUND",
		"CRM_IDEMPOTENCY_CONFLICT",
	} {
		if !strings.Contains(codes, want) {
			t.Errorf("error code %s is not documented", want)
		}
	}
}

// TestEveryReferenceResolves catches a spec that parses but points at nothing —
// the failure mode a hand-maintained document reaches first.
func TestEveryReferenceResolves(t *testing.T) {
	raw := rawSpec(t)
	var anything any
	if err := json.Unmarshal([]byte(raw), &anything); err != nil {
		t.Fatalf("the specification is not valid JSON: %v", err)
	}
	document := load(t)
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
	var tree any
	if err := json.Unmarshal([]byte(raw), &tree); err != nil {
		t.Fatal(err)
	}
	walk(tree)
}

// TestNoLaterSectionSurface keeps Section 8H onwards, and Section 9, out.
func TestNoLaterSectionSurface(t *testing.T) {
	document := load(t)
	for path := range document.Paths {
		lowered := strings.ToLower(path)
		for _, forbidden := range []string{"rfm", "churn", "score", "recommendation", "lifecycle-stage",
			"insight", "report", "fulfilment", "menu"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("path %s enters later-section authority %q", path, forbidden)
			}
		}
	}
	for name := range document.Components.Schemas {
		lowered := strings.ToLower(name)
		for _, forbidden := range []string{"rfm", "churn", "recommendation", "insight", "fulfilment", "menu"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("schema %s enters later-section authority %q", name, forbidden)
			}
		}
	}
}
