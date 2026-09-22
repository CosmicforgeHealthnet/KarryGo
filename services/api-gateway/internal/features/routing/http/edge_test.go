package http

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/api-gateway/internal/features/routing/models"
	"cosmicforge/logistics/shared/go/httpx"
)

// recordingUpstream is a canonical service that remembers what reached it.
type recordingUpstream struct {
	name   string
	mu     sync.Mutex
	hits   []*http.Request
	bodies [][]byte
	status int
	delay  time.Duration
	reply  string
	server *httptest.Server
}

func newUpstream(t *testing.T, name string) *recordingUpstream {
	u := &recordingUpstream{name: name, status: http.StatusOK}
	u.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		u.mu.Lock()
		u.hits = append(u.hits, r.Clone(r.Context()))
		u.bodies = append(u.bodies, body)
		status, delay, reply := u.status, u.delay, u.reply
		u.mu.Unlock()
		if delay > 0 {
			time.Sleep(delay)
		}
		if reply == "" {
			reply = `{"success":true,"data":{"served_by":"` + u.name + `"}}`
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(u.server.Close)
	return u
}

func (u *recordingUpstream) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.hits)
}

func (u *recordingUpstream) last() (*http.Request, []byte) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.hits[len(u.hits)-1], u.bodies[len(u.bodies)-1]
}

type fixture struct {
	router    *gin.Engine
	edge      *Edge
	upstreams map[string]*recordingUpstream
}

func newFixture(t *testing.T, timeout time.Duration) *fixture {
	gin.SetMode(gin.TestMode)
	upstreams := map[string]*recordingUpstream{}
	origins := map[string]string{}
	for _, name := range []string{models.UpstreamCustomer, models.UpstreamDispatch, models.UpstreamWallet,
		models.UpstreamTaxi, models.UpstreamFleet} {
		upstreams[name] = newUpstream(t, name)
		origins[name] = upstreams[name].server.URL
	}
	edge, err := NewEdge(Options{Routes: models.DefaultRoutes(), Origins: origins, Timeout: timeout,
		BreakerThreshold: 3, BreakerCooldown: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(httpx.RequestID())
	// Gateway-owned routes, registered before the edge exactly as in main.
	router.GET("/health", func(c *gin.Context) { c.String(http.StatusOK, "gateway-health") })
	router.POST("/api/v1/internal/callbacks/dispatch", func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		c.String(http.StatusOK, "gateway-callback:"+string(body))
	})
	router.GET("/api/v1/gateway/customer/overview", edge.Aggregate(CustomerOverviewParts, nil))
	router.NoRoute(edge.Handle)
	return &fixture{router: router, edge: edge, upstreams: upstreams}
}

func (f *fixture) do(method, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	return recorder
}

func TestTheLongestPrefixOwnsThePath(t *testing.T) {
	f := newFixture(t, time.Second)
	cases := map[string]string{
		"/api/v1/customer/me":                models.UpstreamCustomer,
		"/api/v1/customer/dispatch/bookings": models.UpstreamDispatch,
		"/api/v1/customer/notifications/ws":  models.UpstreamDispatch,
		"/api/v1/payment-wallet/wallets/me":  models.UpstreamWallet,
		"/api/v1/taxi/customer/bookings":     models.UpstreamTaxi,
		"/fleet-partner/v1/organizations/o1": models.UpstreamFleet,
	}
	for path, want := range cases {
		before := f.upstreams[want].count()
		if code := f.do(http.MethodGet, path, nil, nil).Code; code != http.StatusOK {
			t.Errorf("%s -> %d", path, code)
		}
		if f.upstreams[want].count() != before+1 {
			t.Errorf("%s did not reach %s", path, want)
		}
		request, _ := f.upstreams[want].last()
		if request.URL.Path != path {
			t.Errorf("%s was rewritten to %s", path, request.URL.Path)
		}
	}
	if code := f.do(http.MethodGet, "/api/v1/taxicab", nil, nil).Code; code != http.StatusNotFound {
		t.Errorf("a prefix must match only at a segment boundary, got %d", code)
	}
}

func TestInternalRoutesAreNeverForwarded(t *testing.T) {
	f := newFixture(t, time.Second)
	for _, path := range []string{
		"/internal/vendor/v1/verification-events",
		"/api/v1/payment-wallet/internal/payments/ref",
		"/api/v1/payment-wallet/internal/jobs/driver-hauling-service/b1/execution-provider",
		"/api/v1/customer/internal/customers/c1",
		"/api/v1/taxi/INTERNAL/commerce/capabilities",
		"/api/v1/taxi/%69nternal/x",
		"/api/v1/customer/../internal/x",
		"/api/v1/internal/marketplace/offers",
	} {
		if code := f.do(http.MethodGet, path, nil, nil).Code; code != http.StatusNotFound {
			t.Errorf("%s -> %d, want 404", path, code)
		}
	}
	for name, upstream := range f.upstreams {
		if upstream.count() != 0 {
			t.Errorf("an internal route reached %s", name)
		}
	}
}

// The gateway's own routes -- including its own internal callbacks -- are
// served by the gateway, never forwarded.
func TestGatewayOwnedRoutesWin(t *testing.T) {
	f := newFixture(t, time.Second)
	if body := f.do(http.MethodGet, "/health", nil, nil).Body.String(); body != "gateway-health" {
		t.Fatalf("health: %q", body)
	}
	raw := []byte(`{"event":"trip.completed",  "note":"  spacing kept  "}`)
	if body := f.do(http.MethodPost, "/api/v1/internal/callbacks/dispatch", raw, nil).Body.String(); body != "gateway-callback:"+string(raw) {
		t.Fatalf("the gateway's own callback lost its raw body: %q", body)
	}
}

func TestClientsCannotAssertPlatformIdentity(t *testing.T) {
	f := newFixture(t, time.Second)
	f.do(http.MethodGet, "/api/v1/customer/me", nil, map[string]string{
		"Authorization":          "Bearer customer-token",
		"X-Service-Name":         "payment-wallet-service",
		"X-Service-Signature":    "forged",
		"X-Service-Nonce":        "n",
		"X-Internal-Service-Key": "guess",
		"X-Gateway-Scope":        "ADMIN",
		"X-Forwarded-For":        "10.0.0.1",
		"X-Real-IP":              "10.0.0.2",
	})
	request, _ := f.upstreams[models.UpstreamCustomer].last()
	for _, header := range []string{"X-Service-Name", "X-Service-Signature", "X-Service-Nonce", "X-Internal-Service-Key", "X-Real-IP"} {
		if value := request.Header.Get(header); value != "" {
			t.Errorf("%s reached the service: %q", header, value)
		}
	}
	if request.Header.Get("Authorization") != "Bearer customer-token" {
		t.Error("the caller's own credential must pass through; the service authenticates it")
	}
	if request.Header.Get(HeaderScope) != string(models.ScopeCustomer) {
		t.Errorf("scope=%q, want the route's, not the client's", request.Header.Get(HeaderScope))
	}
	if strings.Contains(request.Header.Get("X-Forwarded-For"), "10.0.0.1") {
		t.Error("a client-supplied X-Forwarded-For was trusted")
	}
}

func TestCorrelationReachesTheServiceAndTheClient(t *testing.T) {
	f := newFixture(t, time.Second)
	response := f.do(http.MethodGet, "/api/v1/customer/me", nil, map[string]string{"X-Correlation-ID": "trace-123"})
	request, _ := f.upstreams[models.UpstreamCustomer].last()
	if request.Header.Get(HeaderCorrelationID) != "trace-123" || request.Header.Get(HeaderRequestID) != "trace-123" {
		t.Fatalf("correlation not propagated: %v", request.Header)
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("the client got no request id")
	}
	f.do(http.MethodGet, "/api/v1/customer/me", nil, map[string]string{"X-Correlation-ID": "bad id with spaces"})
	request, _ = f.upstreams[models.UpstreamCustomer].last()
	if strings.Contains(request.Header.Get(HeaderCorrelationID), " ") || request.Header.Get(HeaderCorrelationID) == "" {
		t.Fatalf("a malformed correlation id was forwarded: %q", request.Header.Get(HeaderCorrelationID))
	}
}

// A payment provider's signature is over the exact bytes it sent.
func TestAWebhookBodyIsForwardedByteForByte(t *testing.T) {
	f := newFixture(t, time.Second)
	raw := []byte("{\"event\":\"charge.success\",\n  \"data\": {\"amount\": 100000, \"note\": \"é  trailing  \"}}\r\n")
	response := f.do(http.MethodPost, "/api/v1/payment-wallet/webhooks/paystack", raw,
		map[string]string{"X-Paystack-Signature": "sig-abc", "Content-Type": "application/json"})
	if response.Code != http.StatusOK {
		t.Fatalf("webhook -> %d", response.Code)
	}
	request, body := f.upstreams[models.UpstreamWallet].last()
	if !bytes.Equal(body, raw) {
		t.Fatalf("the webhook body changed in transit:\nsent %q\ngot  %q", raw, body)
	}
	if request.Header.Get("X-Paystack-Signature") != "sig-abc" {
		t.Fatal("the webhook signature header was dropped")
	}
}

func TestUpstreamAnswersPassThroughUntouched(t *testing.T) {
	f := newFixture(t, time.Second)
	customer := f.upstreams[models.UpstreamCustomer]
	for _, status := range []int{http.StatusUnauthorized, http.StatusUnprocessableEntity, http.StatusInternalServerError} {
		customer.mu.Lock()
		customer.status, customer.reply = status, `{"success":false,"error":{"code":"from_service"}}`
		customer.mu.Unlock()
		response := f.do(http.MethodGet, "/api/v1/customer/me", nil, nil)
		if response.Code != status || !strings.Contains(response.Body.String(), "from_service") {
			t.Errorf("status %d was rewritten: %d %s", status, response.Code, response.Body.String())
		}
	}
	// A service that answers -- even 500 -- is up; the breaker stays closed.
	if f.edge.upstreams[models.UpstreamCustomer].Breaker.Open() {
		t.Fatal("service answers opened the breaker")
	}
}

func TestASlowServiceTimesOutAtTheEdge(t *testing.T) {
	f := newFixture(t, 100*time.Millisecond)
	taxi := f.upstreams[models.UpstreamTaxi]
	taxi.mu.Lock()
	taxi.delay = 400 * time.Millisecond
	taxi.mu.Unlock()
	if code := f.do(http.MethodGet, "/api/v1/taxi/customer/bookings", nil, nil).Code; code != http.StatusGatewayTimeout {
		t.Fatalf("slow upstream -> %d, want 504", code)
	}
}

func TestAFailingServiceTripsItsBreakerAndOnlyItsBreaker(t *testing.T) {
	f := newFixture(t, time.Second)
	wallet := f.upstreams[models.UpstreamWallet]
	wallet.mu.Lock()
	wallet.status = http.StatusServiceUnavailable
	wallet.mu.Unlock()
	for i := 0; i < 3; i++ {
		f.do(http.MethodGet, "/api/v1/payment-wallet/wallets/me", nil, nil)
	}
	hits := wallet.count()
	response := f.do(http.MethodGet, "/api/v1/payment-wallet/wallets/me", nil, nil)
	if response.Code != http.StatusServiceUnavailable || response.Header().Get("Retry-After") == "" {
		t.Fatalf("open breaker -> %d retry-after=%q", response.Code, response.Header().Get("Retry-After"))
	}
	if wallet.count() != hits {
		t.Fatal("an open breaker still sent the request")
	}
	if code := f.do(http.MethodGet, "/api/v1/customer/me", nil, nil).Code; code != http.StatusOK {
		t.Fatalf("one service's outage blocked another: %d", code)
	}
}

func TestAnUnreachableServiceIsABadGateway(t *testing.T) {
	gin.SetMode(gin.TestMode)
	edge, err := NewEdge(Options{Routes: models.DefaultRoutes(),
		Origins: map[string]string{models.UpstreamHauling: "http://127.0.0.1:1"}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.NoRoute(edge.Handle)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/hauling/customer/bookings", nil))
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("unreachable -> %d", recorder.Code)
	}
}

func TestAnUnconfiguredServiceIsNotRouted(t *testing.T) {
	f := newFixture(t, time.Second)
	if code := f.do(http.MethodGet, "/api/v1/hauling/customer/bookings", nil, nil).Code; code != http.StatusNotFound {
		t.Fatalf("unconfigured upstream -> %d, want 404", code)
	}
}

func TestAnInvalidOriginRefusesToStart(t *testing.T) {
	if _, err := NewEdge(Options{Origins: map[string]string{"taxi": "not a url"}}); err == nil {
		t.Fatal("an invalid origin was accepted")
	}
}

// A websocket upgrade is carried through with no deadline.
func TestAWebsocketUpgradeIsCarriedThrough(t *testing.T) {
	var upgraded atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		upgraded.Store(true)
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		_ = rw.Flush()
		line, _ := rw.ReadString('\n')
		_, _ = rw.WriteString("echo:" + line)
		_ = rw.Flush()
	}))
	t.Cleanup(upstream.Close)
	edge, err := NewEdge(Options{Routes: models.DefaultRoutes(),
		Origins: map[string]string{models.UpstreamNotification: upstream.URL}, Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.NoRoute(edge.Handle)
	gateway := httptest.NewServer(router)
	t.Cleanup(gateway.Close)

	conn, err := net.Dial("tcp", strings.TrimPrefix(gateway.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = io.WriteString(conn, "GET /api/v1/notifications/ws?token=t HTTP/1.1\r\nHost: gateway\r\n"+
		"Upgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n"+
		"Sec-WebSocket-Version: 13\r\n\r\n")
	reader := bufio.NewReader(conn)
	status, _ := reader.ReadString('\n')
	if !strings.Contains(status, "101") {
		t.Fatalf("upgrade status %q", status)
	}
	for line, _ := reader.ReadString('\n'); line != "\r\n" && line != ""; line, _ = reader.ReadString('\n') {
	}
	// Longer than the edge's request timeout: an open socket has no deadline.
	time.Sleep(120 * time.Millisecond)
	_, _ = io.WriteString(conn, "hello\n")
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	echo, _ := reader.ReadString('\n')
	if echo != "echo:hello\n" || !upgraded.Load() {
		t.Fatalf("socket did not carry traffic: %q", echo)
	}
}

func TestTheOverviewComposesWithoutInterpreting(t *testing.T) {
	f := newFixture(t, time.Second)
	f.upstreams[models.UpstreamCustomer].reply = `{"success":true,"data":{"first_name":"Ada","onboarding_status":"complete"}}`
	f.upstreams[models.UpstreamWallet].reply = `{"success":true,"data":{"available_kobo":150000}}`
	response := f.do(http.MethodGet, "/api/v1/gateway/customer/overview", nil,
		map[string]string{"Authorization": "Bearer customer-token"})
	var body struct {
		Data struct {
			Profile     map[string]any `json:"profile"`
			Wallet      map[string]any `json:"wallet"`
			Unavailable []string       `json:"unavailable"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Profile["first_name"] != "Ada" || body.Data.Wallet["available_kobo"] != float64(150000) ||
		len(body.Data.Unavailable) != 0 {
		t.Fatalf("overview %s", response.Body.String())
	}
	for _, name := range []string{models.UpstreamCustomer, models.UpstreamWallet} {
		request, _ := f.upstreams[name].last()
		if request.Header.Get("Authorization") != "Bearer customer-token" || request.Method != http.MethodGet {
			t.Errorf("%s read without the caller's own credential", name)
		}
	}
}

func TestTheOverviewSurvivesAPartialOutage(t *testing.T) {
	f := newFixture(t, time.Second)
	f.upstreams[models.UpstreamWallet].status = http.StatusServiceUnavailable
	response := f.do(http.MethodGet, "/api/v1/gateway/customer/overview", nil, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"unavailable":["wallet"]`) ||
		!strings.Contains(response.Body.String(), `"wallet":null`) {
		t.Fatalf("partial outage: %d %s", response.Code, response.Body.String())
	}
}
