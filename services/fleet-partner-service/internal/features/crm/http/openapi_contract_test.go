package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/usecases"
	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ratelimit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

type openAPIDocument map[string]any

func loadOpenAPI(t *testing.T) openAPIDocument {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "docs", "openapi", "section14-crm.openapi.json"))
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read OpenAPI document: %v", err)
	}
	var doc openAPIDocument
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("parse OpenAPI JSON: %v", err)
	}
	return doc
}

func TestSection14OpenAPIStructuralValidation(t *testing.T) {
	doc := loadOpenAPI(t)
	if got := stringValue(doc["openapi"]); got != "3.1.0" {
		t.Fatalf("openapi = %q, want 3.1.0", got)
	}
	info := object(t, doc["info"], "info")
	if stringValue(info["title"]) == "" || stringValue(info["version"]) == "" {
		t.Fatal("info.title and info.version are required")
	}
	paths := object(t, doc["paths"], "paths")
	if len(paths) == 0 {
		t.Fatal("paths must not be empty")
	}
	components := object(t, doc["components"], "components")
	schemas := object(t, components["schemas"], "components.schemas")
	securitySchemes := object(t, components["securitySchemes"], "components.securitySchemes")
	operationIDs := map[string]string{}
	for path, rawItem := range paths {
		if !strings.HasPrefix(path, "/") {
			t.Errorf("path %q must start with /", path)
		}
		item := object(t, rawItem, "path item "+path)
		for method, rawOperation := range item {
			if !openAPIMethod(method) {
				continue
			}
			op := object(t, rawOperation, strings.ToUpper(method)+" "+path)
			id := stringValue(op["operationId"])
			if id == "" {
				t.Errorf("%s %s has no operationId", method, path)
			} else if previous := operationIDs[id]; previous != "" {
				t.Errorf("duplicate operationId %q at %s and %s %s", id, previous, method, path)
			} else {
				operationIDs[id] = method + " " + path
			}
			validatePathParameters(t, path, op)
			validateSecurity(t, path, op, securitySchemes)
			validateResponses(t, method, path, op)
		}
	}
	validateRefs(t, doc, doc, "#")
	validateEnums(t, schemas)
}

func TestSection14OpenAPIRouteParity(t *testing.T) {
	doc := loadOpenAPI(t)
	paths := object(t, doc["paths"], "paths")
	h := NewHandler(&usecases.Service{}, nil)
	p := NewPublicHandler(&usecases.Service{}, ratelimit.NewMemoryLimiter())
	i := NewInternalHandler(&usecases.Service{})
	manifest := AllRoutes(h, p, i)

	// Verify the manifest is what the real registrars mount, not merely a
	// second list that happens to agree with the document.
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h.RegisterTenantRoutes(router.Group(CorporatePrefix))
	p.RegisterPublicRoutes(router.Group(CustomerTokenPrefix))
	i.RegisterInternalRoutes(router.Group(InternalPrefix))
	mounted := map[string]bool{}
	for _, route := range router.Routes() {
		mounted[route.Method+" "+route.Path] = true
	}
	if len(mounted) != len(manifest) {
		t.Fatalf("mounted route count %d != manifest count %d", len(mounted), len(manifest))
	}

	want := map[string]Route{}
	for _, route := range manifest {
		key := route.Method + " " + openAPIPath(route.FullPath())
		want[key] = route
		if !mounted[route.Method+" "+route.FullPath()] {
			t.Errorf("manifest route is not mounted: %s %s", route.Method, route.FullPath())
		}
		rawItem, ok := paths[openAPIPath(route.FullPath())]
		if !ok {
			t.Errorf("route missing from spec: %s", key)
			continue
		}
		op, ok := rawItem.(map[string]any)[strings.ToLower(route.Method)].(map[string]any)
		if !ok {
			t.Errorf("route method missing from spec: %s", key)
			continue
		}
		if got := stringValue(op["operationId"]); got != route.OperationID {
			t.Errorf("%s operationId = %q, want %q", key, got, route.OperationID)
		}
		if got := stringValue(op["x-handler"]); got != route.Handler {
			t.Errorf("%s x-handler = %q, want %q", key, got, route.Handler)
		}
		assertSecurityClass(t, key, op, route.Security)
	}
	for path, rawItem := range paths {
		for method := range object(t, rawItem, path) {
			if !openAPIMethod(method) {
				continue
			}
			key := strings.ToUpper(method) + " " + path
			if _, ok := want[key]; !ok {
				t.Errorf("spec operation is not a mounted Section 14 route: %s", key)
			}
		}
	}
}

func TestSection14OpenAPIMetadataParity(t *testing.T) {
	doc := loadOpenAPI(t)
	paths := object(t, doc["paths"], "paths")
	routes := AllRoutes(NewHandler(nil, nil), NewPublicHandler(nil, nil), NewInternalHandler(nil))
	for _, route := range routes {
		key := route.Method + " " + openAPIPath(route.FullPath())
		op := object(t, object(t, paths[openAPIPath(route.FullPath())], key)[strings.ToLower(route.Method)], key)
		if got := stringValue(op["x-permission"]); got != route.Permission {
			t.Errorf("%s permission = %q, want %q", key, got, route.Permission)
		}
		if got := stringValue(op["x-feature-flag"]); got != route.FeatureFlag {
			t.Errorf("%s feature flag = %q, want %q", key, got, route.FeatureFlag)
		}
		if got := boolValue(op["x-idempotency-required"]); got != route.IdempotencyRequired {
			t.Errorf("%s idempotency extension = %t, want %t", key, got, route.IdempotencyRequired)
		}
		if got := boolValue(op["x-expected-version-required"]); got != route.ExpectedVersionRequired {
			t.Errorf("%s expected-version extension = %t, want %t", key, got, route.ExpectedVersionRequired)
		}
		params := resolvedParameterNames(t, doc, op)
		if params["Idempotency-Key"] != route.IdempotencyRequired {
			t.Errorf("%s Idempotency-Key required parity failed", key)
		}
		if route.ExpectedVersionRequired {
			if _, ok := params["If-Match"]; !ok {
				t.Errorf("%s does not document If-Match/expectedVersion", key)
			}
			if _, ok := object(t, op["responses"], key+" responses")["409"]; !ok {
				t.Errorf("%s needs a 409 response", key)
			}
		}
		if route.IdempotencyRequired {
			if _, ok := object(t, op["responses"], key+" responses")["409"]; !ok {
				t.Errorf("%s needs an idempotency 409 response", key)
			}
		}
		if strings.HasPrefix(route.Method, "GET") && params["Idempotency-Key"] {
			t.Errorf("read operation requires Idempotency-Key: %s", key)
		}
	}
}

// TestSection14OpenAPIPermissionBehavior verifies the manifest/spec permission
// against the real handler's authorization decision. With no grants the real
// handler must return 403; with exactly the documented grant it must pass that
// boundary (and may then return validation/500 from the deliberately empty
// service). A differently documented permission therefore fails either half.
func TestSection14OpenAPIPermissionBehavior(t *testing.T) {
	doc := loadOpenAPI(t)
	paths := object(t, doc["paths"], "paths")
	for _, route := range AllRoutes(NewHandler(nil, nil), nil, nil) {
		if route.Security != SecurityCorporate {
			continue
		}
		op := object(t, object(t, paths[openAPIPath(route.FullPath())], route.FullPath())[strings.ToLower(route.Method)], route.OperationID)
		if got := stringValue(op["x-permission"]); got != route.Permission {
			t.Fatalf("%s spec permission %q != manifest %q", route.OperationID, got, route.Permission)
		}
		if status := exercisePermission(t, route, nil); status != 403 {
			t.Errorf("%s without grants returned %d, want 403", route.OperationID, status)
		}
		binding := staffmodels.RoleBinding{OrganizationID: "org_test", OrganizationUserID: "member_test", ScopeType: staffmodels.ScopeOrganization, EffectiveFrom: time.Now().Add(-time.Minute)}
		if status := exercisePermission(t, route, fixedBindings{route.Permission: {binding}}); status == 403 {
			t.Errorf("%s still denied with documented permission %q", route.OperationID, route.Permission)
		}
	}
}

type fixedBindings map[string][]staffmodels.RoleBinding

func (r fixedBindings) EffectivePermissions(_ context.Context, _, _ string) (map[string][]staffmodels.RoleBinding, error) {
	return r, nil
}

func exercisePermission(t *testing.T, route Route, grants fixedBindings) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewHandler(&usecases.Service{}, accesscontrol.NewAuthorizer(grants, nil, 0))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		defer func() {
			if recover() != nil {
				c.AbortWithStatus(500)
			}
		}()
		c.Next()
	}, func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, tenant.Context{OrganizationID: "org_test", OrganizationUserID: "member_test", ActorType: tenant.ActorTypeOrganizationUser, ActorID: "member_test"})
		c.Next()
	})
	h.RegisterTenantRoutes(router.Group(CorporatePrefix))
	path := route.FullPath()
	for _, name := range pathParameterNames(openAPIPath(path)) {
		path = strings.ReplaceAll(path, ":"+name, "id_test")
	}
	req := httptest.NewRequest(route.Method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp.Code
}

func pathParameterNames(path string) []string {
	result := []string{}
	rest := path
	for {
		a := strings.Index(rest, "{")
		if a < 0 {
			break
		}
		b := strings.Index(rest[a:], "}")
		if b < 0 {
			break
		}
		result = append(result, rest[a+1:a+b])
		rest = rest[a+b+1:]
	}
	return result
}

func TestSection14OpenAPISection15AndDataMinimization(t *testing.T) {
	doc := loadOpenAPI(t)
	b, _ := json.Marshal(doc["paths"])
	lower := strings.ToLower(string(b))
	for _, forbidden := range []string{"/orders", "/shipments", "/packages", "/manifests", "/service-catalog", "/recurring-orders", "/orders/imports", "/private-execution"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("Section 15 path present: %s", forbidden)
		}
	}
	schemas := object(t, object(t, doc["components"], "components")["schemas"], "schemas")
	intent := object(t, schemas["ConversionIntent"], "ConversionIntent")
	props := object(t, intent["properties"], "ConversionIntent.properties")
	if stringValue(object(t, props["conversionType"], "conversionType")["const"]) != "FUTURE_PRIVATE_ORDER" {
		t.Error("conversionType must be FUTURE_PRIVATE_ORDER")
	}
	if stringValue(object(t, props["status"], "status")["const"]) != "PENDING_SECTION_15" {
		t.Error("conversion status must be PENDING_SECTION_15")
	}
	for _, field := range []string{"orderId", "shipmentId", "packageId", "assignmentId", "reservationId", "tripId", "walletOperationId"} {
		if _, ok := props[field]; ok {
			t.Errorf("ConversionIntent exposes forbidden field %s", field)
		}
	}
	public := object(t, schemas["SecureCustomerQuotationResponse"], "SecureCustomerQuotationResponse")
	publicFields := schemaPropertyNames(public)
	for _, field := range []string{"internalNotes", "approvalRecords", "riskNotes", "creditLimit", "providerInformation", "platformCommission", "companyMargin", "storagePath", "accessToken", "walletData"} {
		if publicFields[field] {
			t.Errorf("public quotation schema exposes %s", field)
		}
	}
	for _, name := range []string{"CustomerResponse", "ContactResponse", "LocationResponse", "CreditProfileResponse"} {
		fields := schemaPropertyNames(object(t, schemas[name], name))
		for _, field := range []string{"taxReference", "registrationReference", "email\"", "phone\"", "latitude", "longitude", "accessInstructions", "safetyInstructions", "creditLimit", "holdReason"} {
			field = strings.TrimSuffix(field, "\"")
			if fields[field] {
				t.Errorf("lower-privilege %s exposes %s", name, field)
			}
		}
	}
}

func TestSection14OpenAPIErrorCatalogue(t *testing.T) {
	doc := loadOpenAPI(t)
	components := object(t, doc["components"], "components")
	responses := object(t, components["responses"], "components.responses")
	for _, status := range []string{"400", "401", "403", "404", "409", "410", "422", "429", "500", "503"} {
		response := object(t, responses["Error"+status], "Error"+status)
		content := object(t, response["content"], "Error"+status+" content")
		media := object(t, content["application/json"], "Error"+status+" JSON")
		if stringValue(object(t, media["schema"], "error schema")["$ref"]) != "#/components/schemas/ErrorEnvelope" {
			t.Errorf("Error%s does not use ErrorEnvelope", status)
		}
	}
	// Stable codes documented in narrative descriptions must come from the
	// production catalogue; the contract intentionally never serializes SQL,
	// table, HMAC, stack, or provider implementation details.
	b, _ := json.Marshal(doc)
	text := strings.ToLower(string(b))
	for _, forbidden := range []string{"sql constraint", "stack trace", "raw provider error", "database table"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("unsafe error implementation detail in contract: %s", forbidden)
		}
	}
	codes := object(t, schemasFromDocument(t, doc)["StableErrorCode"], "StableErrorCode")
	documented := map[string]bool{}
	for _, raw := range array(codes["enum"]) {
		documented[stringValue(raw)] = true
	}
	production := []string{
		string(errorcodes.CustomerNotFound), string(errorcodes.CustomerAlreadyExists), string(errorcodes.CustomerOnHold), string(errorcodes.CustomerRiskRestricted), string(errorcodes.CustomerArchived), string(errorcodes.CustomerInvalidState),
		string(errorcodes.CustomerContactNotFound), string(errorcodes.CustomerContactConflict), string(errorcodes.CustomerLocationNotFound), string(errorcodes.CustomerLocationInvalidTZ), string(errorcodes.CustomerLocationInvalidCoord),
		string(errorcodes.OpportunityNotFound), string(errorcodes.OpportunityInvalidTransition), string(errorcodes.QuotationNotFound), string(errorcodes.QuotationInvalidState), string(errorcodes.QuotationExpired), string(errorcodes.QuotationAlreadyResolved), string(errorcodes.QuotationApprovalRequired), string(errorcodes.QuotationVersionConflict), string(errorcodes.QuotationDocumentNotReady), string(errorcodes.QuotationAcceptanceConflict), string(errorcodes.QuotationTokenInvalid), string(errorcodes.QuotationTokenExpired),
		string(errorcodes.ContractNotFound), string(errorcodes.ContractInvalidState), string(errorcodes.ContractNotActive), string(errorcodes.ContractActivationBlocked), string(errorcodes.ContractApprovalRequired), string(errorcodes.ContractDocumentNotReady), string(errorcodes.ContractVersionConflict),
		string(errorcodes.SLANotFound), string(errorcodes.SLAInvalidState), string(errorcodes.SLAObservationConflict), string(errorcodes.SLASourceUnknown), string(errorcodes.CreditProfileNotFound), string(errorcodes.CustomerCreditHold), string(errorcodes.CreditChangeApprovalRequired),
	}
	for _, code := range production {
		if !documented[code] {
			t.Errorf("production CRM error code missing from OpenAPI: %s", code)
		}
	}
}

func schemasFromDocument(t *testing.T, doc openAPIDocument) map[string]any {
	t.Helper()
	return object(t, object(t, doc["components"], "components")["schemas"], "components.schemas")
}

func validatePathParameters(t *testing.T, path string, op map[string]any) {
	t.Helper()
	expected := map[string]bool{}
	rest := path
	for {
		a := strings.Index(rest, "{")
		if a < 0 {
			break
		}
		b := strings.Index(rest[a:], "}")
		if b < 0 {
			t.Errorf("malformed path template %s", path)
			break
		}
		expected[rest[a+1:a+b]] = true
		rest = rest[a+b+1:]
	}
	declared := map[string]bool{}
	for _, raw := range array(op["parameters"]) {
		p, ok := raw.(map[string]any)
		if !ok || stringValue(p["in"]) != "path" {
			continue
		}
		name := stringValue(p["name"])
		declared[name] = true
		if !boolValue(p["required"]) {
			t.Errorf("path parameter %s on %s is not required", name, path)
		}
	}
	for name := range expected {
		if !declared[name] {
			t.Errorf("path parameter %s is undeclared on %s", name, path)
		}
	}
	for name := range declared {
		if !expected[name] {
			t.Errorf("declared path parameter %s is absent from %s", name, path)
		}
	}
}

func validateSecurity(t *testing.T, path string, op map[string]any, schemes map[string]any) {
	t.Helper()
	security := array(op["security"])
	if len(security) != 1 {
		t.Errorf("%s must have exactly one security requirement", path)
		return
	}
	for name := range object(t, security[0], path+" security") {
		if _, ok := schemes[name]; !ok {
			t.Errorf("%s uses undefined security scheme %s", path, name)
		}
	}
}
func validateResponses(t *testing.T, method, path string, op map[string]any) {
	t.Helper()
	responses := object(t, op["responses"], method+" "+path+" responses")
	found := false
	for code, raw := range responses {
		if strings.HasPrefix(code, "2") {
			found = true
			response := object(t, raw, code)
			content := object(t, response["content"], code+" content")
			media := object(t, content["application/json"], code+" JSON")
			if media["schema"] == nil {
				t.Errorf("%s %s success response has no schema", method, path)
			}
		}
	}
	if !found {
		t.Errorf("%s %s has no success response", method, path)
	}
	if body, ok := op["requestBody"].(map[string]any); ok && boolValue(body["required"]) {
		content := object(t, body["content"], "request content")
		media := object(t, content["application/json"], "request JSON")
		if media["schema"] == nil {
			t.Errorf("%s %s required request body has no schema", method, path)
		}
	}
}

func validateRefs(t *testing.T, root any, current any, location string) {
	t.Helper()
	switch v := current.(type) {
	case map[string]any:
		if raw, ok := v["$ref"]; ok {
			ref := stringValue(raw)
			if !strings.HasPrefix(ref, "#/") {
				t.Errorf("external or malformed ref at %s: %s", location, ref)
			} else if _, ok := resolve(root, ref); !ok {
				t.Errorf("unresolved ref at %s: %s", location, ref)
			}
		}
		for k, child := range v {
			validateRefs(t, root, child, location+"/"+k)
		}
	case []any:
		for i, child := range v {
			validateRefs(t, root, child, fmt.Sprintf("%s/%d", location, i))
		}
	}
}
func validateEnums(t *testing.T, schemas map[string]any) {
	t.Helper()
	for name, raw := range schemas {
		walkSchema(raw, func(s map[string]any) {
			seen := map[string]bool{}
			for _, v := range array(s["enum"]) {
				key := fmt.Sprint(v)
				if seen[key] {
					t.Errorf("schema %s has duplicate enum value %q", name, key)
				}
				seen[key] = true
			}
			if typ, ok := s["type"]; ok {
				switch typ.(type) {
				case string, []any:
				case map[string]any: // A property named "type", not a schema type keyword.
				default:
					t.Errorf("schema %s has malformed type %T", name, typ)
				}
			}
		})
	}
}
func walkSchema(v any, fn func(map[string]any)) {
	switch x := v.(type) {
	case map[string]any:
		fn(x)
		for _, child := range x {
			walkSchema(child, fn)
		}
	case []any:
		for _, child := range x {
			walkSchema(child, fn)
		}
	}
}
func resolve(root any, ref string) (any, bool) {
	current := root
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		m, ok := current.(map[string]any)
		if !ok {
			if doc, isDocument := current.(openAPIDocument); isDocument {
				m, ok = map[string]any(doc), true
			}
		}
		if !ok {
			return nil, false
		}
		current, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func schemaPropertyNames(schema map[string]any) map[string]bool {
	result := map[string]bool{}
	if props, ok := schema["properties"].(map[string]any); ok {
		for name := range props {
			result[name] = true
		}
	}
	for _, raw := range array(schema["allOf"]) {
		if child, ok := raw.(map[string]any); ok {
			for name := range schemaPropertyNames(child) {
				result[name] = true
			}
		}
	}
	return result
}

func resolvedParameterNames(t *testing.T, doc openAPIDocument, op map[string]any) map[string]bool {
	t.Helper()
	result := map[string]bool{}
	for _, raw := range array(op["parameters"]) {
		p := object(t, raw, "parameter")
		if refName := stringValue(p["$ref"]); refName != "" {
			resolved, ok := resolve(doc, refName)
			if !ok {
				t.Fatalf("unresolved parameter %s", refName)
			}
			p = object(t, resolved, refName)
		}
		name := stringValue(p["name"])
		if name != "" {
			result[name] = boolValue(p["required"])
		}
	}
	return result
}
func assertSecurityClass(t *testing.T, key string, op map[string]any, class SecurityClass) {
	t.Helper()
	security := array(op["security"])
	if len(security) != 1 {
		t.Errorf("%s security requirement count = %d", key, len(security))
		return
	}
	req := object(t, security[0], key+" security")
	want := "CorporateBearer"
	if class == SecurityCustomerToken {
		want = "QuotationToken"
	}
	if class == SecurityInternalHMAC {
		want = "ServiceHMACv2"
	}
	if _, ok := req[want]; !ok {
		t.Errorf("%s security = %v, want %s", key, req, want)
	}
}
func object(t *testing.T, v any, name string) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s is %T, want object", name, v)
	}
	return m
}
func array(v any) []any        { a, _ := v.([]any); return a }
func stringValue(v any) string { s, _ := v.(string); return s }
func boolValue(v any) bool     { b, _ := v.(bool); return b }
func openAPIMethod(v string) bool {
	switch v {
	case "get", "post", "put", "patch", "delete", "options", "head", "trace":
		return true
	}
	return false
}
func openAPIPath(path string) string {
	parts := strings.Split(path, "/")
	for n, p := range parts {
		if strings.HasPrefix(p, ":") {
			parts[n] = "{" + strings.TrimPrefix(p, ":") + "}"
		}
	}
	return strings.Join(parts, "/")
}

func TestSection14OpenAPIInventoryCounts(t *testing.T) {
	doc := loadOpenAPI(t)
	paths := object(t, doc["paths"], "paths")
	operations := 0
	ids := []string{}
	for _, raw := range paths {
		for method, op := range object(t, raw, "path") {
			if openAPIMethod(method) {
				operations++
				ids = append(ids, stringValue(object(t, op, "operation")["operationId"]))
			}
		}
	}
	sort.Strings(ids)
	t.Logf("validated %d paths, %d operations, operationId range %s..%s", len(paths), operations, ids[0], ids[len(ids)-1])
	if len(paths) != 63 || operations != 80 {
		t.Fatalf("inventory counts changed: %d paths, %d operations", len(paths), operations)
	}
}
