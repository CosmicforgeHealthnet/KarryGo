package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/services/api-gateway/internal/features/routing/models"
	"github.com/gin-gonic/gin"
)

func TestPublicAdminIsolation(t *testing.T) {
	u := newUpstream(t, models.UpstreamAdmin)
	edge, err := NewEdge(Options{Routes: append(models.DefaultRoutes(), models.Route{Prefix: "/innocent", Upstream: models.UpstreamAdmin}), Origins: map[string]string{models.UpstreamAdmin: u.server.URL}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.NoRoute(edge.Handle)
	for _, method := range []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS", "CONNECT"} {
		for _, path := range []string{"/api/v1/admin", "/api/v1/admin/", "/API/v1/AdMiN/x", "/api/v1%2fadmin/x", "/api/v1%252fadmin/x", "/api/v1/%2561dmin/x", "/api/v1/./admin/x", "/api/v1/customer/../admin/x", "/api/v1/customer/%252e%252e/admin/x", "/internal/karryam-admin/v1/capabilities", "/api/v1/customer/%255c..%255cadmin/x", "/innocent"} {
			req := httptest.NewRequest(method, path, nil)
			req.Host = "admin-backoffice-service:8110"
			for k, v := range map[string]string{"Upgrade": "websocket", "Connection": "upgrade", "X-Gateway-Scope": "ADMIN", "X-HQ-Principal": "cosmicforge-hq-karryam-admin", "X-Forwarded-Host": "admin-backoffice-service", "X-Service-Name": "admin-backoffice-service", "X-Admin-Role": "owner"} {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusNotFound {
				t.Errorf("%s %s: %d", method, path, w.Code)
			}
		}
	}
	if u.count() != 0 {
		t.Fatalf("privileged upstream reached %d times", u.count())
	}
	if _, ok := edge.Upstream(models.UpstreamAdmin); ok {
		t.Fatal("Admin upstream was registered")
	}
}
