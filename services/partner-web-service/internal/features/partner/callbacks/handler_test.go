package callbacks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/billing"
	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
)

const testInternalKey = "test-internal-key"

// fakeDispatch records processed dispatch callbacks.
type fakeDispatch struct {
	matched bool
	err     error
	topics  []string
	raws    [][]byte
}

func (f *fakeDispatch) ProcessDispatchEventResult(_ context.Context, topic string, raw []byte) (bool, error) {
	f.topics = append(f.topics, topic)
	f.raws = append(f.raws, raw)
	return f.matched, f.err
}

// fakeBilling records charge-result callbacks.
type fakeBilling struct {
	outcome *billing.ChargeResultOutcome
	err     error
	inputs  []billing.ChargeResultInput
}

func (f *fakeBilling) HandleChargeResult(_ context.Context, in billing.ChargeResultInput) (*billing.ChargeResultOutcome, error) {
	f.inputs = append(f.inputs, in)
	if f.err != nil {
		return nil, f.err
	}
	return f.outcome, nil
}

func newCallbackRouter(dispatch DispatchProcessor, bill ChargeResultHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterCallbackRoutes(r, Dependencies{
		InternalServiceKey: testInternalKey,
		Dispatch:           dispatch,
		Billing:            bill,
	})
	return r
}

func post(r *gin.Engine, path, body string, withKey bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if withKey {
		req.Header.Set("X-Internal-Service-Key", testInternalKey)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestDispatchCallbacksMapToTopics(t *testing.T) {
	cases := []struct {
		path      string
		wantTopic string
	}{
		{"/api/v1/internal/callbacks/request-accepted", "request.accepted"},
		{"/api/v1/internal/callbacks/no-provider-found", "request.no_provider_found"},
		{"/api/v1/internal/callbacks/provider-arrived", "trip.provider_arrived"},
		{"/api/v1/internal/callbacks/trip-started", "trip.started"},
		{"/api/v1/internal/callbacks/trip-completed", "trip.completed"},
		{"/api/v1/internal/callbacks/trip-cancelled", "trip.cancelled"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			fd := &fakeDispatch{matched: true}
			r := newCallbackRouter(fd, nil)
			w := post(r, tc.path, `{"booking_id":"disp-1","occurred_at":"2026-07-05T10:00:00Z"}`, true)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
			}
			if len(fd.topics) != 1 || fd.topics[0] != tc.wantTopic {
				t.Fatalf("topics = %v, want [%s]", fd.topics, tc.wantTopic)
			}
			if !strings.Contains(w.Body.String(), `"processed":true`) {
				t.Fatalf("body = %s, want processed:true", w.Body.String())
			}
		})
	}
}

func TestDispatchCallbackTripStartedForwardsRiderFields(t *testing.T) {
	fd := &fakeDispatch{matched: true}
	r := newCallbackRouter(fd, nil)
	body := `{"event":"trip.started","booking_id":"disp-1","provider_id":"prov-1",
		"rider_name":"Taiwo Adeleke","rider_phone":"+2348033445566","rider_rating":4.85,
		"vehicle_color":"Red","vehicle_model":"Honda CB125","plate_number":"LAG-234-XY"}`
	w := post(r, "/api/v1/internal/callbacks/trip-started", body, true)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}
	// The raw payload (with rider fields) is passed through to the processor,
	// which stores the snapshot + enqueues webhooks.
	var forwarded map[string]any
	if err := json.Unmarshal(fd.raws[0], &forwarded); err != nil {
		t.Fatalf("unmarshal forwarded payload: %v", err)
	}
	if forwarded["rider_name"] != "Taiwo Adeleke" || forwarded["plate_number"] != "LAG-234-XY" {
		t.Fatalf("forwarded payload = %v", forwarded)
	}
}

func TestDispatchCallbackUnknownBookingSkipped(t *testing.T) {
	fd := &fakeDispatch{matched: false}
	r := newCallbackRouter(fd, nil)
	w := post(r, "/api/v1/internal/callbacks/trip-completed", `{"booking_id":"not-a-partner-booking"}`, true)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"skipped":true`) {
		t.Fatalf("body = %s, want skipped:true", w.Body.String())
	}
}

func TestDispatchCallbackMissingBookingIDReturns400(t *testing.T) {
	fd := &fakeDispatch{matched: true}
	r := newCallbackRouter(fd, nil)
	w := post(r, "/api/v1/internal/callbacks/request-accepted", `{"event":"request.accepted"}`, true)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
	if len(fd.topics) != 0 {
		t.Fatal("processor must not run without booking_id")
	}
}

func TestDispatchCallbackMalformedJSONReturns400(t *testing.T) {
	fd := &fakeDispatch{matched: true}
	r := newCallbackRouter(fd, nil)
	w := post(r, "/api/v1/internal/callbacks/trip-started", `{not json`, true)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
}

func TestCallbacksRejectInvalidInternalKey(t *testing.T) {
	fd := &fakeDispatch{matched: true}
	fb := &fakeBilling{outcome: &billing.ChargeResultOutcome{}}
	r := newCallbackRouter(fd, fb)

	for _, path := range []string{
		"/api/v1/internal/callbacks/request-accepted",
		"/api/v1/internal/callbacks/charge-result",
	} {
		w := post(r, path, `{"booking_id":"x","source_reference":"y","status":"success"}`, false)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d, want 401", path, w.Code)
		}
	}
	if len(fd.topics) != 0 || len(fb.inputs) != 0 {
		t.Fatal("handlers must not run without a valid internal key")
	}
}

func TestChargeResultCallbackDelegatesToBilling(t *testing.T) {
	fb := &fakeBilling{outcome: &billing.ChargeResultOutcome{InvoiceID: "inv-1", Status: "paid"}}
	r := newCallbackRouter(nil, fb)
	w := post(r, "/api/v1/internal/callbacks/charge-result",
		`{"source_reference":"inv-1","status":"success","reference":"paystack_ref","amount_kobo":500000}`, true)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}
	if len(fb.inputs) != 1 || fb.inputs[0].SourceReference != "inv-1" || fb.inputs[0].AmountKobo != 500000 {
		t.Fatalf("billing inputs = %+v", fb.inputs)
	}
}

func TestChargeResultCallbackRendersBillingErrors(t *testing.T) {
	fb := &fakeBilling{err: delivery.NewAPIError(http.StatusBadRequest, "amount_mismatch", "mismatch", nil)}
	r := newCallbackRouter(nil, fb)
	w := post(r, "/api/v1/internal/callbacks/charge-result",
		`{"source_reference":"inv-1","status":"success","amount_kobo":1}`, true)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "amount_mismatch") {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestDispatchCallbackUnavailableWhenWebhooksNotConfigured(t *testing.T) {
	r := newCallbackRouter(nil, nil)
	w := post(r, "/api/v1/internal/callbacks/trip-completed", `{"booking_id":"x"}`, true)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (body=%s)", w.Code, w.Body.String())
	}
}
