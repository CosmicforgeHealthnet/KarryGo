package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/httpx"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
)

func queryAllowlistRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.RequestID())
	router.Use(respond.ErrorHandler())
	group := router.Group("/fleet-partner/v1")
	group.Use(QueryAllowlist())
	ok := func(c *gin.Context) { respond.OK(c, gin.H{"reached": true}) }
	group.GET("/assets", ok)          // declared offset list
	group.GET("/assets/:assetId", ok) // single-record read: not declared
	group.POST("/assets", ok)         // writes are not query-checked
	group.GET("/wallet/policy", ok)   // declared with no parameters
	group.GET("/staff", ok)           // declared cursor list
	return router
}

func TestQueryAllowlistMiddleware(t *testing.T) {
	router := queryAllowlistRouter()
	cases := []struct {
		method, target string
		want           int
		field          string
	}{
		{"GET", "/fleet-partner/v1/assets?branchId=b1&status=ACTIVE&limit=50&offset=100", 200, ""},
		{"GET", "/fleet-partner/v1/assets", 200, ""},
		{"GET", "/fleet-partner/v1/assets?offset=-1", 422, "offset"},
		{"GET", "/fleet-partner/v1/assets?limit=100000", 422, "limit"},
		{"GET", "/fleet-partner/v1/assets?bogusFilter=1", 422, "bogusFilter"},
		{"GET", "/fleet-partner/v1/assets?cursor=abc", 422, "cursor"},
		{"GET", "/fleet-partner/v1/staff?cursor=abc&limit=10", 200, ""},
		{"GET", "/fleet-partner/v1/staff?offset=0", 422, "offset"},
		{"GET", "/fleet-partner/v1/wallet/policy?organizationId=org_other", 422, "organizationId"},
		{"GET", "/fleet-partner/v1/assets/ast_1?anything=1", 200, ""},
		{"POST", "/fleet-partner/v1/assets?anything=1", 200, ""},
	}
	for _, tc := range cases {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(tc.method, tc.target, nil))
		if recorder.Code != tc.want {
			t.Fatalf("%s %s = %d, want %d: %s", tc.method, tc.target, recorder.Code, tc.want, recorder.Body.String())
		}
		if tc.want != 422 {
			continue
		}
		var body struct {
			Error struct {
				Code      string `json:"code"`
				RequestID string `json:"requestId"`
				Details   []struct {
					Field string `json:"field"`
				} `json:"details"`
			} `json:"error"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Error.Code != "validation_failed" || body.Error.RequestID == "" ||
			len(body.Error.Details) != 1 || body.Error.Details[0].Field != tc.field {
			t.Fatalf("%s: error envelope = %s", tc.target, recorder.Body.String())
		}
	}
	_ = http.MethodGet
}
