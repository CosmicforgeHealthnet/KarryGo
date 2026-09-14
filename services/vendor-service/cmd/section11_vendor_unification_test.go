package main

// Section 11 — Vendor/Commerce unification.
//
// These exercise the composition root's mission authority: the capability
// gate, the explicit authority routing, and the guarantee that what Vendor
// sends names a service and never a provider.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/logisticscapability"
)

type fixedCapabilityAdapter struct {
	capability logisticscapability.Capability
	err        error
}

func (f fixedCapabilityAdapter) CheckCapability(context.Context) (logisticscapability.Capability, error) {
	return f.capability, f.err
}

func (f fixedCapabilityAdapter) CheckServiceability(_ context.Context, requirement logisticscapability.Requirement) (logisticscapability.Serviceability, error) {
	if f.err != nil {
		return logisticscapability.Serviceability{}, f.err
	}
	return logisticscapability.Evaluate(f.capability, requirement), nil
}

func capabilityFor(authority logistics.Authority, maxPickups int64) logisticscapability.Capability {
	return logisticscapability.Capability{
		Authority: authority, Version: 1, Available: true,
		MultiPickupSupported: maxPickups > 1, MaxPickups: maxPickups,
		MaxDropoffs:           1,
		SecurePickupSupported: true, SecureDeliverySupported: true,
		CustodyEvidenceSupported: true,
		ObservedAt:               time.Unix(1_700_000_000, 0).UTC(),
	}
}

func missionRequestWithPickups(service orders.LogisticsService, pickups int) orders.MissionRequest {
	request := orders.MissionRequest{
		MissionRef: "pfm_s11", ParentOrderID: "po_s11", VendorOrderIDs: []string{"vo_s11"},
		CustomerID: "11111111-1111-1111-1111-111111111111",
		Service:    service, VehicleClass: "VAN", Currency: "NGN", AgreedFeeMinor: 5000,
		DeliveryAddress: orders.AddressSnapshot{SourceAddressRef: "addr", Address: "Dropoff", Latitude: 6.43, Longitude: 3.45},
		Receiver:        orders.ReceiverSnapshot{ReceiverName: "Ada", ReceiverPhone: "+2348011111111"},
		WeightGrams:     4000, PackageCount: 2,
		SecurePickupRequired: true, SecureDeliveryRequired: true,
		CorrelationID: "corr_s11", SourceVersion: 2,
		Execution: &orders.MissionExecution{PlanRef: "plan", PlanVersion: 2},
	}
	for i := 0; i < pickups; i++ {
		stopRef := fmt.Sprintf("stop_%d", i)
		request.Execution.Stops = append(request.Execution.Stops, orders.MissionStop{
			StopRef: stopRef, PhysicalStopRef: stopRef, UnitRef: "unit_" + stopRef,
			BusinessID: "biz", BranchID: "branch", Address: "Pickup",
			Latitude: 6.45, Longitude: 3.39,
		})
		request.Execution.Parcels = append(request.Execution.Parcels, orders.MissionParcel{
			ParcelRef: "parcel_" + stopRef, UnitRef: "unit_" + stopRef, VendorOrderID: "vo_s11",
			BusinessID: "biz", BranchID: "branch", ManifestVersion: 2,
			Manifest: []byte(`{"items":1}`), WeightGrams: 2000, PackageCount: 1,
		})
	}
	return request
}

// TestCapabilityGateRefusesAMissionTheAuthorityCannotExecute is VND-005 …
// VND-008 and VND-030 at the point they actually matter: before a vehicle is
// asked for.
func TestCapabilityGateRefusesAMissionTheAuthorityCannotExecute(t *testing.T) {
	authority := (&section17FMissionAuthority{}).withCapabilities(logistics.NewRegistry(logistics.RegistryOptions{
		Adapters: map[logistics.Authority]logistics.CapabilityAdapter{
			logistics.Taxi: fixedCapabilityAdapter{capability: capabilityFor(logistics.Taxi, 1)},
		},
		Enabled: map[logistics.Authority]bool{logistics.Taxi: true},
	}))

	result, err := authority.CreateMission(context.Background(), missionRequestWithPickups(orders.LogisticsTaxi, 3))
	if err != nil {
		t.Fatalf("a declared incapability is an answer, not a fault: %v", err)
	}
	if result.Status != orders.MissionRefused {
		t.Fatalf("status=%q, want REFUSED", result.Status)
	}
	if result.RefusalCode != logisticscapability.ReasonMultiPickupNotAllowed {
		t.Fatalf("refusal code=%q, want the authority's own reason", result.RefusalCode)
	}
	if result.ExternalRef != "" {
		t.Fatal("a refused mission must not carry an external reference: nothing was created")
	}
}

// TestCapabilityGateAllowsAMissionTheAuthorityDeclaresItCanExecute proves the
// gate is not simply refusing everything.
func TestCapabilityGateAllowsAMissionTheAuthorityDeclaresItCanExecute(t *testing.T) {
	authority := (&section17FMissionAuthority{}).withCapabilities(logistics.NewRegistry(logistics.RegistryOptions{
		Adapters: map[logistics.Authority]logistics.CapabilityAdapter{
			logistics.Hauling: fixedCapabilityAdapter{capability: capabilityFor(logistics.Hauling, 10)},
		},
		Enabled: map[logistics.Authority]bool{logistics.Hauling: true},
	}))

	// The gate passes, so creation proceeds and fails for the real reason:
	// no hauling client is configured in this fixture.
	_, err := authority.CreateMission(context.Background(), missionRequestWithPickups(orders.LogisticsHauling, 3))
	if err == nil {
		t.Fatal("expected the unconfigured client to refuse creation")
	}
	if logistics.Classify(logistics.Hauling, err).Category != logistics.FailureDependencyUnavailable {
		t.Fatalf("expected to reach mission creation and fail on configuration, got %v", err)
	}
}

// TestUnreachableCapabilityServiceDoesNotRefuseTheMission pins the deliberate
// asymmetry: a capability outage must not fail a paid customer order.
func TestUnreachableCapabilityServiceDoesNotRefuseTheMission(t *testing.T) {
	authority := (&section17FMissionAuthority{}).withCapabilities(logistics.NewRegistry(logistics.RegistryOptions{
		Adapters: map[logistics.Authority]logistics.CapabilityAdapter{
			logistics.Hauling: fixedCapabilityAdapter{err: logisticscapability.ErrAuthorityUnavailable},
		},
		Enabled: map[logistics.Authority]bool{logistics.Hauling: true},
	}))

	result, err := authority.CreateMission(context.Background(), missionRequestWithPickups(orders.LogisticsHauling, 1))
	if result.Status == orders.MissionRefused {
		t.Fatal("an unreachable capability endpoint is not evidence the authority cannot carry the load")
	}
	if err == nil {
		t.Fatal("creation should still have been attempted and failed on the unconfigured client")
	}
}

// TestAnUnknownAuthorityIsRefusedRatherThanFallingThroughToHauling is VND-004.
//
// The routing switch used to end in a default branch, so any unrecognised
// value silently became a hauling mission. An unknown authority must refuse.
func TestAnUnknownAuthorityIsRefusedRatherThanFallingThroughToHauling(t *testing.T) {
	authority := &section17FMissionAuthority{}
	_, err := authority.CreateMission(context.Background(), missionRequestWithPickups(orders.LogisticsService("UBER"), 1))
	if err == nil {
		t.Fatal("an unrecognised authority must refuse")
	}
	failure := logistics.Classify(logistics.Authority("UBER"), err)
	if failure.ReasonCode != logistics.ReasonUnknownAuthority {
		t.Fatalf("reason=%q, want %q", failure.ReasonCode, logistics.ReasonUnknownAuthority)
	}
}

// TestTaxiGoodsSpellingRoutesToTheTaxiAuthority proves the canonical
// vocabulary is actually used for routing, not just for validation.
func TestTaxiGoodsSpellingRoutesToTheTaxiAuthority(t *testing.T) {
	var seenPath, seenService string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		var sent commercemission.Request
		_ = json.NewDecoder(r.Body).Decode(&sent)
		seenService = string(sent.Service)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{
			"mission_ref": sent.MissionRef, "operation_ref": sent.OperationReference(),
			"external_ref": "taxi_goods_mission_1", "status": "ACCEPTED",
			"accepted_fee_minor": sent.AgreedFeeMinor, "currency": sent.Currency, "version": 1,
		}})
	}))
	defer server.Close()

	authority := &section17FMissionAuthority{
		taxi: commercemission.New(commercemission.Options{
			BaseURL: server.URL, APIBase: commercemission.TaxiAPIBase,
			Service: string(orders.LogisticsTaxi), Secret: "s11-secret", Timeout: 5 * time.Second,
		}),
	}
	request := missionRequestWithPickups(orders.LogisticsService(logistics.TaxiGoodsAlias), 1)
	// Taxi Goods executes only against the quote it already froze, so the
	// mission carries that binding rather than asking Taxi to price again.
	request.Execution.AuthorityQuoteID = "taxi_quote_1"
	request.Execution.AuthorityQuoteReference = "taxi_quote_ref_1"
	_, err := authority.CreateMission(context.Background(), request)
	if err != nil {
		t.Fatalf("TAXI_GOODS must route to the Taxi client: %v", err)
	}
	if !strings.Contains(seenPath, commercemission.TaxiAPIBase) {
		t.Fatalf("mission went to %q, which is not the Taxi authority", seenPath)
	}
	if seenService != string(commercemission.ServiceTaxi) {
		t.Fatalf("the wire carried service %q; the shared contract only accepts the canonical TAXI", seenService)
	}
}

// TestTheMissionRequestNamesAServiceAndNeverAProvider is VND-003 at the
// serialized-payload level: the reflective test in the logistics package
// proves no field exists; this proves nothing provider-shaped is put on the
// wire by the adapter either.
func TestTheMissionRequestNamesAServiceAndNeverAProvider(t *testing.T) {
	payload, err := haulingMissionRequest(missionRequestWithPickups(orders.LogisticsHauling, 2))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	for _, banned := range logistics.ProviderSelectionFields() {
		for key := range wire {
			if strings.ReplaceAll(strings.ToLower(key), "_", "") == banned {
				t.Fatalf("the mission payload carries %q: Vendor names the execution service, never the provider (VND-003)", key)
			}
		}
	}
	if payload.Service != commercemission.ServiceHauling {
		t.Fatalf("service=%q, want HAULING", payload.Service)
	}
}

// TestExecutionIdentityIsStableAcrossRetriesAndDistinctAcrossReplans is
// VND-012 and Scenario U expressed on the request the adapters actually build.
func TestExecutionIdentityIsStableAcrossRetriesAndDistinctAcrossReplans(t *testing.T) {
	first := missionRequestWithPickups(orders.LogisticsHauling, 1)
	retry := missionRequestWithPickups(orders.LogisticsHauling, 1)
	if first.IdempotencyKey() != retry.IdempotencyKey() {
		t.Fatal("a retry must reuse the execution identity, or the authority creates a second mission")
	}

	replan := missionRequestWithPickups(orders.LogisticsHauling, 1)
	replan.SourceVersion = first.SourceVersion + 1
	if replan.IdempotencyKey() == first.IdempotencyKey() {
		t.Fatal("an explicit replan must not collide with the mission it replaces")
	}

	// And the conceptual model is derived, not duplicated.
	intent := first.Intent()
	if intent.Authority != logistics.Hauling || intent.CorrelationID != "corr_s11" {
		t.Fatalf("intent lost the authority or the correlation id: %+v", intent)
	}
	if !intent.SecurePickupRequired || !intent.SecureDeliveryRequired {
		t.Fatal("protected-handover requirements must survive into the conceptual mission")
	}
	if err := intent.Validate(); err != nil {
		t.Fatalf("a mission request built by the adapters must produce a valid intent: %v", err)
	}
}

// TestCorrelationFallsBackToTheParentOrder keeps VND-025 true for missions
// created before the correlation id was threaded through.
func TestCorrelationFallsBackToTheParentOrder(t *testing.T) {
	request := missionRequestWithPickups(orders.LogisticsHauling, 1)
	request.CorrelationID = ""
	if got := request.Intent().CorrelationID; got != request.ParentOrderID {
		t.Fatalf("correlation=%q, want the parent order %q", got, request.ParentOrderID)
	}
}
