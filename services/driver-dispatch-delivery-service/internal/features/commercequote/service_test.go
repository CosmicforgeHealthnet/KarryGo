package commercequote

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
	"cosmicforge/logistics/shared/go/commercequote"
)

type fakeStore struct {
	stored map[string]Record
	fail   bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{stored: map[string]Record{}}
}

// Upsert mirrors the real repository's contract: an identical fingerprint
// returns the stored row untouched, a changed one supersedes it and bumps the
// revision.
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

type fakeLive struct {
	providers int
	err       error
}

func (f fakeLive) GetNearby(
	_ context.Context,
	_ availability.NearbyProvidersRequest,
) ([]availability.NearbyProvider, error) {
	if f.err != nil {
		return nil, f.err
	}

	out := make([]availability.NearbyProvider, f.providers)

	for i := range out {
		out[i] = availability.NearbyProvider{ProviderID: "rider"}
	}

	return out, nil
}

func newService(store Store, live LiveAvailability) *Service {
	return New(Options{Store: store, Live: live, RadiusKM: 10})
}

func baseRequest() commercequote.Request {
	return commercequote.Request{
		QuoteReference: "vq_1",
		VehicleClass:   commercequote.VehicleMotorcycle,
		Pickup:         commercequote.Point{Latitude: 6.4550, Longitude: 3.4100},
		Dropoff:        commercequote.Point{Latitude: 6.4300, Longitude: 3.4500},
		Load:           commercequote.Load{PackageCount: 2, TotalWeightGrams: 3000},
		Currency:       "NGN",
	}
}

func TestQuote_ServiceableWhenRidersAreOnline(t *testing.T) {
	service := newService(newFakeStore(), fakeLive{providers: 3})

	quote, err := service.Quote(context.Background(), "vendor-service", baseRequest())
	if err != nil {
		t.Fatal(err)
	}

	if !quote.Serviceable || quote.ReasonCode != commercequote.ReasonServiceable {
		t.Fatalf("unexpected quote: %+v", quote)
	}

	if quote.FeeMinor <= 0 || quote.Currency != "NGN" {
		t.Fatalf("expected a real fee in NGN, got %+v", quote)
	}

	if quote.QuoteID == "" || quote.PickupRef == "" || quote.DropoffRef == "" {
		t.Fatalf("a serviceable quote must carry its authority handles: %+v", quote)
	}

	if quote.AvailableProviders != 3 {
		t.Fatalf("available providers = %d, want 3", quote.AvailableProviders)
	}

	if quote.ExpiresAt.IsZero() {
		t.Fatal("a quote must expire")
	}
}

// The invariant: a refusal never carries a fee.
func TestQuote_RefusalsCarryNoFee(t *testing.T) {
	cases := map[string]struct {
		mutate func(*commercequote.Request)
		live   fakeLive
		want   string
	}{
		"no riders online": {
			mutate: func(*commercequote.Request) {},
			live:   fakeLive{providers: 0},
			want:   commercequote.ReasonNoProviderAvailable,
		},
		"van asked of dispatch": {
			mutate: func(r *commercequote.Request) {
				r.VehicleClass = commercequote.VehicleVan
			},
			live: fakeLive{providers: 5},
			want: commercequote.ReasonVehicleClassUnsupported,
		},
		"cold chain": {
			mutate: func(r *commercequote.Request) {
				r.Load.Capabilities = []string{commercequote.CapabilityColdChain}
			},
			live: fakeLive{providers: 5},
			want: commercequote.ReasonHandlingUnsupported,
		},
		"too heavy": {
			mutate: func(r *commercequote.Request) {
				r.Load.TotalWeightGrams = 90000
			},
			live: fakeLive{providers: 5},
			want: commercequote.ReasonLoadExceedsCapacity,
		},
		"large load": {
			mutate: func(r *commercequote.Request) { r.Load.LargeLoad = true },
			live:   fakeLive{providers: 5},
			want:   commercequote.ReasonLoadExceedsCapacity,
		},
		"foreign currency": {
			mutate: func(r *commercequote.Request) { r.Currency = "USD" },
			live:   fakeLive{providers: 5},
			want:   commercequote.ReasonCurrencyUnsupported,
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			request := baseRequest()
			testCase.mutate(&request)

			quote, err := newService(newFakeStore(), testCase.live).
				Quote(context.Background(), "vendor-service", request)
			if err != nil {
				t.Fatal(err)
			}

			if quote.Serviceable {
				t.Fatal("expected a refusal")
			}

			if quote.ReasonCode != testCase.want {
				t.Fatalf("reason = %q, want %q", quote.ReasonCode, testCase.want)
			}

			if quote.FeeMinor != 0 || quote.Currency != "" {
				t.Fatalf("refusal carried a fee: %+v", quote)
			}
		})
	}
}

// An insulated need is what a rider's box is for; refusing it would push every
// hot meal into a van.
func TestQuote_InsulatedAndHotAreServiceable(t *testing.T) {
	for _, capability := range []string{
		commercequote.CapabilityHotHold,
		commercequote.CapabilityInsulated,
	} {
		request := baseRequest()
		request.Load.Capabilities = []string{capability}

		quote, err := newService(newFakeStore(), fakeLive{providers: 2}).
			Quote(context.Background(), "vendor-service", request)
		if err != nil {
			t.Fatal(err)
		}

		if !quote.Serviceable {
			t.Fatalf("%s should be serviceable, got %q",
				capability, quote.ReasonCode)
		}
	}
}

// An availability fault must not be reported as "no riders": one is a fact
// about the world, the other is a fact about this service.
func TestQuote_AvailabilityFaultIsAFaultNotARefusal(t *testing.T) {
	_, err := newService(newFakeStore(), fakeLive{err: errors.New("redis down")}).
		Quote(context.Background(), "vendor-service", baseRequest())

	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

func TestQuote_StoreFailureIsUnavailable(t *testing.T) {
	store := newFakeStore()
	store.fail = true

	if _, err := newService(store, fakeLive{providers: 2}).
		Quote(context.Background(), "vendor-service", baseRequest()); !errors.Is(
		err, ErrUnavailable,
	) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

// Idempotency: an identical retry returns the same quote, a changed load
// supersedes it with a new revision.
func TestQuote_IdempotencyAndRevisions(t *testing.T) {
	store := newFakeStore()
	service := newService(store, fakeLive{providers: 2})

	first, err := service.Quote(context.Background(), "vendor-service", baseRequest())
	if err != nil {
		t.Fatal(err)
	}

	repeat, err := service.Quote(context.Background(), "vendor-service", baseRequest())
	if err != nil {
		t.Fatal(err)
	}

	if repeat.QuoteID != first.QuoteID || repeat.Revision != first.Revision {
		t.Fatalf("a retry must return the same quote: %+v vs %+v", first, repeat)
	}

	changed := baseRequest()
	changed.Load.TotalWeightGrams = 9000

	third, err := service.Quote(context.Background(), "vendor-service", changed)
	if err != nil {
		t.Fatal(err)
	}

	if third.Revision != first.Revision+1 {
		t.Fatalf("revision = %d, want %d", third.Revision, first.Revision+1)
	}

	if third.FeeMinor == first.FeeMinor {
		t.Fatal("a heavier load should have changed the fee")
	}
}

// Two callers using the same reference must not collide.
func TestQuote_ReferenceIsScopedToCaller(t *testing.T) {
	store := newFakeStore()
	service := newService(store, fakeLive{providers: 2})

	first, err := service.Quote(context.Background(), "vendor-service", baseRequest())
	if err != nil {
		t.Fatal(err)
	}

	other, err := service.Quote(context.Background(), "other-service", baseRequest())
	if err != nil {
		t.Fatal(err)
	}

	if other.QuoteID == first.QuoteID {
		t.Fatal("two callers shared one quote row")
	}
}

func TestQuote_InvalidRequestRefused(t *testing.T) {
	request := baseRequest()
	request.Dropoff = commercequote.Point{}

	if _, err := newService(newFakeStore(), fakeLive{providers: 2}).
		Quote(context.Background(), "vendor-service", request); !errors.Is(
		err, ErrInvalidRequest,
	) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}

	if _, err := newService(newFakeStore(), fakeLive{providers: 2}).
		Quote(context.Background(), "", baseRequest()); !errors.Is(
		err, ErrInvalidRequest,
	) {
		t.Fatalf("an unnamed caller must be refused, got %v", err)
	}
}

// Urgency reaches the fare as the express tier, so an immediate group actually
// costs more rather than merely being labelled differently.
func TestQuote_ImmediateUrgencyPricesAsExpress(t *testing.T) {
	service := newService(newFakeStore(), fakeLive{providers: 2})

	standard, err := service.Quote(
		context.Background(), "vendor-service", baseRequest(),
	)
	if err != nil {
		t.Fatal(err)
	}

	urgentRequest := baseRequest()
	urgentRequest.QuoteReference = "vq_2"
	urgentRequest.Urgency = commercequote.UrgencyImmediate

	urgent, err := service.Quote(
		context.Background(), "vendor-service", urgentRequest,
	)
	if err != nil {
		t.Fatal(err)
	}

	if urgent.FeeMinor <= standard.FeeMinor {
		t.Fatalf("express fee %d should exceed standard %d",
			urgent.FeeMinor, standard.FeeMinor)
	}
}

func TestPackageSizeFor(t *testing.T) {
	cases := []struct {
		load commercequote.Load
		want string
	}{
		{commercequote.Load{TotalWeightGrams: 500}, "small"},
		{commercequote.Load{TotalWeightGrams: 5000}, "medium"},
		{commercequote.Load{TotalWeightGrams: 12000}, "large"},
		{commercequote.Load{LongestDimensionMm: 300}, "medium"},
		{commercequote.Load{TotalVolumeMl: 40000}, "large"},
	}

	for _, testCase := range cases {
		if got := packageSizeFor(testCase.load); got != testCase.want {
			t.Fatalf("%+v -> %q, want %q", testCase.load, got, testCase.want)
		}
	}
}
