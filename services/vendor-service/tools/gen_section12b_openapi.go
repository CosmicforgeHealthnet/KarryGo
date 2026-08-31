//go:build ignore

package main

import (
	"encoding/json"
	"os"
	"strings"
)

type route struct {
	method, path, summary string
	created               bool
}

var publicRoutes = []route{
	{"PUT", "/shop/branches/{branchId}/inventory/stock-bases", "Configure an authoritative stock basis", false},
	{"GET", "/shop/branches/{branchId}/inventory/balances", "List effective inventory balances", false},
	{"GET", "/shop/branches/{branchId}/inventory/balances/{stockBasisId}", "Get an inventory balance", false},
	{"GET", "/shop/branches/{branchId}/inventory/movements", "Read bounded append-only movement history", false},
	{"GET", "/shop/branches/{branchId}/inventory/revision", "Read the monotonic inventory revision", false},
	{"GET", "/shop/branches/{branchId}/inventory/variants/{variantId}/availability", "Preview effective stock availability", false},
	{"GET", "/shop/branches/{branchId}/inventory/variants/{variantId}/sellability", "Preview effective Shop sellability", false},
	{"GET", "/shop/branches/{branchId}/inventory/receipts", "List bounded branch stock receipts", false},
	{"GET", "/shop/branches/{branchId}/inventory/transfers", "List bounded branch transfers", false},
	{"GET", "/shop/branches/{branchId}/inventory/stocktakes", "List bounded branch stocktakes", false},
	{"GET", "/shop/branches/{branchId}/inventory/quarantines", "List bounded branch quarantines", false},
	{"GET", "/shop/branches/{branchId}/inventory/reorder-rules", "List branch reorder rules", false},
	{"GET", "/shop/branches/{branchId}/inventory/alerts", "List bounded operational inventory alerts", false},
	{"POST", "/shop/branches/{branchId}/inventory/stocktakes", "Start a full stocktake or cycle count", true},
	{"POST", "/shop/inventory/receipts", "Create a draft stock receipt", true},
	{"GET", "/shop/inventory/receipts/{receiptId}", "Get a stock receipt", false},
	{"POST", "/shop/inventory/receipts/{receiptId}/post", "Post a receipt and stock movements", false},
	{"POST", "/shop/inventory/receipts/{receiptId}/cancel", "Cancel a draft receipt or reverse an intact posted receipt", false},
	{"POST", "/shop/inventory/reservations", "Atomically reserve one or more inventory lines", true},
	{"GET", "/shop/inventory/reservations/{reservationId}", "Get an inventory reservation", false},
	{"POST", "/shop/inventory/reservations/{reservationId}/confirm", "Confirm a held reservation", false},
	{"POST", "/shop/inventory/reservations/{reservationId}/commit", "Commit and consume a reservation", false},
	{"POST", "/shop/inventory/reservations/{reservationId}/release", "Release an uncommitted reservation", false},
	{"POST", "/shop/inventory/adjustments", "Post a reasoned stock adjustment", true},
	{"POST", "/shop/inventory/quarantines", "Quarantine physical stock", true},
	{"POST", "/shop/inventory/quarantines/{quarantineId}/resolve", "Release or dispose quarantined stock", false},
	{"POST", "/shop/inventory/transfers", "Create a same-business branch transfer", true},
	{"GET", "/shop/inventory/transfers/{transferId}", "Get a stock transfer and its lines", false},
	{"POST", "/shop/inventory/transfers/{transferId}/dispatch", "Dispatch a stock transfer", false},
	{"POST", "/shop/inventory/transfers/{transferId}/receive", "Record full or partial transfer receipt", false},
	{"POST", "/shop/inventory/transfers/{transferId}/cancel", "Cancel a stock-neutral draft transfer", false},
	{"GET", "/shop/inventory/stocktakes/{stocktakeId}", "Get a stocktake and counted lines", false},
	{"PUT", "/shop/inventory/stocktakes/{stocktakeId}/lines/{stockBasisId}", "Record a counted stocktake line", false},
	{"POST", "/shop/inventory/stocktakes/{stocktakeId}/post", "Post a non-stale stocktake", false},
	{"PUT", "/shop/listings/{listingId}/inventory-policy", "Set preorder or backorder policy", false},
	{"POST", "/shop/inventory/fulfilment-measurements", "Record actual variable fulfilment", true},
	{"PUT", "/shop/inventory/stock-bases/{stockBasisId}/reorder-rule", "Set low-stock and reorder thresholds", false},
	{"POST", "/shop/inventory/alerts/{alertId}/acknowledge", "Acknowledge an operational inventory alert", false},
	{"POST", "/shop/inventory/alerts/{alertId}/resolve", "Resolve an operational inventory alert", false},
	{"POST", "/shop/bulk-operations", "Validate a bounded bulk operation or dry run", true},
	{"GET", "/shop/bulk-operations/{jobId}", "Read a bulk job and bounded row error report", false},
	{"POST", "/shop/bulk-operations/{jobId}/apply", "Atomically apply a validated bulk operation", false},
}

var internalRoutes = []route{
	{"GET", "/shop/branches/{branchId}/catalogue", "Read effective Shop catalogue and inventory", false},
	{"GET", "/shop/branches/{branchId}/search", "Search effective Shop catalogue", false},
	{"GET", "/shop/branches/{branchId}/products/{productId}", "Read effective branch product detail", false},
	{"GET", "/shop/branches/{branchId}/variants/{variantId}/availability", "Read future-Order availability preview", false},
	{"GET", "/shop/branches/{branchId}/variants/{variantId}/sellability", "Read effective variant sellability", false},
	{"POST", "/shop/branches/{branchId}/pricing/resolve", "Resolve authoritative commercial price", false},
	{"GET", "/shop/branches/{branchId}/revision", "Read catalogue and inventory revisions", false},
	{"POST", "/shop/branches/{branchId}/inventory/reservations", "Atomically reserve vendor-owned inventory", true},
	{"POST", "/shop/branches/{branchId}/inventory/reservations/{reservationId}/confirm", "Confirm a future-Order reservation", false},
	{"POST", "/shop/branches/{branchId}/inventory/reservations/{reservationId}/commit", "Commit a future-Order reservation", false},
	{"POST", "/shop/branches/{branchId}/inventory/reservations/{reservationId}/release", "Release a future-Order reservation", false},
}

func schemaRef(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}
func operation(route route, internal bool) map[string]any {
	status := "200"
	if route.created {
		status = "201"
	}
	op := map[string]any{"summary": route.summary, "operationId": strings.ToLower(route.method) + strings.NewReplacer("/", "_", "{", "", "}", "").Replace(route.path), "responses": map[string]any{status: map[string]any{"description": "Successful bounded inventory response", "content": map[string]any{"application/json": map[string]any{"schema": schemaRef("SuccessEnvelope")}}}, "400": map[string]any{"$ref": "#/components/responses/Error"}, "401": map[string]any{"$ref": "#/components/responses/Error"}, "403": map[string]any{"$ref": "#/components/responses/Error"}, "409": map[string]any{"$ref": "#/components/responses/Error"}}}
	params := []any{}
	for _, segment := range strings.Split(route.path, "/") {
		if strings.HasPrefix(segment, "{") {
			params = append(params, map[string]any{"name": strings.Trim(segment, "{}"), "in": "path", "required": true, "schema": map[string]any{"type": "string", "maxLength": 200}})
		}
	}
	if internal {
		for _, name := range []string{"X-Service-Name", "X-Service-Timestamp", "X-Service-Nonce", "X-Service-Signature"} {
			params = append(params, map[string]any{"name": name, "in": "header", "required": true, "schema": map[string]any{"type": "string"}})
		}
	} else if route.method != "GET" {
		params = append(params, map[string]any{"$ref": "#/components/parameters/IdempotencyKey"})
	}
	if len(params) > 0 {
		op["parameters"] = params
	}
	if route.method != "GET" {
		op["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": schemaRef("InventoryCommand")}}}
	}
	return op
}
func schemas() map[string]any {
	return map[string]any{
		"InventoryCommand": map[string]any{"type": "object", "additionalProperties": false, "description": "Strict bounded command. Fields are interpreted by the selected operation; unknown fields are rejected.", "properties": map[string]any{
			"listing_id": map[string]any{"type": "string"}, "measure_id": map[string]any{"type": "string"}, "consumption_mode": map[string]any{"type": "string", "enum": []string{"SAME_UNIT", "EXACT_CONVERSION", "VARIABLE_FULFILMENT"}}, "allocation_policy": map[string]any{"type": "string"}, "lot_tracking": map[string]any{"type": "boolean"}, "expected_version": map[string]any{"type": "integer", "format": "int64"},
			"branch_id": map[string]any{"type": "string"}, "source_branch_id": map[string]any{"type": "string"}, "destination_branch_id": map[string]any{"type": "string"}, "source_type": map[string]any{"type": "string"}, "source_ref": map[string]any{"type": "string"}, "supplier_ref": map[string]any{"type": "string"}, "notes": map[string]any{"type": "string", "maxLength": 2000}, "received_at": map[string]any{"type": "string", "format": "date-time"}, "expires_at": map[string]any{"type": "string", "format": "date-time", "nullable": true},
			"lines": map[string]any{"type": "array", "maxItems": 1000, "items": map[string]any{"type": "object"}}, "line_id": map[string]any{"type": "string"}, "source_stock_basis_id": map[string]any{"type": "string"}, "destination_stock_basis_id": map[string]any{"type": "string"}, "stock_basis_id": map[string]any{"type": "string"}, "stock_basis_ids": map[string]any{"type": "array", "maxItems": 1000, "items": map[string]any{"type": "string"}}, "quantity": map[string]any{"type": "integer", "format": "int64"}, "quantity_delta": map[string]any{"type": "integer", "format": "int64"}, "received_quantity": map[string]any{"type": "integer", "format": "int64"}, "damaged_quantity": map[string]any{"type": "integer", "format": "int64"}, "missing_quantity": map[string]any{"type": "integer", "format": "int64"},
			"lot_number": map[string]any{"type": "string"}, "batch_number": map[string]any{"type": "string"}, "manufactured_at": map[string]any{"type": "string", "format": "date-time", "nullable": true}, "best_before_at": map[string]any{"type": "string", "format": "date-time", "nullable": true}, "unit_cost_minor": map[string]any{"type": "integer", "format": "int64", "nullable": true},
			"reason_code": map[string]any{"type": "string"}, "note": map[string]any{"type": "string", "maxLength": 2000}, "expected_revision": map[string]any{"type": "integer", "format": "int64"}, "action": map[string]any{"type": "string"}, "stocktake_type": map[string]any{"type": "string"}, "counted_quantity": map[string]any{"type": "integer", "format": "int64"},
			"mode": map[string]any{"type": "string", "enum": []string{"NONE", "PREORDER", "BACKORDER"}}, "maximum_outstanding_quantity": map[string]any{"type": "integer", "format": "int64", "nullable": true}, "maximum_per_request": map[string]any{"type": "integer", "format": "int64", "nullable": true}, "available_from": map[string]any{"type": "string", "format": "date-time", "nullable": true}, "expected_restock_at": map[string]any{"type": "string", "format": "date-time", "nullable": true}, "lead_time_days": map[string]any{"type": "integer", "nullable": true}, "cutoff_at": map[string]any{"type": "string", "format": "date-time", "nullable": true},
			"actual_quantity": map[string]any{"type": "integer", "format": "int64"}, "actual_measure_id": map[string]any{"type": "string"}, "reservation_line_id": map[string]any{"type": "string"}, "measured_at": map[string]any{"type": "string", "format": "date-time"}, "measured_by": map[string]any{"type": "string"}, "minimum_quantity": map[string]any{"type": "integer", "format": "int64"}, "target_quantity": map[string]any{"type": "integer", "format": "int64"}, "expiring_soon_hours": map[string]any{"type": "integer", "nullable": true},
			"operation_type": map[string]any{"type": "string"}, "dry_run": map[string]any{"type": "boolean"}, "expected_catalogue_revision": map[string]any{"type": "integer", "format": "int64", "nullable": true}, "expected_inventory_revision": map[string]any{"type": "integer", "format": "int64", "nullable": true}, "rows": map[string]any{"type": "array", "maxItems": 10000, "items": map[string]any{"type": "object"}}, "listing_ref": map[string]any{"type": "string"}, "variant_ref": map[string]any{"type": "string"}, "sell_unit_ref": map[string]any{"type": "string"}, "sales_mode": map[string]any{"type": "string", "enum": []string{"RETAIL", "WHOLESALE"}},
		}},
		"SuccessEnvelope":  map[string]any{"type": "object", "required": []string{"success", "data"}, "properties": map[string]any{"success": map[string]any{"type": "boolean", "enum": []bool{true}}, "data": map[string]any{"type": "object"}}},
		"Error":            map[string]any{"type": "object", "required": []string{"success", "error"}, "properties": map[string]any{"success": map[string]any{"type": "boolean", "enum": []bool{false}}, "error": map[string]any{"type": "object"}}},
		"InventoryBalance": map[string]any{"type": "object", "properties": map[string]any{"on_hand_quantity": map[string]any{"type": "integer", "format": "int64", "minimum": 0}, "reserved_quantity": map[string]any{"type": "integer", "format": "int64", "minimum": 0}, "quarantined_quantity": map[string]any{"type": "integer", "format": "int64", "minimum": 0}, "physical_available": map[string]any{"type": "integer", "format": "int64", "minimum": 0}, "inventory_revision": map[string]any{"type": "integer", "format": "int64"}}},
		"Reservation":      map[string]any{"type": "object", "properties": map[string]any{"status": map[string]any{"type": "string", "enum": []string{"HELD", "CONFIRMED", "COMMITTED", "RELEASED", "CANCELLED", "EXPIRED"}}, "lines": map[string]any{"type": "array", "items": map[string]any{"type": "object"}}}},
		"Sellability":      map[string]any{"type": "object", "properties": map[string]any{"sellable": map[string]any{"type": "boolean"}, "reason_code": map[string]any{"type": "string"}, "catalogue_revision": map[string]any{"type": "integer"}, "inventory_revision": map[string]any{"type": "integer"}}},
	}
}
func document(title, server string, routes []route, internal bool) map[string]any {
	paths := map[string]any{}
	for _, route := range routes {
		if paths[route.path] == nil {
			paths[route.path] = map[string]any{}
		}
		paths[route.path].(map[string]any)[strings.ToLower(route.method)] = operation(route, internal)
	}
	security := []any{map[string]any{"bearerAuth": []string{}}}
	if internal {
		security = []any{map[string]any{"hmacV2": []string{}}}
	}
	return map[string]any{"openapi": "3.0.3", "info": map[string]any{"title": title, "version": "1.0.0", "description": "Section 12B authoritative vendor-owned inventory, reservations, stock operations, preorder/backorder, variable fulfilment and effective discovery. Physical stock never becomes negative. Expired or quarantined lots are not sellable. Approximate calibration is never exact reservation arithmetic. No Customer PII, Order authority, Wallet/payment, logistics execution or KarryGo dark-store inventory is implemented."}, "servers": []any{map[string]any{"url": server}}, "security": security, "paths": paths, "components": map[string]any{"securitySchemes": map[string]any{"bearerAuth": map[string]any{"type": "http", "scheme": "bearer"}, "hmacV2": map[string]any{"type": "apiKey", "in": "header", "name": "X-Service-Signature"}}, "parameters": map[string]any{"IdempotencyKey": map[string]any{"name": "Idempotency-Key", "in": "header", "required": true, "schema": map[string]any{"type": "string", "minLength": 8, "maxLength": 200}}}, "responses": map[string]any{"Error": map[string]any{"description": "Stable platform error envelope", "content": map[string]any{"application/json": map[string]any{"schema": schemaRef("Error")}}}}, "schemas": schemas()}}
}
func write(path string, doc map[string]any) {
	raw, _ := json.MarshalIndent(doc, "", "  ")
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		panic(err)
	}
}
func main() {
	write("docs/openapi/section12b-shop-inventory.openapi.json", document("KarryGo Vendor Service — Section 12B Shop Inventory", "/api/v1/vendor", publicRoutes, false))
	write("docs/openapi/section12-shop-discovery.openapi.json", document("KarryGo Vendor Service — Section 12 Internal Shop Contract", "/internal/vendor/v1", internalRoutes, true))
}
