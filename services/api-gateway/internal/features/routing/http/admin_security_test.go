package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestPublicOwnerAdministrativePathsNeverReachOwners(t *testing.T) {
	origins := map[string]string{}
	// Every actual public owner is configured, so a 404 cannot be explained
	// by a missing upstream. The positive control below verifies forwarding.
	u := newUpstream(t, models.UpstreamTaxi)
	for _, route := range models.DefaultRoutes() {
		origins[route.Upstream] = u.server.URL
	}
	origins["analytics"] = u.server.URL
	edge, err := NewEdge(Options{Routes: models.DefaultRoutes(), Origins: origins, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.NoRoute(edge.Handle)
	paths := []string{
		"/internal/karryam-admin/v1/media/case-grants",
		"/internal/karryam-admin/v1/media/case-grants/ref/content",
		"/internal/karryam-admin/v1/media/case-grants/ref/revocations",
		"/internal/media/v1/admin/case-grants/ref/content",
		"/api/v1/media/internal/media/v1/admin/case-grants/ref/content",
		"/internal/customer/v1/admin/case-authority/proofs/review-work-items/ref/CUSTOMER/ref",
		"/api/v1/taxi/internal/taxi/v1/admin/case-authority/proofs/recoveries/ref/CASE/ref",
		"/api/v1/dispatch/internal/dispatch/v1/admin/case-authority/proofs/incidents/ref/TRIP/ref",
		"/api/v1/hauling/internal/hauling/v1/admin/case-authority/proofs/recoveries/ref/CASE/ref",
		"/internal/karryam-admin/v1/taxi/rides",
		"/internal/taxi/v1/admin/rides",
		"/api/v1/taxi/internal/bookings/ref/actions/force-recover",
		"/api/v1/internal/assignments/ref/reconcile",
		"/api/v1/customer/internal/admin/customers/ref",
		"/api/v1/hauling/internal/admin/missions/ref",
		"/internal/fleet-partner/v1/assignments/ref/provider-accept",
		"/api/v1/analytics/network/mission-success",
		"/api/v1/internal/callbacks/dispatch",
	}
	for _, base := range paths {
		for _, path := range []string{base, strings.ReplaceAll(base, "internal", "%2569nternal"), "/api/v1/taxi/../.." + base} {
			for _, method := range []string{"GET", "POST", "PATCH", "DELETE"} {
				r := httptest.NewRequest(method, path, nil)
				r.Header.Set("X-Service-Name", "admin-backoffice-service")
				r.Header.Set("X-HQ-Principal", "cosmicforge-hq-karryam-admin")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				if w.Code != 404 {
					t.Fatalf("%s %s reached public route: %d", method, path, w.Code)
				}
			}
		}
	}
	if u.count() != 0 {
		t.Fatalf("privileged requests reached an owner: %d", u.count())
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/taxi/bookings/public-control", nil))
	if u.count() != 1 {
		t.Fatal("public route positive control did not reach configured owner")
	}
}
