package commercequote

import (
	"context"
	"errors"
	"testing"
	"time"

	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/commercequote"
)

type fakeStore struct {
	stored map[string]Record
	fail   bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{stored: map[string]Record{}}
}

func (f *fakeStore) Upsert(_ context.Context, record Record) (Record, error) {
	if f.fail {
		return Record{}, errors.New("store down")
	}

	key := record.CallerService + "|" + record.QuoteReference

	if existing, ok := f.stored[key]; ok {
		if existing.RequestFingerprint == record.RequestFingerprint {
			return existing, nil
		}

		record.Revision = existing.Revision + 1
	}

	record.CreatedAt = time.Now().UTC()
	f.stored[key] = record

	return record, nil
}

type fakeAvailability struct {
	providers []availabilityrepositories.ProviderStatus
	err       error
}

func (f fakeAvailability) GetOnlineProviders(
	_ context.Context,
) ([]availabilityrepositories.ProviderStatus, error) {
	return f.providers, f.err
}

type fakeTrucks struct {
	trucks map[string]TruckInfo
}

func (f fakeTrucks) GetTruck(_ context.Context, truckID string) (TruckInfo, error) {
	truck, ok := f.trucks[truckID]
	if !ok {
		return TruckInfo{}, errors.New("not found")
	}

	return truck, nil
}

// Pickup is at the Lagos reference point; this provider sits right on it.
func nearProvider(id, truckID string) availabilityrepositories.ProviderStatus {
	return availabilityrepositories.ProviderStatus{
		ProviderID: id, TruckID: truckID, Lat: 6.4550, Lng: 3.4100,
	}
}

func baseRequest() commercequote.Request {
	return commercequote.Request{
		QuoteReference: "vq_1",
		VehicleClass:   commercequote.VehicleVan,
		Pickup:         commercequote.Point{Latitude: 6.4550, Longitude: 3.4100},
		Dropoff:        commercequote.Point{Latitude: 6.4300, Longitude: 3.4500},
		Load: commercequote.Load{
			PackageCount: 4, TotalWeightGrams: 120000,
		},
		Currency: "NGN",
	}
}

func newService(
	store Store,
	availability AvailabilityReader,
	trucks TruckLookup,
) *Service {
	return New(Options{
		Store:        store,
		Availability: availability,
		Trucks:       trucks,
		MaxRadiusKm:  25,
	})
}

func TestQuote_ServiceableVan(t *testing.T) {
	service := newService(
		newFakeStore(),
		fakeAvailability{providers: []availabilityrepositories.ProviderStatus{
			nearProvider("p1", "t1"),
		}},
		fakeTrucks{trucks: map[string]TruckInfo{
			"t1": {TruckType: "van", CapacityKg: 1000, Status: "active"},
		}},
	)

	quote, err := service.Quote(context.Background(), "vendor-service", baseRequest())
	if err != nil {
		t.Fatal(err)
	}

	if !quote.Serviceable {
		t.Fatalf("expected serviceable, got %q", quote.ReasonCode)
	}

	if quote.VehicleClass != commercequote.VehicleVan {
		t.Fatalf("class = %q, want VAN", quote.VehicleClass)
	}

	if quote.FeeMinor <= 0 || quote.Currency != "NGN" {
		t.Fatalf("expected a real fee in NGN, got %+v", quote)
	}

	if quote.EstimatedMinutes <= 0 {
		t.Fatal("a hauling quote must carry a duration")
	}
}

func TestQuote_LoadingDurationExtendsETAWithoutChangingFare(t *testing.T) {
	service := newService(
		newFakeStore(),
		fakeAvailability{providers: []availabilityrepositories.ProviderStatus{
			nearProvider("p1", "t1"),
		}},
		fakeTrucks{trucks: map[string]TruckInfo{
			"t1": {TruckType: "van", CapacityKg: 1000, Status: "active"},
		}},
	)
	legacy, err := service.Quote(context.Background(), "vendor-service", baseRequest())
	if err != nil {
		t.Fatal(err)
	}
	request := baseRequest()
	request.QuoteReference = "vq_loading"
	request.Load.LoadingDurationSeconds = 90 * 60
	withLoading, err := service.Quote(context.Background(), "vendor-service", request)
	if err != nil {
		t.Fatal(err)
	}
	if withLoading.EstimatedMinutes <= legacy.EstimatedMinutes {
		t.Fatalf("loading duration did not extend ETA: legacy=%d loading=%d", legacy.EstimatedMinutes, withLoading.EstimatedMinutes)
	}
	if withLoading.FeeMinor != legacy.FeeMinor {
		t.Fatalf("caller loading duration changed price: legacy=%d loading=%d", legacy.FeeMinor, withLoading.FeeMinor)
	}
}

func TestQuote_ServiceableTruckMultiStopScheduledRoute(t *testing.T) {
	request := baseRequest()
	request.VehicleClass = commercequote.VehicleTruck
	request.PickupStops = []commercequote.PickupStop{{
		StopRef:  "stop_a",
		Location: commercequote.Point{Latitude: 6.4550, Longitude: 3.4100, Address: "Branch A"},
		ReadyWindow: commercequote.ReadyWindow{
			From: time.Unix(3600, 0).UTC(), To: time.Unix(7200, 0).UTC(), Timezone: "Africa/Lagos",
		},
	}, {
		StopRef:  "stop_b",
		Location: commercequote.Point{Latitude: 6.4600, Longitude: 3.4200, Address: "Branch B"},
	}}

	quote, err := newService(
		newFakeStore(),
		fakeAvailability{providers: []availabilityrepositories.ProviderStatus{
			nearProvider("p1", "t1"),
		}},
		fakeTrucks{trucks: map[string]TruckInfo{
			"t1": {TruckType: "flatbed", CapacityKg: 5000, Status: "active"},
		}},
	).Quote(context.Background(), "vendor-service", request)
	if err != nil {
		t.Fatal(err)
	}
	if !quote.Serviceable {
		t.Fatalf("truck multi-stop quote refused: %s", quote.ReasonCode)
	}
	if quote.VehicleClass != commercequote.VehicleTruck {
		t.Fatalf("class=%s", quote.VehicleClass)
	}
	if quote.DistanceKm <= 0 {
		t.Fatalf("distance missing")
	}
}

// A motorcycle asked of hauling belongs to dispatch, and saying so is what
// keeps the two services from both claiming the same class.
func TestQuote_MotorcycleIsUnsupportedHere(t *testing.T) {
	request := baseRequest()
	request.VehicleClass = commercequote.VehicleMotorcycle

	quote, err := newService(
		newFakeStore(),
		fakeAvailability{providers: []availabilityrepositories.ProviderStatus{
			nearProvider("p1", "t1"),
		}},
		fakeTrucks{},
	).Quote(context.Background(), "vendor-service", request)
	if err != nil {
		t.Fatal(err)
	}

	if quote.Serviceable ||
		quote.ReasonCode != commercequote.ReasonVehicleClassUnsupported {
		t.Fatalf("unexpected quote: %+v", quote)
	}

	if quote.FeeMinor != 0 {
		t.Fatalf("refusal carried a fee: %+v", quote)
	}
}

// A cooling chain is served by a refrigerated truck and nothing else. A fleet
// of ordinary vans must therefore refuse it rather than quote one.
func TestQuote_CoolingChainRequiresARefrigeratedTruck(t *testing.T) {
	request := baseRequest()
	request.Load.Capabilities = []string{commercequote.CapabilityFrozenChain}

	availabilityWithVanOnly := fakeAvailability{
		providers: []availabilityrepositories.ProviderStatus{
			nearProvider("p1", "t1"),
		},
	}

	plainVans := fakeTrucks{trucks: map[string]TruckInfo{
		"t1": {TruckType: "van", CapacityKg: 1000, Status: "active"},
	}}

	quote, err := newService(newFakeStore(), availabilityWithVanOnly, plainVans).
		Quote(context.Background(), "vendor-service", request)
	if err != nil {
		t.Fatal(err)
	}

	if quote.Serviceable {
		t.Fatal("a plain van must not be quoted for a frozen chain")
	}

	// The same request against a refrigerated fleet is serviceable.
	fridges := fakeTrucks{trucks: map[string]TruckInfo{
		"t1": {TruckType: "refrigerated", CapacityKg: 1000, Status: "active"},
	}}

	ok, err := newService(newFakeStore(), availabilityWithVanOnly, fridges).
		Quote(context.Background(), "vendor-service", request)
	if err != nil {
		t.Fatal(err)
	}

	if !ok.Serviceable {
		t.Fatalf("a refrigerated truck should serve a frozen chain, got %q",
			ok.ReasonCode)
	}
}

// Capacity is checked with the same rule the matcher uses, so a quote is never
// issued that the matcher would refuse to fill.
func TestQuote_CapacityMirrorsTheMatcher(t *testing.T) {
	request := baseRequest()
	request.Load.TotalWeightGrams = 900000 // 900 kg

	quote, err := newService(
		newFakeStore(),
		fakeAvailability{providers: []availabilityrepositories.ProviderStatus{
			nearProvider("p1", "t1"),
		}},
		fakeTrucks{trucks: map[string]TruckInfo{
			"t1": {TruckType: "van", CapacityKg: 500, Status: "active"},
		}},
	).Quote(context.Background(), "vendor-service", request)
	if err != nil {
		t.Fatal(err)
	}

	if quote.Serviceable {
		t.Fatal("a 900kg load must not be quoted onto a 500kg van")
	}

	// Somebody WAS in range, so the refusal must say capacity, not absence.
	if quote.ReasonCode != commercequote.ReasonLoadExceedsCapacity {
		t.Fatalf("reason = %q, want %q",
			quote.ReasonCode, commercequote.ReasonLoadExceedsCapacity)
	}
}

// Nobody in range is a different fact from nobody big enough, and the customer
// sees a different message for each.
func TestQuote_NobodyInRangeSaysSo(t *testing.T) {
	far := availabilityrepositories.ProviderStatus{
		ProviderID: "p1", TruckID: "t1", Lat: 9.0765, Lng: 7.3986, // Abuja
	}

	quote, err := newService(
		newFakeStore(),
		fakeAvailability{providers: []availabilityrepositories.ProviderStatus{far}},
		fakeTrucks{trucks: map[string]TruckInfo{
			"t1": {TruckType: "van", CapacityKg: 5000, Status: "active"},
		}},
	).Quote(context.Background(), "vendor-service", baseRequest())
	if err != nil {
		t.Fatal(err)
	}

	if quote.ReasonCode != commercequote.ReasonNoProviderAvailable {
		t.Fatalf("reason = %q, want %q",
			quote.ReasonCode, commercequote.ReasonNoProviderAvailable)
	}
}

// The matcher degrades to proximity-only when a truck cannot be resolved. A
// quote stricter than the matcher would refuse work the service can do.
func TestQuote_UnresolvableTruckDegradesLikeTheMatcher(t *testing.T) {
	quote, err := newService(
		newFakeStore(),
		fakeAvailability{providers: []availabilityrepositories.ProviderStatus{
			nearProvider("p1", ""),
		}},
		fakeTrucks{trucks: map[string]TruckInfo{}},
	).Quote(context.Background(), "vendor-service", baseRequest())
	if err != nil {
		t.Fatal(err)
	}

	if !quote.Serviceable {
		t.Fatalf("expected proximity-only fallback, got %q", quote.ReasonCode)
	}
}

func TestQuote_AvailabilityFaultIsAFaultNotARefusal(t *testing.T) {
	_, err := newService(
		newFakeStore(),
		fakeAvailability{err: errors.New("redis down")},
		fakeTrucks{},
	).Quote(context.Background(), "vendor-service", baseRequest())

	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

func TestQuote_IdempotencyAndRevisions(t *testing.T) {
	store := newFakeStore()

	service := newService(
		store,
		fakeAvailability{providers: []availabilityrepositories.ProviderStatus{
			nearProvider("p1", "t1"),
		}},
		fakeTrucks{trucks: map[string]TruckInfo{
			"t1": {TruckType: "van", CapacityKg: 5000, Status: "active"},
		}},
	)

	first, err := service.Quote(context.Background(), "vendor-service", baseRequest())
	if err != nil {
		t.Fatal(err)
	}

	repeat, err := service.Quote(context.Background(), "vendor-service", baseRequest())
	if err != nil {
		t.Fatal(err)
	}

	if repeat.QuoteID != first.QuoteID || repeat.Revision != first.Revision {
		t.Fatal("a retry must return the same quote")
	}

	changed := baseRequest()
	changed.Load.TotalWeightGrams = 600000

	third, err := service.Quote(context.Background(), "vendor-service", changed)
	if err != nil {
		t.Fatal(err)
	}

	if third.Revision != first.Revision+1 {
		t.Fatalf("revision = %d, want %d", third.Revision, first.Revision+1)
	}
}

func TestTruckTypesFor(t *testing.T) {
	if truckTypesFor(commercequote.VehicleMotorcycle) != nil {
		t.Fatal("hauling must not claim the motorcycle class")
	}

	van := truckTypesFor(commercequote.VehicleVan)
	if len(van) == 0 || van[0] != "van" {
		t.Fatalf("van types = %v", van)
	}

	// Refrigerated is never an ordinary option: a plain load must not be
	// quoted onto a fridge and charged for it.
	for _, class := range []commercequote.VehicleClass{
		commercequote.VehicleVan, commercequote.VehicleTruck,
	} {
		for _, truckType := range truckTypesFor(class) {
			if truckType == "refrigerated" {
				t.Fatalf("%s must not list refrigerated by default", class)
			}
		}
	}
}
