package http

import (
	"net/http/httptest"
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/httpx"
	"github.com/gin-gonic/gin"
)

func TestPrivateDecisionCannotSilentlyDiscardEvidence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpx.ErrorHandler())
	h := NewHandler(nil, nil, nil)
	engine.POST("/private-assignment/:assignmentId/decline", func(c *gin.Context) { c.Set("service_name", "driver-dispatch-delivery-service"); h.decline(c) })
	request := httptest.NewRequest("POST", "/private-assignment/pas_test/decline", strings.NewReader(`{"providerId":"provider-1","reasonCode":"FATIGUE_OR_REST_REQUIRED","evidenceFileIds":["protected-file-1"]}`))
	request.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, request)
	// No service/database port is supplied: reaching a write or source lookup
	// would panic. Unsupported evidence must be refused before either occurs.
	if res.Code != 400 {
		t.Fatalf("unrecorded evidence accepted: %d %s", res.Code, res.Body.String())
	}
}
