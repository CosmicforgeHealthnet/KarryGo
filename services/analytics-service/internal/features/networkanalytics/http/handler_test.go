package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/networkanalytics"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type nonceSet map[string]bool

func (n nonceSet) Consume(caller, nonce string, _ time.Time) bool {
	key := caller + ":" + nonce
	if nonce == "" || n[key] {
		return false
	}
	n[key] = true
	return true
}

var secrets = serviceauth.Secrets{
	"driver-hauling-service": []byte("h"), "vendor-service": []byte("v"),
	"admin-backoffice-service": []byte("a"), "support-dispute-service": []byte("s"),
}

func router() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	// The service is never reached by these tests: every request is refused
	// at the boundary under test, which is the point.
	h := NewHandler(Options{
		Verifier: serviceauth.NewVerifier(secrets, 5*time.Minute), Nonces: nonceSet{},
		IngestCallers: []string{"driver-hauling-service", "vendor-service"},
		ReadCallers:   []string{"driver-hauling-service", "vendor-service"},
	})
	RegisterInternalRoutes(engine, h)
	RegisterAdminRoutes(engine.Group("/api/v1/analytics"), h)
	return engine
}

func signed(t *testing.T, method, path, caller string, body []byte, nonce string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if nonce != "" {
		request.Header.Set(serviceauth.HeaderNonce, nonce)
	}
	if caller != "" {
		if err := serviceauth.SignRequest(request, caller, secrets[caller], body, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	return request
}

func status(engine *gin.Engine, request *http.Request) int {
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder.Code
}

func TestUnsignedRequestsAreRefused(t *testing.T) {
	engine := router()
	for _, path := range []string{networkanalytics.BranchReadinessPath + "b1",
		networkanalytics.ProviderQualityPath + "HAULING/p1", "/api/v1/analytics/network/recovery"} {
		if code := status(engine, signed(t, http.MethodGet, path, "", nil, "")); code != http.StatusUnauthorized {
			t.Errorf("%s unsigned -> %d", path, code)
		}
	}
	if code := status(engine, signed(t, http.MethodPost, networkanalytics.IngestPath, "", []byte(`{}`), "")); code != http.StatusUnauthorized {
		t.Errorf("unsigned ingest -> %d", code)
	}
}

func TestOnlyListedServicesMayIngest(t *testing.T) {
	code := status(router(), signed(t, http.MethodPost, networkanalytics.IngestPath, "support-dispute-service", []byte(`{}`), ""))
	if code != http.StatusForbidden {
		t.Fatalf("an unlisted producer -> %d, want 403", code)
	}
}

func TestASignedRequestCannotBeReplayed(t *testing.T) {
	engine := router()
	// A malformed body is fine: the second send must be refused before the
	// body is ever looked at.
	first := signed(t, http.MethodPost, networkanalytics.IngestPath, "vendor-service", []byte(`not json`), "fixed-nonce")
	status(engine, first)
	again := signed(t, http.MethodPost, networkanalytics.IngestPath, "vendor-service", []byte(`not json`), "fixed-nonce")
	if code := status(engine, again); code != http.StatusUnauthorized {
		t.Fatalf("replayed request -> %d, want 401", code)
	}
}

func TestAnExecutionServiceReadsOnlyItsOwnProviders(t *testing.T) {
	code := status(router(), signed(t, http.MethodGet,
		networkanalytics.ProviderQualityPath+"DISPATCH/rider-1", "driver-hauling-service", nil, ""))
	if code != http.StatusForbidden {
		t.Fatalf("hauling reading dispatch riders -> %d, want 403", code)
	}
}

func TestDashboardsAreForTheBackofficeOnly(t *testing.T) {
	engine := router()
	for _, caller := range []string{"vendor-service", "driver-hauling-service"} {
		code := status(engine, signed(t, http.MethodGet, "/api/v1/analytics/network/mission-success", caller, nil, ""))
		if code != http.StatusForbidden {
			t.Errorf("%s reading a dashboard -> %d, want 403", caller, code)
		}
	}
	code := status(engine, signed(t, http.MethodGet, "/api/v1/analytics/network/recovery?authority=VENDOR",
		"admin-backoffice-service", nil, ""))
	if code != http.StatusBadRequest {
		t.Fatalf("an unknown authority -> %d, want 400", code)
	}
}

func TestBothEnvelopeDialectsNormalize(t *testing.T) {
	canonical, _ := json.Marshal(map[string]any{"event_id": "e1", "event_name": "hauling.recovery.started",
		"schema_version": 1, "source_service": "driver-hauling-service", "source_entity_type": "recovery_case",
		"source_entity_id": "c1", "source_version": 2, "occurred_at": time.Now(), "payload": map[string]any{}})
	envelope, err := normalize(canonical)
	if err != nil || envelope.EventName != "hauling.recovery.started" || envelope.SourceVersion != 2 {
		t.Fatalf("canonical: %+v err=%v", envelope, err)
	}
	camel, _ := json.Marshal(map[string]any{"eventId": "e2", "eventType": networkanalytics.EventBranchReadiness,
		"schemaVersion": 1, "producer": "vendor-service", "aggregateType": "vendor_order", "aggregateId": "o1",
		"aggregateVersion": 4, "occurredAt": time.Now(), "payload": map[string]any{"branch_id": "b"}})
	envelope, err = normalize(camel)
	if err != nil || envelope.SourceService != "vendor-service" || envelope.SourceEntityID != "o1" ||
		envelope.SourceVersion != 4 || envelope.Validate() != nil {
		t.Fatalf("camelCase: %+v err=%v", envelope, err)
	}
	dispatch, _ := json.Marshal(map[string]any{"eventId": "e3", "eventType": "trip.completed.v1",
		"schemaVersion": 1, "sourceService": "driver-dispatch-delivery-service", "aggregateType": "trip",
		"aggregateId": "t1", "aggregateVersion": 1, "occurredAt": time.Now(), "payload": map[string]any{}})
	if envelope, err = normalize(dispatch); err != nil || envelope.SourceService != "driver-dispatch-delivery-service" {
		t.Fatalf("dispatch: %+v err=%v", envelope, err)
	}
}
