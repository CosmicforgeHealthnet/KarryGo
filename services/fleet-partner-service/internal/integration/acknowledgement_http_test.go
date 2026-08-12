package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	mkthttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/http"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

const acknowledgeCallerSecret = "acknowledge-route-test-service-secret-value"

// ackRouter builds the real internal router around the acknowledge route.
//
// It mounts the production registrar rather than the handler directly, so the
// test covers what a deployment actually exposes: HMAC service auth, then the
// per-route caller allowlist, then the handler.
func ackRouter(t *testing.T, f *ackFixture, allowedCallers []string) http.Handler {
	t.Helper()
	handler := mkthttp.NewInternalHandler(f.service, allowedCallers)
	return app.BuildRouter(app.Options{
		Config:      config.Config{AppEnv: "test", MaxRequestBodyBytes: 1 << 20},
		Metrics:     observability.NewMetrics(),
		TokenSigner: auth.NewTokenSigner([]byte("unused-acknowledge-route-user-token-secret")),
		Verifier: serviceauth.NewVerifier(serviceauth.Secrets{
			"driver-dispatch-delivery-service": []byte(acknowledgeCallerSecret),
			"driver-taxi-service":              []byte(acknowledgeCallerSecret),
		}, time.Minute),
		InternalRoutes: func(group *gin.RouterGroup) {
			mkthttp.RegisterProviderResponseRoutes(group, handler)
		},
	})
}

// postAcknowledge issues a signed acknowledge call as callerService.
func postAcknowledge(t *testing.T, router http.Handler, callerService string,
	assignmentID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode body: %v", err)
	}
	path := "/internal/fleet-partner/v1/assignments/" + assignmentID + "/provider-acknowledge"
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	if callerService != "" {
		if err := serviceauth.SignRequest(req, callerService,
			[]byte(acknowledgeCallerSecret), encoded, time.Now()); err != nil {
			t.Fatalf("sign: %v", err)
		}
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

// TestAcknowledgeRouteAcceptsAllowedCaller proves the happy path end to end
// through the real router.
func TestAcknowledgeRouteAcceptsAllowedCaller(t *testing.T) {
	f := newAckFixture(t, "Ack Route OK", "RC-ACKR-1", false)
	router := ackRouter(t, f, []string{"driver-dispatch-delivery-service"})

	recorder := postAcknowledge(t, router, "driver-dispatch-delivery-service", f.assignment.ID,
		map[string]any{
			"providerId":        f.assignment.ExecutingProviderID,
			"providerService":   "DISPATCH",
			"providerSessionId": "sess-ack-route-1",
			"idempotencyKey":    "ack-route-key-1",
			"expectedVersion":   f.assignment.Version,
		})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Data mkthttp.ProviderAcknowledgementView `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if response.Data.AcknowledgementState != mktmodels.AcknowledgementAcknowledged {
		t.Fatalf("acknowledgementState = %q", response.Data.AcknowledgementState)
	}
	if response.Data.Status != mktmodels.AssignmentPendingProviderResponse {
		t.Fatalf("status = %q; the route must not move the assignment", response.Data.Status)
	}
	if len(response.Data.AllowedActions) != 2 {
		t.Fatalf("allowedActions = %v; both must survive acknowledgement",
			response.Data.AllowedActions)
	}
}

// TestAcknowledgeRouteRejectsUnauthenticatedCaller proves service auth guards
// the route.
func TestAcknowledgeRouteRejectsUnauthenticatedCaller(t *testing.T) {
	f := newAckFixture(t, "Ack Route Unauth", "RC-ACKR-2", false)
	router := ackRouter(t, f, []string{"driver-dispatch-delivery-service"})

	recorder := postAcknowledge(t, router, "", f.assignment.ID, map[string]any{
		"providerId":        f.assignment.ExecutingProviderID,
		"providerService":   "DISPATCH",
		"providerSessionId": "sess-ack-route-2",
		"idempotencyKey":    "ack-route-key-2",
	})
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body = %s", recorder.Code, recorder.Body.String())
	}
}

// TestAcknowledgeRouteRejectsCallerOutsideAllowlist proves authentication is
// not authorization: a correctly signed caller that is not on this route's
// allowlist is still refused.
func TestAcknowledgeRouteRejectsCallerOutsideAllowlist(t *testing.T) {
	f := newAckFixture(t, "Ack Route Allowlist", "RC-ACKR-3", false)
	router := ackRouter(t, f, []string{"driver-hauling-service"})

	recorder := postAcknowledge(t, router, "driver-dispatch-delivery-service", f.assignment.ID,
		map[string]any{
			"providerId":        f.assignment.ExecutingProviderID,
			"providerService":   "DISPATCH",
			"providerSessionId": "sess-ack-route-3",
			"idempotencyKey":    "ack-route-key-3",
		})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", recorder.Code, recorder.Body.String())
	}

	stored, _ := f.repo.GetByID(context.Background(), f.orgID, f.assignment.ID)
	if stored.Acknowledged() {
		t.Fatal("a refused caller must not have acknowledged anything")
	}
}

// TestAcknowledgeRouteRejectsCallerSpeakingForAnotherService proves a taxi
// caller cannot acknowledge on behalf of a dispatch provider.
func TestAcknowledgeRouteRejectsCallerSpeakingForAnotherService(t *testing.T) {
	f := newAckFixture(t, "Ack Route Cross", "RC-ACKR-4", false)
	router := ackRouter(t, f, []string{
		"driver-dispatch-delivery-service", "driver-taxi-service"})

	recorder := postAcknowledge(t, router, "driver-taxi-service", f.assignment.ID,
		map[string]any{
			// The caller is authenticated as taxi but asserts a dispatch
			// provider. The identity boundary must refuse it.
			"providerId":        f.assignment.ExecutingProviderID,
			"providerService":   "DISPATCH",
			"providerSessionId": "sess-ack-route-4",
			"idempotencyKey":    "ack-route-key-4",
		})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", recorder.Code, recorder.Body.String())
	}
}

// TestAcknowledgeRouteRequiresIdempotencyKey proves the route will not accept
// a command it cannot deduplicate.
func TestAcknowledgeRouteRequiresIdempotencyKey(t *testing.T) {
	f := newAckFixture(t, "Ack Route Key", "RC-ACKR-5", false)
	router := ackRouter(t, f, []string{"driver-dispatch-delivery-service"})

	recorder := postAcknowledge(t, router, "driver-dispatch-delivery-service", f.assignment.ID,
		map[string]any{
			"providerId":        f.assignment.ExecutingProviderID,
			"providerService":   "DISPATCH",
			"providerSessionId": "sess-ack-route-5",
		})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", recorder.Code, recorder.Body.String())
	}
}

// TestAcknowledgeRouteViewCarriesNoCommercialFields proves the DTO stays
// allowlisted.
func TestAcknowledgeRouteViewCarriesNoCommercialFields(t *testing.T) {
	f := newAckFixture(t, "Ack Route DTO", "RC-ACKR-6", false)
	router := ackRouter(t, f, []string{"driver-dispatch-delivery-service"})

	recorder := postAcknowledge(t, router, "driver-dispatch-delivery-service", f.assignment.ID,
		map[string]any{
			"providerId":        f.assignment.ExecutingProviderID,
			"providerService":   "DISPATCH",
			"providerSessionId": "sess-ack-route-6",
			"idempotencyKey":    "ack-route-key-6",
		})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, forbidden := range []string{
		"fareAmount", "fare_amount", "customerFare", "settlement", "commission",
		"receiverPhone", "receiver_phone", "customerPhone", "margin",
	} {
		if bytes.Contains([]byte(body), []byte(forbidden)) {
			t.Fatalf("acknowledgement view leaked %q: %s", forbidden, body)
		}
	}

	_ = mktrepos.NewAssignmentRepository(f.pool)
}
