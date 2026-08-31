package main

import (
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
				TemperatureControl: "FROZEN",
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
	if !payload.Handling.Frozen {
		t.Fatalf("handling not merged: %+v", payload.Handling)
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
