package privateexecution

// The acceptance gate, and what happens when a provider has not agreed.
//
// This is the safety property of Section 10's corporate path. Fleet decides
// which of an organization's drivers is offered work and whether that driver
// agreed; Taxi decides whether a booking exists. The gate between them is the
// only thing stopping a caller that can sign as Fleet from dispatching any
// driver, for any organization, by asserting an acceptance that never happened.
//
// So the tests below are mostly about refusal. A create that succeeds is one
// line; the interesting cases are every way the claim can fail to match what
// Fleet actually holds, and the requirement that NOTHING is written when it
// does not.

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/geo"
)

// fakeVerifier answers with whatever Fleet is pretended to hold.
type fakeVerifier struct {
	acceptance Acceptance
	err        error
	calls      int
}

func (f *fakeVerifier) Verify(context.Context, string) (Acceptance, error) {
	f.calls++
	return f.acceptance, f.err
}

// countingStore records whether anything was ever written.
//
// "Refused" and "refused without writing" are different claims, and only the
// second one is worth anything: a rejected create that still inserted a
// mapping row would leave Taxi holding a record of work nobody accepted.
type countingStore struct {
	creates int
	keys    map[string]record
}

func newCountingStore() *countingStore {
	return &countingStore{keys: map[string]record{}}
}

func (r *countingStore) GetKey(_ context.Context, key string) (record, error) {
	stored, ok := r.keys[key]
	if !ok {
		return record{}, ErrNotFound
	}
	return stored, nil
}

func (r *countingStore) GetOperation(context.Context, string) (record, error) {
	return record{}, ErrNotFound
}

func (r *countingStore) GetBooking(context.Context, string) (record, error) {
	return record{}, ErrNotFound
}

func (r *countingStore) Cancel(context.Context, string, string, string, string) (CancelView, error) {
	return CancelView{}, ErrNotFound
}

func (r *countingStore) Create(_ context.Context, in CreateRequest,
	digest string, _ Zone) (record, error) {
	r.creates++
	stored := record{
		RequestFingerprint:  digest,
		IdempotencyKey:      in.IdempotencyKey,
		OrganizationID:      in.OrganizationID,
		ExecutingProviderID: in.ExecutingProviderID,
		PrivateAssignmentID: in.PrivateAssignmentID,
		View: View{
			FleetOperationID:   in.FleetOperationID,
			Status:             "ACCEPTED",
			CanonicalBookingID: "bk_" + in.IdempotencyKey,
		},
	}
	r.keys[in.IdempotencyKey] = stored
	return stored, nil
}

// staticZones resolves every pickup into one operating zone.
type staticZones struct{ err error }

func (z staticZones) ResolveZone(context.Context, geo.Point) (Zone, error) {
	if z.err != nil {
		return Zone{}, z.err
	}
	return Zone{MarketID: "mkt-lagos", ID: "zone-mainland"}, nil
}

// acceptedFor builds the acceptance Fleet would hold for a matching request.
func acceptedFor(in CreateRequest) Acceptance {
	return Acceptance{
		AssignmentID:             in.PrivateAssignmentID,
		OrganizationID:           in.OrganizationID,
		ProviderID:               in.ExecutingProviderID,
		ProviderService:          ServiceCategoryTaxi,
		OrganizationMembershipID: in.OrganizationMembershipID,
		ProviderResponseID:       in.ProviderResponseID,
		ResponseStatus:           "ACCEPTED",
	}
}

func validRequest() CreateRequest {
	return CreateRequest{
		OrganizationID:           "org-1",
		SettlementOrganizationID: "org-1",
		ExecutingProviderID:      "prv-1",
		OrganizationMembershipID: "mem-1",
		PrivateOrderID:           "ord-1",
		PrivateAssignmentID:      "asg-1",
		ProviderResponseID:       "rsp-1",
		FleetOperationID:         "fop-1",
		ServiceCategory:          ServiceCategoryTaxi,
		VehicleID:                "veh-1",
		IdempotencyKey:           "idem-1",
		Route: RouteSnapshot{
			PickupAddress: "Ikeja", PickupLatitude: 6.43, PickupLongitude: 3.42,
			DropoffAddress: "Lekki", DropoffLatitude: 6.45, DropoffLongitude: 3.40,
		},
	}
}

func newTestService(verifier AcceptanceVerifier, repo store) *Service {
	return NewService(repo, verifier, staticZones{}, true, false)
}

func TestAcceptedAssignmentCreatesExactlyOneExecution(t *testing.T) {
	in := validRequest()
	repo := newCountingStore()
	service := newTestService(&fakeVerifier{acceptance: acceptedFor(in)}, repo)

	view, err := service.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if view.CanonicalBookingID == "" {
		t.Fatal("an accepted assignment produced no booking")
	}
	if repo.creates != 1 {
		t.Fatalf("creates = %d, want 1", repo.creates)
	}
}

// TestADeclinedProviderCannotBeDispatched is the safety property.
//
// A provider who said no must not end up driving. Fleet holds the response, so
// Taxi refuses on the response status rather than on anything the caller sent.
func TestADeclinedProviderCannotBeDispatched(t *testing.T) {
	for _, status := range []string{"DECLINED", "PENDING", "EXPIRED", "CANCELLED", ""} {
		t.Run(status, func(t *testing.T) {
			in := validRequest()
			acceptance := acceptedFor(in)
			acceptance.ResponseStatus = status

			repo := newCountingStore()
			service := newTestService(&fakeVerifier{acceptance: acceptance}, repo)

			if _, err := service.Create(context.Background(), in); err == nil {
				t.Fatalf("a %q response was allowed to create a booking", status)
			}
			if repo.creates != 0 {
				t.Fatalf("a refused create still wrote %d rows", repo.creates)
			}
		})
	}
}

// TestEveryAcceptanceFieldIsCompared proves the gate is not just an assignment
// id lookup.
//
// Each case is a claim Fleet does not hold. A gate that checked only the
// assignment would accept every one of them, and each accepted case dispatches
// somebody nobody agreed to: a different driver, a different organization's
// driver, a driver accepted for a different service, or a stale response.
func TestEveryAcceptanceFieldIsCompared(t *testing.T) {
	cases := map[string]func(*Acceptance){
		"different assignment":   func(a *Acceptance) { a.AssignmentID = "asg-other" },
		"different organization": func(a *Acceptance) { a.OrganizationID = "org-other" },
		"different provider":     func(a *Acceptance) { a.ProviderID = "prv-other" },
		"different membership":   func(a *Acceptance) { a.OrganizationMembershipID = "mem-other" },
		"stale response":         func(a *Acceptance) { a.ProviderResponseID = "rsp-other" },
		"wrong service":          func(a *Acceptance) { a.ProviderService = "DISPATCH" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := validRequest()
			acceptance := acceptedFor(in)
			mutate(&acceptance)

			repo := newCountingStore()
			service := newTestService(&fakeVerifier{acceptance: acceptance}, repo)

			if _, err := service.Create(context.Background(), in); err == nil {
				t.Fatalf("%s was accepted", name)
			}
			if repo.creates != 0 {
				t.Fatalf("%s still wrote %d rows", name, repo.creates)
			}
		})
	}
}

// TestFleetOutageRefusesRatherThanAssumingAcceptance pins the direction of the
// failure.
//
// An unreachable Fleet means Taxi does not know whether anybody agreed. The
// safe answer is to refuse: a booking created on an unverified claim cannot be
// un-driven, while a refused one is retried a second later.
func TestFleetOutageRefusesRatherThanAssumingAcceptance(t *testing.T) {
	repo := newCountingStore()
	service := newTestService(&fakeVerifier{err: errors.New("connection refused")}, repo)

	if _, err := service.Create(context.Background(), validRequest()); err == nil {
		t.Fatal("an unreachable Fleet was treated as an acceptance")
	}
	if repo.creates != 0 {
		t.Fatalf("a create proceeded during a Fleet outage and wrote %d rows", repo.creates)
	}
}

// TestReplayReturnsTheSameExecutionWithoutReaskingFleet covers the idempotency
// contract and one thing that is easy to get wrong with it: a replay must not
// turn a Fleet outage into a Taxi outage for work already accepted.
func TestReplayReturnsTheSameExecutionWithoutReaskingFleet(t *testing.T) {
	in := validRequest()
	repo := newCountingStore()
	verifier := &fakeVerifier{acceptance: acceptedFor(in)}
	service := newTestService(verifier, repo)

	first, err := service.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	second, err := service.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if first.CanonicalBookingID != second.CanonicalBookingID {
		t.Fatalf("replay produced a second booking: %s then %s",
			first.CanonicalBookingID, second.CanonicalBookingID)
	}
	if repo.creates != 1 {
		t.Fatalf("replay created %d executions", repo.creates)
	}
	if verifier.calls != 1 {
		t.Fatalf("replay re-asked Fleet %d times; an answered question was asked again",
			verifier.calls)
	}
}

// TestSameKeyDifferentRequestIsARefusal keeps the idempotency key honest. A key
// that returns somebody else's booking is worse than an error.
func TestSameKeyDifferentRequestIsARefusal(t *testing.T) {
	in := validRequest()
	repo := newCountingStore()
	service := newTestService(&fakeVerifier{acceptance: acceptedFor(in)}, repo)

	if _, err := service.Create(context.Background(), in); err != nil {
		t.Fatalf("first create: %v", err)
	}

	changed := in
	changed.ExecutingProviderID = "prv-2"
	if _, err := service.Create(context.Background(), changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("reused key with a different request returned %v, want conflict", err)
	}
	if repo.creates != 1 {
		t.Fatalf("a conflicting key still created %d executions", repo.creates)
	}
}

// TestDisabledFeatureCreatesNothing pins the flag. A rollout switch that leaves
// the create path reachable is not a rollout switch.
func TestDisabledFeatureCreatesNothing(t *testing.T) {
	in := validRequest()
	repo := newCountingStore()
	service := NewService(repo, &fakeVerifier{acceptance: acceptedFor(in)},
		staticZones{}, false, false)

	if _, err := service.Create(context.Background(), in); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled create returned %v, want disabled", err)
	}
	if repo.creates != 0 {
		t.Fatalf("a disabled feature created %d executions", repo.creates)
	}
}

// TestKillSwitchOverridesTheFeatureFlag: the whole point of a kill switch is
// that it wins.
func TestKillSwitchOverridesTheFeatureFlag(t *testing.T) {
	in := validRequest()
	repo := newCountingStore()
	service := NewService(repo, &fakeVerifier{acceptance: acceptedFor(in)},
		staticZones{}, true, true)

	if _, err := service.Create(context.Background(), in); !errors.Is(err, ErrDisabled) {
		t.Fatalf("kill-switched create returned %v, want disabled", err)
	}
	if repo.creates != 0 {
		t.Fatalf("the kill switch let %d executions through", repo.creates)
	}
}

// TestPrivateExecutionCarriesNoPricing is the standing guard for Section 10's
// own rule: corporate settlement is the organization's, and a fare on this
// contract would be a price Taxi neither computed nor can honour.
func TestPrivateExecutionCarriesNoPricing(t *testing.T) {
	banned := []string{"fare", "price", "amount", "kobo", "currency",
		"tariff", "surge", "commission", "payout"}

	for _, subject := range []any{CreateRequest{}, View{}, RouteSnapshot{},
		RideSnapshot{}, Acceptance{}} {
		subjectType := reflect.TypeOf(subject)
		for i := 0; i < subjectType.NumField(); i++ {
			field := subjectType.Field(i).Name
			lowered := strings.ToLower(field)
			for _, word := range banned {
				if strings.Contains(lowered, word) {
					t.Errorf("%s declares %s: corporate private execution carries no money",
						subjectType.Name(), field)
				}
			}
		}
	}
}
