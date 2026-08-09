package privateexecution

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPrivateExecutionOpenAPIMatchesRegisteredRoutes(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/openapi/private-execution.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{
		"/internal/private-executions":                             "post",
		"/internal/private-executions/by-operation/{operationId}":  "get",
		"/internal/private-executions/by-trip/{tripId}":            "get",
		"/internal/private-executions/{operationId}/cancellations": "post",
		"/internal/private-executions/{operationId}/cancellation":  "get",
	}
	if len(spec.Paths) != len(expected) {
		t.Fatalf("paths=%d want=%d", len(spec.Paths), len(expected))
	}
	for path, method := range expected {
		if spec.Paths[path] == nil || spec.Paths[path][method] == nil {
			t.Errorf("missing %s %s", method, path)
		}
	}
}
