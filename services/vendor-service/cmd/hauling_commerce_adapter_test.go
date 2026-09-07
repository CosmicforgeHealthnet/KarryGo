package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/shared/go/commercemission"
)

func TestHaulingMissionRequestCarriesExecutionEvidence(t *testing.T) {
	request := orders.MissionRequest{
		MissionRef:       "pfm_haul_1",
		ParentOrderID:    "po_1",
		VendorOrderIDs:   []string{"vo_1"},
		CustomerID:       "11111111-1111-1111-1111-111111111111",
		Service:          orders.LogisticsHauling,
		VehicleClass:     "TRUCK",
		Currency:         "NGN",
		AgreedFeeMinor:   12345,
		FundingReference: "fund_1",
		PlanMissionRef:   "slot_1",
		DeliveryAddress: orders.AddressSnapshot{
			SourceAddressRef: "addr_1", Address: "Dropoff", Latitude: 6.43, Longitude: 3.45,
		},
		Receiver:     orders.ReceiverSnapshot{ReceiverName: "Ada", ReceiverPhone: "+2348011111111"},
		WeightGrams:  120000,
		PackageCount: 3,
		Execution: &orders.MissionExecution{
			PlanRef: "plan_1", PlanVersion: 9, PlanFingerprint: "fp_1",
			LoadingDurationSeconds: 45 * 60,
			Stops: []orders.MissionStop{{
				StopRef: "stop_1", PhysicalStopRef: "phys_1", UnitRef: "unit_1",
				BusinessID: "biz_1", BranchID: "branch_1", Address: "Pickup",
				Latitude: 6.455, Longitude: 3.41,
				ReadyFrom: time.Unix(3600, 0).UTC(), ReadyTo: time.Unix(7200, 0).UTC(),
			}},
			Parcels: []orders.MissionParcel{{
				ParcelRef: "parcel_1", UnitRef: "unit_1", VendorOrderID: "vo_1",
				BusinessID: "biz_1", BranchID: "branch_1", ManifestVersion: 9,
				Manifest: []byte(`{"items":1}`), WeightGrams: 120000, PackageCount: 3,
				TemperatureControl: "FROZEN", Upright: true, StackRestricted: true,
			}},
		},
	}

	payload, err := haulingMissionRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if payload.OperationReference() != request.MissionRef {
		t.Fatalf("operation ref=%q", payload.OperationReference())
	}
	if payload.Plan == nil || payload.Plan.PlanRef != "plan_1" || payload.Plan.MissionSlotRef != "slot_1" {
		t.Fatalf("plan binding missing: %+v", payload.Plan)
	}
	if len(payload.PickupStops) != 1 || payload.PickupStops[0].ReadyWindow.Timezone != "Africa/Lagos" {
		t.Fatalf("pickup stop not preserved: %+v", payload.PickupStops)
	}
	if len(payload.Parcels) != 1 || payload.Parcels[0].PickupStopRef != "stop_1" ||
		payload.Parcels[0].ManifestVersion != 9 {
		t.Fatalf("parcel evidence not preserved: %+v", payload.Parcels)
	}
	if !payload.Handling.Frozen || !payload.Handling.Upright || !payload.Handling.StackRestricted {
		t.Fatalf("handling not merged: %+v", payload.Handling)
	}
	if payload.Load.LoadingDurationSeconds != 45*60 {
		t.Fatalf("loading duration=%d, want %d", payload.Load.LoadingDurationSeconds, 45*60)
	}
	if reason, ok := payload.Validate(); !ok {
		t.Fatalf("shared contract rejected payload: %s", reason)
	}
}

func TestHaulingMissionResultMapsRefusal(t *testing.T) {
	result := haulingMissionResult(commercemission.Mission{
		MissionRef: "m_1", Status: commercemission.StatusRefused, RefusalCode: "NO_TRUCK",
	})
	if result.Status != orders.MissionRefused || result.RefusalCode != "NO_TRUCK" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestTaxiGoodsMissionRequiresAndCarriesFrozenAuthorityQuote(t *testing.T) {
	request := orders.MissionRequest{
		MissionRef: "taxi-mission-1", ParentOrderID: "parent-1",
		VendorOrderIDs: []string{"vendor-order-1"}, CustomerID: "customer-1",
		Service: orders.LogisticsTaxi, VehicleClass: "CAR", Currency: "NGN", AgreedFeeMinor: 4500,
		PlanMissionRef:  "quoted-mission-1",
		DeliveryAddress: orders.AddressSnapshot{Address: "Dropoff", Latitude: 6.50, Longitude: 3.45},
		Execution: &orders.MissionExecution{
			PlanRef: "plan-1", PlanVersion: 1, PlanFingerprint: "plan-fingerprint",
			AuthorityQuoteID: "taxi-quote-1", AuthorityQuoteReference: "taxi-quote-reference-1",
			Stops:   []orders.MissionStop{{StopRef: "stop-1", UnitRef: "unit-1", Address: "Pickup", Latitude: 6.45, Longitude: 3.40}},
			Parcels: []orders.MissionParcel{{ParcelRef: "parcel-1", UnitRef: "unit-1", VendorOrderID: "vendor-order-1", ManifestVersion: 1, Manifest: []byte(`{"items":1}`), WeightGrams: 1000, PackageCount: 1}},
		},
	}
	payload, err := haulingMissionRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Service != commercemission.ServiceTaxi || payload.VehicleClass != commercemission.VehicleCar || payload.QuoteID != "taxi-quote-1" || payload.QuoteReference != "taxi-quote-reference-1" {
		t.Fatalf("Taxi Goods authority binding lost: %+v", payload)
	}

	request.Execution.AuthorityQuoteID = ""
	if _, err = haulingMissionRequest(request); err == nil {
		t.Fatal("Taxi Goods mission without frozen quote identity was accepted")
	}
}

func TestHaulingMissionUnknownCreateReadsBackStableOperationWithoutDuplicate(t *testing.T) {
	posts, reads := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == commercemission.HaulingAPIBase+commercemission.Path:
			posts++
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(commercemission.Envelope{Success: false})
		case r.Method == http.MethodGet && r.URL.Path == commercemission.HaulingAPIBase+commercemission.OperationPath+"/mission-unknown-1":
			reads++
			_ = json.NewEncoder(w).Encode(commercemission.Envelope{Success: true, Data: commercemission.Mission{
				MissionRef: "mission-unknown-1", OperationRef: "mission-unknown-1",
				ExternalRef: "booking-1", Status: commercemission.StatusAccepted,
				AcceptedFeeMinor: 12345, Version: 1,
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	authority := &section17FMissionAuthority{hauling: commercemission.New(commercemission.Options{
		BaseURL: server.URL, APIBase: commercemission.HaulingAPIBase,
		Service: string(orders.LogisticsHauling), Secret: "test-secret",
	})}
	result, err := authority.createHaulingMission(context.Background(), orders.MissionRequest{
		MissionRef: "mission-unknown-1", ParentOrderID: "po-1",
		VendorOrderIDs: []string{"vo-1"}, CustomerID: "11111111-1111-1111-1111-111111111111",
		Service: orders.LogisticsHauling, VehicleClass: "TRUCK",
		Currency: "NGN", AgreedFeeMinor: 12345, WeightGrams: 1000, PackageCount: 1,
		PlanMissionRef:  "slot-1",
		DeliveryAddress: orders.AddressSnapshot{Address: "Dropoff", Latitude: 6.43, Longitude: 3.45},
		Receiver:        orders.ReceiverSnapshot{ReceiverName: "Ada", ReceiverPhone: "+2348011111111"},
		Execution: &orders.MissionExecution{
			PlanRef: "plan-1", PlanVersion: 1, PlanFingerprint: "fingerprint-1",
			Stops:   []orders.MissionStop{{StopRef: "stop-1", UnitRef: "unit-1", Address: "Pickup", Latitude: 6.455, Longitude: 3.41}},
			Parcels: []orders.MissionParcel{{ParcelRef: "parcel-1", UnitRef: "unit-1", VendorOrderID: "vo-1", WeightGrams: 1000, PackageCount: 1}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExternalRef != "booking-1" || posts != 1 || reads != 1 {
		t.Fatalf("result=%+v posts=%d reads=%d", result, posts, reads)
	}
}
