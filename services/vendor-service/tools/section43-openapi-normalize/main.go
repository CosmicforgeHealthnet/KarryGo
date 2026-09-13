// Command section43-openapi-normalize validates globally unique operationIds
// across Vendor Service's split OpenAPI contract authority. With -write it
// adds deterministic IDs and structurally required path parameters/responses
// only where a mature document omitted them; it never renames an accepted
// operationId or changes a route.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var nonIdentifier = regexp.MustCompile(`[^A-Za-z0-9]+`)

func main() {
	write := flag.Bool("write", false, "add missing deterministic operationIds")
	dir := flag.String("dir", "docs/openapi", "OpenAPI directory")
	flag.Parse()

	entries, err := os.ReadDir(*dir)
	must(err)
	seen := map[string]string{}
	missing := 0
	operations := 0
	qualityChanges := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(*dir, entry.Name())
		raw, err := os.ReadFile(path)
		must(err)
		var document map[string]any
		must(json.Unmarshal(raw, &document))
		version, _ := document["openapi"].(string)
		if !strings.HasPrefix(version, "3.") {
			fatalf("%s has unsupported OpenAPI version %q", entry.Name(), version)
		}
		paths, _ := document["paths"].(map[string]any)
		changed := false
		if normalizeComponentPlacement(document, *write) {
			changed = true
			qualityChanges++
		}
		if *write && isSection16VendorContract(entry.Name()) {
			if ensureVendorBearer(document) {
				changed = true
				qualityChanges++
			}
		}
		pathNames := make([]string, 0, len(paths))
		for name := range paths {
			pathNames = append(pathNames, name)
		}
		sort.Strings(pathNames)
		for _, routePath := range pathNames {
			item, _ := paths[routePath].(map[string]any)
			for _, method := range []string{"get", "post", "put", "patch", "delete"} {
				rawOperation, ok := item[method]
				if !ok {
					continue
				}
				operations++
				operation, _ := rawOperation.(map[string]any)
				resolvedOperation := resolveObject(document, operation)
				if *write && requiresIdempotency(entry.Name(), method, routePath) &&
					ensureHeader(operation, resolvedOperation, "Idempotency-Key", "Retry key. Same tenant/key/fingerprint replays the original result; a different fingerprint returns 409.") {
					changed = true
					qualityChanges++
				}
				if *write && requiresIfMatch(entry.Name(), method, routePath) &&
					ensureHeader(operation, resolvedOperation, "If-Match", "Required optimistic version. A stale or missing version returns 409.") {
					changed = true
					qualityChanges++
				}
				id, _ := operation["operationId"].(string)
				if strings.TrimSpace(id) == "" {
					fmt.Printf("adding operationId %s %s\n", method, routePath)
					missing++
					if *write {
						id = deterministicID(entry.Name(), method, routePath)
						operation["operationId"] = id
						changed = true
					}
				}
				if id != "" {
					if prior, exists := seen[id]; exists {
						fatalf("duplicate operationId %q: %s and %s %s", id, prior, method, routePath)
					}
					seen[id] = entry.Name() + " " + method + " " + routePath
				}
				for _, name := range templateParameters(routePath) {
					if !hasPathParameter(document, item, resolvedOperation, name) {
						if !*write {
							fatalf("%s %s %s does not declare required path parameter %q", entry.Name(), method, routePath, name)
						}
						parameters, _ := operation["parameters"].([]any)
						operation["parameters"] = append(parameters, map[string]any{
							"name": name, "in": "path", "required": true,
							"schema": map[string]any{"type": "string", "minLength": float64(1)},
						})
						changed = true
						qualityChanges++
					}
				}
				if _, ok := resolvedOperation["responses"].(map[string]any); !ok {
					if !*write || operation["$ref"] != nil {
						fatalf("%s %s %s has no responses", entry.Name(), method, routePath)
					}
					fmt.Printf("adding responses %s %s\n", method, routePath)
					status := successStatus(method, routePath)
					operation["responses"] = map[string]any{
						status: map[string]any{"description": successDescription(status)},
						"400":  map[string]any{"description": "Invalid request"},
						"401":  map[string]any{"description": "Unauthenticated"},
						"403":  map[string]any{"description": "Caller is not authorized for the Vendor resource"},
						"404":  map[string]any{"description": "Resource not found or ownership is intentionally opaque"},
						"409":  map[string]any{"description": "Idempotency, version, or state conflict"},
						"422":  map[string]any{"description": "Domain validation failed"},
					}
					changed = true
					qualityChanges++
				}
			}
		}
		validateLocalReferences(document, document, entry.Name())
		validateSecuritySchemes(document, entry.Name())
		if changed {
			fmt.Printf("updating %s\n", entry.Name())
			encoded, err := json.MarshalIndent(document, "", "  ")
			must(err)
			encoded = append(encoded, '\n')
			must(replaceFile(path, encoded))
		}
	}
	if !*write && missing != 0 {
		fatalf("%d of %d OpenAPI operations have no operationId", missing, operations)
	}
	fmt.Printf("OPENAPI_VALIDATION_PASS operations=%d uniqueOperationIds=%d operationIdsAdded=%d qualityChanges=%d\n", operations, len(seen), func() int {
		if *write {
			return missing
		}
		return 0
	}(), qualityChanges)
}

func normalizeComponentPlacement(document map[string]any, write bool) bool {
	components, _ := document["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	misplaced, found := schemas["requestBodies"]
	if !found {
		return false
	}
	if !write {
		fatalf("components.requestBodies is incorrectly nested under components.schemas")
	}
	components["requestBodies"] = misplaced
	delete(schemas, "requestBodies")
	return true
}

func validateLocalReferences(document map[string]any, value any, filename string) {
	switch typed := value.(type) {
	case map[string]any:
		if ref, ok := typed["$ref"].(string); ok && strings.HasPrefix(ref, "#/") {
			_ = resolveObject(document, map[string]any{"$ref": ref})
		}
		for _, child := range typed {
			validateLocalReferences(document, child, filename)
		}
	case []any:
		for _, child := range typed {
			validateLocalReferences(document, child, filename)
		}
	}
}

func validateSecuritySchemes(document map[string]any, filename string) {
	components, _ := document["components"].(map[string]any)
	schemes, _ := components["securitySchemes"].(map[string]any)
	check := func(raw any, location string) {
		requirements, _ := raw.([]any)
		for _, rawRequirement := range requirements {
			requirement, _ := rawRequirement.(map[string]any)
			for name := range requirement {
				if _, ok := schemes[name]; !ok {
					fatalf("%s references undefined security scheme %q at %s", filename, name, location)
				}
			}
		}
	}
	if raw, ok := document["security"]; ok {
		check(raw, "document")
	}
	paths, _ := document["paths"].(map[string]any)
	for routePath, rawItem := range paths {
		item, _ := rawItem.(map[string]any)
		for _, method := range []string{"get", "post", "put", "patch", "delete"} {
			operation, ok := item[method].(map[string]any)
			if !ok {
				continue
			}
			resolved := resolveObject(document, operation)
			if raw, ok := resolved["security"]; ok {
				check(raw, method+" "+routePath)
			}
		}
	}
}

// replaceFile avoids truncating a file in place. On Windows, editors and diff
// viewers can hold a read-only mapped view that permits replacement but rejects
// truncation with ERROR_USER_MAPPED_FILE.
func replaceFile(path string, contents []byte) error {
	temporary := path + ".section43-tmp"
	if err := os.WriteFile(temporary, contents, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func templateParameters(routePath string) []string {
	matches := regexp.MustCompile(`\{([^}]+)\}`).FindAllStringSubmatch(routePath, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		out = append(out, match[1])
	}
	return out
}

func hasPathParameter(document, item, operation map[string]any, name string) bool {
	for _, owner := range []map[string]any{item, operation} {
		if owner == nil {
			continue
		}
		parameters, _ := owner["parameters"].([]any)
		for _, raw := range parameters {
			parameter, _ := raw.(map[string]any)
			parameter = resolveObject(document, parameter)
			if parameter["name"] == name && parameter["in"] == "path" && parameter["required"] == true {
				return true
			}
		}
	}
	return false
}

func resolveObject(document, object map[string]any) map[string]any {
	ref, _ := object["$ref"].(string)
	if !strings.HasPrefix(ref, "#/") {
		return object
	}
	var current any = document
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		container, ok := current.(map[string]any)
		if !ok {
			fatalf("invalid local reference %q", ref)
		}
		current, ok = container[part]
		if !ok {
			fatalf("unresolved local reference %q", ref)
		}
	}
	resolved, ok := current.(map[string]any)
	if !ok {
		fatalf("local reference %q does not resolve to an object", ref)
	}
	return resolved
}

func successStatus(method, routePath string) string {
	if method != "post" {
		return "200"
	}
	created := routePath == "/api/v1/vendor/ingredients" ||
		routePath == "/api/v1/vendor/packaging-materials" ||
		strings.HasSuffix(routePath, "/conversions") ||
		routePath == "/api/v1/vendor/recipes" ||
		strings.HasSuffix(routePath, "/versions") ||
		routePath == "/api/v1/vendor/production-batches" ||
		routePath == "/api/v1/vendor/cost/policies" ||
		strings.HasSuffix(routePath, "/snapshots") ||
		strings.HasSuffix(routePath, "/expected") ||
		strings.HasSuffix(routePath, "/actual")
	if created {
		return "201"
	}
	return "200"
}

func successDescription(status string) string {
	if status == "201" {
		return "Created using the production response envelope"
	}
	return "Successful production response"
}

func isSection16VendorContract(filename string) bool {
	return strings.HasPrefix(filename, "section16a-") ||
		strings.HasPrefix(filename, "section16b-") ||
		strings.HasPrefix(filename, "section16c-") ||
		strings.HasPrefix(filename, "section16d-")
}

func ensureVendorBearer(document map[string]any) bool {
	changed := false
	if _, ok := document["security"]; !ok {
		document["security"] = []any{map[string]any{"VendorBearer": []any{}}}
		changed = true
	}
	components, _ := document["components"].(map[string]any)
	if components == nil {
		components = map[string]any{}
		document["components"] = components
	}
	schemes, _ := components["securitySchemes"].(map[string]any)
	if schemes == nil {
		schemes = map[string]any{}
		components["securitySchemes"] = schemes
	}
	if _, ok := schemes["VendorBearer"]; !ok {
		schemes["VendorBearer"] = map[string]any{
			"type": "http", "scheme": "bearer", "bearerFormat": "JWT",
			"description": "Verified Vendor staff token; business and branch scope are derived server-side from membership.",
		}
		changed = true
	}
	return changed
}

func ensureHeader(operation, resolved map[string]any, name, description string) bool {
	for _, owner := range []map[string]any{operation, resolved} {
		parameters, _ := owner["parameters"].([]any)
		for _, raw := range parameters {
			parameter, _ := raw.(map[string]any)
			if parameter["name"] == name {
				return false
			}
		}
	}
	parameters, _ := operation["parameters"].([]any)
	operation["parameters"] = append(parameters, map[string]any{
		"name": name, "in": "header", "required": true,
		"description": description,
		"schema":      map[string]any{"type": "string", "minLength": float64(1), "maxLength": float64(200)},
	})
	return true
}

func requiresIdempotency(filename, method, routePath string) bool {
	if method == "get" {
		return false
	}
	if isSection16VendorContract(filename) || filename == "commerce-support-cases.openapi.json" ||
		filename == "section9b-food-operations.openapi.json" || filename == "promotions-delivery-funding.openapi.json" {
		return true
	}
	if filename == "section17e-parent-vendor-orders.openapi.json" && strings.Contains(routePath, "/vendor/commerce/orders/") {
		return true
	}
	return filename == "section18-vendor-order-lifecycle.openapi.json" ||
		filename == "shop-picking-substitution-variable-measurement.openapi.json"
}

func requiresIfMatch(filename, method, routePath string) bool {
	if method == "get" {
		return false
	}
	if filename == "section18-vendor-order-lifecycle.openapi.json" ||
		filename == "shop-picking-substitution-variable-measurement.openapi.json" {
		return true
	}
	return filename == "section17e-parent-vendor-orders.openapi.json" &&
		strings.Contains(routePath, "/vendor/commerce/orders/") &&
		!strings.HasSuffix(routePath, "/support-cases")
}

func deterministicID(filename, method, routePath string) string {
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	value := strings.Trim(nonIdentifier.ReplaceAllString(stem+"_"+method+"_"+routePath, "_"), "_")
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(method + " " + routePath))
	return fmt.Sprintf("%s_%08x", value, hash.Sum32())
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
