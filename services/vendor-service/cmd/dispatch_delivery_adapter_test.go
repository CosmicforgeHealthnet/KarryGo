package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func dispatchAdapterRequest(now time.Time) orders.MissionRequest {
	return orders.MissionRequest{
		MissionRef: "vfm-operation-1", ParentOrderID: "parent-1",
		VendorOrderIDs: []string{"vendor-order-1"}, CustomerID: "11111111-1111-1111-1111-111111111111",
		Service: orders.LogisticsDispatch, Currency: "NGN", AgreedFeeMinor: 12_000,
		FundingReference: "approved-quote-1", PlanMissionRef: "slot-1",
		DeliveryAddress: orders.AddressSnapshot{SourceAddressRef: "address-1", Address: "Receiver", Latitude: 6.5, Longitude: 3.5},
		Receiver:        orders.ReceiverSnapshot{ReceiverName: "Ada", ReceiverPhone: "+2348011111111"},
		Execution: &orders.MissionExecution{
			PlanRef: "plan-1", PlanVersion: 2, PlanFingerprint: "plan-fingerprint",
			Mode: "BALANCED", ServiceClass: "FOOD", PromisedBy: now.Add(3 * time.Hour),
			Stops:   []orders.MissionStop{{StopRef: "stop-1", PhysicalStopRef: "market-1", UnitRef: "unit-1", BusinessID: "vendor-1", BranchID: "branch-1", Address: "Kitchen", Latitude: 6.4, Longitude: 3.4, ReadyFrom: now, ReadyTo: now.Add(time.Hour)}},
			Parcels: []orders.MissionParcel{{ParcelRef: "PKG-1", UnitRef: "unit-1", VendorOrderID: "vendor-order-1", BusinessID: "vendor-1", BranchID: "branch-1", Manifest: json.RawMessage(`{"lines":[]}`), ManifestVersion: 1, WeightGrams: 1000, PackageCount: 1}},
		},
	}
}

func TestDispatchAdapterRecoversUnknownCreateByStableOperation(t *testing.T) {
	now := time.Now().UTC()
	secret := "adapter-secret"
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{dispatchcommerce.SignatureCaller: []byte(secret)}, time.Minute)
	var quote dispatchcommerce.Quote
	createAttempts, readbacks := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if caller, err := verifier.VerifyRequest(r); err != nil || caller != dispatchcommerce.SignatureCaller {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == dispatchcommerce.APIBase+dispatchcommerce.CapabilityPath:
			_ = json.NewEncoder(w).Encode(dispatchcommerce.Envelope[dispatchcommerce.MotorcycleCapability]{Success: true, Data: dispatchcommerce.MotorcycleCapability{Class: "MOTORCYCLE", Version: 1, MaxWeightGrams: 25_000, MaxPackageCount: 40}})
		case r.Method == http.MethodPost && r.URL.Path == dispatchcommerce.APIBase+dispatchcommerce.QuotePath:
			var request dispatchcommerce.QuoteRequest
			_ = json.NewDecoder(r.Body).Decode(&request)
			fingerprint, _ := dispatchcommerce.Fingerprint(request)
			quote = dispatchcommerce.Quote{QuoteID: "quote-1", QuoteReference: request.QuoteReference, Revision: 1, RequestFingerprint: fingerprint, Serviceable: true, ReasonCode: "SERVICEABLE", FeeMinor: 12_000, Currency: "NGN", ExpiresAt: now.Add(time.Hour)}
			_ = json.NewEncoder(w).Encode(dispatchcommerce.Envelope[dispatchcommerce.Quote]{Success: true, Data: quote})
		case r.Method == http.MethodPost && r.URL.Path == dispatchcommerce.APIBase+dispatchcommerce.MissionPath:
			createAttempts++
			connection, _, _ := w.(http.Hijacker).Hijack()
			_ = connection.Close() // Dispatch committed, response was lost.
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/operations/"):
			readbacks++
			_ = json.NewEncoder(w).Encode(dispatchcommerce.Envelope[dispatchcommerce.Mission]{Success: true, Data: dispatchcommerce.Mission{MissionRef: "dcm-canonical-1", OperationRef: "vfm-operation-1", State: "SEARCHING", Version: 1}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	authority := newSection17FMissionAuthority(true, server.URL, secret, false, "", "", time.Second)
	result, err := authority.CreateMission(t.Context(), dispatchAdapterRequest(now))
	if err != nil {
		t.Fatal(err)
	}
	if result.ExternalRef != "dcm-canonical-1" || result.Status != orders.MissionAccepted || createAttempts != 1 || readbacks != 1 {
		t.Fatalf("unknown outcome was not recovered: result=%+v creates=%d reads=%d", result, createAttempts, readbacks)
	}
}

func TestDispatchAdapterRefusesRoutePriceChangeBeforeMissionCreation(t *testing.T) {
	now := time.Now().UTC()
	secret := "adapter-secret"
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{dispatchcommerce.SignatureCaller: []byte(secret)}, time.Minute)
	createAttempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := verifier.VerifyRequest(r); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(dispatchcommerce.Envelope[dispatchcommerce.MotorcycleCapability]{Success: true, Data: dispatchcommerce.MotorcycleCapability{Class: "MOTORCYCLE", Version: 1}})
			return
		}
		if r.URL.Path == dispatchcommerce.APIBase+dispatchcommerce.QuotePath {
			var request dispatchcommerce.QuoteRequest
			_ = json.NewDecoder(r.Body).Decode(&request)
			fingerprint, _ := dispatchcommerce.Fingerprint(request)
			_ = json.NewEncoder(w).Encode(dispatchcommerce.Envelope[dispatchcommerce.Quote]{Success: true, Data: dispatchcommerce.Quote{QuoteID: "quote-2", QuoteReference: request.QuoteReference, Revision: 1, RequestFingerprint: fingerprint, Serviceable: true, FeeMinor: 13_000, Currency: "NGN", ExpiresAt: now.Add(time.Hour)}})
			return
		}
		createAttempts++
	}))
	defer server.Close()

	authority := newSection17FMissionAuthority(true, server.URL, secret, false, "", "", time.Second)
	if _, err := authority.CreateMission(t.Context(), dispatchAdapterRequest(now)); err == nil {
		t.Fatal("route price change was accepted")
	}
	if createAttempts != 0 {
		t.Fatal("mission was created after the route quote changed customer terms")
	}
}

func TestDispatchAdapterRecoversUnknownCancelByCanonicalMission(t *testing.T) {
	secret := "adapter-secret"
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{dispatchcommerce.SignatureCaller: []byte(secret)}, time.Minute)
	cancelAttempts, missionReadbacks := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := verifier.VerifyRequest(r); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/operations/"):
			_ = json.NewEncoder(w).Encode(dispatchcommerce.Envelope[dispatchcommerce.Mission]{Success: true, Data: dispatchcommerce.Mission{MissionRef: "dcm-canonical-1", OperationRef: "vfm-operation-1", State: "ASSIGNED", Version: 7}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/dcm-canonical-1/cancel"):
			cancelAttempts++
			connection, _, _ := w.(http.Hijacker).Hijack()
			_ = connection.Close() // Dispatch committed cancellation; response was lost.
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/dcm-canonical-1"):
			missionReadbacks++
			_ = json.NewEncoder(w).Encode(dispatchcommerce.Envelope[dispatchcommerce.Mission]{Success: true, Data: dispatchcommerce.Mission{MissionRef: "dcm-canonical-1", OperationRef: "vfm-operation-1", State: "CANCELLED", Version: 8}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	authority := newSection17FMissionAuthority(true, server.URL, secret, false, "", "", time.Second)
	if err := authority.CancelMission(t.Context(), "vfm-operation-1", "CUSTOMER_CANCELLED"); err != nil {
		t.Fatal(err)
	}
	if cancelAttempts != 1 || missionReadbacks != 1 {
		t.Fatalf("unknown cancellation was not reconciled once: cancels=%d mission reads=%d", cancelAttempts, missionReadbacks)
	}
}
