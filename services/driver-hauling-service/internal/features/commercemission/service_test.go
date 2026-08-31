package commercemission

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/commercemission"
)

type fakeStore struct {
	existing map[string]Record
	created  []CreateInput
	refused  []RefusalInput
}

func newFakeStore() *fakeStore { return &fakeStore{existing: map[string]Record{}} }

func (s *fakeStore) FindByMissionRef(_ context.Context, ref string) (Record, bool, error) {
	record, found := s.existing[ref]
	return record, found, nil
}

func (s *fakeStore) CreateMissionWithBooking(_ context.Context, in CreateInput) (Record, error) {
	s.created = append(s.created, in)
	record := Record{
		MissionRef:     in.Request.MissionRef,
		ParentOrderRef: in.Request.ParentOrderRef,
		CustomerID:     in.Request.CustomerRef,
		BookingID:      "booking_1",
		AgreedFeeMinor: in.Request.AgreedFeeMinor,
		Currency:       in.Request.Currency,
		Status:         "accepted",
		CreatedAt:      in.CreatedAt,
	}
	s.existing[record.MissionRef] = record
	return record, nil
}

func (s *fakeStore) MarkRefused(_ context.Context, in RefusalInput) (Record, error) {
	s.refused = append(s.refused, in)
	record := Record{
		MissionRef:     in.Request.MissionRef,
		AgreedFeeMinor: in.Request.AgreedFeeMinor,
		Status:         "refused",
		RefusalCode:    in.RefusalCode,
		CreatedAt:      in.CreatedAt,
	}
	s.existing[record.MissionRef] = record
	return record, nil
}

type fakeMatcher struct{ started []string }

func (m *fakeMatcher) StartCommerceMatching(
	_ context.Context, bookingID string, _, _ float64,
) {
	m.started = append(m.started, bookingID)
}

func missionRequest() commercemission.Request {
	return commercemission.Request{
		MissionRef:      "vfm_1",
		ParentOrderRef:  "cpo_1",
		VendorOrderRefs: []string{"cvo_a"},
		CustomerRef:     "11111111-1111-1111-1111-111111111111",
		Service:         commercemission.ServiceHauling,
		VehicleClass:    commercemission.VehicleTruck,
		Pickup: commercemission.Point{
			Address: "1 Depot", Latitude: 6.455, Longitude: 3.41,
		},
		Dropoff: commercemission.Point{
			Address: "2 Site", Latitude: 6.43, Longitude: 3.45,
			Contact: "Ada", Phone: "+2348011111111",
		},
		Load:           commercemission.Load{WeightGrams: 750_000, PackageCount: 20},
		Currency:       "NGN",
		AgreedFeeMinor: 900000,
		RequestedAt:    time.Unix(0, 0).UTC(),
	}
}

func newService(store Store, matcher Matcher) *Service {
	return New(Options{Store: store, Matcher: matcher, Now: func() time.Time {
		return time.Unix(0, 0).UTC()
	}})
}

// The whole point of this contract: hauling executes for the fee the customer
// already paid and never re-prices.
func TestCreate_ExecutesForTheAgreedFeeWithoutRepricing(t *testing.T) {
	store, matcher := newFakeStore(), &fakeMatcher{}
	service := newService(store, matcher)

	mission, err := service.Create(context.Background(), missionRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if mission.Status != commercemission.StatusAccepted {
		t.Fatalf("status=%s", mission.Status)
	}
	if mission.AcceptedFeeMinor != 900000 {
		t.Fatalf("accepted fee=%d, want the agreed 900000", mission.AcceptedFeeMinor)
	}
	if len(matcher.started) != 1 {
		t.Fatalf("matching starts=%d, want 1", len(matcher.started))
	}
	if err = commercemission.VerifyMission(missionRequest(), mission); err != nil {
		t.Fatalf("VerifyMission: %v", err)
	}
}

// A retried request must return the same mission rather than dispatching a
// second truck for the same goods.
func TestCreate_IsIdempotentOnTheMissionReference(t *testing.T) {
	store, matcher := newFakeStore(), &fakeMatcher{}
	service := newService(store, matcher)

	first, err := service.Create(context.Background(), missionRequest())
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(context.Background(), missionRequest())
	if err != nil {
		t.Fatal(err)
	}

	if first.ExternalRef != second.ExternalRef {
		t.Fatalf("retry produced a different booking: %s vs %s",
			first.ExternalRef, second.ExternalRef)
	}
	if len(store.created) != 1 {
		t.Fatalf("missions created=%d, want 1", len(store.created))
	}
	if len(matcher.started) != 1 {
		t.Fatalf("matching starts=%d, want 1 - a retry dispatched a second truck",
			len(matcher.started))
	}
}

// Hauling runs trucks and vans. Work it cannot serve is refused with a reason.
func TestCreate_RefusesWorkItCannotServe(t *testing.T) {
	cases := map[string]func(*commercemission.Request){
		"VEHICLE_CLASS_NOT_SERVED": func(r *commercemission.Request) {
			r.VehicleClass = commercemission.VehicleMotorcycle
		},
		"LOAD_WEIGHT_REQUIRED": func(r *commercemission.Request) {
			r.Load.WeightGrams = 0
		},
		"RECIPIENT_CONTACT_REQUIRED": func(r *commercemission.Request) {
			r.Dropoff.Phone = ""
		},
	}

	for wantCode, mutate := range cases {
		store, matcher := newFakeStore(), &fakeMatcher{}
		service := newService(store, matcher)

		in := missionRequest()
		mutate(&in)

		mission, err := service.Create(context.Background(), in)
		if err != nil {
			t.Fatalf("%s: %v", wantCode, err)
		}
		if mission.Status != commercemission.StatusRefused {
			t.Errorf("%s: status=%s, want REFUSED", wantCode, mission.Status)
		}
		if mission.RefusalCode != wantCode {
			t.Errorf("refusal code=%q, want %q", mission.RefusalCode, wantCode)
		}
		if len(matcher.started) != 0 {
			t.Errorf("%s: a refused mission still started matching", wantCode)
		}
		if err = commercemission.VerifyMission(in, mission); err != nil {
			t.Errorf("%s: refusal did not verify: %v", wantCode, err)
		}
	}
}

// This authority executes its own work only.
func TestCreate_RefusesAnotherAuthoritysWork(t *testing.T) {
	service := newService(newFakeStore(), &fakeMatcher{})

	in := missionRequest()
	in.Service = commercemission.ServiceDispatch
	if _, err := service.Create(context.Background(), in); err == nil {
		t.Fatal("hauling accepted a dispatch mission")
	}
}

// The load is converted for matching, never for pricing.
func TestCargoWeightKg_ConvertsForMatchingOnly(t *testing.T) {
	if got := cargoWeightKg(750_000); got != 750 {
		t.Errorf("cargoWeightKg(750000)=%d, want 750", got)
	}
	// A sub-kilogram haul still needs a matchable class rather than zero.
	if got := cargoWeightKg(400); got != 1 {
		t.Errorf("cargoWeightKg(400)=%d, want 1", got)
	}
}
