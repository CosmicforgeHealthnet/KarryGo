package bookinghttp

import (
	"context"
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	bookingrepo "cosmicforge/logistics/services/hauling-service/internal/features/booking/repositories"
	bookingusecases "cosmicforge/logistics/services/hauling-service/internal/features/booking/usecases"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/livetrack"
)

// trackRepo answers only the one read the tracking socket performs; any other
// repository call would panic through the nil embedded interface, which keeps
// the test honest about what the handler touches.
type trackRepo struct {
	bookingrepo.BookingRepository
	bookings map[string]bookingmodels.Booking
}

func (r trackRepo) GetByIDForCustomer(_ context.Context, id, customerID string) (bookingmodels.Booking, error) {
	b, ok := r.bookings[id]
	if !ok || b.CustomerID != customerID {
		return bookingmodels.Booking{}, apperrors.NotFound("Booking could not be found.", nil)
	}
	return b, nil
}

// TestTrackSocket_AuthUpgradeRelayCloseAndReconnect exercises the customer
// live-tracking websocket end to end on the websocket library version this
// service builds with: authentication refused before any upgrade, ownership
// enforced, upgrade, the initial frame, relay of a published fix, client
// close ending the stream, and a clean reconnect.
func TestTrackSocket_AuthUpgradeRelayCloseAndReconnect(t *testing.T) {
	// JOURNEY_TEST_REDIS_ADDR is the disposable Redis this service's other
	// live-tracking tests use (the certification gate provides it).
	addr := os.Getenv("JOURNEY_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set JOURNEY_TEST_REDIS_ADDR to run the tracking socket test against Redis")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis at %s unavailable: %v", addr, err)
	}

	providerID := "prv_ws_" + time.Now().Format("150405.000000")
	repo := trackRepo{bookings: map[string]bookingmodels.Booking{
		"bkg_active": {ID: "bkg_active", CustomerID: "cus_1", ProviderID: &providerID, Status: bookingmodels.StatusEnRoutePickup},
	}}
	svc := bookingusecases.NewBookingService(bookingusecases.Options{Bookings: repo})
	signer := sharedauth.NewTokenSigner([]byte("track-socket-test-secret"))
	handler := NewTrackHandler(svc, signer, rdb)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	// The production router's request-id and error middleware turn an
	// aborted request into its status code and error envelope.
	engine.Use(httpx.RequestID(), httpx.ErrorHandler())
	engine.GET("/customer/bookings/:id/track", handler.StreamBookingTrack)
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	wsBase := "ws" + strings.TrimPrefix(server.URL, "http")

	token := func(role, subject string) string {
		tok, err := signer.Sign(sharedauth.Claims{Subject: subject, Role: role, Service: "customer", SessionID: "ses_1", ExpiresAt: time.Now().Add(time.Hour).Unix()})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	dial := func(path string) (*websocket.Conn, *nethttp.Response, error) {
		return websocket.DefaultDialer.Dial(wsBase+path, nil)
	}

	// Refusals happen before the upgrade, so the handshake fails with the
	// handler's own status rather than a socket being opened.
	for name, tc := range map[string]struct {
		path string
		want int
	}{
		"no token":        {"/customer/bookings/bkg_active/track", nethttp.StatusUnauthorized},
		"provider token":  {"/customer/bookings/bkg_active/track?token=" + token("truck_provider", "cus_1"), nethttp.StatusForbidden},
		"not the owner":   {"/customer/bookings/bkg_active/track?token=" + token("customer", "cus_other"), nethttp.StatusNotFound},
		"unknown booking": {"/customer/bookings/bkg_missing/track?token=" + token("customer", "cus_1"), nethttp.StatusNotFound},
	} {
		conn, resp, err := dial(tc.path)
		if err == nil {
			_ = conn.Close()
			t.Fatalf("%s: handshake succeeded, want refusal %d", name, tc.want)
		}
		if resp == nil || resp.StatusCode != tc.want {
			t.Fatalf("%s: handshake status = %v, want %d", name, resp, tc.want)
		}
	}

	path := "/customer/bookings/bkg_active/track?token=" + token("customer", "cus_1")
	readUpdate := func(conn *websocket.Conn) livetrack.Update {
		t.Helper()
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read frame: %v", err)
		}
		var u livetrack.Update
		if err := json.Unmarshal(raw, &u); err != nil {
			t.Fatalf("decode frame %s: %v", raw, err)
		}
		return u
	}
	for attempt := 1; attempt <= 2; attempt++ { // the second pass proves reconnect after close
		conn, resp, err := dial(path)
		if err != nil {
			t.Fatalf("attempt %d: dial = %v (%v)", attempt, err, resp)
		}
		if resp.StatusCode != nethttp.StatusSwitchingProtocols {
			t.Fatalf("attempt %d: handshake status = %d, want 101", attempt, resp.StatusCode)
		}
		// No booking-bound location is wired, so the first frame says the
		// driver is offline rather than leaking a global heartbeat.
		first := readUpdate(conn)
		raw, _ := json.Marshal(first)
		if !strings.Contains(strings.ToLower(string(raw)), "offline") {
			t.Fatalf("attempt %d: first frame = %s, want an offline update", attempt, raw)
		}
		// A published fix reaches the subscriber. Publish until it lands,
		// since the subscription is established asynchronously.
		fix, _ := json.Marshal(livetrack.LocationUpdate(livetrack.Fix{EntityID: providerID, Lat: 6.5, Lng: 3.4, RecordedAt: time.Now().UTC()}))
		got := make(chan []byte, 1)
		go func() {
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, frame, err := conn.ReadMessage()
			if err == nil {
				got <- frame
			}
			close(got)
		}()
		deadline := time.Now().Add(5 * time.Second)
		var frame []byte
	publish:
		for time.Now().Before(deadline) {
			if err := rdb.Publish(context.Background(), livetrack.ChannelKey(availabilityrepositories.TrackingNamespace, providerID), fix).Err(); err != nil {
				t.Fatal(err)
			}
			select {
			case frame = <-got:
				break publish
			case <-time.After(100 * time.Millisecond):
			}
		}
		if string(frame) != string(fix) {
			t.Fatalf("attempt %d: relayed frame = %s, want %s", attempt, frame, fix)
		}
		if err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")); err != nil {
			t.Fatalf("attempt %d: close: %v", attempt, err)
		}
		_ = conn.Close()
	}

	// The relay unsubscribes once the client is gone.
	deadline := time.Now().Add(5 * time.Second)
	for {
		n, err := rdb.PubSubNumSub(context.Background(), livetrack.ChannelKey(availabilityrepositories.TrackingNamespace, providerID)).Result()
		if err != nil {
			t.Fatal(err)
		}
		if n[livetrack.ChannelKey(availabilityrepositories.TrackingNamespace, providerID)] == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("tracking relay still subscribed after the client closed")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
