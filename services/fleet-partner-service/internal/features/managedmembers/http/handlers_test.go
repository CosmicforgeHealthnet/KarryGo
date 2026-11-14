package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/managedmembers/models"
)

// TestAuthorityRefusalUsesTheStandardErrorEnvelope pins spec 6: the refusal
// used to be written as {"success":false,"error":{"code":"AUTHORITY_REFUSED"}},
// the only error on the corporate surface without a requestId or details.
func TestAuthorityRefusalUsesTheStandardErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/fleet-partner/v1/managed-members/x/links", nil)
	write(c, nil, models.ErrAuthorityRefused)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", recorder.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, legacy := body["success"]; legacy {
		t.Fatalf("legacy envelope field present: %s", recorder.Body.String())
	}
	envelope, ok := body["error"].(map[string]any)
	if !ok || envelope["code"] != "authority_refused" || envelope["message"] == "" {
		t.Fatalf("error envelope = %s", recorder.Body.String())
	}
	for _, key := range []string{"details", "requestId", "retryable"} {
		if _, present := envelope[key]; !present {
			t.Fatalf("error envelope missing %s: %s", key, recorder.Body.String())
		}
	}
}
