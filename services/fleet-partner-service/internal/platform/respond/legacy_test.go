package respond

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
)

func run(t *testing.T, handler gin.HandlerFunc) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	handler(c)
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return recorder.Code, body
}

func TestJSONWithAliasesKeepsEnvelopeAndNeverShadowsIt(t *testing.T) {
	code, body := run(t, func(c *gin.Context) {
		JSONWithAliases(c, http.StatusOK, gin.H{"status": "ACCEPTED"},
			map[string]any{"status": "ACCEPTED", "data": "shadow", "meta": "shadow", "error": "shadow"})
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	data, ok := body["data"].(map[string]any)
	if !ok || data["status"] != "ACCEPTED" || body["status"] != "ACCEPTED" {
		t.Fatalf("body = %v", body)
	}
	if _, ok := body["meta"].(map[string]any); !ok || body["error"] != nil {
		t.Fatalf("an alias shadowed an envelope member: %v", body)
	}
}

func TestErrorWithAliasesWritesStandardErrorPlusAliases(t *testing.T) {
	code, body := run(t, func(c *gin.Context) {
		ErrorWithAliases(c, apperrors.New(http.StatusBadRequest, "unknown_event_type", "Rejected.", nil),
			map[string]any{"status": "REJECTED", "code": "unknown_event_type"})
	})
	errBody, ok := body["error"].(map[string]any)
	if code != http.StatusBadRequest || !ok || errBody["code"] != "unknown_event_type" || errBody["message"] != "Rejected." {
		t.Fatalf("code=%d body=%v", code, body)
	}
	if body["status"] != "REJECTED" || body["code"] != "unknown_event_type" || body["data"] != nil {
		t.Fatalf("aliases = %v", body)
	}
}

func TestAliasesOfFlattensObject(t *testing.T) {
	type view struct {
		ProviderMode string `json:"providerMode"`
		Optional     string `json:"optional,omitempty"`
	}
	got := AliasesOf(view{ProviderMode: "INDEPENDENT"})
	if len(got) != 1 || got["providerMode"] != "INDEPENDENT" {
		t.Fatalf("aliases = %v", got)
	}
}
