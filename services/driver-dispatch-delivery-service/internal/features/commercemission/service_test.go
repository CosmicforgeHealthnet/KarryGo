package commercemission

import (
	"context"
	"errors"
	"testing"
	"time"

	requestfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/request"
	"cosmicforge/logistics/shared/go/commercemission"
)

type fakeStore struct {
	existing map[string]Record
	created  []CreateInput
	refused  []RefusalInput
}

func newFakeStore() *fakeStore {
	return &fakeStore{existing: map[string]Record{}}
}

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

type fakeBroadcaster struct {
	events []requestfeature.BookingDispatchCreatedEvent
	fail   bool
}

func (b *fakeBroadcaster) StartBroadcast(
	_ context.Context, event requestfeature.BookingDispatchCreatedEvent,
) (requestfeature.RequestBroadcast, error) {
	if b.fail {
		return requestfeature.RequestBroadcast{}, errors.New("no riders")
	}
	b.events = append(b.events, event)
	return requestfeature.RequestBroadcast{}, nil
}

func missionRequest() commercemission.Request {
	return commercemission.Request{
		MissionRef:      "vfm_1",
		ParentOrderRef:  "cpo_1",
		VendorOrderRefs: []string{"cvo_a"},
		CustomerRef:     "11111111-1111-1111-1111-111111111111",
		Service:         commercemission.ServiceDispatch,
		VehicleClass:    commercemission.VehicleMotorcycle,
		Pickup: commercemission.Point{
			Address: "1 Market", Latitude: 6.455, Longitude: 3.41,
		},
		Dropoff: commercemission.Point{
			Address: "2 Customer", Latitude: 6.43, Longitude: 3.45,
			Contact: "Ada", Phone: "+2348011111111",
		},
		Load:           commercemission.Load{WeightGrams: 4000, PackageCount: 2},
		Currency:       "NGN",
		AgreedFeeMinor: 50000,
		RequestedAt:    time.Unix(0, 0).UTC(),
	}
}

func newService(store Store, broadcast Broadcaster) *Service {
	return New(Options{Store: store, Broadcast: broadcast, Now: func() time.Time {
		return time.Unix(0, 0).UTC()
	}})
}

// The whole point of this contract: dispatch executes for the fee the customer
// already paid and never re-prices.
func TestCreate_ExecutesForTheAgreedFeeWithoutRepricing(t *testing.T) {
	store, broadcast := newFakeStore(), &fakeBroadcaster{}
	service := newService(store, broadcast)

	mission, err := service.Create(context.Background(), missionRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if mission.Status != commercemission.StatusAccepted {
		t.Fatalf("status=%s", mission.Status)
	}
	if mission.AcceptedFeeMinor != 50000 {
		t.Fatalf("accepted fee=%d, want the agreed 50000", mission.AcceptedFeeMinor)
	}
	if mission.ExternalRef == "" {
		t.Fatal("an accepted mission names no booking")
	}

	// The booking that will actually be worked carries the agreed fee, not a
	// recomputed one.
	if len(broadcast.events) != 1 {
		t.Fatalf("broadcasts=%d, want 1", len(broadcast.events))
	}
	if broadcast.events[0].FareAmount != 50000 {
		t.Fatalf("broadcast fare=%d, want the agreed 50000", broadcast.events[0].FareAmount)
	}

	// And the shared verifier agrees the answer was not re-priced.
	if err = commercemission.VerifyMission(missionRequest(), mission); err != nil {
		t.Fatalf("VerifyMission: %v", err)
	}
}

// A retried request must return the same mission rather than dispatching a
// second rider for the same goods.
func TestCreate_IsIdempotentOnTheMissionReference(t *testing.T) {
	store, broadcast := newFakeStore(), &fakeBroadcaster{}
	service := newService(store, broadcast)

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
	if len(broadcast.events) != 1 {
		t.Fatalf("broadcasts=%d, want 1 - a retry dispatched a second rider",
			len(broadcast.events))
	}
}

// Dispatch is a motorcycle fleet. Work it cannot physically do is refused with
// a reason rather than accepted and failed later, when a customer is waiting.
func TestCreate_RefusesWorkItCannotServe(t *testing.T) {
	cases := map[string]func(*commercemission.Request){
		"VEHICLE_CLASS_NOT_SERVED": func(r *commercemission.Request) {
			r.VehicleClass = commercemission.VehicleTruck
		},
		"LOAD_EXCEEDS_RIDER_CAPACITY": func(r *commercemission.Request) {
			r.Load.WeightGrams = 500_000
		},
		"RECIPIENT_CONTACT_REQUIRED": func(r *commercemission.Request) {
			r.Dropoff.Phone = ""
		},
	}

	for wantCode, mutate := range cases {
		store, broadcast := newFakeStore(), &fakeBroadcaster{}
		service := newService(store, broadcast)

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
		if len(broadcast.events) != 0 {
			t.Errorf("%s: a refused mission still dispatched a rider", wantCode)
		}
		// A refusal is a real answer the caller can act on, and it verifies.
		if err = commercemission.VerifyMission(in, mission); err != nil {
			t.Errorf("%s: refusal did not verify: %v", wantCode, err)
		}
	}
}

// This authority executes its own work only.
func TestCreate_RefusesAnotherAuthoritysWork(t *testing.T) {
	service := newService(newFakeStore(), &fakeBroadcaster{})

	in := missionRequest()
	in.Service = commercemission.ServiceHauling
	if _, err := service.Create(context.Background(), in); err == nil {
		t.Fatal("dispatch accepted a hauling mission")
	}
}

// The mission is durable before matching starts, so a matching failure leaves
// something a retry can find rather than a rider chasing nothing.
func TestCreate_MissionIsDurableBeforeMatchingStarts(t *testing.T) {
	store, broadcast := newFakeStore(), &fakeBroadcaster{fail: true}
	service := newService(store, broadcast)

	if _, err := service.Create(context.Background(), missionRequest()); err == nil {
		t.Fatal("expected the matching failure to surface")
	}
	if len(store.created) != 1 {
		t.Fatal("the mission was not durable before matching was attempted")
	}

	// A retry finds it rather than creating a second.
	broadcast.fail = false
	if _, err := service.Create(context.Background(), missionRequest()); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if len(store.created) != 1 {
		t.Fatalf("missions created=%d after retry, want 1", len(store.created))
	}
}

func TestCreate_RefusesAnInvalidRequest(t *testing.T) {
	service := newService(newFakeStore(), &fakeBroadcaster{})

	in := missionRequest()
	in.MissionRef = ""
	if _, err := service.Create(context.Background(), in); err == nil {
		t.Fatal("a request with no mission reference was accepted")
	}
}
