package orchestration

// Catering and Manufacturer through the production JIT path.
//
// Nothing here is category-specific by design: an orchestration is opened from
// a physical requirement and a readiness signal, and neither carries a vendor
// category. That is the property worth testing -- the two new categories reach
// the same planner, the same commit-once guarantee and the same authority
// selection as the seven that came before, with no branch anywhere on "is this
// a caterer".

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	readiness "cosmicforge/logistics/services/vendor-service/internal/features/fulfilment_readiness"
	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
)

// productionHarness is the shared harness with the readiness signal coming
// from a producing vendor rather than a restaurant.
func productionHarness(t *testing.T, committer *recordingCommitter,
	vendorType readiness.VendorType) *orchestrationHarness {
	t.Helper()
	pool := orchestrationTestPool(t)
	now := time.Now().UTC()
	readyAt := now.Add(-time.Minute)
	harness := &orchestrationHarness{pool: pool, committer: committer, now: now}
	harness.service = NewService(Options{
		Repository: NewRepository(pool),
		Gate: staticGate{
			usable: map[logistics.Authority]bool{logistics.Dispatch: true},
			answers: map[logistics.Authority]logistics.Serviceability{
				logistics.Dispatch: {Authority: logistics.Dispatch, Serviceable: true, Version: 3},
			},
		},
		Preview:   staticPreview{},
		Committer: committer,
		Readiness: staticReadiness{state: readiness.Readiness{
			VendorType: vendorType, State: readiness.StateReady,
			PredictedReadyStart: &readyAt, PredictedReadyEnd: &readyAt,
			ConfidenceBand: readiness.ConfidenceHigh, UpdatedAt: now,
		}},
		JITEnabled: true, AutoSelectionEnabled: true,
		Now: func() time.Time { return harness.now },
	})
	return harness
}

// openProduction opens one orchestration for a producing vendor's load.
func openProduction(t *testing.T, h *orchestrationHarness, requirement Requirement) Orchestration {
	t.Helper()
	suffix := uuid.NewString()
	requirement.PickupCount, requirement.DropoffCount = 1, 1
	requirement.PickupLatitude, requirement.PickupLongitude = 6.43, 3.42
	requirement.DropoffLatitude, requirement.DropoffLongitude = 6.45, 3.40
	requirement.DesiredPickupStart = h.now
	requirement.DesiredPickupEnd = h.now.Add(30 * time.Minute)
	opened, err := h.service.Open(context.Background(), OpenRequest{
		FulfilmentGroupRef: "fg_" + suffix, ParentOrderID: "po_" + suffix,
		Requirement: requirement,
		Pickups:     []Pickup{{VendorOrderID: "vo_" + suffix, Sequence: 1, BusinessID: "biz-1", BranchID: "br-1"}},
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return opened
}

func TestCateringCanOpenJITOrchestration(t *testing.T) {
	committer := &recordingCommitter{
		outcome: MissionOutcome{Definite: true, Accepted: true, MissionRef: "mission-catering"}}
	h := productionHarness(t, committer, readiness.VendorCatering)

	// A catering tray load: heavy and bulky for a bike, trivial for a truck.
	opened := openProduction(t, h, Requirement{WeightGrams: 45_000, VolumeMl: 120_000, PackageCount: 12})
	committed, err := h.service.Evaluate(context.Background(), opened.ID)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if committed.State != StateCommitted {
		t.Fatalf("state = %s, want COMMITTED", committed.State)
	}
	if committed.SelectedAuthority == "" {
		t.Fatal("committed without choosing an authority")
	}
	if creates, _ := committer.counts(); creates != 1 {
		t.Fatalf("creates = %d, want exactly 1", creates)
	}
	assertNamesNoProvider(t, committed)
}

func TestManufacturerCanOpenJITOrchestration(t *testing.T) {
	committer := &recordingCommitter{
		outcome: MissionOutcome{Definite: true, Accepted: true, MissionRef: "mission-manufacturer"}}
	h := productionHarness(t, committer, readiness.VendorManufacturer)

	// A palletised factory shipment.
	opened := openProduction(t, h, Requirement{WeightGrams: 850_000, VolumeMl: 2_400_000,
		LongestDimensionMm: 1_200, PackageCount: 4})
	committed, err := h.service.Evaluate(context.Background(), opened.ID)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if committed.State != StateCommitted {
		t.Fatalf("state = %s, want COMMITTED", committed.State)
	}
	if committed.SelectedAuthority == "" {
		t.Fatal("committed without choosing an authority")
	}
	if creates, _ := committer.counts(); creates != 1 {
		t.Fatalf("creates = %d, want exactly 1", creates)
	}
	assertNamesNoProvider(t, committed)
}

func TestCateringAuthoritySelectionNeverChoosesProvider(t *testing.T) {
	committer := &recordingCommitter{
		outcome: MissionOutcome{Definite: true, Accepted: true, MissionRef: "mission-catering-2"}}
	h := productionHarness(t, committer, readiness.VendorCatering)
	opened := openProduction(t, h, Requirement{WeightGrams: 45_000, VolumeMl: 120_000, PackageCount: 12})
	committed, err := h.service.Evaluate(context.Background(), opened.ID)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	assertNamesNoProvider(t, committed)
}

func TestManufacturerAuthoritySelectionNeverChoosesProvider(t *testing.T) {
	committer := &recordingCommitter{
		outcome: MissionOutcome{Definite: true, Accepted: true, MissionRef: "mission-manufacturer-2"}}
	h := productionHarness(t, committer, readiness.VendorManufacturer)
	opened := openProduction(t, h, Requirement{WeightGrams: 850_000, VolumeMl: 2_400_000,
		LongestDimensionMm: 1_200, PackageCount: 4})
	committed, err := h.service.Evaluate(context.Background(), opened.ID)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	assertNamesNoProvider(t, committed)
}

// assertNamesNoProvider serialises whatever it is given and fails on any field
// that could name who executes the work. Vendor chooses an authority and a
// product; the authority chooses the provider.
func assertNamesNoProvider(t *testing.T, subject any) {
	t.Helper()
	raw, err := json.Marshal(subject)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var walk func(value any, path string)
	forbidden := []string{"provider", "driver", "rider", "vehicle", "fleet_member", "assignee", "plate"}
	walk = func(value any, path string) {
		switch typed := value.(type) {
		case map[string]any:
			for key, nested := range typed {
				lower := strings.ToLower(key)
				for _, word := range forbidden {
					if strings.Contains(lower, word) {
						t.Errorf("%s%s names an executor", path, key)
					}
				}
				walk(nested, path+key+".")
			}
		case []any:
			for _, nested := range typed {
				walk(nested, path)
			}
		}
	}
	var decoded any
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	walk(decoded, "")
}

// The guard above is only worth having if it would catch one.
func TestTheProductionProviderGuardIsNotVacuous(t *testing.T) {
	fake := &testing.T{}
	assertNamesNoProvider(fake, map[string]any{
		"selected_authority": "DISPATCH",
		"plan":               map[string]any{"provider_id": "prv-1"},
	})
	if !fake.Failed() {
		t.Fatal("a planted provider_id was not caught")
	}
}
