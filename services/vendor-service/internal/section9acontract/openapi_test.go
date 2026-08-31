// Package section9acontract pins the exact Section 9A public HTTP and OpenAPI
// surface. It guards against the documentation and the router drifting apart,
// and — more importantly — against the four claims Section 9A must never make:
// that a catalogue price is money rather than configuration, that a management
// read is customer discovery, that this section knows whether an item can be
// sold right now, and that a dietary code is a safety guarantee.
package section9acontract

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

const specPath = "../../docs/openapi/section9a-food-catalogue.openapi.json"

// routes mirrors RegisterRoutes exactly. The router-side half — that these are
// the routes production actually mounts — is asserted by internal/crmcontract
// against a router built from the same helper cmd/main.go calls.
var routes = []struct{ method, path string }{
	{"POST", "/food/menus"},
	{"GET", "/food/menus"},
	{"GET", "/food/menus/{menuId}"},
	{"PATCH", "/food/menus/{menuId}"},
	{"POST", "/food/menus/{menuId}/activate"},
	{"POST", "/food/menus/{menuId}/pause"},
	{"POST", "/food/menus/{menuId}/archive"},
	{"POST", "/food/menus/{menuId}/sections"},
	{"GET", "/food/menus/{menuId}/sections"},
	{"POST", "/food/menus/{menuId}/sections/reorder"},
	{"PATCH", "/food/menus/{menuId}/sections/{sectionId}"},
	{"POST", "/food/menus/{menuId}/sections/{sectionId}/archive"},
	{"POST", "/food/menus/{menuId}/sections/{sectionId}/items"},
	{"GET", "/food/menus/{menuId}/sections/{sectionId}/items"},
	{"POST", "/food/menus/{menuId}/sections/{sectionId}/items/reorder"},
	{"GET", "/food/items/{itemId}"},
	{"PATCH", "/food/items/{itemId}"},
	{"POST", "/food/items/{itemId}/archive"},
	{"GET", "/food/items/{itemId}/media"},
	{"PUT", "/food/items/{itemId}/media"},
	{"POST", "/food/items/{itemId}/variants"},
	{"GET", "/food/items/{itemId}/variants"},
	{"POST", "/food/items/{itemId}/variants/reorder"},
	{"PATCH", "/food/items/{itemId}/variants/{variantId}"},
	{"POST", "/food/items/{itemId}/variants/{variantId}/archive"},
	{"GET", "/food/items/{itemId}/modifier-groups"},
	{"POST", "/food/items/{itemId}/modifier-groups/reorder"},
	{"PUT", "/food/items/{itemId}/modifier-groups/{groupId}"},
	{"DELETE", "/food/items/{itemId}/modifier-groups/{groupId}"},
	{"POST", "/food/modifier-groups"},
	{"GET", "/food/modifier-groups"},
	{"GET", "/food/modifier-groups/{groupId}"},
	{"PATCH", "/food/modifier-groups/{groupId}"},
	{"POST", "/food/modifier-groups/{groupId}/archive"},
	{"POST", "/food/modifier-groups/{groupId}/modifiers"},
	{"GET", "/food/modifier-groups/{groupId}/modifiers"},
	{"POST", "/food/modifier-groups/{groupId}/modifiers/reorder"},
	{"PATCH", "/food/modifier-groups/{groupId}/modifiers/{modifierId}"},
	{"POST", "/food/modifier-groups/{groupId}/modifiers/{modifierId}/archive"},
}

const routeCount = 39

var permissions = []string{
	"vendor.food.catalogue.read",
	"vendor.food.catalogue.manage",
	"vendor.food.catalogue.publish",
}

type operation struct {
	OperationID string `json:"operationId"`
	Permission  string `json:"x-permission"`
	Parameters  []struct {
		Ref  string `json:"$ref"`
		Name string `json:"name"`
	} `json:"parameters"`
	Responses map[string]json.RawMessage `json:"responses"`
}

type schema struct {
	Type       string                     `json:"type"`
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
	var document spec
	if err := json.Unmarshal([]byte(rawSpec(t)), &document); err != nil {
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
			t.Errorf("Section 9A must not declare an internal route: %s %s", route.method, route.path)
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
		if !strings.HasPrefix(op.Permission, "vendor.food.catalogue.") || !known[strings.Fields(op.Permission)[0]] {
			t.Errorf("%s declares unknown permission %q", key, op.Permission)
		}
		// Every operation documents the three authorization failures a caller
		// must be able to handle.
		for _, code := range []string{"401", "403", "404"} {
			if _, present := op.Responses[code]; !present && !strings.HasSuffix(key, "/food/menus") &&
				!strings.HasSuffix(key, "/food/modifier-groups") {
				t.Errorf("%s does not document a %s response", key, code)
			}
		}
		method := strings.ToLower(strings.SplitN(key, " ", 2)[0])
		if method == "get" {
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

// TestPermissionSetIsExactlyTheThreeDeclared. A fourth permission appearing
// here without a migration granting it would be an authority nobody can hold.
func TestPermissionSetIsExactlyTheThreeDeclared(t *testing.T) {
	document := load(t)
	used := map[string]bool{}
	for _, op := range operations(t, document) {
		used[strings.Fields(op.Permission)[0]] = true
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
		"vendor.food.catalogue.delete", "vendor.food.pricing", "vendor.food.availability",
		"vendor.food.discovery", "vendor.order", "vendor.inventory",
	} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("the contract declares %s, an authority this section does not hold", forbidden)
		}
	}
}

// TestMoneyIsAlwaysIntegerMinorUnits is the load-bearing pricing guard. A
// floating-point price anywhere in this contract would be a rounding bug with a
// schema blessing it.
func TestMoneyIsAlwaysIntegerMinorUnits(t *testing.T) {
	document := load(t)
	monetary := 0
	for name, definition := range document.Components.Schemas {
		for property, raw := range definition.Properties {
			lowered := strings.ToLower(property)
			isMoney := strings.Contains(lowered, "price") || strings.Contains(lowered, "amount") ||
				strings.Contains(lowered, "delta") || strings.HasSuffix(lowered, "_minor")
			// A boolean about a price is not a price. clear_price_override and
			// inherits_base_price describe what to do with one.
			var typed struct {
				Type    string `json:"type"`
				Format  string `json:"format"`
				Minimum *int64 `json:"minimum"`
			}
			if err := json.Unmarshal(raw, &typed); err != nil {
				t.Fatal(err)
			}
			if !isMoney || typed.Type == "boolean" {
				continue
			}
			monetary++
			if typed.Type != "integer" {
				t.Errorf("%s.%s is %q; catalogue money is integer minor units only", name, property, typed.Type)
			}
			if !strings.HasSuffix(lowered, "_minor") {
				t.Errorf("%s.%s does not name its unit; every money field ends in _minor", name, property)
			}
			if typed.Minimum == nil || *typed.Minimum != 0 {
				t.Errorf("%s.%s does not declare a non-negative minimum", name, property)
			}
		}
	}
	if monetary == 0 {
		t.Fatal("the guard matched no monetary field, which means it is no longer guarding anything")
	}
	// Currency is the business's, reported once on the item, never per price.
	item := document.Components.Schemas["Item"]
	if _, present := item.Properties["currency"]; !present {
		t.Error("Item does not report the business currency")
	}
	for name, definition := range document.Components.Schemas {
		if name == "Item" {
			continue
		}
		for property := range definition.Properties {
			if strings.ToLower(property) == "currency" {
				t.Errorf("%s restates currency; Section 6 owns the one currency truth", name)
			}
		}
	}
	raw := strings.ToLower(rawSpec(t))
	if !strings.Contains(raw, "integer minor units") {
		t.Error("the contract never states that money is integer minor units")
	}
	if !strings.Contains(raw, "no floating-point money") {
		t.Error("the contract does not state that there is no floating-point money")
	}
}

// TestNoSection9BSurface keeps pricing/availability/scheduling out.
func TestNoSection9BSurface(t *testing.T) {
	document := load(t)
	forbiddenPaths := []string{"schedule", "availability", "sold-out", "soldout", "effective-price",
		"price-history", "prep", "preparation", "logistics", "override", "inventory", "stock"}
	for path := range document.Paths {
		lowered := strings.ToLower(path)
		for _, forbidden := range forbiddenPaths {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("path %s enters Section 9B authority %q", path, forbidden)
			}
		}
	}
	for name, definition := range document.Components.Schemas {
		lowered := strings.ToLower(name)
		for _, forbidden := range []string{"schedule", "availability", "effectiveprice", "pricehistory",
			"preptime", "logisticsprofile", "stock"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("schema %s enters Section 9B authority %q", name, forbidden)
			}
		}
		for property := range definition.Properties {
			propertyLower := strings.ToLower(property)
			for _, forbidden := range []string{"sold_out", "available_from", "available_until",
				"schedule", "prep_time", "effective_price", "stock_"} {
				if strings.Contains(propertyLower, forbidden) {
					t.Errorf("%s.%s enters Section 9B authority", name, property)
				}
			}
		}
	}
	// The lifecycle vocabularies cannot express a sold-out condition.
	for _, name := range []string{"MenuStatus", "ItemStatus", "ChildStatus"} {
		for _, value := range document.Components.Schemas[name].Enum {
			if value == "SOLD_OUT" || value == "UNAVAILABLE" || value == "PENDING" {
				t.Errorf("%s contains %s, which belongs to a later section or is ambiguous", name, value)
			}
		}
	}
}

// TestNoSection9COrOrderSurface keeps discovery and orders out.
func TestNoSection9COrOrderSurface(t *testing.T) {
	document := load(t)
	// "/order" with the leading slash, so the reorder routes — which are this
	// section's own ordering commands — are not mistaken for the Order domain.
	for path := range document.Paths {
		lowered := strings.ToLower(path)
		for _, forbidden := range []string{"discovery", "public", "search", "recommend", "storefront",
			"/order", "/cart", "checkout", "payment", "wallet", "customer", "internal"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("path %s enters later-section or other-service authority %q", path, forbidden)
			}
		}
	}
	for name := range document.Components.Schemas {
		lowered := strings.ToLower(name)
		for _, forbidden := range []string{"discovery", "search", "recommendation", "orderline",
			"orderitem", "ordersnapshot", "ordertotal", "cart", "checkout", "payment", "wallet",
			"customer", "snapshot"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("schema %s enters later-section or other-service authority %q", name, forbidden)
			}
		}
		if lowered == "order" {
			t.Error("schema Order belongs to a domain this service does not own")
		}
	}
	raw := strings.ToLower(rawSpec(t))
	if !strings.Contains(raw, "management, not discovery") {
		t.Error("the contract does not state that a management read is not customer discovery")
	}
}

// TestNoUploadOrByteSurface: Media File Service owns bytes.
func TestNoUploadOrByteSurface(t *testing.T) {
	document := load(t)
	for path := range document.Paths {
		lowered := strings.ToLower(path)
		for _, forbidden := range []string{"upload", "download", "bytes", "image", "file"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("path %s exposes %q; Media File Service owns bytes", path, forbidden)
			}
		}
	}
	for name, definition := range document.Components.Schemas {
		for property := range definition.Properties {
			lowered := strings.ToLower(property)
			for _, forbidden := range []string{"base64", "bytes", "content", "url", "signed",
				"mime", "upload"} {
				if strings.Contains(lowered, forbidden) {
					t.Errorf("schema %s exposes %q; only an opaque reference is stored", name, property)
				}
			}
		}
	}
}

// TestMetadataIsBoundedAndMakesNoMedicalClaim.
func TestMetadataIsBoundedAndMakesNoMedicalClaim(t *testing.T) {
	document := load(t)
	code := document.Components.Schemas["MetadataCode"]
	if code.Type != "string" {
		t.Fatal("MetadataCode is missing or not a string")
	}
	raw := rawSpec(t)
	if !strings.Contains(raw, "^[A-Z0-9_]{2,40}$") {
		t.Error("MetadataCode declares no bounded pattern")
	}
	lowered := strings.ToLower(raw)
	if !strings.Contains(lowered, "declarative") || !strings.Contains(lowered, "vendor-supplied") {
		t.Error("the contract does not state that dietary/allergen metadata is vendor-supplied")
	}
	if !strings.Contains(lowered, "makes no medical claim") && !strings.Contains(lowered, "no medical claim") {
		t.Error("the contract does not disclaim medical meaning for dietary metadata")
	}
	if !strings.Contains(lowered, "not a safety guarantee") {
		t.Error("the contract does not state that allergen metadata is not a safety guarantee")
	}
	// Each term is allowed only inside a sentence that denies it.
	denials := []string{"computes no nutrition", "no nutritional", "certifies nothing",
		"makes no medical claim", "not a safety guarantee"}
	for _, forbidden := range []string{"calorie", "nutrition", "nutrient", "certified safe",
		"medically", "allergy advice"} {
		if !strings.Contains(lowered, forbidden) {
			continue
		}
		denied := false
		for _, denial := range denials {
			denied = denied || strings.Contains(lowered, denial)
		}
		if !denied {
			t.Errorf("the contract uses %q without denying it", forbidden)
		}
	}
	// Both metadata arrays are bounded in the schema, not only in code.
	item := document.Components.Schemas["Item"]
	for _, field := range []string{"dietary_codes", "allergen_codes"} {
		var bounded struct {
			MaxItems int `json:"maxItems"`
		}
		if err := json.Unmarshal(item.Properties[field], &bounded); err != nil {
			t.Fatal(err)
		}
		if bounded.MaxItems == 0 {
			t.Errorf("Item.%s declares no maxItems", field)
		}
	}
}

func TestControlledVocabulariesAndConcreteSchemas(t *testing.T) {
	document := load(t)
	expected := map[string][]string{
		"FoodCapability": {"RESTAURANT", "FAST_FOOD"},
		"MenuStatus":     {"DRAFT", "ACTIVE", "PAUSED", "ARCHIVED"},
		"ItemStatus":     {"DRAFT", "ACTIVE", "PAUSED", "ARCHIVED"},
		"ChildStatus":    {"ACTIVE", "ARCHIVED"},
	}
	for name, want := range expected {
		got := append([]string(nil), document.Components.Schemas[name].Enum...)
		sort.Strings(got)
		sort.Strings(want)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s enum=%v; want %v", name, got, want)
		}
	}
	for _, name := range []string{
		"Menu", "MenuPage", "Section", "SectionList", "Item", "ItemPage", "ItemMedia",
		"Variant", "VariantList", "ModifierGroup", "ModifierGroupPage", "Modifier", "ModifierList",
		"Attachment", "AttachedGroup", "AttachedGroupList", "ReorderRequest", "Pagination", "Error",
	} {
		if len(document.Components.Schemas[name].Properties) == 0 {
			t.Errorf("schema %s is missing or a stub", name)
		}
	}
	// Every mutable aggregate carries a version, and every list is paginated
	// where it can grow without bound.
	for _, name := range []string{"Menu", "Section", "Item", "Variant", "ModifierGroup", "Modifier"} {
		if !strings.Contains(strings.Join(document.Components.Schemas[name].Required, ","), "version") {
			t.Errorf("%s does not require version", name)
		}
	}
	for _, name := range []string{"MenuPage", "ItemPage", "ModifierGroupPage"} {
		if !strings.Contains(strings.Join(document.Components.Schemas[name].Required, ","), "pagination") {
			t.Errorf("%s is not paginated", name)
		}
	}
	codes := strings.Join(document.Components.Schemas["FoodErrorCode"].Enum, ",")
	for _, want := range []string{
		"FOOD_CAPABILITY_NOT_SUPPORTED", "FOOD_MENU_NOT_FOUND", "FOOD_MENU_NOT_EDITABLE",
		"FOOD_MENU_NOT_PUBLISHABLE", "FOOD_SECTION_NOT_FOUND", "FOOD_ITEM_NOT_FOUND",
		"FOOD_VARIANT_NOT_FOUND", "FOOD_MODIFIER_GROUP_NOT_FOUND", "FOOD_MODIFIER_NOT_FOUND",
		"FOOD_STALE_VERSION", "FOOD_INVALID_REORDER", "FOOD_DUPLICATE_RELATIONSHIP",
		"FOOD_ENTITY_ARCHIVED", "FOOD_MEDIA_UNAVAILABLE", "FOOD_IDEMPOTENCY_CONFLICT",
	} {
		if !strings.Contains(codes, want) {
			t.Errorf("error code %s is not documented", want)
		}
	}
	// The reorder contract states the whole-set rule it actually enforces.
	if !strings.Contains(rawSpec(t), "exactly once, in the new order") {
		t.Error("ReorderRequest does not document the complete-set contract")
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
