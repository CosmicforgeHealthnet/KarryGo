package http

import (
	"encoding/json"
	"os"
	"testing"
)

func TestCustomerUploadOpenAPIPathParity(t *testing.T) {
	raw, err := os.ReadFile("../../../../docs/customer-evidence-upload.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Paths map[string]any `json:"paths"`
	}
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Paths) != 1 || document.Paths["/api/v1/media-files/customer-uploads/{id}/content"] == nil {
		t.Fatalf("route parity: %v", document.Paths)
	}
}
