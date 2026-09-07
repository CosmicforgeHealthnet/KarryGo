package outbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestVendorClientSignsAndFansOutDeterministicLifecycleEvents(t *testing.T) {
	secret := []byte("taxi-vendor-test-secret")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"driver-taxi-service": secret}, time.Minute)
	seen := map[string]bool{}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(serviceauth.Middleware(verifier))
	router.POST(vendorEventPath, func(c *gin.Context) {
		var body struct {
			EventID       string `json:"event_id"`
			VendorOrderID string `json:"vendor_order_id"`
			EventType     string `json:"event_type"`
		}
		if err := json.NewDecoder(c.Request.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		seen[body.EventID] = body.EventType == "DELIVERED" && body.VendorOrderID != ""
		c.Status(http.StatusOK)
	})
	server := httptest.NewServer(router)
	defer server.Close()
	payload := json.RawMessage(`{"mission_ref":"mission-1","parent_order_id":"parent-1","vendor_order_ids":["vendor-1","vendor-2"],"source_version":7,"vendor_event_type":"DELIVERED"}`)
	err := (VendorClient{BaseURL: server.URL, Secret: secret, HTTPClient: server.Client()}).Deliver(context.Background(), Record{ID: "event-1", EventType: "taxi.goods.completed.v1", Payload: payload, OccurredAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || !seen["event-1:vendor-1"] || !seen["event-1:vendor-2"] {
		t.Fatalf("fan-out=%v", seen)
	}
}

func TestVendorClientRejectsInvalidHMACAndStaleTimestamp(t *testing.T) {
	secret := []byte("correct-secret")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"driver-taxi-service": secret}, time.Second)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(serviceauth.Middleware(verifier))
	router.POST(vendorEventPath, func(c *gin.Context) { c.Status(http.StatusOK) })
	server := httptest.NewServer(router)
	defer server.Close()
	payload := json.RawMessage(`{"mission_ref":"mission-1","parent_order_id":"parent-1","vendor_order_ids":["vendor-1"],"source_version":1,"vendor_event_type":"PICKED_UP"}`)
	record := Record{ID: "event-1", Payload: payload, OccurredAt: time.Now()}
	if err := (VendorClient{BaseURL: server.URL, Secret: []byte("wrong"), HTTPClient: server.Client()}).Deliver(context.Background(), record); err == nil {
		t.Fatal("invalid HMAC was accepted")
	}
	if err := (VendorClient{BaseURL: server.URL, Secret: secret, HTTPClient: server.Client(), Now: func() time.Time { return time.Now().Add(-time.Hour) }}).Deliver(context.Background(), record); err == nil {
		t.Fatal("stale signature timestamp was accepted")
	}
}
