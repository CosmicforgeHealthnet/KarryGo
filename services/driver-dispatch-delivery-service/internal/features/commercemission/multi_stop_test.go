package commercemission

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/commercequote"
	requestfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/request"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
)

type missionStoreFake struct {
	mission     dispatchcommerce.Mission
	fingerprint string
	caller      string
	creates     int
}

func (s *missionStoreFake) CreateMultiStop(_ context.Context, input createMultiStopInput) (dispatchcommerce.Mission, error) {
	s.creates++
	s.caller, s.fingerprint = input.Caller, input.Fingerprint
	s.mission = dispatchcommerce.Mission{MissionRef: "dcm-1", OperationRef: input.Request.OperationRef,
		BookingRef: "booking-1", State: "SEARCHING", QueueState: "SEARCHING", Version: 1,
		Plan: input.Request.Plan, Quote: input.Request.Quote, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	return s.mission, nil
}
func (s *missionStoreFake) FindMultiStopByOperation(_ context.Context, caller, operation string) (dispatchcommerce.Mission, string, bool, error) {
	if s.creates > 0 && caller == s.caller && operation == s.mission.OperationRef {
		return s.mission, s.fingerprint, true, nil
	}
	return dispatchcommerce.Mission{}, "", false, nil
}
func (s *missionStoreFake) FindMultiStopByCanonical(_ context.Context, caller, canonical string) (dispatchcommerce.Mission, string, bool, error) {
	if s.creates > 0 && caller == s.caller && canonical == s.mission.MissionRef {
		return s.mission, s.fingerprint, true, nil
	}
	return dispatchcommerce.Mission{}, "", false, nil
}

type quoteReaderFake struct{ record commercequote.MultiStopRecord }

func (f quoteReaderFake) FindMultiStopByID(_ context.Context, caller, quoteID string) (commercequote.MultiStopRecord, bool, error) {
	return f.record, caller == f.record.CallerService && quoteID == f.record.ID, nil
}

type recoverBroadcastFake struct{ starts, recovers int }

func (f *recoverBroadcastFake) StartBroadcast(_ context.Context, _ requestfeature.BookingDispatchCreatedEvent) (requestfeature.RequestBroadcast, error) {
	f.starts++
	return requestfeature.RequestBroadcast{}, nil
}
func (f *recoverBroadcastFake) RecoverBroadcast(_ context.Context, _ requestfeature.BookingDispatchCreatedEvent) (requestfeature.RequestBroadcast, error) {
	f.recovers++
	return requestfeature.RequestBroadcast{}, nil
}

func missionV1Fixture(now time.Time) (dispatchcommerce.QuoteRequest, dispatchcommerce.MissionCreateRequest, commercequote.MultiStopRecord) {
	plan := dispatchcommerce.PlanBinding{PlanRef: "plan-1", PlanVersion: 4, PlanFingerprint: "immutable-plan", MissionSlotRef: "slot-1"}
	quoteRequest := dispatchcommerce.QuoteRequest{
		QuoteReference: "quote-ref", Mode: dispatchcommerce.ModeBalanced, ServiceClass: "FOOD", Plan: &plan,
		PickupStops: []dispatchcommerce.PickupStop{{StopRef: "stop-1", VendorRef: "vendor-1", BranchRef: "branch-1", ManifestRef: "manifest-1", ManifestVersion: 1, Location: dispatchcommerce.Point{Address: "Kitchen", Latitude: 6.4, Longitude: 3.4}, ReadyWindow: dispatchcommerce.ReadyWindow{From: now, To: now.Add(time.Hour)}, ParcelRefs: []string{"PKG-1"}}},
		Receiver:    dispatchcommerce.Point{Address: "Customer", Latitude: 6.5, Longitude: 3.5},
		Parcels:     []dispatchcommerce.Parcel{{ParcelRef: "PKG-1", VendorRef: "vendor-1", BranchRef: "branch-1", FulfilmentUnit: "unit-1", VendorOrderRef: "order-1", PickupStopRef: "stop-1", ManifestRef: "manifest-1-v1", ManifestVersion: 1, WeightGrams: 1000, PackageCount: 1}},
		Vehicle:     dispatchcommerce.VehicleRequirement{Class: "MOTORCYCLE", CapabilityVersion: 1}, PromisedBy: now.Add(2 * time.Hour), Currency: "NGN",
	}
	fingerprint, _ := dispatchcommerce.Fingerprint(quoteRequest)
	snapshot, _ := json.Marshal(quoteRequest)
	create := dispatchcommerce.MissionCreateRequest{
		OperationRef: "operation-1", ParentOrderRef: "parent-1", CustomerRef: "11111111-1111-1111-1111-111111111111",
		Plan: plan, Quote: dispatchcommerce.QuoteBinding{QuoteID: "quote-1", QuoteReference: quoteRequest.QuoteReference, QuoteRevision: 1, QuoteFingerprint: fingerprint},
		Mode: quoteRequest.Mode, ServiceClass: quoteRequest.ServiceClass, PickupStops: quoteRequest.PickupStops,
		Receiver: quoteRequest.Receiver, Parcels: quoteRequest.Parcels, Vehicle: quoteRequest.Vehicle,
		PromisedBy: quoteRequest.PromisedBy, FundingReference: "funding-1",
	}
	record := commercequote.MultiStopRecord{ID: "quote-1", CallerService: dispatchcommerce.SignatureCaller,
		QuoteReference: quoteRequest.QuoteReference, Revision: 1, RequestFingerprint: fingerprint,
		RequestSnapshot: snapshot, Serviceable: true, FeeMinor: 12_000, Currency: "NGN", ExpiresAt: now.Add(time.Hour)}
	return quoteRequest, create, record
}

func TestMultiStopMissionBindsImmutablePlanQuoteRouteAndParcels(t *testing.T) {
	now := time.Now().UTC()
	_, request, quote := missionV1Fixture(now)
	store, broadcast := &missionStoreFake{}, &recoverBroadcastFake{}
	service := NewMultiStopMission(MultiStopMissionOptions{Store: store, Quotes: quoteReaderFake{quote}, Broadcast: broadcast, Now: func() time.Time { return now }})
	first, err := service.Create(context.Background(), dispatchcommerce.SignatureCaller, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(context.Background(), dispatchcommerce.SignatureCaller, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.MissionRef != second.MissionRef || store.creates != 1 || broadcast.starts != 1 || broadcast.recovers != 1 {
		t.Fatalf("retry created duplicate outcome: first=%+v second=%+v creates=%d starts=%d recovers=%d", first, second, store.creates, broadcast.starts, broadcast.recovers)
	}
}

func TestMultiStopMissionRejectsQuotePlanAndManifestMismatch(t *testing.T) {
	now := time.Now().UTC()
	_, base, quote := missionV1Fixture(now)
	for name, mutate := range map[string]func(*dispatchcommerce.MissionCreateRequest){
		"foreign plan":   func(r *dispatchcommerce.MissionCreateRequest) { r.Plan.PlanFingerprint = "foreign" },
		"changed route":  func(r *dispatchcommerce.MissionCreateRequest) { r.Receiver.Latitude += 0.1 },
		"changed parcel": func(r *dispatchcommerce.MissionCreateRequest) { r.Parcels[0].ManifestRef = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			request := base
			request.PickupStops = append([]dispatchcommerce.PickupStop(nil), base.PickupStops...)
			request.Parcels = append([]dispatchcommerce.Parcel(nil), base.Parcels...)
			mutate(&request)
			service := NewMultiStopMission(MultiStopMissionOptions{Store: &missionStoreFake{}, Quotes: quoteReaderFake{quote}, Broadcast: &recoverBroadcastFake{}, Now: func() time.Time { return now }})
			if _, err := service.Create(context.Background(), dispatchcommerce.SignatureCaller, request); err == nil {
				t.Fatal("mismatched mission was accepted")
			}
		})
	}
}

func TestMultiStopMissionReadbackIsCallerScoped(t *testing.T) {
	now := time.Now().UTC()
	_, request, quote := missionV1Fixture(now)
	store := &missionStoreFake{}
	service := NewMultiStopMission(MultiStopMissionOptions{Store: store, Quotes: quoteReaderFake{quote}, Broadcast: &recoverBroadcastFake{}, Now: func() time.Time { return now }})
	if _, err := service.Create(context.Background(), dispatchcommerce.SignatureCaller, request); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReadOperation(context.Background(), "other-service", request.OperationRef); err == nil {
		t.Fatal("foreign caller read Vendor's operation")
	}
	if _, err := service.Create(context.Background(), "other-service", request); err == nil {
		t.Fatal("foreign caller was not rejected")
	}
}
