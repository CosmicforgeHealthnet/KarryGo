package section12acontract

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/features/shop"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
)

const (
	specPath           = "../../docs/openapi/section12a-shop-catalogue.openapi.json"
	expectedOperations = 58
)

type operation struct {
	OperationID string                     `json:"operationId"`
	Responses   map[string]json.RawMessage `json:"responses"`
	Parameters  []struct {
		Ref      string `json:"$ref"`
		Name     string `json:"name"`
		In       string `json:"in"`
		Required bool   `json:"required"`
	} `json:"parameters"`
	RequestBody json.RawMessage `json:"requestBody"`
}

type document struct {
	Security []map[string][]string                 `json:"security"`
	Paths    map[string]map[string]json.RawMessage `json:"paths"`
	Info     struct {
		Description string `json:"description"`
	} `json:"info"`
	Components struct {
		SecuritySchemes map[string]json.RawMessage `json:"securitySchemes"`
		Schemas         map[string]json.RawMessage `json:"schemas"`
	} `json:"components"`
}

func load(t *testing.T) document {
	t.Helper()
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	var out document
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func isMethod(value string) bool {
	switch value {
	case "get", "post", "put", "patch", "delete":
		return true
	}
	return false
}

func documented(t *testing.T, doc document) map[string]operation {
	t.Helper()
	out := map[string]operation{}
	for path, methods := range doc.Paths {
		for name, raw := range methods {
			if !isMethod(name) {
				continue
			}
			var op operation
			if err := json.Unmarshal(raw, &op); err != nil {
				t.Fatalf("%s %s: %v", name, path, err)
			}
			out[strings.ToUpper(name)+" "+path] = op
		}
	}
	return out
}

func normalize(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/api/v1/vendor"), "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "{" + strings.TrimPrefix(part, ":") + "}"
		}
	}
	return strings.Join(parts, "/")
}

func registered(t *testing.T) map[string]bool {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	flags := shop.Flags{ShopEnabled: true, WritesEnabled: true}
	service := shop.NewService(shop.Options{Flags: flags})
	shop.RegisterRoutes(router.Group("/api/v1/vendor"), shop.NewHandler(service, idempotency.NewStore(nil, time.Hour)), flags)
	out := map[string]bool{}
	for _, route := range router.Routes() {
		if strings.Contains(route.Path, "/inventory") || strings.Contains(route.Path, "/bulk-operations") {
			continue
		}
		out[route.Method+" "+normalize(route.Path)] = true
	}
	return out
}

func TestBidirectionalRouteParity(t *testing.T) {
	docRoutes, codeRoutes := documented(t, load(t)), registered(t)
	if len(docRoutes) != expectedOperations || len(codeRoutes) != expectedOperations {
		t.Fatalf("Section 12A route count: code=%d OpenAPI=%d want=%d", len(codeRoutes), len(docRoutes), expectedOperations)
	}
	var specOnly, codeOnly []string
	for key := range docRoutes {
		if !codeRoutes[key] {
			specOnly = append(specOnly, key)
		}
	}
	for key := range codeRoutes {
		if _, ok := docRoutes[key]; !ok {
			codeOnly = append(codeOnly, key)
		}
	}
	sort.Strings(specOnly)
	sort.Strings(codeOnly)
	if len(specOnly) != 0 || len(codeOnly) != 0 {
		t.Fatalf("route drift: spec-only=%v code-only=%v", specOnly, codeOnly)
	}
}

func TestMutationsDeclareIdempotencyAndBodies(t *testing.T) {
	doc := load(t)
	if len(doc.Security) != 1 || doc.Security[0]["bearerAuth"] == nil {
		t.Fatal("bearer authentication must be global")
	}
	if _, ok := doc.Components.SecuritySchemes["bearerAuth"]; !ok {
		t.Fatal("bearerAuth scheme missing")
	}
	seen := map[string]string{}
	for key, op := range documented(t, doc) {
		if op.OperationID == "" {
			t.Errorf("%s has no operationId", key)
		}
		if previous := seen[op.OperationID]; previous != "" {
			t.Errorf("duplicate operationId %q: %s and %s", op.OperationID, previous, key)
		}
		seen[op.OperationID] = key
		if len(op.Responses) == 0 {
			t.Errorf("%s has no responses", key)
		}
		success := op.Responses["200"]
		if len(success) == 0 {
			success = op.Responses["201"]
		}
		var response struct {
			Content map[string]struct {
				Schema json.RawMessage `json:"schema"`
			} `json:"content"`
		}
		if len(success) == 0 || json.Unmarshal(success, &response) != nil || len(response.Content["application/json"].Schema) == 0 {
			t.Errorf("%s has no documented success envelope", key)
		}
		if strings.HasPrefix(key, "GET ") {
			continue
		}
		hasKey := false
		for _, parameter := range op.Parameters {
			if strings.HasSuffix(parameter.Ref, "/IdempotencyKey") || (parameter.Name == "Idempotency-Key" && parameter.Required) {
				hasKey = true
			}
		}
		if !hasKey {
			t.Errorf("%s lacks required Idempotency-Key", key)
		}
		if len(op.RequestBody) == 0 || string(op.RequestBody) == "null" {
			t.Errorf("%s lacks command body", key)
			continue
		}
		var body struct {
			Content map[string]struct {
				Schema struct {
					Ref string `json:"$ref"`
				} `json:"schema"`
			} `json:"content"`
		}
		if err := json.Unmarshal(op.RequestBody, &body); err != nil {
			t.Errorf("%s command body: %v", key, err)
			continue
		}
		ref := body.Content["application/json"].Schema.Ref
		name := strings.TrimPrefix(ref, "#/components/schemas/")
		raw, exists := doc.Components.Schemas[name]
		if ref == "" || !exists {
			t.Errorf("%s request schema %q is missing", key, ref)
			continue
		}
		var schema struct {
			Type                 string                     `json:"type"`
			AdditionalProperties *bool                      `json:"additionalProperties"`
			Properties           map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Errorf("%s request schema %s: %v", key, name, err)
			continue
		}
		if schema.Type != "object" || schema.AdditionalProperties == nil || *schema.AdditionalProperties {
			t.Errorf("%s request schema %s must be a strict object", key, name)
		}
		if name != "EmptyCommand" && len(schema.Properties) == 0 {
			t.Errorf("%s request schema %s documents no fields", key, name)
		}
	}
}

func TestMeasurementArchitectureAndPartABoundaryAreExplicit(t *testing.T) {
	doc := load(t)
	for _, schema := range []string{"OperatingModel", "OperatingModelConfiguration", "MeasurementFamily", "ConversionType", "MeasurementMode", "Fraction", "Calibration", "MinorMoney", "InventoryAuthority", "CatalogueRevision", "Category", "MeasureDefinition", "MeasureAlias", "Product", "Variant", "SellUnit", "PackageRelation", "Identifier", "BranchListing", "ListingSellUnit", "QuantityBreak", "FreshAttribute", "LogisticsProfile", "PriceVersion", "ResolvedPrice"} {
		if _, ok := doc.Components.Schemas[schema]; !ok {
			t.Errorf("schema %s missing", schema)
		}
	}
	var listingUnitCommand struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(doc.Components.Schemas["ListingSellUnitCommand"], &listingUnitCommand); err != nil {
		t.Fatal(err)
	}
	if _, ok := listingUnitCommand.Properties["calibration_id"]; !ok {
		t.Error("branch listing sell-unit command does not expose scoped calibration assignment")
	}
	description := strings.ToUpper(doc.Info.Description)
	for _, concept := range []string{"PAINT", "DERICA", "MUDU", "NO UNIVERSAL WEIGHT", "INTEGER MINOR UNITS", "SECTION 12B"} {
		if !strings.Contains(description, concept) {
			t.Errorf("contract does not explain %q", concept)
		}
	}
	for path := range doc.Paths {
		lower := strings.ToLower(path)
		for _, forbidden := range []string{"inventory", "stock", "reservation", "availability", "order"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("Part B/external authority route leaked into 12A: %s", path)
			}
		}
	}
}

func TestResponseSchemasUsePublicViewVocabulary(t *testing.T) {
	doc := load(t)
	expected := map[string][]string{
		"MeasureDefinition": {"measurement_family", "measure_origin", "scope", "converts_exactly"},
		"Product":           {"category_ref", "operating_models", "media_refs"},
		"Variant":           {"product_ref", "package_level", "temperature_zone", "handling_flags"},
		"SellUnit":          {"product_ref", "variant_ref", "measure_ref", "fraction_numerator", "measurement_mode", "calibration_ref", "actual_measurement_required"},
		"PackageRelation":   {"product_ref", "parent_variant_ref", "child_variant_ref", "units_per_parent"},
		"Identifier":        {"product_ref", "variant_ref", "identifier_type", "identifier_value", "externally_verified"},
		"BranchListing":     {"branch_ref", "product_ref", "variant_ref", "retail_price_override_minor", "inventory_authority"},
		"QuantityBreak":     {"listing_ref", "sell_unit_ref", "wholesale_only", "unit_price_minor"},
		"FreshAttribute":    {"product_ref", "attribute_type", "selection_required", "options"},
		"PriceVersion":      {"subject_ref", "change_reason", "subject_version", "recorded_at"},
		"ResolvedPrice":     {"price_source", "quantity_break_ref", "order_rules_satisfied", "availability"},
	}
	for name, fields := range expected {
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(doc.Components.Schemas[name], &schema); err != nil {
			t.Fatalf("%s schema: %v", name, err)
		}
		for _, field := range fields {
			if _, ok := schema.Properties[field]; !ok {
				t.Errorf("%s response schema is missing public field %s", name, field)
			}
		}
	}
}

func TestDisabledFeatureRegistersNoRoutes(t *testing.T) {
	router := gin.New()
	shop.RegisterRoutes(router.Group("/api/v1/vendor"), &shop.Handler{}, shop.Flags{})
	if len(router.Routes()) != 0 {
		t.Fatalf("disabled Shop registered %d routes", len(router.Routes()))
	}
}
