package bookingusecases

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"cosmicforge/logistics/services/hauling-service/internal/cargofit"
	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/commercemission"
)

// rejectionStore is the slice of the availability store eligibleProviders
// touches.
type rejectionStore struct {
	availabilityrepositories.AvailabilityStore
	rejected map[string]bool
}

func (s rejectionStore) GetRejectedProviders(context.Context, string) (map[string]bool, error) {
	return s.rejected, nil
}

// eligibleProviders is the ONE chokepoint every dispatch path goes through:
// first match, rescan after a rejection, and the recovery worker's resumed
// match all call it. These tests pin that the capability gate sits inside it,
// so a replacement can never be chosen by a route that skipped hard fit.
func gatedService(capability CapabilityAuthority, config CapabilityMatchingConfig) *BookingService {
	return &BookingService{
		availability:       rejectionStore{rejected: map[string]bool{}},
		capability:         capability,
		capabilityMatching: config,
		maxRadiusKm:        25,
	}
}

func lagosProvider(id string) availabilityrepositories.ProviderStatus {
	return availabilityrepositories.ProviderStatus{ProviderID: id, Lat: 6.5244, Lng: 3.3792}
}

// Enforced: eligibleProviders itself drops the candidate that cannot carry the
// cargo, so every caller inherits the gate without having to remember it.
func TestEligibleProvidersEnforcesHardFit(t *testing.T) {
	capability := &fakeCapability{trucks: map[string][]cargofit.TruckCapability{
		"fits":      {bigTruck("t1")},
		"too-small": {smallTruck("t2")},
	}}
	service := gatedService(capability, CapabilityMatchingConfig{Enabled: true, FailClosed: true})

	result := service.eligibleProviders(context.Background(),
		[]availabilityrepositories.ProviderStatus{lagosProvider("fits"), lagosProvider("too-small")},
		twoTonneBooking(), 6.5244, 3.3792)

	if len(result) != 1 || result[0].ProviderID != "fits" {
		t.Fatalf("result = %+v, want only the fitting provider", result)
	}
}

// Shadow: the same call records the decision and returns the legacy list, so
// the rollout is measurable without changing a single dispatch.
func TestEligibleProvidersKeepsLegacySelectionInShadow(t *testing.T) {
	capability := &fakeCapability{trucks: map[string][]cargofit.TruckCapability{
		"too-small": {smallTruck("t1")},
	}}
	service := gatedService(capability, CapabilityMatchingConfig{Enabled: true, FailClosed: false})

	input := []availabilityrepositories.ProviderStatus{lagosProvider("too-small")}
	result := service.eligibleProviders(context.Background(), input, twoTonneBooking(), 6.5244, 3.3792)

	if len(result) != len(input) {
		t.Fatalf("shadow mode dropped a candidate: %d of %d", len(result), len(input))
	}
	if len(capability.recorded) != 1 || !capability.recorded[0].Shadow {
		t.Fatalf("shadow decision was not recorded: %+v", capability.recorded)
	}
}

// Flags off: the legacy weight-and-type path runs untouched, so a deployment
// that has not enabled Section 9 dispatches exactly as it did before.
func TestEligibleProvidersIsUnchangedWhenTheFlagIsOff(t *testing.T) {
	capability := &fakeCapability{trucks: map[string][]cargofit.TruckCapability{
		"too-small": {smallTruck("t1")},
	}}
	service := gatedService(capability, CapabilityMatchingConfig{Enabled: false})

	result := service.eligibleProviders(context.Background(),
		[]availabilityrepositories.ProviderStatus{lagosProvider("too-small")},
		twoTonneBooking(), 6.5244, 3.3792)

	if len(result) != 1 {
		t.Fatal("the disabled gate still removed a candidate")
	}
	if len(capability.recorded) != 0 {
		t.Fatal("the disabled gate still wrote evidence")
	}
}

// A store outage empties the candidate list rather than falling through to
// proximity-only. This is the behaviour the old code got wrong, asserted at
// the level the whole service actually uses.
func TestEligibleProvidersRefusesEveryoneOnAnOutage(t *testing.T) {
	capability := &fakeCapability{loadErr: context.DeadlineExceeded}
	service := gatedService(capability, CapabilityMatchingConfig{Enabled: true, FailClosed: true})

	result := service.eligibleProviders(context.Background(),
		[]availabilityrepositories.ProviderStatus{lagosProvider("p1"), lagosProvider("p2")},
		twoTonneBooking(), 6.5244, 3.3792)

	if len(result) != 0 {
		t.Fatalf("an outage still produced %d dispatchable providers", len(result))
	}
}

// Ranking runs before the gate and only ever loses members to it, so a nearer
// truck cannot be scored back into a job it does not fit.
func TestProximityOrderSurvivesTheGateAndCannotResurrectACandidate(t *testing.T) {
	near := lagosProvider("near")
	far := lagosProvider("far")
	far.Lat, far.Lng = 6.60, 3.45 // roughly 12km away

	capability := &fakeCapability{trucks: map[string][]cargofit.TruckCapability{
		"near": {smallTruck("t-near")}, // closest, and cannot carry the load
		"far":  {bigTruck("t-far")},
	}}
	service := gatedService(capability, CapabilityMatchingConfig{Enabled: true, FailClosed: true})

	result := service.eligibleProviders(context.Background(),
		[]availabilityrepositories.ProviderStatus{far, near}, twoTonneBooking(), 6.5244, 3.3792)

	if len(result) != 1 || result[0].ProviderID != "far" {
		t.Fatalf("result = %+v, want the farther truck that actually fits", result)
	}
}

// ── Vendor boundary ───────────────────────────────────────────────────────────

// Vendor states what the goods need. It never states who carries them. The
// mission contract is shared and signed, so this is checked structurally
// rather than by reading a handler that could change.
func TestCommerceMissionRequestCannotNameAnExecutor(t *testing.T) {
	forbidden := []string{"provider", "driver", "rider", "truck", "bike", "vehicleid", "assignee", "fleetmember"}

	// VehicleClass is a requirement ("this needs a truck"), not an identity,
	// and every quote and mission already carries it. The distinction the rule
	// draws is between describing what is needed and choosing who provides it.
	allowed := map[string]bool{"VehicleClass": true}

	var walk func(reflect.Type, string)
	seen := map[reflect.Type]bool{}
	walk = func(t2 reflect.Type, path string) {
		for t2.Kind() == reflect.Ptr || t2.Kind() == reflect.Slice || t2.Kind() == reflect.Array {
			t2 = t2.Elem()
		}
		if t2.Kind() != reflect.Struct || seen[t2] {
			return
		}
		seen[t2] = true
		for i := 0; i < t2.NumField(); i++ {
			field := t2.Field(i)
			if !allowed[field.Name] {
				name := strings.ToLower(field.Name)
				for _, needle := range forbidden {
					if strings.Contains(name, needle) {
						t.Errorf("%s.%s lets Vendor name an executor", path, field.Name)
					}
				}
			}
			walk(field.Type, path+"."+field.Name)
		}
	}
	walk(reflect.TypeOf(commercemission.Request{}), "commercemission.Request")
}

// A fragile booking must narrow the candidate set, never widen it.
func TestFragileBookingOnlyEverNarrowsTheCandidateSet(t *testing.T) {
	plain := twoTonneBooking()
	fragile := plain
	fragile.IsFragile = true

	capability := &fakeCapability{trucks: map[string][]cargofit.TruckCapability{
		"tipper": {tipperTruck("t1")},
	}}
	service := gatedService(capability, CapabilityMatchingConfig{Enabled: true, FailClosed: true})

	providers := []availabilityrepositories.ProviderStatus{lagosProvider("tipper")}
	plainResult := service.eligibleProviders(context.Background(), providers, plain, 6.5244, 3.3792)
	fragileResult := service.eligibleProviders(context.Background(), providers, fragile, 6.5244, 3.3792)

	if len(plainResult) != 1 {
		t.Fatalf("a tipper was refused ordinary cargo: %+v", plainResult)
	}
	if len(fragileResult) != 0 {
		t.Fatal("a tipper was offered fragile cargo")
	}
}

func tipperTruck(id string) cargofit.TruckCapability {
	truck := bigTruck(id)
	truck.TruckType = "tipper"
	return truck
}

// The legacy booking model must keep working: a weight-only booking against an
// unsurveyed-but-payload-known truck still dispatches.
func TestLegacyWeightOnlyBookingStillDispatches(t *testing.T) {
	capability := &fakeCapability{trucks: map[string][]cargofit.TruckCapability{
		"p1": {bigTruck("t1")},
	}}
	service := gatedService(capability, CapabilityMatchingConfig{Enabled: true, FailClosed: true})

	booking := bookingmodels.Booking{ID: "legacy", CargoWeightKg: 1_500}
	result := service.eligibleProviders(context.Background(),
		[]availabilityrepositories.ProviderStatus{lagosProvider("p1")}, booking, 6.5244, 3.3792)

	if len(result) != 1 {
		t.Fatal("a legacy weight-only booking stopped dispatching")
	}
}
