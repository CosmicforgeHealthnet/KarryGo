//go:build ignore

// Command gen_section12a_openapi emits the Section 12A management contract.
// The route table is intentionally explicit so contract tests can compare it
// bidirectionally with shop.RegisterRoutes.
package main

import (
	"encoding/json"
	"os"
	"strings"
	"unicode"
)

type route struct{ method, path, summary string }

var routes = []route{
	{"GET", "/shop/catalogue-revision", "Read the scoped monotonic Shop catalogue revision"},
	{"POST", "/shop/categories", "Create a Shop category"}, {"GET", "/shop/categories", "List the category tree"},
	{"GET", "/shop/categories/{categoryId}", "Get a category"}, {"PATCH", "/shop/categories/{categoryId}", "Update or move a category"},
	{"POST", "/shop/categories/{categoryId}/archive", "Archive a category"}, {"POST", "/shop/categories/{categoryId}/restore", "Restore a category"},
	{"POST", "/shop/measurements", "Create a tenant-scoped measurement definition"}, {"GET", "/shop/measurements", "List platform and tenant measurement definitions"},
	{"GET", "/shop/measurements/{measureId}", "Get a measurement and aliases"}, {"PATCH", "/shop/measurements/{measureId}", "Update a tenant measurement"},
	{"POST", "/shop/measurements/{measureId}/archive", "Archive a tenant measurement"}, {"GET", "/shop/measurements/{measureId}/aliases", "List measurement aliases"},
	{"POST", "/shop/measurements/{measureId}/aliases", "Add a scoped measurement alias"}, {"DELETE", "/shop/measurements/{measureId}/aliases/{aliasId}", "Remove a scoped measurement alias"},
	{"POST", "/shop/products", "Create a Shop product master"}, {"GET", "/shop/products", "List Shop products"},
	{"GET", "/shop/products/{productId}", "Get a Shop product"}, {"PATCH", "/shop/products/{productId}", "Update a Shop product"},
	{"POST", "/shop/products/{productId}/archive", "Archive a Shop product"},
	{"POST", "/shop/products/{productId}/variants", "Create a product variant or SKU"}, {"GET", "/shop/products/{productId}/variants", "List product variants"},
	{"PATCH", "/shop/products/{productId}/variants/{variantId}", "Update a product variant"}, {"POST", "/shop/products/{productId}/variants/{variantId}/archive", "Archive a product variant"},
	{"POST", "/shop/products/{productId}/measure-calibrations", "Create a product or branch-specific local-measure calibration"},
	{"GET", "/shop/products/{productId}/measure-calibrations", "List product measure calibrations"},
	{"PATCH", "/shop/products/{productId}/measure-calibrations/{calibrationId}", "Update an approximate calibration"},
	{"POST", "/shop/products/{productId}/measure-calibrations/{calibrationId}/archive", "Archive an approximate calibration"},
	{"POST", "/shop/products/{productId}/sell-units", "Create a fixed, fractional, calibrated or price-portion sell unit"},
	{"GET", "/shop/products/{productId}/sell-units", "List product sell units"},
	{"PATCH", "/shop/products/{productId}/sell-units/{sellUnitId}", "Update a sell unit and commercial terms"},
	{"POST", "/shop/products/{productId}/sell-units/{sellUnitId}/archive", "Archive a sell unit"},
	{"POST", "/shop/products/{productId}/packaging", "Create an explicit packaging relationship"},
	{"GET", "/shop/products/{productId}/packaging", "List the product packaging hierarchy"},
	{"POST", "/shop/products/{productId}/packaging/{relationId}/archive", "Archive a packaging relationship"},
	{"PUT", "/shop/products/{productId}/fresh-attributes", "Create or replace a controlled fresh attribute configuration"},
	{"GET", "/shop/products/{productId}/fresh-attributes", "List fresh attribute configurations"},
	{"POST", "/shop/products/{productId}/fresh-attributes/{attributeId}/archive", "Archive a fresh attribute configuration"},
	{"GET", "/shop/variants/{variantId}", "Get a variant"},
	{"POST", "/shop/variants/{variantId}/identifiers", "Add an SKU, GTIN, EAN, UPC or barcode"},
	{"GET", "/shop/variants/{variantId}/identifiers", "List variant identifiers"},
	{"POST", "/shop/variants/{variantId}/identifiers/{identifierId}/archive", "Archive a variant identifier"},
	{"GET", "/shop/identifier-lookup", "Look up a Shop variant by exact identifier"},
	{"GET", "/shop/branches/{branchId}/operating-models", "Read branch Shop operating models and eligible capabilities"},
	{"PUT", "/shop/branches/{branchId}/operating-models", "Set enabled and optional primary operating models"},
	{"POST", "/shop/branches/{branchId}/operating-models/archive", "Archive branch operating-model configuration"},
	{"POST", "/shop/branches/{branchId}/listings", "Create a branch commercial listing without stock"},
	{"GET", "/shop/branches/{branchId}/listings", "List branch commercial listings"},
	{"GET", "/shop/listings/{listingId}", "Get a listing and selected sell units"},
	{"PATCH", "/shop/listings/{listingId}", "Update listing channels, terms and overrides"},
	{"POST", "/shop/listings/{listingId}/archive", "Archive a branch listing"},
	{"PUT", "/shop/listings/{listingId}/sell-units/{listingUnitId}", "Set branch sell-unit selection, calibration and price overrides"},
	{"POST", "/shop/listings/{listingId}/quantity-breaks", "Create a non-overlapping wholesale quantity band"},
	{"GET", "/shop/listings/{listingId}/quantity-breaks", "List wholesale quantity bands"},
	{"PATCH", "/shop/listings/{listingId}/quantity-breaks/{breakId}", "Update a wholesale quantity band"},
	{"POST", "/shop/listings/{listingId}/quantity-breaks/{breakId}/archive", "Archive a wholesale quantity band"},
	{"GET", "/shop/listings/{listingId}/resolved-price", "Resolve deterministic retail or wholesale commercial price"},
	{"GET", "/shop/price-history", "Read append-only price versions"},
}

func operationID(r route) string {
	name := strings.ToLower(r.method)
	for _, part := range strings.FieldsFunc(r.path, func(ch rune) bool { return !unicode.IsLetter(ch) && !unicode.IsDigit(ch) }) {
		if part == "shop" || part == "api" || part == "v1" || part == "vendor" {
			continue
		}
		name += strings.ToUpper(part[:1]) + part[1:]
	}
	return name
}

func requestSchema(r route) string {
	if r.method == "GET" {
		return ""
	}
	key := r.method + " " + r.path
	if strings.HasSuffix(r.path, "/archive") || strings.HasSuffix(r.path, "/restore") {
		return "VersionCommand"
	}
	switch key {
	case "POST /shop/categories":
		return "CreateCategoryCommand"
	case "PATCH /shop/categories/{categoryId}":
		return "UpdateCategoryCommand"
	case "POST /shop/measurements":
		return "CreateMeasureCommand"
	case "PATCH /shop/measurements/{measureId}":
		return "UpdateMeasureCommand"
	case "POST /shop/measurements/{measureId}/aliases":
		return "AliasCommand"
	case "DELETE /shop/measurements/{measureId}/aliases/{aliasId}":
		return "EmptyCommand"
	case "POST /shop/products":
		return "CreateProductCommand"
	case "PATCH /shop/products/{productId}":
		return "UpdateProductCommand"
	case "POST /shop/products/{productId}/variants":
		return "CreateVariantCommand"
	case "PATCH /shop/products/{productId}/variants/{variantId}":
		return "UpdateVariantCommand"
	case "POST /shop/products/{productId}/measure-calibrations":
		return "CreateCalibrationCommand"
	case "PATCH /shop/products/{productId}/measure-calibrations/{calibrationId}":
		return "UpdateCalibrationCommand"
	case "POST /shop/products/{productId}/sell-units":
		return "CreateSellUnitCommand"
	case "PATCH /shop/products/{productId}/sell-units/{sellUnitId}":
		return "UpdateSellUnitCommand"
	case "POST /shop/products/{productId}/packaging":
		return "PackageCommand"
	case "PUT /shop/products/{productId}/fresh-attributes":
		return "FreshAttributeCommand"
	case "POST /shop/variants/{variantId}/identifiers":
		return "IdentifierCommand"
	case "PUT /shop/branches/{branchId}/operating-models":
		return "OperatingModelsCommand"
	case "POST /shop/branches/{branchId}/listings":
		return "CreateListingCommand"
	case "PATCH /shop/listings/{listingId}":
		return "UpdateListingCommand"
	case "PUT /shop/listings/{listingId}/sell-units/{listingUnitId}":
		return "ListingSellUnitCommand"
	case "POST /shop/listings/{listingId}/quantity-breaks":
		return "CreateQuantityBreakCommand"
	case "PATCH /shop/listings/{listingId}/quantity-breaks/{breakId}":
		return "UpdateQuantityBreakCommand"
	}
	panic("missing Section 12A request schema for " + key)
}

func queryParameters(r route) []any {
	key := r.method + " " + r.path
	stringQuery := func(name string, required bool) map[string]any {
		return map[string]any{"name": name, "in": "query", "required": required,
			"schema": map[string]any{"type": "string", "maxLength": 200}}
	}
	boolQuery := func(name string) map[string]any {
		return map[string]any{"name": name, "in": "query", "schema": map[string]any{"type": "boolean"}}
	}
	integerQuery := func(name string, minimum, maximum, fallback int) map[string]any {
		schema := map[string]any{"type": "integer", "minimum": minimum, "default": fallback}
		if maximum > 0 {
			schema["maximum"] = maximum
		}
		return map[string]any{"name": name, "in": "query", "schema": schema}
	}
	page := func() []any { return []any{integerQuery("limit", 1, 100, 50), integerQuery("offset", 0, 0, 0)} }
	archived := func() []any { return []any{boolQuery("include_archived")} }
	switch key {
	case "GET /shop/catalogue-revision":
		return []any{stringQuery("branch_id", true)}
	case "GET /shop/categories":
		return append([]any{stringQuery("parent_id", false), boolQuery("include_archived")}, page()...)
	case "GET /shop/measurements":
		return append([]any{stringQuery("family", false), stringQuery("origin", false), stringQuery("search", false), boolQuery("include_archived")}, page()...)
	case "GET /shop/products":
		return append([]any{stringQuery("category_id", false), stringQuery("product_class", false), stringQuery("brand", false), stringQuery("status", false), stringQuery("search", false), boolQuery("include_archived")}, page()...)
	case "GET /shop/products/{productId}/variants",
		"GET /shop/products/{productId}/measure-calibrations",
		"GET /shop/products/{productId}/packaging",
		"GET /shop/products/{productId}/fresh-attributes",
		"GET /shop/variants/{variantId}/identifiers",
		"GET /shop/listings/{listingId}/quantity-breaks":
		return archived()
	case "GET /shop/products/{productId}/sell-units":
		return []any{stringQuery("variant_id", false), boolQuery("include_archived")}
	case "GET /shop/identifier-lookup":
		return []any{stringQuery("value", true), stringQuery("type", false)}
	case "GET /shop/branches/{branchId}/listings":
		return append([]any{stringQuery("product_id", false), stringQuery("variant_id", false), stringQuery("status", false), boolQuery("include_archived")}, page()...)
	case "GET /shop/listings/{listingId}/resolved-price":
		return []any{stringQuery("sell_unit_id", false), integerQuery("quantity", 1, 1000000000, 1),
			map[string]any{"name": "channel", "in": "query", "schema": map[string]any{"type": "string", "enum": []string{"retail", "wholesale"}, "default": "retail"}}}
	case "GET /shop/price-history":
		return append([]any{stringQuery("subject_type", false), stringQuery("subject_ref", false), stringQuery("variant_ref", false), stringQuery("product_ref", false)}, page()...)
	}
	return nil
}

func schemaRef(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func entitySchema(r route) string {
	path := r.path
	switch {
	case path == "/shop/catalogue-revision":
		return "CatalogueRevision"
	case strings.Contains(path, "/categories"):
		return "Category"
	case strings.Contains(path, "/measurements/") && strings.Contains(path, "/aliases"):
		return "MeasureAlias"
	case strings.Contains(path, "/measurements"):
		return "MeasureDefinition"
	case strings.Contains(path, "/measure-calibrations"):
		return "Calibration"
	case strings.Contains(path, "/sell-units") && strings.Contains(path, "/listings/"):
		return "ListingSellUnit"
	case strings.Contains(path, "/sell-units"):
		return "SellUnit"
	case strings.Contains(path, "/packaging"):
		return "PackageRelation"
	case strings.Contains(path, "/fresh-attributes"):
		return "FreshAttribute"
	case strings.Contains(path, "/identifiers") || path == "/shop/identifier-lookup":
		return "Identifier"
	case strings.Contains(path, "/variants"):
		return "Variant"
	case strings.Contains(path, "/products"):
		return "Product"
	case strings.Contains(path, "/operating-models"):
		return "OperatingModelConfiguration"
	case strings.Contains(path, "/quantity-breaks"):
		return "QuantityBreak"
	case strings.Contains(path, "/resolved-price"):
		return "ResolvedPrice"
	case path == "/shop/price-history":
		return "PriceVersion"
	case strings.Contains(path, "/listings"):
		return "BranchListing"
	}
	panic("missing Section 12A response schema for " + r.method + " " + r.path)
}

func responseDataSchema(r route) map[string]any {
	key := r.method + " " + r.path
	entity := entitySchema(r)
	arrayOf := func(name string) map[string]any {
		return map[string]any{"type": "array", "items": schemaRef(name)}
	}
	paged := func(name string) map[string]any {
		return map[string]any{"type": "object", "required": []string{"items", "has_more"},
			"properties": map[string]any{"items": arrayOf(name), "has_more": map[string]any{"type": "boolean"}}}
	}
	switch key {
	case "GET /shop/categories", "GET /shop/measurements", "GET /shop/products",
		"GET /shop/branches/{branchId}/listings", "GET /shop/price-history":
		return paged(entity)
	case "GET /shop/products/{productId}/variants", "GET /shop/products/{productId}/measure-calibrations",
		"GET /shop/products/{productId}/sell-units", "GET /shop/products/{productId}/packaging",
		"GET /shop/products/{productId}/fresh-attributes", "GET /shop/variants/{variantId}/identifiers",
		"GET /shop/listings/{listingId}/quantity-breaks", "GET /shop/measurements/{measureId}/aliases":
		return arrayOf(entity)
	case "GET /shop/measurements/{measureId}":
		return map[string]any{"type": "object", "required": []string{"measurement", "aliases"}, "properties": map[string]any{
			"measurement": schemaRef("MeasureDefinition"), "aliases": arrayOf("MeasureAlias")}}
	case "GET /shop/listings/{listingId}":
		return map[string]any{"type": "object", "required": []string{"listing", "sell_units"}, "properties": map[string]any{
			"listing": schemaRef("BranchListing"), "sell_units": arrayOf("ListingSellUnit")}}
	case "GET /shop/identifier-lookup":
		return map[string]any{"type": "object", "required": []string{"identifier", "variant", "product"}, "properties": map[string]any{
			"identifier": schemaRef("Identifier"), "variant": schemaRef("Variant"), "product": schemaRef("Product")}}
	case "GET /shop/branches/{branchId}/operating-models":
		return map[string]any{"type": "object", "required": []string{"configuration", "eligible_capabilities"}, "properties": map[string]any{
			"configuration":         schemaRef("OperatingModelConfiguration"),
			"eligible_capabilities": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}}
	case "DELETE /shop/measurements/{measureId}/aliases/{aliasId}":
		return map[string]any{"type": "object", "properties": map[string]any{"removed": map[string]any{"type": "boolean", "enum": []bool{true}}}}
	}
	return schemaRef(entity)
}

func successStatus(r route) string {
	key := r.method + " " + r.path
	for _, created := range []string{
		"POST /shop/categories", "POST /shop/measurements", "POST /shop/measurements/{measureId}/aliases",
		"POST /shop/products", "POST /shop/products/{productId}/variants",
		"POST /shop/products/{productId}/measure-calibrations", "POST /shop/products/{productId}/sell-units",
		"POST /shop/products/{productId}/packaging", "POST /shop/variants/{variantId}/identifiers",
		"POST /shop/branches/{branchId}/listings", "POST /shop/listings/{listingId}/quantity-breaks",
	} {
		if key == created {
			return "201"
		}
	}
	return "200"
}

func successResponse(r route) map[string]any {
	return map[string]any{"description": "Successful authenticated management response", "content": map[string]any{
		"application/json": map[string]any{"schema": map[string]any{"type": "object", "required": []string{"success", "data"},
			"properties": map[string]any{"success": map[string]any{"type": "boolean", "enum": []bool{true}}, "data": responseDataSchema(r)}}}}}
}

func main() {
	paths := map[string]any{}
	for _, r := range routes {
		status := successStatus(r)
		op := map[string]any{
			"tags": []string{"Shop Management"}, "summary": r.summary, "operationId": operationID(r),
			"responses": map[string]any{status: successResponse(r), "400": map[string]any{"$ref": "#/components/responses/Error"}, "401": map[string]any{"$ref": "#/components/responses/Error"}, "403": map[string]any{"$ref": "#/components/responses/Error"}, "409": map[string]any{"$ref": "#/components/responses/Error"}},
		}
		params := []any{}
		for _, segment := range strings.Split(r.path, "/") {
			if strings.HasPrefix(segment, "{") {
				params = append(params, map[string]any{"name": strings.Trim(segment, "{}"), "in": "path", "required": true, "schema": map[string]any{"type": "string", "maxLength": 100}})
			}
		}
		params = append(params, queryParameters(r)...)
		if r.method != "GET" {
			params = append(params, map[string]any{"$ref": "#/components/parameters/IdempotencyKey"})
			op["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/" + requestSchema(r)}}}}
		}
		if len(params) > 0 {
			op["parameters"] = params
		}
		method := strings.ToLower(r.method)
		if paths[r.path] == nil {
			paths[r.path] = map[string]any{}
		}
		paths[r.path].(map[string]any)[method] = op
	}

	doc := map[string]any{
		"openapi": "3.0.3",
		"info":    map[string]any{"title": "KarryGo Vendor Service — Section 12A Shop Catalogue", "version": "1.0.0", "description": "Authenticated Shop/Grocery/Market/Wholesale management only. Supports SUPERMARKET, GROCERY_SHOP, FOODSTUFF_DEALER, MARKET_TRADER and WHOLESALER operating models subject to branch capabilities. Measurement is first-class: exact gram/millilitre/count conversions are separate from product/branch-specific calibrated approximations; PAINT, DERICA and MUDU have no universal weight. Fractional sell units use bounded numerator/denominator values. All money is integer minor units. Price history is append-only. Listings hold commercial configuration and never authoritative inventory. Section 12B inventory, stock, reservations, customer discovery, Orders, Wallet and logistics execution are not implemented here."},
		"servers": []any{map[string]any{"url": "/api/v1/vendor"}}, "security": []any{map[string]any{"bearerAuth": []string{}}},
		"tags":  []any{map[string]any{"name": "Shop Management", "description": "Section 12A catalogue and commercial management; never customer discovery or inventory."}},
		"paths": paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{"bearerAuth": map[string]any{"type": "http", "scheme": "bearer", "bearerFormat": "JWT"}},
			"parameters":      map[string]any{"IdempotencyKey": map[string]any{"name": "Idempotency-Key", "in": "header", "required": true, "schema": map[string]any{"type": "string", "minLength": 8, "maxLength": 200}}},
			"responses":       map[string]any{"Error": map[string]any{"description": "Stable platform error envelope", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Error"}}}}},
			"schemas":         schemas(),
		},
	}
	raw, _ := json.MarshalIndent(doc, "", "  ")
	raw = append(raw, '\n')
	if err := os.WriteFile("docs/openapi/section12a-shop-catalogue.openapi.json", raw, 0o644); err != nil {
		panic(err)
	}
}

func schemas() map[string]any {
	out := map[string]any{
		"Error":                       map[string]any{"type": "object", "required": []string{"success", "error"}, "properties": map[string]any{"success": map[string]any{"type": "boolean", "enum": []bool{false}}, "error": map[string]any{"type": "object"}}},
		"OperatingModel":              map[string]any{"type": "string", "enum": []string{"SUPERMARKET", "GROCERY_SHOP", "FOODSTUFF_DEALER", "MARKET_TRADER", "WHOLESALER"}},
		"MeasurementFamily":           map[string]any{"type": "string", "enum": []string{"MASS", "VOLUME", "COUNT", "LOCAL_CONTAINER", "PACKAGE", "GROUPED_COUNT", "LOAD", "VARIABLE_WEIGHT", "VARIABLE_COUNT", "PRICE_PORTION", "OTHER_CONTROLLED"}},
		"ConversionType":              map[string]any{"type": "string", "enum": []string{"EXACT", "CALIBRATED_APPROXIMATE", "NONE"}},
		"MeasurementMode":             map[string]any{"type": "string", "enum": []string{"FIXED", "VARIABLE_WEIGHT", "VARIABLE_COUNT", "LOCAL_CONTAINER", "PACKAGE_COUNT", "PRICE_PORTION"}},
		"ProductClass":                map[string]any{"type": "string", "enum": []string{"PACKAGED", "DRY_BULK", "FRESH_PRODUCE", "MEAT", "FISH", "FROZEN", "CHILLED", "DAIRY", "BEVERAGE", "BAKED_PACKAGED", "HOUSEHOLD_GROCERY", "LOCAL_FOODSTUFF", "OTHER"}},
		"TemperatureZone":             map[string]any{"type": "string", "enum": []string{"AMBIENT", "FRESH", "CHILLED", "FROZEN"}},
		"MinorMoney":                  map[string]any{"type": "integer", "format": "int64", "minimum": 0, "maximum": 99999999999, "description": "Whole minor units in server-authoritative business currency; never float."},
		"Fraction":                    map[string]any{"type": "object", "required": []string{"numerator", "denominator"}, "properties": map[string]any{"numerator": map[string]any{"type": "integer", "minimum": 1, "maximum": 1000}, "denominator": map[string]any{"type": "integer", "minimum": 1, "maximum": 1000}}},
		"Calibration":                 map[string]any{"type": "object", "description": "Product/variant and optional branch scoped approximation. It can never be treated as an exact stock conversion.", "required": []string{"id", "product_ref", "measure_ref", "tolerance_bps", "calibration_source", "approximate", "status", "version"}, "properties": map[string]any{"id": map[string]any{"type": "string"}, "branch_ref": map[string]any{"type": "string", "nullable": true}, "product_ref": map[string]any{"type": "string"}, "variant_ref": map[string]any{"type": "string", "nullable": true}, "measure_ref": map[string]any{"type": "string"}, "approximate_weight_grams": map[string]any{"type": "integer", "minimum": 1, "nullable": true}, "approximate_volume_ml": map[string]any{"type": "integer", "minimum": 1, "nullable": true}, "tolerance_bps": map[string]any{"type": "integer", "minimum": 0, "maximum": 10000}, "calibration_source": map[string]any{"type": "string"}, "approximate": map[string]any{"type": "boolean", "enum": []bool{true}}, "status": map[string]any{"type": "string"}, "version": map[string]any{"type": "integer", "format": "int64"}}},
		"VariableMeasurement":         map[string]any{"type": "object", "properties": map[string]any{"mode": map[string]any{"$ref": "#/components/schemas/MeasurementMode"}, "underfill_tolerance_bps": map[string]any{"type": "integer", "minimum": 0, "maximum": 10000}, "overfill_tolerance_bps": map[string]any{"type": "integer", "minimum": 0, "maximum": 10000}, "actual_measurement_required": map[string]any{"type": "boolean"}}},
		"InventoryAuthority":          map[string]any{"type": "string", "enum": []string{"NOT_IMPLEMENTED"}, "description": "Section 12A intentionally publishes no inventory authority."},
		"CatalogueRevision":           map[string]any{"type": "object", "required": []string{"revision", "updated_at", "domain"}, "properties": map[string]any{"revision": map[string]any{"type": "integer", "format": "int64", "minimum": 0}, "updated_at": map[string]any{"type": "string", "format": "date-time"}, "domain": map[string]any{"type": "string", "enum": []string{"SHOP"}}}},
		"MeasureDefinition":           map[string]any{"type": "object", "required": []string{"id", "code", "display_name", "measurement_family", "measure_origin", "scope", "converts_exactly", "status", "version"}, "properties": map[string]any{"id": map[string]any{"type": "string"}, "code": map[string]any{"type": "string", "pattern": "^[A-Z][A-Z0-9_]{1,39}$"}, "display_name": map[string]any{"type": "string", "maxLength": 80}, "measurement_family": map[string]any{"$ref": "#/components/schemas/MeasurementFamily"}, "measure_origin": map[string]any{"type": "string", "enum": []string{"STANDARD", "LOCAL", "PACKAGE", "VENDOR_DEFINED"}}, "scope": map[string]any{"type": "string", "enum": []string{"PLATFORM", "BUSINESS"}}, "region_code": map[string]any{"type": "string"}, "base_unit": map[string]any{"type": "string"}, "base_quantity": map[string]any{"type": "integer", "format": "int64", "nullable": true, "description": "Exact canonical grams, millilitres or count only; always null for local containers."}, "converts_exactly": map[string]any{"type": "boolean"}, "status": map[string]any{"type": "string"}, "version": map[string]any{"type": "integer", "format": "int64"}}},
		"Category":                    map[string]any{"type": "object", "required": []string{"id", "name", "slug", "position", "depth", "status", "version"}, "properties": map[string]any{"id": map[string]any{"type": "string"}, "parent_ref": map[string]any{"type": "string", "nullable": true}, "name": map[string]any{"type": "string", "maxLength": 120}, "slug": map[string]any{"type": "string", "maxLength": 80}, "description": map[string]any{"type": "string", "maxLength": 2000}, "position": map[string]any{"type": "integer", "minimum": 0}, "depth": map[string]any{"type": "integer", "minimum": 0, "maximum": 5}, "status": map[string]any{"type": "string"}, "version": map[string]any{"type": "integer", "format": "int64"}}},
		"MeasureAlias":                map[string]any{"type": "object", "required": []string{"id", "measure_ref", "alias", "scope"}, "properties": map[string]any{"id": map[string]any{"type": "string"}, "measure_ref": map[string]any{"type": "string"}, "alias": map[string]any{"type": "string", "maxLength": 80}, "scope": map[string]any{"type": "string", "enum": []string{"PLATFORM", "BUSINESS"}}, "region_code": map[string]any{"type": "string"}}},
		"OperatingModelConfiguration": map[string]any{"type": "object", "required": []string{"branch_ref", "models", "status", "version"}, "properties": map[string]any{"id": map[string]any{"type": "string"}, "branch_ref": map[string]any{"type": "string"}, "primary_model": map[string]any{"$ref": "#/components/schemas/OperatingModel"}, "models": map[string]any{"type": "array", "maxItems": 5, "items": map[string]any{"type": "object", "properties": map[string]any{"operating_model": map[string]any{"$ref": "#/components/schemas/OperatingModel"}, "admitted_capability": map[string]any{"type": "string"}}}}, "status": map[string]any{"type": "string"}, "version": map[string]any{"type": "integer", "format": "int64"}}},
		"Product":                     map[string]any{"type": "object", "required": []string{"id", "category_ref", "name", "product_class", "operating_models", "media_refs", "status", "version"}, "properties": map[string]any{"id": map[string]any{"type": "string"}, "category_ref": map[string]any{"type": "string"}, "name": map[string]any{"type": "string", "maxLength": 200}, "description": map[string]any{"type": "string", "maxLength": 4000}, "brand": map[string]any{"type": "string", "nullable": true}, "manufacturer": map[string]any{"type": "string", "nullable": true}, "origin_country": map[string]any{"type": "string", "nullable": true}, "product_class": map[string]any{"$ref": "#/components/schemas/ProductClass"}, "operating_models": map[string]any{"type": "array", "maxItems": 5, "items": map[string]any{"$ref": "#/components/schemas/OperatingModel"}}, "media_refs": map[string]any{"type": "array", "maxItems": 12, "items": map[string]any{"type": "string"}}, "status": map[string]any{"type": "string"}, "version": map[string]any{"type": "integer", "format": "int64"}}},
		"LogisticsProfile":            map[string]any{"type": "object", "properties": map[string]any{"temperature_zone": map[string]any{"$ref": "#/components/schemas/TemperatureZone"}, "weight_grams": map[string]any{"type": "integer", "format": "int64", "minimum": 1}, "length_mm": map[string]any{"type": "integer", "minimum": 1}, "width_mm": map[string]any{"type": "integer", "minimum": 1}, "height_mm": map[string]any{"type": "integer", "minimum": 1}, "handling_flags": map[string]any{"type": "array", "maxItems": 10, "items": map[string]any{"type": "string"}}, "consolidation_class": map[string]any{"type": "string"}, "separation_group": map[string]any{"type": "string"}, "large_load": map[string]any{"type": "boolean"}}},
		"Variant":                     map[string]any{"type": "object", "required": []string{"id", "product_ref", "name", "package_level", "pack_quantity", "temperature_zone", "handling_flags", "status", "version"}, "properties": map[string]any{"id": map[string]any{"type": "string"}, "product_ref": map[string]any{"type": "string"}, "sku": map[string]any{"type": "string", "nullable": true}, "name": map[string]any{"type": "string"}, "package_level": map[string]any{"type": "string"}, "pack_quantity": map[string]any{"type": "integer", "minimum": 1}, "net_weight_grams": map[string]any{"type": "integer", "nullable": true}, "net_volume_ml": map[string]any{"type": "integer", "nullable": true}, "length_mm": map[string]any{"type": "integer", "nullable": true}, "width_mm": map[string]any{"type": "integer", "nullable": true}, "height_mm": map[string]any{"type": "integer", "nullable": true}, "retail_price_minor": map[string]any{"$ref": "#/components/schemas/MinorMoney"}, "wholesale_price_minor": map[string]any{"$ref": "#/components/schemas/MinorMoney"}, "temperature_zone": map[string]any{"$ref": "#/components/schemas/TemperatureZone"}, "handling_flags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "consolidation_class": map[string]any{"type": "string", "nullable": true}, "separation_group": map[string]any{"type": "string", "nullable": true}, "stackable": map[string]any{"type": "boolean"}, "large_load": map[string]any{"type": "boolean"}, "status": map[string]any{"type": "string"}, "version": map[string]any{"type": "integer", "format": "int64"}}},
		"SellUnit": map[string]any{"type": "object", "required": []string{"id", "product_ref", "variant_ref", "measure_ref", "fraction_numerator", "fraction_denominator", "measurement_mode", "conversion_type", "retail_enabled", "wholesale_enabled", "minimum_order_quantity", "order_multiple", "status", "position", "version"}, "properties": map[string]any{
			"id": map[string]any{"type": "string"}, "product_ref": map[string]any{"type": "string"}, "variant_ref": map[string]any{"type": "string"}, "measure_ref": map[string]any{"type": "string"},
			"fraction_numerator": map[string]any{"type": "integer", "minimum": 1}, "fraction_denominator": map[string]any{"type": "integer", "minimum": 1}, "display_name": map[string]any{"type": "string"},
			"measurement_mode": map[string]any{"$ref": "#/components/schemas/MeasurementMode"}, "conversion_type": map[string]any{"$ref": "#/components/schemas/ConversionType"},
			"exact_base_family": map[string]any{"type": "string", "nullable": true}, "exact_base_quantity": map[string]any{"type": "integer", "format": "int64", "nullable": true}, "calibration_ref": map[string]any{"type": "string", "nullable": true}, "estimated_base_quantity": map[string]any{"type": "integer", "format": "int64", "nullable": true},
			"underfill_tolerance_bps": map[string]any{"type": "integer", "minimum": 0, "maximum": 10000}, "overfill_tolerance_bps": map[string]any{"type": "integer", "minimum": 0, "maximum": 10000}, "actual_measurement_required": map[string]any{"type": "boolean"}, "display_precision": map[string]any{"type": "integer", "minimum": 0},
			"price_minor": map[string]any{"$ref": "#/components/schemas/MinorMoney"}, "wholesale_price_minor": map[string]any{"$ref": "#/components/schemas/MinorMoney"}, "retail_enabled": map[string]any{"type": "boolean"}, "wholesale_enabled": map[string]any{"type": "boolean"},
			"minimum_order_quantity": map[string]any{"type": "integer", "format": "int64", "minimum": 1}, "order_multiple": map[string]any{"type": "integer", "format": "int64", "minimum": 1}, "maximum_order_quantity": map[string]any{"type": "integer", "format": "int64", "nullable": true}, "status": map[string]any{"type": "string"}, "position": map[string]any{"type": "integer"}, "version": map[string]any{"type": "integer", "format": "int64"},
		}},
		"PackageRelation": map[string]any{"type": "object", "description": "Explicit same-product parent/child packaging ratio; no ratio is inferred from names.", "required": []string{"id", "product_ref", "parent_variant_ref", "child_variant_ref", "units_per_parent", "status", "version"}, "properties": map[string]any{"id": map[string]any{"type": "string"}, "product_ref": map[string]any{"type": "string"}, "parent_variant_ref": map[string]any{"type": "string"}, "child_variant_ref": map[string]any{"type": "string"}, "units_per_parent": map[string]any{"type": "integer", "minimum": 1}, "status": map[string]any{"type": "string"}, "version": map[string]any{"type": "integer", "format": "int64"}}},
		"Identifier":      map[string]any{"type": "object", "required": []string{"id", "product_ref", "variant_ref", "identifier_type", "identifier_value", "externally_verified", "status", "version"}, "properties": map[string]any{"id": map[string]any{"type": "string"}, "product_ref": map[string]any{"type": "string"}, "variant_ref": map[string]any{"type": "string"}, "identifier_type": map[string]any{"type": "string", "enum": []string{"INTERNAL_SKU", "GTIN", "EAN", "UPC", "BARCODE", "MANUFACTURER_CODE", "OTHER_CONTROLLED"}}, "identifier_value": map[string]any{"type": "string", "maxLength": 100}, "package_level": map[string]any{"type": "string", "nullable": true}, "externally_verified": map[string]any{"type": "boolean", "enum": []bool{false}}, "status": map[string]any{"type": "string"}, "version": map[string]any{"type": "integer", "format": "int64"}}},
		"BranchListing": map[string]any{"type": "object", "description": "Commercial listing only; inventory_authority is always NOT_IMPLEMENTED in Part A.", "required": []string{"id", "branch_ref", "product_ref", "variant_ref", "retail_enabled", "wholesale_enabled", "inventory_authority", "status", "version"}, "properties": map[string]any{
			"id": map[string]any{"type": "string"}, "branch_ref": map[string]any{"type": "string"}, "product_ref": map[string]any{"type": "string"}, "variant_ref": map[string]any{"type": "string"}, "operating_model": map[string]any{"$ref": "#/components/schemas/OperatingModel", "nullable": true},
			"retail_enabled": map[string]any{"type": "boolean"}, "wholesale_enabled": map[string]any{"type": "boolean"}, "retail_price_override_minor": map[string]any{"$ref": "#/components/schemas/MinorMoney"}, "wholesale_price_override_minor": map[string]any{"$ref": "#/components/schemas/MinorMoney"},
			"minimum_order_quantity": map[string]any{"type": "integer", "format": "int64", "minimum": 1}, "order_multiple": map[string]any{"type": "integer", "format": "int64", "minimum": 1}, "maximum_order_quantity": map[string]any{"type": "integer", "format": "int64", "nullable": true},
			"preorder_policy": map[string]any{"type": "string"}, "backorder_policy": map[string]any{"type": "string"}, "auto_hide_when_unavailable": map[string]any{"type": "boolean"}, "logistics_available": map[string]any{"type": "boolean"}, "inventory_authority": map[string]any{"$ref": "#/components/schemas/InventoryAuthority"}, "status": map[string]any{"type": "string"}, "version": map[string]any{"type": "integer", "format": "int64"},
		}},
		"ListingSellUnit": map[string]any{"type": "object", "description": "A sell unit selected for one branch listing. calibration_ref, when present, is an approximate calibration for that same branch only.", "required": []string{"id", "listing_ref", "sell_unit_ref", "enabled", "version"}, "properties": map[string]any{"id": map[string]any{"type": "string"}, "listing_ref": map[string]any{"type": "string"}, "sell_unit_ref": map[string]any{"type": "string"}, "calibration_ref": map[string]any{"type": "string", "nullable": true}, "enabled": map[string]any{"type": "boolean"}, "price_override_minor": map[string]any{"$ref": "#/components/schemas/MinorMoney"}, "wholesale_price_override_minor": map[string]any{"$ref": "#/components/schemas/MinorMoney"}, "position": map[string]any{"type": "integer", "minimum": 0}, "version": map[string]any{"type": "integer", "format": "int64"}}},
		"QuantityBreak":   map[string]any{"type": "object", "description": "Inclusive, non-overlapping quantity band; null maximum is the single open-ended final band.", "required": []string{"id", "listing_ref", "min_quantity", "unit_price_minor", "wholesale_only", "status", "version"}, "properties": map[string]any{"id": map[string]any{"type": "string"}, "listing_ref": map[string]any{"type": "string"}, "sell_unit_ref": map[string]any{"type": "string", "nullable": true}, "min_quantity": map[string]any{"type": "integer", "format": "int64", "minimum": 1}, "max_quantity": map[string]any{"type": "integer", "format": "int64", "nullable": true}, "unit_price_minor": map[string]any{"$ref": "#/components/schemas/MinorMoney"}, "wholesale_only": map[string]any{"type": "boolean"}, "lead_time_days": map[string]any{"type": "integer", "minimum": 0, "maximum": 365, "nullable": true}, "status": map[string]any{"type": "string"}, "version": map[string]any{"type": "integer", "format": "int64"}}},
		"FreshAttribute":  map[string]any{"type": "object", "required": []string{"id", "product_ref", "attribute_type", "selection_required", "position", "options", "status", "version"}, "properties": map[string]any{"id": map[string]any{"type": "string"}, "product_ref": map[string]any{"type": "string"}, "attribute_type": map[string]any{"type": "string", "enum": []string{"RIPENESS", "QUALITY_GRADE", "SIZE_GRADE", "COLOUR", "FIRMNESS", "CUT", "CLEANING", "PEELING", "TRIMMING", "GUTTING", "DESCALING", "HEAD", "BONE", "PRESERVATION", "SELECTION", "PACKAGING_PREFERENCE"}}, "selection_required": map[string]any{"type": "boolean"}, "position": map[string]any{"type": "integer", "minimum": 0}, "options": map[string]any{"type": "array", "minItems": 1, "maxItems": 24, "items": map[string]any{"type": "object", "required": []string{"id", "option_value", "display_name", "is_default", "position", "status"}, "properties": map[string]any{"id": map[string]any{"type": "string"}, "option_value": map[string]any{"type": "string"}, "display_name": map[string]any{"type": "string"}, "is_default": map[string]any{"type": "boolean"}, "position": map[string]any{"type": "integer"}, "status": map[string]any{"type": "string"}}}}, "status": map[string]any{"type": "string"}, "version": map[string]any{"type": "integer", "format": "int64"}}},
		"PriceVersion":    map[string]any{"type": "object", "description": "Immutable integer-minor-unit commercial evidence.", "required": []string{"id", "subject_type", "subject_ref", "currency", "inherited", "change_reason", "subject_version", "revision", "actor_type", "recorded_at", "effective_at"}, "properties": map[string]any{"id": map[string]any{"type": "string"}, "branch_ref": map[string]any{"type": "string", "nullable": true}, "subject_type": map[string]any{"type": "string"}, "subject_ref": map[string]any{"type": "string"}, "product_ref": map[string]any{"type": "string", "nullable": true}, "variant_ref": map[string]any{"type": "string", "nullable": true}, "currency": map[string]any{"type": "string", "minLength": 3, "maxLength": 3}, "previous_amount_minor": map[string]any{"$ref": "#/components/schemas/MinorMoney"}, "amount_minor": map[string]any{"$ref": "#/components/schemas/MinorMoney"}, "inherited": map[string]any{"type": "boolean"}, "change_reason": map[string]any{"type": "string"}, "subject_version": map[string]any{"type": "integer", "format": "int64", "minimum": 1}, "revision": map[string]any{"type": "integer", "format": "int64", "minimum": 1}, "actor_type": map[string]any{"type": "string"}, "command_ref": map[string]any{"type": "string", "nullable": true}, "recorded_at": map[string]any{"type": "string", "format": "date-time"}, "effective_at": map[string]any{"type": "string", "format": "date-time"}}},
		"ResolvedPrice":   map[string]any{"type": "object", "description": "Commercial configuration result only; never checkout, charge or stock availability.", "required": []string{"listing_ref", "branch_ref", "variant_ref", "currency", "quantity", "mode", "price_source", "minimum_order_quantity", "order_multiple", "order_rules_satisfied", "availability"}, "properties": map[string]any{"listing_ref": map[string]any{"type": "string"}, "branch_ref": map[string]any{"type": "string"}, "variant_ref": map[string]any{"type": "string"}, "sell_unit_ref": map[string]any{"type": "string", "nullable": true}, "currency": map[string]any{"type": "string"}, "quantity": map[string]any{"type": "integer", "format": "int64", "minimum": 1}, "mode": map[string]any{"type": "string", "enum": []string{"RETAIL", "WHOLESALE"}}, "unit_price_minor": map[string]any{"$ref": "#/components/schemas/MinorMoney"}, "total_price_minor": map[string]any{"$ref": "#/components/schemas/MinorMoney"}, "price_source": map[string]any{"type": "string"}, "quantity_break_ref": map[string]any{"type": "string", "nullable": true}, "minimum_order_quantity": map[string]any{"type": "integer", "format": "int64", "minimum": 1}, "order_multiple": map[string]any{"type": "integer", "format": "int64", "minimum": 1}, "maximum_order_quantity": map[string]any{"type": "integer", "format": "int64", "nullable": true}, "order_rules_satisfied": map[string]any{"type": "boolean"}, "reason": map[string]any{"type": "string"}, "availability": map[string]any{"type": "string", "enum": []string{"UNAVAILABLE"}}}},
	}
	for name, schema := range commandSchemas() {
		out[name] = schema
	}
	return out
}

func commandSchemas() map[string]any {
	stringField := func(max int) map[string]any {
		return map[string]any{"type": "string", "maxLength": max}
	}
	integer := func(min, max int64) map[string]any {
		return map[string]any{"type": "integer", "format": "int64", "minimum": min, "maximum": max}
	}
	nullable := func(schema map[string]any) map[string]any {
		copy := map[string]any{}
		for key, value := range schema {
			copy[key] = value
		}
		copy["nullable"] = true
		return copy
	}
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/components/schemas/" + name} }
	array := func(items map[string]any, max int) map[string]any {
		return map[string]any{"type": "array", "maxItems": max, "items": items}
	}
	object := func(required []string, properties map[string]any) map[string]any {
		return map[string]any{"type": "object", "required": required, "additionalProperties": false, "properties": properties}
	}
	version := integer(1, 9223372036854775807)
	boolField := map[string]any{"type": "boolean"}
	money := ref("MinorMoney")
	operatingModels := array(ref("OperatingModel"), 5)
	mediaRefs := array(stringField(200), 12)
	handlingFlags := array(map[string]any{"type": "string", "enum": []string{"FRAGILE", "UPRIGHT_REQUIRED", "CRUSH_SENSITIVE", "LEAK_RISK", "LIQUID", "ODOR_SENSITIVE", "RAW_PROTEIN", "KEEP_DRY", "KEEP_COLD", "STACK_RESTRICTED"}}, 10)

	variantFields := map[string]any{
		"sku": stringField(64), "name": stringField(200), "package_level": stringField(32),
		"pack_quantity": integer(1, 100000), "net_weight_grams": nullable(integer(1, 100000000)),
		"net_volume_ml": nullable(integer(1, 100000000)), "length_mm": nullable(integer(1, 100000)),
		"width_mm": nullable(integer(1, 100000)), "height_mm": nullable(integer(1, 100000)),
		"retail_price_minor": nullable(money), "wholesale_price_minor": nullable(money),
		"temperature_zone": ref("TemperatureZone"), "consolidation_class": stringField(40),
		"separation_group": stringField(60), "stackable": boolField, "large_load": boolField,
		"handling_flags": handlingFlags,
	}
	updateVariantFields := cloneProperties(variantFields)
	updateVariantFields["status"] = map[string]any{"type": "string", "enum": []string{"ACTIVE", "PAUSED"}}
	updateVariantFields["expected_version"] = version

	sellUnitFields := map[string]any{
		"variant_id": stringField(100), "measure_id": stringField(100),
		"fraction_numerator": integer(1, 1000000), "fraction_denominator": integer(1, 1000),
		"display_name": stringField(120), "measurement_mode": ref("MeasurementMode"),
		"calibration_id": nullable(stringField(100)), "estimated_base_qty": nullable(integer(1, 100000000000)),
		"underfill_bps": integer(0, 10000), "overfill_bps": integer(0, 10000),
		"measurement_required": boolField, "display_precision": integer(0, 3),
		"price_minor": nullable(money), "wholesale_price_minor": nullable(money),
		"retail_enabled": boolField, "wholesale_enabled": boolField,
		"minimum_order_qty": integer(1, 1000000000), "order_multiple": integer(1, 1000000000),
		"maximum_order_qty": nullable(integer(1, 1000000000)),
	}
	updateSellUnitFields := cloneProperties(sellUnitFields)
	delete(updateSellUnitFields, "variant_id")
	delete(updateSellUnitFields, "measure_id")
	delete(updateSellUnitFields, "fraction_numerator")
	delete(updateSellUnitFields, "fraction_denominator")
	updateSellUnitFields["status"] = map[string]any{"type": "string", "enum": []string{"ACTIVE", "PAUSED"}}
	updateSellUnitFields["expected_version"] = version

	listingFields := map[string]any{
		"operating_model": nullable(ref("OperatingModel")), "retail_enabled": boolField,
		"wholesale_enabled": boolField, "retail_override": nullable(money),
		"wholesale_override": nullable(money), "minimum_order_qty": integer(1, 1000000000),
		"order_multiple": integer(1, 1000000000), "maximum_order_qty": nullable(integer(1, 1000000000)),
		"preorder_policy":       map[string]any{"type": "string", "enum": []string{"NOT_ALLOWED", "ALLOWED"}},
		"backorder_policy":      map[string]any{"type": "string", "enum": []string{"NOT_ALLOWED", "ALLOWED"}},
		"auto_hide_unavailable": boolField, "logistics_available": boolField,
		"sell_unit_ids": array(stringField(100), 24),
	}
	updateListingFields := cloneProperties(listingFields)
	updateListingFields["status"] = map[string]any{"type": "string", "enum": []string{"ACTIVE", "INACTIVE"}}
	updateListingFields["expected_version"] = version

	return map[string]any{
		"EmptyCommand":   object(nil, map[string]any{}),
		"VersionCommand": object([]string{"expected_version"}, map[string]any{"expected_version": version}),
		"CreateCategoryCommand": object([]string{"name"}, map[string]any{
			"parent_id": stringField(100), "name": stringField(120), "slug": stringField(80),
			"description": stringField(2000), "position": integer(0, 2147483647),
		}),
		"UpdateCategoryCommand": object([]string{"expected_version"}, map[string]any{
			"parent_id": nullable(stringField(100)), "name": stringField(120), "slug": stringField(80),
			"description": stringField(2000), "position": integer(0, 2147483647), "expected_version": version,
		}),
		"CreateMeasureCommand": object([]string{"code", "display_name", "family"}, map[string]any{
			"code": stringField(40), "display_name": stringField(80), "family": ref("MeasurementFamily"),
			"region_code": stringField(16), "base_quantity": nullable(integer(1, 1000000000)),
		}),
		"UpdateMeasureCommand": object([]string{"expected_version"}, map[string]any{
			"display_name": stringField(80), "region_code": nullable(stringField(16)), "expected_version": version,
		}),
		"AliasCommand": object([]string{"alias"}, map[string]any{"alias": stringField(80), "region_code": stringField(16)}),
		"CreateProductCommand": object([]string{"category_id", "name", "product_class"}, map[string]any{
			"category_id": stringField(100), "name": stringField(200), "description": stringField(4000),
			"brand": stringField(120), "manufacturer": stringField(160), "origin_country": stringField(2),
			"product_class": ref("ProductClass"), "regulatory_reference": stringField(80),
			"manufacturer_code": stringField(80), "operating_models": operatingModels, "media_refs": mediaRefs,
		}),
		"UpdateProductCommand": object([]string{"expected_version"}, map[string]any{
			"category_id": stringField(100), "name": stringField(200), "description": stringField(4000),
			"brand": stringField(120), "manufacturer": stringField(160), "origin_country": stringField(2),
			"product_class": ref("ProductClass"), "regulatory_reference": stringField(80),
			"manufacturer_code": stringField(80), "operating_models": operatingModels, "media_refs": mediaRefs,
			"status": map[string]any{"type": "string", "enum": []string{"DRAFT", "ACTIVE", "PAUSED"}}, "expected_version": version,
		}),
		"CreateVariantCommand": object([]string{"name"}, variantFields),
		"UpdateVariantCommand": object([]string{"expected_version"}, updateVariantFields),
		"CreateCalibrationCommand": object([]string{"measure_id"}, map[string]any{
			"branch_id": stringField(100), "variant_id": stringField(100), "measure_id": stringField(100),
			"weight_grams": nullable(integer(1, 100000000)), "volume_ml": nullable(integer(1, 100000000)),
			"tolerance_bps": integer(0, 10000), "source": map[string]any{"type": "string", "enum": []string{"VENDOR_MEASURED", "MANUFACTURER_DECLARED", "PACKAGE_DECLARED", "PLATFORM_VERIFIED", "OTHER_CONTROLLED"}},
		}),
		"UpdateCalibrationCommand": object([]string{"expected_version"}, map[string]any{
			"weight_grams": nullable(integer(1, 100000000)), "volume_ml": nullable(integer(1, 100000000)),
			"tolerance_bps": integer(0, 10000), "source": stringField(40), "expected_version": version,
		}),
		"CreateSellUnitCommand": object([]string{"variant_id", "measure_id"}, sellUnitFields),
		"UpdateSellUnitCommand": object([]string{"expected_version"}, updateSellUnitFields),
		"PackageCommand": object([]string{"parent_variant_id", "child_variant_id", "units_per_parent"}, map[string]any{
			"parent_variant_id": stringField(100), "child_variant_id": stringField(100), "units_per_parent": integer(1, 1000000),
		}),
		"FreshAttributeCommand": object([]string{"attribute_type", "options"}, map[string]any{
			"attribute_type": stringField(40), "required": boolField, "position": integer(0, 2147483647),
			"options":          array(object([]string{"value"}, map[string]any{"value": stringField(40), "display_name": stringField(80), "is_default": boolField}), 24),
			"expected_version": integer(0, 9223372036854775807),
		}),
		"IdentifierCommand": object([]string{"identifier_type", "identifier_value"}, map[string]any{
			"identifier_type": stringField(40), "identifier_value": stringField(64), "package_level": stringField(32),
		}),
		"OperatingModelsCommand": object([]string{"models"}, map[string]any{
			"models": operatingModels, "primary_model": nullable(ref("OperatingModel")), "expected_version": integer(0, 9223372036854775807),
		}),
		"CreateListingCommand": object([]string{"variant_id"}, mergeProperties(map[string]any{"variant_id": stringField(100)}, listingFields)),
		"UpdateListingCommand": object([]string{"expected_version"}, updateListingFields),
		"ListingSellUnitCommand": object([]string{"expected_version"}, map[string]any{
			"enabled": boolField, "calibration_id": nullable(stringField(100)), "price_override": nullable(money),
			"wholesale_override": nullable(money), "expected_version": version,
		}),
		"CreateQuantityBreakCommand": object([]string{"min_quantity", "unit_price_minor"}, map[string]any{
			"sell_unit_id": stringField(100), "min_quantity": integer(1, 1000000000),
			"max_quantity": nullable(integer(1, 1000000000)), "unit_price_minor": money,
			"wholesale_only": boolField, "lead_time_days": nullable(integer(0, 365)),
		}),
		"UpdateQuantityBreakCommand": object([]string{"expected_version"}, map[string]any{
			"min_quantity": integer(1, 1000000000), "max_quantity": nullable(integer(1, 1000000000)),
			"unit_price_minor": money, "wholesale_only": boolField, "lead_time_days": nullable(integer(0, 365)),
			"expected_version": version,
		}),
	}
}

func cloneProperties(source map[string]any) map[string]any {
	return mergeProperties(map[string]any{}, source)
}

func mergeProperties(target map[string]any, sources ...map[string]any) map[string]any {
	for _, source := range sources {
		for key, value := range source {
			target[key] = value
		}
	}
	return target
}
