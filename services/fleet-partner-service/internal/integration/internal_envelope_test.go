package integration_test

// FLEET-6h: every internal service route answers with the standard success
// envelope ({"data": ..., "meta": {"requestId": ...}}) or the standard error
// envelope ({"error": {"code", "message", "requestId", ...}}). Routes whose
// live callers decoded a bare top-level field keep that field as a deprecated
// alias; these tests pin both the envelope and every alias.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/providercompat"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	documenthttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/http"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement"
	financefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/finance"
	mkthttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/http"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	orghttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/http"
	providershttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/http"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	providerusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/noncestore"
)

const (
	envelopeCaller   = "driver-dispatch-delivery-service"
	envelopeIntruder = "some-other-service"
)

// envelopeRouter builds the real internal router (HMAC auth, nonce guard,
// error handler) with every FLEET-6h route mounted for envelopeCaller only.
func envelopeRouter(t *testing.T, pool *pgxpool.Pool) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	metrics := observability.NewMetrics()
	allowed := []string{envelopeCaller}
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	providers := providerusecases.NewService(providerusecases.Options{
		Pool: pool, Providers: providerrepos.NewRepository(pool), Metrics: metrics,
	})
	engagementService := engagement.NewService(engagement.Options{Pool: pool, Events: publisher,
		Flags: engagement.Flags{Support: true}})
	return app.BuildRouter(app.Options{
		Config:      config.Config{AppEnv: "test", MaxRequestBodyBytes: 1 << 20},
		Metrics:     metrics,
		TokenSigner: auth.NewTokenSigner([]byte("unused-envelope-user-token-secret")),
		Verifier: serviceauth.NewVerifier(serviceauth.Secrets{
			envelopeCaller:   []byte(workerOpsCallerSecret),
			envelopeIntruder: []byte(workerOpsCallerSecret),
		}, time.Minute),
		NonceStore: noncestore.New(pool),
		InternalRoutes: func(group *gin.RouterGroup) {
			mkthttp.RegisterEventIngestionRoutes(group, mkthttp.NewInternalHandler(newIngestionService(pool), allowed))
			providershttp.RegisterInternalRoutes(group, providershttp.NewInternalHandler(providers, allowed))
			documenthttp.RegisterInternalRoutes(group, documenthttp.NewInternalHandler(
				app.NewDocumentService(config.Config{}, pool, metrics), allowed))
			financefeature.RegisterInternalRoutes(group, financefeature.NewService(pool,
				financefeature.NewRepository(pool), nil, publisher), allowed)
			orghttp.RegisterVerificationEventRoutes(group, orghttp.NewVerificationEventHandler(nil, allowed))
			engagement.NewInternalHandler(engagementService).RegisterInternalRoutes(group)
		},
	})
}

func serveSigned(t *testing.T, router http.Handler, method, path, caller string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedInternalRequest(t, method, path, caller, body))
	return recorder
}

func decodeTopLevel(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v body=%s", err, recorder.Body.String())
	}
	return body
}

// assertNoLegacyTopLevel proves a route with no bare-decoding caller emits only
// the envelope members.
func assertNoLegacyTopLevel(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	for key := range decodeTopLevel(t, recorder) {
		if key != "data" && key != "meta" && key != "error" {
			t.Fatalf("unexpected top-level member %q: %s", key, recorder.Body.String())
		}
	}
}

func TestInternalEnvelope_MarketplaceEventIngestionKeepsAliases(t *testing.T) {
	pool := testPool(t)
	router := envelopeRouter(t, pool)
	org := createOrganization(t, pool, "Envelope Events Org", "RC-6H-EVT-1")
	requestID := ids.New("req")
	seedProjection(t, pool, org.ID, requestID, mktmodels.ProjectionOffered)

	_, event := inboundEvent(t, ids.New("evt"), mktusecases.EventRequestReserved, requestID, 2,
		map[string]any{"status": "reserved"})
	event.SourceService = envelopeCaller
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}

	for attempt, want := range []string{string(mktusecases.IngestAccepted), "DUPLICATE"} {
		recorder := serveSigned(t, router, http.MethodPost, "/internal/fleet-partner/v1/events", envelopeCaller, raw)
		if recorder.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d body=%s", attempt, recorder.Code, recorder.Body.String())
		}
		data := internalEnvelopeData(t, recorder)
		if data["status"] != want || data["eventId"] != event.EventID {
			t.Fatalf("attempt %d data = %v, want status %s", attempt, data, want)
		}
		top := decodeTopLevel(t, recorder)
		if top["status"] != want || top["eventId"] != event.EventID {
			t.Fatalf("attempt %d deprecated aliases missing: %s", attempt, recorder.Body.String())
		}
		if _, ok := top["code"]; !ok {
			t.Fatalf("attempt %d deprecated code alias missing: %s", attempt, recorder.Body.String())
		}
	}

	unknown, _ := json.Marshal(map[string]any{"eventId": ids.New("evt"), "eventType": "no.such.event.v1",
		"schemaVersion": 1, "aggregateId": "x", "payload": map[string]any{}})
	recorder := serveSigned(t, router, http.MethodPost, "/internal/fleet-partner/v1/events", envelopeCaller, unknown)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown event status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	assertInternalErrorEnvelope(t, recorder, "unknown_event_type")
	if top := decodeTopLevel(t, recorder); top["status"] != "REJECTED" || top["code"] != "unknown_event_type" {
		t.Fatalf("deprecated rejection aliases missing: %s", recorder.Body.String())
	}

	recorder = serveSigned(t, router, http.MethodPost, "/internal/fleet-partner/v1/events", envelopeCaller, []byte(`{"eventId":`))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("malformed event status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	assertInternalErrorEnvelope(t, recorder, "invalid_envelope")
}

func TestInternalEnvelope_MembershipSummaryKeepsAliasesAndProvidercompatReadsIt(t *testing.T) {
	pool := testPool(t)
	router := envelopeRouter(t, pool)
	providerID := ids.New("prv")
	path := "/internal/fleet-partner/v1/providers/" + providerID + "/services/" + envelopeCaller + "/membership-summary"

	recorder := serveSigned(t, router, http.MethodGet, path, envelopeCaller, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if data := internalEnvelopeData(t, recorder); data["providerMode"] != providercompat.ModeIndependent {
		t.Fatalf("data = %v", data)
	}
	if top := decodeTopLevel(t, recorder); top["providerMode"] != providercompat.ModeIndependent {
		t.Fatalf("deprecated providerMode alias missing: %s", recorder.Body.String())
	}

	// Cross-check: the shared client decodes the live response.
	server := httptest.NewServer(router)
	defer server.Close()
	client := providercompat.NewClient(server.URL, envelopeCaller, []byte(workerOpsCallerSecret), 5*time.Second)
	summary, err := client.ReadMembershipSummary(context.Background(), envelopeCaller, providerID)
	if err != nil || summary.ProviderMode != providercompat.ModeIndependent {
		t.Fatalf("providercompat read = %+v, %v", summary, err)
	}

	recorder = serveSigned(t, router, http.MethodGet, path, envelopeIntruder, nil)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("intruder status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	assertInternalErrorEnvelope(t, recorder, "caller_not_allowed")
}

func TestInternalEnvelope_StatusClassifiedIngestionRoutesUseStandardEnvelopes(t *testing.T) {
	pool := testPool(t)
	router := envelopeRouter(t, pool)

	cases := []struct {
		name, path, caller string
		body               []byte
		status             int
		code               string
	}{
		{"media events intruder", "/internal/fleet-partner/v1/documents/media-events", envelopeIntruder, []byte(`{}`), http.StatusForbidden, "caller_not_allowed"},
		{"media events malformed", "/internal/fleet-partner/v1/documents/media-events", envelopeCaller, []byte(`{}`), http.StatusUnprocessableEntity, "invalid_envelope"},
		{"wallet events intruder", "/internal/fleet-partner/v1/finance/events", envelopeIntruder, []byte(`{}`), http.StatusForbidden, "caller_not_allowed"},
		{"wallet events malformed", "/internal/fleet-partner/v1/finance/events", envelopeCaller, []byte(`{}`), http.StatusUnprocessableEntity, "invalid_envelope"},
		{"finance replay no organization", "/internal/fleet-partner/v1/finance/operations/op_1/replay", envelopeCaller, []byte(`{}`), http.StatusBadRequest, "organization_required"},
		{"verification events intruder", "/internal/fleet-partner/v1/organization-verification/events", envelopeIntruder, []byte(`{}`), http.StatusForbidden, "caller_not_allowed"},
		{"verification events malformed", "/internal/fleet-partner/v1/organization-verification/events", envelopeCaller, []byte(`{}`), http.StatusBadRequest, "invalid_envelope"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := serveSigned(t, router, http.MethodPost, tc.path, tc.caller, tc.body)
			if recorder.Code != tc.status {
				t.Fatalf("status = %d body=%s, want %d", recorder.Code, recorder.Body.String(), tc.status)
			}
			assertInternalErrorEnvelope(t, recorder, tc.code)
			assertNoLegacyTopLevel(t, recorder)
		})
	}

	// An unsupported verification event type is acknowledged in the envelope.
	ignored, _ := json.Marshal(map[string]any{"eventId": ids.New("evt"), "eventType": "organization.other.v1",
		"organizationId": "org_x", "payload": map[string]any{"status": "APPROVED", "sourceVersion": 1}})
	recorder := serveSigned(t, router, http.MethodPost, "/internal/fleet-partner/v1/organization-verification/events", envelopeCaller, ignored)
	if recorder.Code != http.StatusOK {
		t.Fatalf("ignored verification status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if data := internalEnvelopeData(t, recorder); data["status"] != "IGNORED" || data["code"] != "unsupported_event_type" {
		t.Fatalf("ignored verification data = %v", data)
	}
	assertNoLegacyTopLevel(t, recorder)

	// The support event ingress answers 202 inside the envelope.
	// The projection is seeded first so the event takes the update path the
	// Section 22/23 suite already exercises.
	org := createOrganization(t, pool, "Envelope Support Org", "RC-6H-SUP-1")
	caseID := ids.New("case")
	if _, err := pool.Exec(context.Background(), `INSERT INTO fleet_partner_support_case_projections
		(id,organization_id,support_service_case_id,case_type,status,priority,source_version,last_synced_at,gap_status)
		VALUES($1,$2,$3,'PLATFORM','open','normal',1,now(),'CURRENT')`, ids.New("scp"), org.ID, caseID); err != nil {
		t.Fatalf("seed support projection: %v", err)
	}
	support, _ := json.Marshal(map[string]any{"eventId": ids.New("evt"), "caseId": caseID,
		"eventType": "status_changed", "status": "under_review", "priority": "high", "sourceVersion": 2})
	req := signedInternalRequest(t, http.MethodPost, "/internal/fleet-partner/v1/support/events", envelopeCaller, support)
	req.Header.Set("X-Organization-ID", org.ID)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("support event status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if data := internalEnvelopeData(t, recorder); data["accepted"] != true {
		t.Fatalf("support event data = %v", data)
	}
}
