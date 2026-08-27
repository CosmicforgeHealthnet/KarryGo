package commercequote

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
)

type multiStopStoreFake struct{ records map[string]MultiStopRecord }

func (s *multiStopStoreFake) UpsertMultiStop(_ context.Context, record MultiStopRecord) (MultiStopRecord, error) {
	if s.records == nil {
		s.records = map[string]MultiStopRecord{}
	}
	key := record.CallerService + ":" + record.QuoteReference + ":" + record.RequestFingerprint
	if existing, ok := s.records[key]; ok {
		return existing, nil
	}
	s.records[key] = record
	return record, nil
}
func (s *multiStopStoreFake) FindMultiStopByID(_ context.Context, caller, quoteID string) (MultiStopRecord, bool, error) {
	for _, record := range s.records {
		if record.CallerService == caller && record.ID == quoteID {
			return record, true, nil
		}
	}
	return MultiStopRecord{}, false, nil
}

type routePlannerFake struct {
	points []RoutePoint
	err    error
}

func (f *routePlannerFake) Plan(_ context.Context, points []RoutePoint, _ dispatchcommerce.Mode) (RoutePlan, error) {
	f.points = append([]RoutePoint(nil), points...)
	if f.err != nil {
		return RoutePlan{}, f.err
	}
	refs := make([]string, 0, len(points))
	for index := len(points) - 2; index >= 0; index-- {
		refs = append(refs, points[index].Ref)
	}
	refs = append(refs, "__receiver__")
	return RoutePlan{Provider: "route-authority", EvidenceVersion: "road-v7", DistanceMeters: 12_345, DurationSeconds: 1_800, OrderedRefs: refs}, nil
}

type availabilityFake struct{ count int }

func (f availabilityFake) GetNearby(_ context.Context, _ availability.NearbyProvidersRequest) ([]availability.NearbyProvider, error) {
	return make([]availability.NearbyProvider, f.count), nil
}

func validMultiStopQuote(now time.Time) dispatchcommerce.QuoteRequest {
	return dispatchcommerce.QuoteRequest{
		QuoteReference: "operation-route-v1", Mode: dispatchcommerce.ModeBalanced,
		ServiceClass: "FOOD", Plan: &dispatchcommerce.PlanBinding{PlanRef: "plan-1", PlanVersion: 2, PlanFingerprint: "fingerprint", MissionSlotRef: "slot-1"},
		PickupStops: []dispatchcommerce.PickupStop{
			{StopRef: "stop-a", PhysicalStopRef: "market", VendorRef: "vendor-a", BranchRef: "branch-a", ManifestRef: "manifest-a", ManifestVersion: 1, Location: dispatchcommerce.Point{Address: "A", Latitude: 6.45, Longitude: 3.41}, ReadyWindow: dispatchcommerce.ReadyWindow{From: now, To: now.Add(time.Hour)}, ParcelRefs: []string{"parcel-a"}},
			{StopRef: "stop-b", PhysicalStopRef: "market", VendorRef: "vendor-b", BranchRef: "branch-b", ManifestRef: "manifest-b", ManifestVersion: 1, Location: dispatchcommerce.Point{Address: "B", Latitude: 6.45, Longitude: 3.41}, ReadyWindow: dispatchcommerce.ReadyWindow{From: now, To: now.Add(time.Hour)}, ParcelRefs: []string{"parcel-b"}},
		},
		Receiver: dispatchcommerce.Point{Address: "Receiver", Latitude: 6.50, Longitude: 3.50},
		Parcels: []dispatchcommerce.Parcel{
			{ParcelRef: "parcel-a", VendorRef: "vendor-a", BranchRef: "branch-a", FulfilmentUnit: "unit-a", VendorOrderRef: "order-a", PickupStopRef: "stop-a", ManifestRef: "manifest-a-v1", ManifestVersion: 1, WeightGrams: 2_000, PackageCount: 1},
			{ParcelRef: "parcel-b", VendorRef: "vendor-b", BranchRef: "branch-b", FulfilmentUnit: "unit-b", VendorOrderRef: "order-b", PickupStopRef: "stop-b", ManifestRef: "manifest-b-v1", ManifestVersion: 1, WeightGrams: 2_000, PackageCount: 1},
		},
		Vehicle:    dispatchcommerce.VehicleRequirement{Class: "MOTORCYCLE", CapabilityVersion: 1},
		PromisedBy: now.Add(3 * time.Hour), Currency: "NGN",
	}
}

func TestMultiStopQuoteUsesRouteEvidenceAndGroupsColocatedLogicalStops(t *testing.T) {
	now := time.Now().UTC()
	routes, store := &routePlannerFake{}, &multiStopStoreFake{}
	service := NewMultiStop(MultiStopOptions{Store: store, Routes: routes, Live: availabilityFake{}, Now: func() time.Time { return now }})
	quote, err := service.Quote(context.Background(), dispatchcommerce.SignatureCaller, validMultiStopQuote(now))
	if err != nil {
		t.Fatal(err)
	}
	if !quote.Serviceable || quote.DistanceMeters != 12_345 || quote.RouteEvidenceVersion != "road-v7" {
		t.Fatalf("quote did not preserve route authority evidence: %+v", quote)
	}
	if len(routes.points) != 2 { // one physical pickup plus receiver
		t.Fatalf("co-located logical stops produced %d route points, want 2", len(routes.points))
	}
	if len(quote.StopSequence) != 3 || quote.StopSequence[2].StopRef != "__receiver__" {
		t.Fatalf("logical stop/receiver sequence not preserved: %+v", quote.StopSequence)
	}
	if quote.SupplyState != SupplyQueued {
		t.Fatalf("no current provider must remain serviceable and queued, got %q", quote.SupplyState)
	}
}

func TestMultiStopQuoteFailsClosedWhenRouteAuthorityIsUnavailable(t *testing.T) {
	now := time.Now().UTC()
	service := NewMultiStop(MultiStopOptions{Store: &multiStopStoreFake{}, Routes: &routePlannerFake{err: errors.New("down")}, Now: func() time.Time { return now }})
	if _, err := service.Quote(context.Background(), dispatchcommerce.SignatureCaller, validMultiStopQuote(now)); !errors.Is(err, ErrRouteServiceUnavailable) {
		t.Fatalf("error=%v, want route-service unavailable", err)
	}
}

func TestMultiStopQuoteRejectsUnsupportedHandlingAndOverflow(t *testing.T) {
	now := time.Now().UTC()
	for name, mutate := range map[string]func(*dispatchcommerce.QuoteRequest){
		"cold chain": func(r *dispatchcommerce.QuoteRequest) { r.Parcels[0].Handling.Chilled = true },
		"overflow": func(r *dispatchcommerce.QuoteRequest) {
			r.Parcels[0].WeightGrams = int64(^uint64(0) >> 1)
			r.Parcels[1].WeightGrams = 1
		},
	} {
		t.Run(name, func(t *testing.T) {
			request := validMultiStopQuote(now)
			mutate(&request)
			service := NewMultiStop(MultiStopOptions{Store: &multiStopStoreFake{}, Routes: &routePlannerFake{}, Now: func() time.Time { return now }})
			quote, err := service.Quote(context.Background(), dispatchcommerce.SignatureCaller, request)
			if err != nil {
				t.Fatal(err)
			}
			if quote.Serviceable || quote.ReasonCode == "" {
				t.Fatalf("unsafe load was serviceable: %+v", quote)
			}
		})
	}
}
