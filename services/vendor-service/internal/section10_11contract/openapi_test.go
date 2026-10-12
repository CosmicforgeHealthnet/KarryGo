package section10_11contract

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/features/cloud_kitchen"
	"cosmicforge/logistics/services/vendor-service/internal/features/pastry"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
)

const specPath = "../../docs/openapi/sections10-11-production.openapi.json"
const expectedOperations = 128

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
	Security   []map[string][]string                 `json:"security"`
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		SecuritySchemes map[string]json.RawMessage `json:"securitySchemes"`
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

func method(value string) bool {
	switch value {
	case "get", "post", "put", "patch", "delete":
		return true
	default:
		return false
	}
}

func documented(t *testing.T, doc document) map[string]operation {
	t.Helper()
	out := map[string]operation{}
	for path, methods := range doc.Paths {
		for name, raw := range methods {
			if !method(name) {
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
	for index, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[index] = "{" + strings.TrimPrefix(part, ":") + "}"
		}
	}
	return strings.Join(parts, "/")
}

func registered(t *testing.T) map[string]bool {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	flags := production.Flags{ProductionEnabled: true, WritesEnabled: true}
	service := production.NewService(production.Options{Flags: flags})
	group := router.Group("/api/v1/vendor")
	production.RegisterRoutes(group, production.NewHandler(service), flags)
	cloudkitchen.RegisterRoutes(group, cloudkitchen.NewHandler(cloudkitchen.NewService(nil, service)), flags)
	pastry.RegisterRoutes(group, pastry.NewHandler(pastry.NewService(nil, service)), flags)
	out := map[string]bool{}
	for _, route := range router.Routes() {
		out[route.Method+" "+normalize(route.Path)] = true
	}
	return out
}

func TestBidirectionalRouteParity(t *testing.T) {
	docRoutes := documented(t, load(t))
	codeRoutes := registered(t)
	if len(docRoutes) != expectedOperations || len(codeRoutes) != expectedOperations {
		t.Fatalf("Sections 10/11 route count: code=%d OpenAPI=%d want=%d", len(codeRoutes), len(docRoutes), expectedOperations)
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

func TestOpenAPIIsAuthenticatedAndOperationsAreNamed(t *testing.T) {
	doc := load(t)
	if len(doc.Security) != 1 || doc.Security[0]["bearerAuth"] == nil {
		t.Fatal("Sections 10/11 OpenAPI must require bearerAuth globally")
	}
	if _, ok := doc.Components.SecuritySchemes["bearerAuth"]; !ok {
		t.Fatal("bearerAuth security scheme is not defined")
	}
	seen := map[string]string{}
	for key, op := range documented(t, doc) {
		if op.OperationID == "" {
			t.Errorf("%s has no operationId", key)
		}
		if previous := seen[op.OperationID]; previous != "" {
			t.Errorf("duplicate operationId %q on %s and %s", op.OperationID, previous, key)
		}
		seen[op.OperationID] = key
		if len(op.Responses) == 0 {
			t.Errorf("%s has no responses", key)
		}
	}
}

func TestMutationContractDeclaresIdempotencyAndBodies(t *testing.T) {
	for key, op := range documented(t, load(t)) {
		if strings.HasPrefix(key, "GET ") {
			continue
		}
		hasIdempotency := false
		for _, parameter := range op.Parameters {
			if strings.HasSuffix(parameter.Ref, "/IdempotencyKey") || (parameter.Name == "Idempotency-Key" && parameter.In == "header" && parameter.Required) {
				hasIdempotency = true
			}
		}
		if !hasIdempotency {
			t.Errorf("%s does not declare the required Idempotency-Key", key)
		}
		if len(op.RequestBody) == 0 || string(op.RequestBody) == "null" {
			t.Errorf("%s does not declare a command request body", key)
		}
	}
}

func TestProductionJobTransitionsDeclareIfMatch(t *testing.T) {
	for key, op := range documented(t, load(t)) {
		if !strings.HasPrefix(key, "POST /production/jobs/{jobId}/") {
			continue
		}
		hasIfMatch := false
		for _, parameter := range op.Parameters {
			if strings.HasSuffix(parameter.Ref, "/IfMatchVersion") || (parameter.Name == "If-Match" && parameter.In == "header" && parameter.Required) {
				hasIfMatch = true
			}
		}
		if !hasIfMatch {
			t.Errorf("%s does not declare mandatory If-Match optimistic concurrency", key)
		}
	}
}

func TestBoundaryLanguageRejectsForeignAuthority(t *testing.T) {
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(raw))
	for _, phrase := range []string{"not orders", "not orders, payments", "not orders, payments, inventory"} {
		if strings.Contains(text, phrase) {
			return
		}
	}
	t.Fatal("OpenAPI must state that production reservations are not Order/payment/inventory authority")
}
