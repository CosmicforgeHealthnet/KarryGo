package platformclients

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestDispatchNativeExecutionSpeaksDispatchDialect pins the wire contract of
// driver-dispatch-delivery-service's internal marketplace API: camelCase
// bodies, idempotency in headers, POST eligibility validation and
// reservation-id keyed release. The live-service proof is
// internal/integration TestAcceptanceToOneCanonicalTrip.
func TestDispatchNativeExecutionSpeaksDispatchDialect(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	bodies := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path)
		bodies[r.Method+" "+r.URL.Path] = body
		mu.Unlock()
		if r.Header.Get("X-Service-Signature") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		write := func(data any) { _ = json.NewEncoder(w).Encode(map[string]any{"data": data}) }
		switch r.Method + " " + r.URL.Path {
		case "POST " + dispatchInternalBase + "/reservations":
			if r.Header.Get("Idempotency-Key") != "rsv-key" {
				t.Errorf("reservation idempotency header = %q", r.Header.Get("Idempotency-Key"))
			}
			write(map[string]any{"reservationId": "res-1", "requestId": "req-1", "organizationId": "org-1",
				"status": "RESERVED", "requestVersion": 7, "reservationVersion": 1})
		case "GET " + dispatchInternalBase + "/requests/req-1/reservation":
			write(map[string]any{"reservationId": "res-1", "requestId": "req-1", "organizationId": "org-1",
				"status": "RESERVED", "requestVersion": 7, "reservationVersion": 3})
		case "POST " + dispatchInternalBase + "/reservations/res-1/release":
			write(map[string]any{"reservationId": "res-1", "status": "RELEASED"})
		case "POST " + dispatchInternalBase + "/eligibility/providers/validate":
			write(map[string]any{"eligible": true, "status": "CHECKED", "requestVersion": 7, "reasons": []string{}})
		case "POST " + dispatchInternalBase + "/eligibility/vehicles/validate":
			write(map[string]any{"eligible": false, "requestVersion": 7, "reasons": []string{"vehicle_not_eligible"}})
		case "GET " + dispatchInternalBase + "/requests/req-1":
			write(map[string]any{"requestId": "req-1", "bookingId": "bkg-1", "broadcastStatus": "broadcasting",
				"reservedOrganizationId": "org-1", "sourceVersion": 7, "expiresAt": time.Now().UTC().Format(time.RFC3339)})
		case "POST " + dispatchInternalBase + "/trips/trip-1/cancel":
			_, _ = w.Write([]byte(`{"success":true,"data":{"tripId":"trip-1","status":"cancelled","sourceVersion":4}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	native := NewDispatchExecution(New(Options{ServiceName: "fleet-partner-service",
		BaseURL: server.URL, Secret: []byte("secret"), Timeout: time.Second}))
	ctx := context.Background()
	meta := CallMeta{OrganizationID: "org-1"}

	outcome, err := native.ReserveForOrganization(ctx, meta, ReserveRequest{SourceRequestID: "req-1",
		ExpectedSourceVersion: 6, OrganizationID: "org-1", AcceptanceSource: "ORGANIZATION_DISPATCHER",
		IdempotencyKey: "rsv-key"})
	if err != nil || outcome.ReservationID != "res-1" || outcome.Status != ReservationReserved ||
		outcome.WinnerOrganizationID != "org-1" || outcome.SourceVersion != 7 {
		t.Fatalf("reserve = %+v, %v", outcome, err)
	}
	reserveBody := bodies["POST "+dispatchInternalBase+"/reservations"]
	if reserveBody["requestId"] != "req-1" || reserveBody["expectedRequestVersion"] != float64(6) ||
		reserveBody["organizationId"] != "org-1" || reserveBody["source_request_id"] != nil {
		t.Fatalf("reserve body = %v", reserveBody)
	}

	if err := native.ReleaseReservation(ctx, meta, "req-1", "org-1", "assignment_expired", "rel-key"); err != nil {
		t.Fatalf("release: %v", err)
	}
	releaseBody := bodies["POST "+dispatchInternalBase+"/reservations/res-1/release"]
	if releaseBody["reasonCode"] != "ASSIGNMENT_EXPIRED" || releaseBody["expectedReservationVersion"] != float64(3) {
		t.Fatalf("release body = %v", releaseBody)
	}
	if err := native.ReleaseReservation(ctx, meta, "req-1", "org-other", "x", "rel-2"); err == nil {
		t.Fatal("release of another organization's reservation was not refused")
	}

	eligibility, err := native.ValidateProviderEligibility(ctx, meta, "req-1", "prv-1", "veh-1")
	if err != nil || !eligibility.Eligible || eligibility.VehicleOK ||
		eligibility.ReasonCode != "vehicle_not_eligible" || eligibility.SourceVersion != 7 {
		t.Fatalf("eligibility = %+v, %v", eligibility, err)
	}
	if bodies["POST "+dispatchInternalBase+"/eligibility/providers/validate"]["organizationId"] != "org-1" {
		t.Fatalf("eligibility did not carry the asking organization")
	}

	view, err := native.ReadRequest(ctx, meta, "req-1")
	if err != nil || view.Status != "broadcasting" || view.WinnerOrganizationID != "org-1" || view.SourceVersion != 7 {
		t.Fatalf("read request = %+v, %v", view, err)
	}
	// FLEET-12.2: cancellation goes to Dispatch's organization trip-cancel
	// route and names the asking organization, which Dispatch checks.
	if err := native.CancelExecution(ctx, meta, "trip-1", "customer withdrew", "cancel-k"); err != nil {
		t.Fatalf("cancel = %v", err)
	}
	cancelBody := bodies["POST "+dispatchInternalBase+"/trips/trip-1/cancel"]
	if cancelBody["organizationId"] != "org-1" || cancelBody["idempotencyKey"] != "cancel-k" || cancelBody["reason"] != "customer withdrew" {
		t.Fatalf("cancel body = %v", cancelBody)
	}
	for _, path := range seen {
		if path == "GET "+dispatchInternalBase+"/eligibility" {
			t.Fatal("adapter called the non-existent GET /eligibility route")
		}
	}
}
