// Package section8dcontract pins the exact Section 8D public HTTP and OpenAPI
// surface. It is the guard against the documentation and the router drifting
// apart, and against Section 8D quietly growing an endpoint that would let a
// caller name its own points total.
package section8dcontract

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

const specPath = "../../docs/openapi/section8d-crm-loyalty.openapi.json"

// routes mirrors RegisterLoyaltyRoutes exactly. The router-side half of this
// parity check lives in
// internal/features/crm/loyalty_http_integration_test.go, which asserts the
// live gin engine registers precisely these six and nothing else.
var routes = []struct{ method, path string }{
	{"GET", "/crm/loyalty/program"},
	{"PATCH", "/crm/loyalty/program"},
	{"GET", "/crm/customers/{profileId}/loyalty"},
	{"GET", "/crm/customers/{profileId}/loyalty/entries"},
	{"POST", "/crm/customers/{profileId}/loyalty/adjustments"},
	{"POST", "/crm/loyalty/entries/{entryId}/reverse"},
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
			t.Errorf("Section 8D must not declare an internal route: %s %s", route.method, route.path)
		}
	}
	if len(routes) != 6 || len(documented) != 6 {
		t.Fatalf("route count: registered=%d documented=%d; want 6 public + 0 internal", len(routes), len(documented))
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

// TestNoPublicEarnOrRedeemSurface is the load-bearing scope guard. The earning
// and redemption engines are service-internal primitives; a documented
// endpoint for either would mean any authorised staff member could name their
// own points total, which is a different and far larger authority than the
// bounded manual adjustment.
func TestNoPublicEarnOrRedeemSurface(t *testing.T) {
	document := load(t)
	for path := range document.Paths {
		lowered := strings.ToLower(path)
		for _, forbidden := range []string{"earn", "redeem", "redemption", "internal"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("path %s exposes %q; Section 8D publishes no arbitrary earn or redeem route", path, forbidden)
			}
		}
	}
	for name := range document.Components.Schemas {
		lowered := strings.ToLower(name)
		if strings.Contains(lowered, "earnrequest") || strings.Contains(lowered, "redeemrequest") {
			t.Errorf("schema %s documents a request body for an endpoint that must not exist", name)
		}
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
			if op.OperationID == "" || !strings.HasPrefix(op.Permission, "vendor.crm.loyalty.") {
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

// TestPermissionSetIsExactlyTheFourDeclared keeps the IAM surface from growing
// silently: four permissions are seeded in migration 000017 and four are used.
func TestPermissionSetIsExactlyTheFourDeclared(t *testing.T) {
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
		"vendor.crm.loyalty.read":    true,
		"vendor.crm.loyalty.manage":  true,
		"vendor.crm.loyalty.adjust":  true,
		"vendor.crm.loyalty.reverse": true,
	}
	if len(used) != len(want) {
		t.Fatalf("permissions used = %v; want exactly %v", used, want)
	}
	for permission := range want {
		if !used[permission] {
			t.Errorf("permission %s is declared by no operation", permission)
		}
	}
}

func TestControlledVocabularies(t *testing.T) {
	document := load(t)
	expected := map[string][]string{
		"LoyaltyProgramStatus":       {"DISABLED", "ACTIVE", "PAUSED"},
		"LoyaltyExpiryPolicy":        {"NEVER", "FIXED_DAYS"},
		"LoyaltyAccountStatus":       {"ACTIVE", "MERGED", "REDACTED"},
		"LoyaltyEnrollmentState":     {"NOT_ENROLLED", "ENROLLED"},
		"LoyaltyAdjustmentDirection": {"CREDIT", "DEBIT"},
		"LoyaltyEntryType": {"EARN", "REDEEM", "EXPIRE", "ADJUSTMENT_CREDIT", "ADJUSTMENT_DEBIT",
			"REVERSAL", "MERGE_IN", "MERGE_OUT"},
		"LoyaltySourceType": {"MANUAL_ADJUSTMENT", "ORDER", "SYSTEM_EXPIRY", "REVERSAL", "IDENTITY_MERGE", "EXTERNAL"},
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
	for _, name := range []string{"LoyaltyProgram", "LoyaltySummary", "LoyaltyEntry", "LoyaltyEntryPage",
		"UpdateLoyaltyProgramRequest", "AdjustLoyaltyRequest", "AdjustLoyaltyResponse", "ReverseLoyaltyEntryRequest"} {
		if len(document.Components.Schemas[name].Properties) == 0 {
			t.Errorf("schema %s is missing or a stub", name)
		}
	}
	if _, ok := document.Components.Schemas["UpdateLoyaltyProgramRequest"].Properties["expected_version"]; !ok {
		t.Error("UpdateLoyaltyProgramRequest does not document expected_version")
	}
	// A reason is mandatory on both abuse-sensitive commands.
	for _, name := range []string{"AdjustLoyaltyRequest", "ReverseLoyaltyEntryRequest"} {
		required := strings.Join(document.Components.Schemas[name].Required, ",")
		if !strings.Contains(required, "reason") {
			t.Errorf("%s does not require a reason", name)
		}
	}
	codes := strings.Join(document.Components.Schemas["LoyaltyErrorCode"].Enum, ",")
	for _, required := range []string{
		"LOYALTY_PROGRAM_DISABLED", "LOYALTY_PROGRAM_PAUSED", "LOYALTY_ACCOUNT_NOT_FOUND",
		"LOYALTY_ACCOUNT_NOT_ACTIONABLE", "LOYALTY_INSUFFICIENT_POINTS", "LOYALTY_INVALID_POINTS",
		"LOYALTY_ADJUSTMENT_LIMIT_EXCEEDED", "LOYALTY_INVALID_EXPIRY_POLICY", "LOYALTY_VERSION_CONFLICT",
		"LOYALTY_IDEMPOTENCY_CONFLICT", "LOYALTY_ENTRY_NOT_FOUND", "LOYALTY_ENTRY_NOT_REVERSIBLE",
		"LOYALTY_ENTRY_ALREADY_REVERSED", "LOYALTY_REVERSAL_POINTS_ALREADY_CONSUMED",
		"LOYALTY_CUSTOMER_REDACTED", "LOYALTY_CUSTOMER_MERGED",
	} {
		if !strings.Contains(codes, required) {
			t.Errorf("error code %s is not documented", required)
		}
	}
}

// TestPointsAreIntegersEverywhere guards the single most consequential
// modelling decision: a floating-point points field anywhere in the contract
// would make balances unreproducible.
func TestPointsAreIntegersEverywhere(t *testing.T) {
	document := load(t)
	for name, definition := range document.Components.Schemas {
		for property, raw := range definition.Properties {
			lowered := strings.ToLower(property)
			// points_name_* are the programme's display nouns ("point" /
			// "stars"), not quantities.
			if !strings.Contains(lowered, "points") || strings.HasPrefix(lowered, "points_name") {
				continue
			}
			var field struct {
				Type   string `json:"type"`
				Format string `json:"format"`
				Ref    string `json:"$ref"`
			}
			if err := json.Unmarshal(raw, &field); err != nil {
				t.Fatal(err)
			}
			if field.Ref != "" {
				continue // an enum such as a naming field
			}
			if field.Type != "integer" {
				t.Errorf("%s.%s is %q; loyalty points must be integers", name, property, field.Type)
			}
		}
	}
}

// TestNoMonetaryOrPIISurface is the authority-boundary guard. Section 8D is a
// points ledger: a currency, a minor-unit amount or a wallet reference in this
// contract would mean vendor-service had started behaving like a second
// financial authority alongside payment-wallet-service.
func TestNoMonetaryOrPIISurface(t *testing.T) {
	document := load(t)
	for name, definition := range document.Components.Schemas {
		for property := range definition.Properties {
			lowered := strings.ToLower(property)
			for _, forbidden := range []string{
				"email", "phone", "msisdn", "address", "date_of_birth", "password", "token",
				"currency", "amount_minor", "kobo", "wallet", "payout", "settlement", "refund",
				"idempotency", "source_reference",
			} {
				if strings.Contains(lowered, forbidden) {
					t.Errorf("schema %s exposes %q; loyalty carries points and references only", name, property)
				}
			}
		}
	}
	// No wallet-family event or path may appear anywhere in the document.
	raw := strings.ToLower(rawSpec(t))
	for _, forbidden := range []string{"\"payment.", "\"wallet.", "\"refund.", "\"settlement.", "\"payout."} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("the specification references %q; Section 8D emits no monetary event", forbidden)
		}
	}
}

// TestNoLaterSectionSurface keeps 8E-8H and Section 9 out.
func TestNoLaterSectionSurface(t *testing.T) {
	document := load(t)
	for path := range document.Paths {
		lowered := strings.ToLower(path)
		for _, forbidden := range []string{"campaign", "audience", "segment", "tier", "reward", "voucher",
			"coupon", "giftcard", "cashback", "order", "checkout", "catalogue"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("path %s enters later-section authority %q", path, forbidden)
			}
		}
	}
	for name := range document.Components.Schemas {
		lowered := strings.ToLower(name)
		for _, forbidden := range []string{"tier", "reward", "voucher", "coupon", "giftcard", "cashback", "campaign"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("schema %s enters later-section authority %q", name, forbidden)
			}
		}
	}
}
