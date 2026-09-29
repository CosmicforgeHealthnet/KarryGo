package journeyshare

import (
	"context"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestClientIPRejectsSpoofedForwarding(t *testing.T) {
	prefix := netip.MustParsePrefix("10.1.0.0/24")
	for _, scenario := range []struct {
		peer, header, want string
		trust              bool
	}{
		{"192.0.2.9:5555", "198.51.100.8", "192.0.2.9", true},
		{"10.1.0.4:5555", "spoof, 192.0.2.9", "192.0.2.9", true},
		{"10.1.0.4:5555", "192.0.2.9, 10.1.0.3", "192.0.2.9", true},
		{"10.1.0.4:5555", "bad", "10.1.0.4", true},
		{"10.1.0.4:5555", "192.0.2.9", "10.1.0.4", false},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/", nil)
		c.Request.RemoteAddr = scenario.peer
		c.Request.Header.Set("X-Forwarded-For", scenario.header)
		var gateways []netip.Prefix
		if scenario.trust {
			gateways = []netip.Prefix{prefix}
		}
		if got := ClientIP(c, gateways); got != scenario.want {
			t.Fatal(scenario, got)
		}
	}
}

func TestPublicPrivacyAndPermissions(t *testing.T) {
	p := Projection{CustomerID: "private-owner", SourceRef: "private-id", Authority: "TAXI", ProductCode: "RIDE", Provider: &Provider{DisplayName: "Ada"}, Pickup: &Place{Label: "home"}, Location: Location{Available: true, State: "FRESH"}}
	raw, _ := json.Marshal(p.Public([]string{"provider"}))
	for _, secret := range []string{"private-owner", "private-id", "home", "source_version", "customer_id", "authority"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("private field leaked", secret)
		}
	}
	p.Terminal = true
	if p.Public([]string{"location"}).Location.Available {
		t.Fatal("terminal location leaked")
	}
}
func TestHMACBoundaryAndClient(t *testing.T) {
	server := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer redisClient.Close()
	secret := []byte("test-secret")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"customer-service": secret, "wrong": secret}, 5*time.Minute)
	engine := gin.New()
	RegisterRead(engine, "/internal/taxi/v1/customer-share/bookings/:sourceRef", func(_ context.Context, ref, customer string) (Projection, error) {
		return Projection{Authority: "TAXI", SourceRef: ref, CustomerID: customer, ProductCode: "RIDE", Eligible: true, SourceVersion: 1}, nil
	}, RedisBoundary(verifier, redisClient, "customer-service")...)
	ts := httptest.NewServer(engine)
	defer ts.Close()
	client := Client{Endpoints: map[string]Endpoint{"TAXI": {BaseURL: ts.URL, Secret: secret}}}
	if _, err := client.Read(context.Background(), "TAXI", "ride", "customer"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, caller string
		age          time.Duration
		bad          bool
		want         int
	}{{"missing", "", 0, false, 401}, {"wrong", "wrong", 0, false, 403}, {"expired", "customer-service", -time.Hour, false, 401}, {"bad", "customer-service", 0, true, 401}, {"valid", "customer-service", 0, false, 200}} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/internal/taxi/v1/customer-share/bookings/ride?customer_id=customer", nil)
			if tc.caller != "" {
				_ = serviceauth.SignRequest(req, tc.caller, secret, nil, time.Now().Add(tc.age))
			}
			if tc.bad {
				req.Header.Set(serviceauth.HeaderSignature, "bad")
			}
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("got %d want %d: %s", w.Code, tc.want, w.Body.String())
			}
			if tc.want == 200 {
				w = httptest.NewRecorder()
				engine.ServeHTTP(w, req)
				if w.Code != 409 {
					t.Fatal("nonce replay accepted")
				}
			}
		})
	}
}
func TestRateLimitAndPointBinding(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rc.Close()
	l := RedisLimiter{Client: rc}
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		ok, err := l.Allow(ctx, "bucket", 3, time.Minute)
		if err != nil || ok != (i < 3) {
			t.Fatal(i, ok, err)
		}
	}
	mr.FastForward(time.Minute)
	if ok, _ := l.Allow(ctx, "bucket", 3, time.Minute); !ok {
		t.Fatal("window did not reset")
	}
	store := BoundPointStore{Client: rc, Namespace: "test"}
	_ = store.Put(ctx, "trip", "provider", "assignment1", Point{Lat: 6, Lng: 3, At: time.Now().Add(-time.Minute)})
	p, err := store.Read(ctx, "trip", "provider", "assignment1", 30*time.Second)
	if err != nil || p.State != "STALE" {
		t.Fatal(p, err)
	}
	for _, binding := range []string{"assignment2", "different-provider"} {
		p, _ = store.Read(ctx, "trip", "provider", binding, 30*time.Second)
		if p.Available {
			t.Fatal("unbound point leaked")
		}
	}
}
func TestUnavailableAndMismatchedAuthority(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"success": true, "data": Projection{Authority: "TAXI", SourceRef: "other", CustomerID: "wrong", ProductCode: "RIDE", SourceVersion: 1}})
	}))
	defer ts.Close()
	c := Client{Endpoints: map[string]Endpoint{"TAXI": {BaseURL: ts.URL, Secret: []byte("test")}}}
	if _, err := c.Read(context.Background(), "TAXI", "ride", "owner"); err == nil {
		t.Fatal("wrong ownership accepted")
	}
	if _, err := c.Read(context.Background(), "HAULING", "ride", "owner"); err == nil {
		t.Fatal("missing authority accepted")
	}
}
