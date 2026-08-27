package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/commerceanalytics"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestNormalizeEnvelopeAcceptsVendorOutboxEnvelope(t *testing.T) {
	payload, _ := json.Marshal(commerceanalytics.SupportOutcomePayload{CaseRef: "case_1", Outcome: "CONFIRMED", VendorFault: true})
	raw, _ := json.Marshal(vendorEnvelope{
		EventID:          "evt_1",
		EventType:        commerceanalytics.EventSupportCaseResolved,
		SchemaVersion:    1,
		AggregateType:    "SupportCase",
		AggregateID:      "case_1",
		AggregateVersion: 7,
		VendorID:         "biz_1",
		OccurredAt:       time.Now().UTC(),
		Producer:         "vendor-service",
		Payload:          payload,
	})
	event, err := normalizeEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	if event.SourceService != "vendor-service" || event.SourceAggregate != "SupportCase" || event.BusinessID != "biz_1" {
		t.Fatalf("unexpected normalized event: %+v", event)
	}
	if err := commerceanalytics.ValidateEnvelope(event); err != nil {
		t.Fatalf("normalized event should validate: %v", err)
	}
}

func TestAnalyticsReadRequiresHMACAndBusinessScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := []byte("analytics-secret")
	handler := NewHandler(nil, serviceauth.NewVerifier(serviceauth.Secrets{"vendor-service": secret}, time.Minute), []string{"vendor-service"})
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/analytics"), handler)

	unsigned := httptest.NewRecorder()
	router.ServeHTTP(unsigned, httptest.NewRequest(http.MethodGet, "/api/v1/analytics/commerce/vendor-reports?business_id=biz_1", nil))
	if unsigned.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned analytics read status=%d", unsigned.Code)
	}

	scoped := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/commerce/vendor-reports?business_id=biz_2", nil)
	req.Header.Set("X-Analytics-Business-ID", "biz_1")
	if err := serviceauth.SignRequest(req, "vendor-service", secret, nil, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	router.ServeHTTP(scoped, req)
	if scoped.Code != http.StatusForbidden {
		t.Fatalf("cross-business analytics read status=%d", scoped.Code)
	}
}

func TestAnalyticsHMACReplayRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := []byte("analytics-secret")
	handler := NewHandler(nil, serviceauth.NewVerifier(serviceauth.Secrets{"vendor-service": secret}, time.Minute), []string{"vendor-service"})
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/analytics"), handler)

	makeReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/commerce/vendor-reports?business_id=biz_2", bytes.NewReader(nil))
		req.Header.Set("X-Analytics-Business-ID", "biz_1")
		req.Header.Set(serviceauth.HeaderNonce, "fixed-nonce")
		if err := serviceauth.SignRequest(req, "vendor-service", secret, nil, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		return req
	}

	first := httptest.NewRecorder()
	router.ServeHTTP(first, makeReq())
	if first.Code != http.StatusForbidden {
		t.Fatalf("first scoped failure status=%d", first.Code)
	}
	second := httptest.NewRecorder()
	router.ServeHTTP(second, makeReq())
	if second.Code != http.StatusUnauthorized {
		t.Fatalf("replay status=%d", second.Code)
	}
}
