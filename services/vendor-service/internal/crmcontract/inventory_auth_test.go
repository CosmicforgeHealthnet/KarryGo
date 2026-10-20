package crmcontract

import (
	"net/http/httptest"
	"testing"
)

func TestProductionInventoryRetainsBearerBoundary(t *testing.T) {
	router := buildProductionRouter(t)
	for _, path := range []string{"/api/v1/vendor/media/objects/object-1/content", "/api/v1/vendor/staff/me/access", "/api/v1/vendor/commerce/section21/recurring-setup-contexts"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
			if response.Code != 401 {
				t.Fatalf("inventory lost production authentication: %d %s", response.Code, response.Body.String())
			}
		})
	}
}
