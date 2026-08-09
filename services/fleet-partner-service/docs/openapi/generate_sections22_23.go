//go:build ignore

// Command generate_sections22_23 writes the Section 22 and 23 contracts from
// the same declarative route catalogues used by the Gin registrars.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops"
)

type object = map[string]any
type route struct {
	Method, Path, OperationID, Permission, FeatureFlag, AuthClass string
	IdempotencyRequired, ExpectedVersionRequired                  bool
}

func main() {
	var finance []route
	for _, r := range financeops.TenantRoutes() {
		finance = append(finance, route{r.Method, r.Path, r.OperationID, r.Permission, r.FeatureFlag, r.AuthClass, r.IdempotencyRequired, r.ExpectedVersionRequired})
	}
	var communications []route
	for _, r := range engagement.TenantRoutes() {
		communications = append(communications, route{r.Method, r.Path, r.OperationID, r.Permission, r.FeatureFlag, r.AuthClass, r.IdempotencyRequired, r.ExpectedVersionRequired})
	}
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "..", "..", "..", ".."))
	write(filepath.Join(root, "docs", "openapi", "section22-finance-billing-payroll.openapi.json"), "KarryGo Section 22 Finance, Billing and Payroll", "22.0.0", finance)
	write(filepath.Join(root, "docs", "openapi", "section23-communications-support-sla.openapi.json"), "KarryGo Section 23 Communications, Support and SLA", "23.0.0", communications)
}

func write(path, title, version string, routes []route) {
	paths := object{}
	for _, r := range routes {
		p := "/fleet-partner/v1" + braces(r.Path)
		item, _ := paths[p].(object)
		if item == nil {
			item = object{}
			paths[p] = item
		}
		op := object{"operationId": r.OperationID, "x-permission": r.Permission, "x-feature-flag": r.FeatureFlag, "x-auth-class": r.AuthClass, "x-idempotency-required": r.IdempotencyRequired, "x-expected-version-required": r.ExpectedVersionRequired, "security": []any{object{"CorporateBearer": []any{}}}}
		params := []any{}
		for _, name := range pathParameters(p) {
			params = append(params, object{"name": name, "in": "path", "required": true, "schema": object{"type": "string", "minLength": 1}})
		}
		if r.IdempotencyRequired {
			params = append(params, object{"$ref": "#/components/parameters/IdempotencyKey"})
		}
		if r.ExpectedVersionRequired {
			params = append(params, object{"$ref": "#/components/parameters/IfMatch"})
		}
		op["parameters"] = params
		if r.Method != "GET" {
			op["requestBody"] = object{"required": true, "content": object{"application/json": object{"schema": object{"$ref": "#/components/schemas/CommandRequest"}}}}
		}
		op["responses"] = object{"200": object{"description": "Successful operation", "content": object{"application/json": object{"schema": object{"$ref": "#/components/schemas/Envelope"}}}}, "400": object{"description": "Invalid request", "content": object{"application/json": object{"schema": object{"$ref": "#/components/schemas/Problem"}}}}, "409": object{"description": "Idempotency or version conflict", "content": object{"application/json": object{"schema": object{"$ref": "#/components/schemas/Problem"}}}}}
		item[strings.ToLower(r.Method)] = op
	}
	doc := object{"openapi": "3.0.3", "info": object{"title": title, "version": version, "description": "Tenant-scoped, fail-closed contract. Financial amounts are integer minor units; remote authority identifiers are opaque references."}, "servers": []any{object{"url": "/"}}, "paths": paths, "components": object{"securitySchemes": object{"CorporateBearer": object{"type": "http", "scheme": "bearer", "bearerFormat": "JWT"}}, "parameters": object{"IdempotencyKey": object{"name": "Idempotency-Key", "in": "header", "required": true, "schema": object{"type": "string", "minLength": 1, "maxLength": 128}}, "IfMatch": object{"name": "If-Match", "in": "header", "required": true, "schema": object{"type": "integer", "format": "int64", "minimum": 1}}}, "schemas": object{"CommandRequest": object{"type": "object", "additionalProperties": true, "properties": object{"expectedVersion": object{"type": "integer", "format": "int64", "minimum": 1}, "amountMinor": object{"type": "integer", "format": "int64"}, "currency": object{"type": "string", "pattern": "^[A-Z]{3}$"}}}, "Envelope": object{"type": "object", "required": []any{"data"}, "properties": object{"data": object{"type": "object", "additionalProperties": true}, "meta": object{"type": "object", "additionalProperties": true}}}, "Problem": object{"type": "object", "required": []any{"error"}, "properties": object{"error": object{"type": "object", "required": []any{"code", "message"}, "properties": object{"code": object{"type": "string"}, "message": object{"type": "string"}}}}}}}}
	b, _ := json.MarshalIndent(doc, "", "  ")
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
