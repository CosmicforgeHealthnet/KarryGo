//go:build ignore

// Command generate_sections21_33 writes the Section 21 warehouse contract and
// the Section 22-33 contracts added by the Sections 21-33 campaign from the
// same declarative route catalogues the Gin registrars mount, so a route can
// never be added to code without appearing in its contract (the parity tests
// in internal/section19_21contract and internal/section24_33contract fail
// otherwise).
//
//	go run ./docs/openapi/generate_sections21_33.go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	analyticshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/analytics/http"
	governancehttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/http"
	warehousehttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse/http"
)

type object = map[string]any

type route struct {
	Method, Path, OperationID, Permission, FeatureFlag, AuthClass string
	IdempotencyRequired, ExpectedVersionRequired                  bool
}

func main() {
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "..", "..", "..", ".."))
	out := filepath.Join(root, "docs", "openapi")

	var warehouse []route
	for _, r := range warehousehttp.TenantRoutes() {
		warehouse = append(warehouse, route{r.Method, r.Path, r.OperationID, r.Permission, r.FeatureFlag, r.AuthClass, r.IdempotencyRequired, r.ExpectedVersionRequired})
	}
	write(filepath.Join(out, "section21-warehouse-fulfilment.openapi.json"), "Section 21 Warehouse and Fulfilment", "1.1.0",
		"Authoritative machine-readable contract. All routes are tenant scoped and deny by default. Stock quantities are counts, never money; every stock change is an immutable inventory movement.",
		"Section 21 Warehouse and Fulfilment", warehouse)

	var analytics []route
	for _, r := range analyticshttp.TenantRoutes() {
		analytics = append(analytics, route{r.Method, r.Path, r.OperationID, r.Permission, r.FeatureFlag, r.AuthClass, r.IdempotencyRequired, r.ExpectedVersionRequired})
	}
	write(filepath.Join(out, "section24-analytics-reporting-automation.openapi.json"), "Section 24 Analytics, Reporting, Recommendations and Automation", "24.0.0",
		"Tenant-scoped reports, dashboards, explainable recommendations and human-governed automation. Recommendations never change live assignments, money, safety or compliance; automation actions come only from the approved catalogue.",
		"Section 24 Analytics and Automation", analytics)

	var governance []route
	for _, r := range governancehttp.TenantRoutes() {
		governance = append(governance, route{r.Method, r.Path, r.OperationID, r.Permission, r.FeatureFlag, r.AuthClass, r.IdempotencyRequired, r.ExpectedVersionRequired})
	}
	write(filepath.Join(out, "section28-56-governance.openapi.json"), "Sections 28 and 56 Governance: audit events, security events and privacy requests", "28.1.0",
		"Tenant-scoped governance read models and privacy request workflow. Audit and security events are append-only; privacy requests honour legal holds and never delete financial, safety, trip or audit evidence.",
		"Sections 28 and 56 Governance", governance)
}

func problem(description string) object {
	return object{"description": description, "content": object{"application/json": object{"schema": object{"$ref": "#/components/schemas/Problem"}}}}
}

func write(path, title, version, description, tag string, routes []route) {
	paths := object{}
	for _, r := range routes {
		p := "/fleet-partner/v1" + braces(r.Path)
		item, _ := paths[p].(object)
		if item == nil {
			item = object{}
			paths[p] = item
		}
		op := object{
			"operationId": r.OperationID, "tags": []any{tag}, "security": []any{object{"bearerAuth": []any{}}},
			"x-permission": r.Permission, "x-feature-flag": r.FeatureFlag, "x-auth-class": r.AuthClass,
			"x-idempotency-required": r.IdempotencyRequired, "x-expected-version-required": r.ExpectedVersionRequired,
		}
		params := []any{}
		for _, name := range pathParameters(p) {
			params = append(params, object{"name": name, "in": "path", "required": true, "schema": object{"type": "string"}})
		}
		if r.IdempotencyRequired {
			params = append(params, object{"name": "Idempotency-Key", "in": "header", "required": true, "schema": object{"type": "string", "minLength": 1}})
		}
		if r.ExpectedVersionRequired {
			params = append(params, object{"name": "If-Match", "in": "header", "required": false, "schema": object{"type": "string"}, "description": "Expected aggregate version; expectedVersion in the JSON body is also accepted."})
		}
		if len(params) > 0 {
			op["parameters"] = params
		}
		if r.Method != "GET" {
			op["requestBody"] = object{"required": true, "content": object{"application/json": object{"schema": object{"$ref": "#/components/schemas/CommandRequest"}}}}
		}
		op["responses"] = object{
			"200": object{"description": "Successful response", "content": object{"application/json": object{"schema": object{"$ref": "#/components/schemas/Envelope"}}}},
			"400": problem("Invalid request"), "401": problem("Unauthenticated"), "403": problem("Forbidden"),
			"404": problem("Not found"), "409": problem("Conflict"), "422": problem("Validation failed"),
			"503": problem("Capability or authority unavailable"),
		}
		item[strings.ToLower(r.Method)] = op
	}
	doc := object{
		"openapi": "3.0.3",
		"info":    object{"title": title, "version": version, "description": description},
		"servers": []any{object{"url": "/"}},
		"paths":   paths,
		"components": object{
			"securitySchemes": object{"bearerAuth": object{"type": "http", "scheme": "bearer", "bearerFormat": "JWT"}},
			"schemas": object{
				"CommandRequest": object{"type": "object", "description": "Operation-specific JSON request. Unknown fields are rejected by the HTTP decoder where the operation uses a strict DTO.", "additionalProperties": true,
					"properties": object{"expectedVersion": object{"type": "integer", "format": "int64", "minimum": 1}, "reason": object{"type": "string", "minLength": 1}}},
				"Envelope": object{"type": "object", "required": []any{"success", "data"}, "properties": object{"success": object{"type": "boolean", "enum": []any{true}}, "data": object{"type": "object", "additionalProperties": true}, "meta": object{"type": "object", "additionalProperties": true}}},
				"Problem":  object{"type": "object", "required": []any{"error"}, "properties": object{"error": object{"type": "object", "required": []any{"code", "message"}, "properties": object{"code": object{"type": "string"}, "message": object{"type": "string"}, "details": object{"type": "object", "additionalProperties": true}}}}},
			},
		},
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		panic(err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o644); err != nil {
		panic(err)
	}
}

func braces(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ":") {
			parts[i] = "{" + strings.TrimPrefix(p, ":") + "}"
		}
	}
	return strings.Join(parts, "/")
}

func pathParameters(path string) []string {
	var out []string
	for _, p := range strings.Split(path, "/") {
		if strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}") {
			out = append(out, strings.TrimSuffix(strings.TrimPrefix(p, "{"), "}"))
		}
	}
	return out
}
