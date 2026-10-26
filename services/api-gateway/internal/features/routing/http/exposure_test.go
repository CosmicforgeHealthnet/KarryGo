package http

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/api-gateway/internal/features/routing/models"
)

// allOwnersFixture points every public upstream (and a would-be admin one) at
// one recording server, so a 404 can never be explained by a missing origin.
func allOwnersFixture(t *testing.T) (*gin.Engine, *recordingUpstream, *recordingUpstream) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	public := newUpstream(t, "public")
	admin := newUpstream(t, models.UpstreamAdmin)
	origins := map[string]string{models.UpstreamAdmin: admin.server.URL}
	for _, route := range models.DefaultRoutes() {
		origins[route.Upstream] = public.server.URL
	}
	edge, err := NewEdge(Options{Routes: models.DefaultRoutes(), Origins: origins, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.NoRoute(edge.Handle)
	return router, public, admin
}

func serve(router *gin.Engine, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

var websocketHeaders = map[string]string{
	"Upgrade": "websocket", "Connection": "Upgrade",
	"Sec-WebSocket-Key": "dGhlIHNhbXBsZSBub25jZQ==", "Sec-WebSocket-Version": "13",
}

// Section 20: a protocol switch is carried only to the websocket endpoints
// the apps actually use. Every other path, method or upgrade shape is a 404
// that never reaches an upstream.
func TestUpgradeIsOnlyCarriedToAllowlistedWebsocketEndpoints(t *testing.T) {
	router, public, admin := allOwnersFixture(t)
	refused := []struct {
		method, path string
		headers      map[string]string
	}{
		{"GET", "/api/v1/customer/me", websocketHeaders},
		{"GET", "/api/v1/taxi/customer/bookings", websocketHeaders},
		{"GET", "/api/v1/payment-wallet/wallets/me", websocketHeaders},
		{"GET", "/api/v1/partner/v1/deliveries", websocketHeaders},
		{"GET", "/fleet-partner/anything", websocketHeaders},
		{"GET", "/api/v1/vendor/orders", websocketHeaders},
		{"GET", "/api/v1/customer/notifications/ws", websocketHeaders},
		{"GET", "/api/v1/notifications/ws/", websocketHeaders},
		{"GET", "/API/V1/NOTIFICATIONS/WS", websocketHeaders},
		{"GET", "/api/v1/customer/dispatch/bookings/abc/track/", websocketHeaders},
		{"GET", "/api/v1/customer/dispatch/bookings//track", websocketHeaders},
		{"GET", "/api/v1/customer/dispatch/bookings/a%2Fb/track", websocketHeaders},
		{"GET", "/api/v1/customer/dispatch/bookings/a.b/track", websocketHeaders},
		{"GET", "/api/v1/customer/dispatch/bookings/abc/track/extra", websocketHeaders},
		{"GET", "/api/v1/hauling/customer/bookings/abc/track2", websocketHeaders},
		{"GET", "/api/v1/hauling/provider/bookings/abc/track", websocketHeaders},
		{"GET", "/api/v1/admin/ws", websocketHeaders},
		{"GET", "/internal/notifications/ws", websocketHeaders},
		// Right path, wrong shape.
		{"POST", "/api/v1/notifications/ws", websocketHeaders},
		{"PUT", "/api/v1/customer/dispatch/bookings/abc/track", websocketHeaders},
		{"GET", "/api/v1/notifications/ws", map[string]string{"Upgrade": "h2c", "Connection": "Upgrade, HTTP2-Settings", "HTTP2-Settings": "AAMAAABkAARAAAAAAAIAAAAA"}},
		// A Connection: upgrade token alone is still an upgrade attempt.
		{"GET", "/api/v1/customer/me", map[string]string{"Connection": "keep-alive, Upgrade"}},
		{"GET", "/api/v1/customer/me", map[string]string{"Upgrade": "h2c"}},
	}
	for _, c := range refused {
		if w := serve(router, c.method, c.path, c.headers); w.Code != http.StatusNotFound {
			t.Errorf("%s %s %v: %d", c.method, c.path, c.headers, w.Code)
		}
	}
	if public.count() != 0 || admin.count() != 0 {
		t.Fatalf("refused upgrades reached upstreams: public=%d admin=%d", public.count(), admin.count())
	}

	// Positive controls (through a real listener, which the proxy needs for
	// an upgrade): the three real endpoints are still carried, and an
	// ordinary (non-upgrade) request on any route is untouched.
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	for _, path := range []string{
		"/api/v1/notifications/ws?token=t",
		"/api/v1/customer/dispatch/bookings/7b0f7a5e-2b7d-4c2a-9a55-0f3a2b4c5d6e/track?token=t",
		"/api/v1/hauling/customer/bookings/7b0f7a5e-2b7d-4c2a-9a55-0f3a2b4c5d6e/track?token=t",
	} {
		before := public.count()
		request, _ := http.NewRequest(http.MethodGet, server.URL+path, nil)
		for key, value := range websocketHeaders {
			request.Header.Set(key, value)
		}
		if response, err := server.Client().Do(request); err == nil {
			_ = response.Body.Close()
		}
		if public.count() != before+1 {
			t.Errorf("allowlisted websocket endpoint was not carried: %s", path)
			continue
		}
		forwarded, _ := public.last()
		if !strings.EqualFold(forwarded.Header.Get("Upgrade"), "websocket") {
			t.Errorf("upgrade header lost for %s", path)
		}
	}
	before := public.count()
	if w := serve(router, http.MethodGet, "/api/v1/customer/me", nil); w.Code != http.StatusOK || public.count() != before+1 {
		t.Fatalf("ordinary request regressed: %d", w.Code)
	}
}

func TestWebsocketAllowlistMatchesOnlyExactShapes(t *testing.T) {
	for _, endpoint := range models.PublicWebsocketEndpoints() {
		concrete := strings.ReplaceAll(endpoint.Pattern, "{id}", "booking_1-A")
		got, ok := models.WebsocketEndpointFor(concrete)
		if !ok || got.Upstream != endpoint.Upstream {
			t.Errorf("%s did not match its own pattern", concrete)
		}
		// Every allowlisted endpoint must be reachable through a public route
		// owned by the same upstream, or the allowlist is describing fiction.
		route, ok := models.NewTable(models.DefaultRoutes()).Match(concrete)
		if !ok || route.Upstream != endpoint.Upstream {
			t.Errorf("%s is not routed to %s", concrete, endpoint.Upstream)
		}
	}
	for _, path := range []string{"", "/", "/api/v1/notifications", "/api/v1/notifications/ws/x",
		"/api/v1/customer/dispatch/bookings/{id}/track", "/api/v1/customer/dispatch/bookings/../track",
		"/api/v1/customer/dispatch/bookings/" + strings.Repeat("a", 129) + "/track"} {
		if _, ok := models.WebsocketEndpointFor(path); ok {
			t.Errorf("%q matched the websocket allowlist", path)
		}
	}
}

// Section 20 bypass matrix: every spelling of an admin or internal path, with
// every method and hostile header combination, is a 404 that reaches nobody.
func TestAdminAndInternalBypassMatrix(t *testing.T) {
	router, public, admin := allOwnersFixture(t)
	paths := []string{
		"/api/v1/admin", "/api/v1/admin/", "/api/v1/admin//", "/API/V1/ADMIN", "/API/V1/ADMIN/USERS",
		"/Api/V1/Admin/users", "/api/v1%2Fadmin/users", "/api/v1%2fadmin", "/api%2Fv1%2Fadmin%2Fusers",
		"/api/v1%252Fadmin/users", "/api/v1%25252Fadmin/users", "/api/v1/%61dmin/users",
		"/api/v1/customer/%2e%2e/admin/users", "/api/v1/customer/%2E%2E/admin/users",
		"/api/v1/customer/%252e%252e/admin/users", "/api/v1/customer/.%2e/admin", "/api/v1/./admin/users",
		"//api/v1/admin/users", "/api//v1//admin/users", "/api/v1///admin", "/api/v1/customer/..%2fadmin",
		"/api/v1/customer/..%5cadmin", "/api/v1/customer/%5c..%5cadmin",
		"/internal/karryam-admin/v1/capabilities", "/INTERNAL/karryam-admin/v1/capabilities",
		"/internal/", "/api/v1/customer/internal/admin/customers/x", "/api/v1/customer/Internal/x",
		"/api/v1/customer/%69nternal/x", "/api/v1/taxi/%2569nternal/x", "/api/v1/taxi/internal%2Fx",
		"/api/v1/support-disputes/admin/complaints", "/api/v1/SUPPORT-DISPUTES/ADMIN",
		"/api/v1/verification-compliance/cases", "/api/v1/taxi/metrics", "/api/v1/taxi/outbox-metrics/",
		"/api/v1/notifications", "/api/v1/notifications/send",
	}
	headerSets := []map[string]string{
		nil,
		websocketHeaders,
		{"Host": "admin-backoffice-service:8110"},
		{"X-Forwarded-Host": "admin-backoffice-service:8110", "X-Forwarded-Proto": "https", "Forwarded": "host=admin-backoffice-service"},
		{"X-Original-URL": "/api/v1/admin/users", "X-Rewrite-URL": "/api/v1/admin/users", "X-HQ-Principal": "cosmicforge-hq-karryam-admin"},
	}
	methods := []string{"GET", "HEAD", "OPTIONS", "TRACE", "CONNECT", "POST", "PUT", "PATCH", "DELETE"}
	for _, path := range paths {
		for _, method := range methods {
			for _, headers := range headerSets {
				request := httptest.NewRequest(method, path, nil)
				for key, value := range headers {
					if key == "Host" {
						request.Host = value
						continue
					}
					request.Header.Set(key, value)
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, request)
				if w.Code != http.StatusNotFound {
					t.Errorf("%s %s %v: %d", method, path, headers, w.Code)
				}
			}
		}
	}
	if public.count() != 0 || admin.count() != 0 {
		t.Fatalf("bypass reached upstreams: public=%d admin=%d", public.count(), admin.count())
	}
}

// /api/v1/administrator is not /api/v1/admin: the deny is segment-exact, so
// it neither over-blocks a legitimate nested segment nor is fooled by a
// longer sibling (which simply has no route).
func TestAdminPrefixConfusion(t *testing.T) {
	if !models.IsInternalPath("/api/v1/admin") || !models.IsInternalPath("/api/v1/ADMIN/x") {
		t.Fatal("admin path not classified private")
	}
	if models.IsInternalPath("/api/v1/administrator") || models.IsInternalPath("/api/v1/customer/administrator") {
		t.Fatal("prefix confusion: sibling classified as admin")
	}
	router, public, admin := allOwnersFixture(t)
	for _, path := range []string{"/api/v1/administrator", "/api/v1/admin.json", "/api/v1/admin;x=1", "/api/v1/adminx/users"} {
		if w := serve(router, http.MethodGet, path, nil); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d", path, w.Code)
		}
	}
	if public.count() != 0 || admin.count() != 0 {
		t.Fatal("unrouted sibling reached an upstream")
	}
	if w := serve(router, http.MethodGet, "/api/v1/customer/administrator", nil); w.Code != http.StatusOK || public.count() != 1 {
		t.Fatalf("legitimate nested segment over-blocked: %d", w.Code)
	}
}

// rawExchange sends one raw HTTP/1.1 request to a real listener and returns
// the status line.
func rawExchange(t *testing.T, address, raw string) string {
	t.Helper()
	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.WriteString(conn, raw); err != nil {
		t.Fatal(err)
	}
	status, _ := bufio.NewReader(conn).ReadString('\n')
	return status
}

// Absolute-form request targets and hostile Host values cannot pick the
// destination: the path alone selects an owner, the Host is never relayed.
func TestAbsoluteFormAndHostCannotSelectAnUpstream(t *testing.T) {
	router, public, admin := allOwnersFixture(t)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	address := strings.TrimPrefix(server.URL, "http://")
	for _, target := range []string{
		"http://admin-backoffice-service:8110/api/v1/admin/users",
		"http://admin-backoffice-service:8110/internal/karryam-admin/v1/capabilities",
		"https://127.0.0.1:8110/api/v1/admin",
		"http://admin-backoffice-service:8110/API/V1/ADMIN/",
	} {
		for _, method := range []string{"GET", "HEAD", "OPTIONS", "TRACE"} {
			status := rawExchange(t, address, method+" "+target+" HTTP/1.1\r\nHost: admin-backoffice-service:8110\r\nConnection: close\r\n\r\n")
			if !strings.Contains(status, " 404 ") {
				t.Errorf("%s %s: %q", method, target, status)
			}
		}
	}
	// A malformed Host never reaches the router at all.
	if status := rawExchange(t, address, "GET /api/v1/customer/me HTTP/1.1\r\nHost: bad host\r\nConnection: close\r\n\r\n"); !strings.Contains(status, " 400 ") {
		t.Errorf("invalid Host accepted: %q", status)
	}
	if public.count() != 0 || admin.count() != 0 {
		t.Fatalf("absolute-form reached upstreams: public=%d admin=%d", public.count(), admin.count())
	}
	// Absolute-form to a public path is routed by path; the client's Host is
	// replaced by the owner's origin and is not relayed as X-Forwarded-Host.
	status := rawExchange(t, address, "GET http://admin-backoffice-service:8110/api/v1/customer/me HTTP/1.1\r\nHost: admin-backoffice-service:8110\r\nX-Forwarded-Host: admin-backoffice-service\r\nConnection: close\r\n\r\n")
	if !strings.Contains(status, " 200 ") || public.count() != 1 || admin.count() != 0 {
		t.Fatalf("public absolute-form: %q public=%d admin=%d", status, public.count(), admin.count())
	}
	forwarded, _ := public.last()
	if strings.Contains(forwarded.Host, "admin") || forwarded.Header.Get("X-Forwarded-Host") != "" {
		t.Fatalf("client host relayed: host=%q xfh=%q", forwarded.Host, forwarded.Header.Get("X-Forwarded-Host"))
	}
}
