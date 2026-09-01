package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/commerceanalytics"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
)

func dispatchRouteWire(t *testing.T) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"eventId": "route-event", "eventType": "dispatch.route_alert.opened.v1", "schemaVersion": 1,
		"sourceService": "driver-dispatch-delivery-service", "occurredAt": time.Now().UTC(),
		"aggregateType": "route_alert", "aggregateId": "opaque-alert", "aggregateVersion": 1,
		"payload": commerceanalytics.DispatchRouteMetric{RouteVersion: 2, PolicyVersion: "dispatch_route_integrity_v1", State: "OPEN", AlertType: "STALE_LOCATION", Severity: "HIGH"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestDispatchRouteWireNormalizesWithoutOperationalIdentity(t *testing.T) {
	e, err := normalizeEnvelope(dispatchRouteWire(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := commerceanalytics.ValidateEnvelope(e); err != nil {
		t.Fatal(err)
	}
	if e.SourceID != e.EventID || e.SourceAggregate != "dispatch_route_metric" || e.BusinessID != "" || e.VendorID != "" || e.BranchID != "" {
		t.Fatalf("unsafe analytics correlation: %+v", e)
	}
}

func TestDispatchRouteSignedByDifferentAllowedServiceIsRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := []byte("test-only-vendor-secret")
	h := NewHandler(nil, serviceauth.NewVerifier(serviceauth.Secrets{"vendor-service": secret}, time.Minute), []string{"vendor-service", "driver-dispatch-delivery-service"})
	router := gin.New()
	RegisterInternalRoutes(router, h)
	body := dispatchRouteWire(t)
	req := httptest.NewRequest(http.MethodPost, "/internal/analytics/v1/commerce-events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if err := serviceauth.SignRequest(req, "vendor-service", secret, body, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("valid but wrong source signer: %d %s", w.Code, w.Body.String())
	}
}
