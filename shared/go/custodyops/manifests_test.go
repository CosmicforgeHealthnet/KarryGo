package custodyops

import (
	"errors"
	"testing"
)

func key(ref string) ManifestKey { return ManifestKey{ManifestRef: ref, ManifestVersion: 1} }

// abcMission is A -> B -> C -> customer. Stop B has two separately sealed
// manifests (B1, B2), showing that one stop may own several manifests.
func abcMission() MissionSnapshot {
	s := MissionSnapshot{MissionRef: "m1", Mode: ModeMultiManifest, TenantRef: "t1", Status: "MANIFESTS_REGISTERED", SourceVersion: 1,
		Stops: map[string]StopState{
			"A": {StopRef: "A", StopType: StopPickup, Sequence: 1, Status: "PENDING"},
			"B": {StopRef: "B", StopType: StopPickup, Sequence: 2, Status: "PENDING"},
			"C": {StopRef: "C", StopType: StopPickup, Sequence: 3, Status: "PENDING"},
			"D": {StopRef: "D", StopType: StopDropoff, Sequence: 4, Status: "PENDING"},
		},
		Manifests: map[ManifestKey]ManifestState{},
	}
	for ref, stop := range map[string]string{"MA": "A", "MB1": "B", "MB2": "B", "MC": "C"} {
		s.Manifests[key(ref)] = ManifestState{ManifestKey: key(ref), PickupStopRef: stop, Status: ManifestRegistered, PackageCount: 2, Required: true}
	}
	return s
}

// collect applies a collection decision to the snapshot like the repository does.
func collect(t *testing.T, s *MissionSnapshot, stop, provider string, version int64, refs ...string) CollectionDecision {
	t.Helper()
	keys := make([]ManifestKey, 0, len(refs))
	for _, ref := range refs {
		keys = append(keys, key(ref))
	}
	d, err := DecideCollection(*s, CollectManifestsCommand{OperationRef: "op-" + stop, TenantRef: "t1", StopRef: stop, ProviderRef: provider,
		HandoverChallengeRef: "ch-" + stop, HandoverVerifiedByRef: "vendor", SourceVersion: version, Manifests: keys})
	if err != nil {
		t.Fatalf("collect %v at %s: %v", refs, stop, err)
	}
	for _, k := range d.Manifests {
		m := s.Manifests[k]
		m.Status, m.Custodian = ManifestCollected, provider
		s.Manifests[k] = m
	}
	st := s.Stops[stop]
	st.Status = d.StopStatus
	s.Stops[stop], s.CurrentProvider, s.Status, s.SourceVersion = st, provider, "PICKED_UP", version
	return d
}

func TestRegisterMissionTopologyValidation(t *testing.T) {
	valid := func() RegisterMissionCommand {
		return RegisterMissionCommand{OperationRef: "op", MissionRef: "m", TenantRef: "t", ReceiverChallengeRef: "r", SourceVersion: 1,
			Stops: []StopInput{{StopRef: "A", StopType: "pickup", Sequence: 1}, {StopRef: "B", StopType: "PICKUP", Sequence: 2}, {StopRef: "D", StopType: "DROPOFF", Sequence: 3}},
			Manifests: []ManifestInput{
				{ManifestRef: "MA", ManifestVersion: 1, PickupStopRef: "A", SourceAuthority: "VENDOR", SourceManifestChecksum: "sha", Packages: []PackageInput{{PackageRef: "a1", PackageCount: 1}}},
				{ManifestRef: "MB1", ManifestVersion: 1, PickupStopRef: "B", SourceAuthority: "VENDOR", SourceManifestChecksum: "sha", Packages: []PackageInput{{PackageRef: "b1", PackageCount: 1}}},
				{ManifestRef: "MB2", ManifestVersion: 1, PickupStopRef: "B", SourceAuthority: "VENDOR", SourceManifestChecksum: "sha", Packages: []PackageInput{{PackageRef: "b2", PackageCount: 2}}},
			}}
	}
	ok := valid()
	if err := ok.Normalize(); err != nil {
		t.Fatalf("multiple manifests at one stop must be valid: %v", err)
	}
	cases := map[string]func(*RegisterMissionCommand){
		"duplicate package across manifests": func(c *RegisterMissionCommand) { c.Manifests[1].Packages[0].PackageRef = "a1" },
		"manifest on dropoff stop":           func(c *RegisterMissionCommand) { c.Manifests[0].PickupStopRef = "D" },
		"pickup stop without manifest":       func(c *RegisterMissionCommand) { c.Manifests = c.Manifests[1:] },
		"dropoff not last":                   func(c *RegisterMissionCommand) { c.Stops[0].Sequence = 9 },
		"missing checksum":                   func(c *RegisterMissionCommand) { c.Manifests[0].SourceManifestChecksum = "" },
		"two dropoffs":                       func(c *RegisterMissionCommand) { c.Stops[1].StopType = StopDropoff },
	}
	for name, mutate := range cases {
		c := valid()
		mutate(&c)
		if err := c.Normalize(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestCollectionIsStopScopedAndCumulative(t *testing.T) {
	s := abcMission()
	collect(t, &s, "A", "p1", 2, "MA")
	if held := s.HeldBy("p1"); len(held) != 1 || held[0] != key("MA") {
		t.Fatalf("after A custody = %v, want [MA]", held)
	}
	if s.Manifests[key("MB1")].Status != ManifestRegistered || s.Manifests[key("MC")].Status != ManifestRegistered {
		t.Fatal("a successful A handover must not collect B or C")
	}
	if _, err := DecideCollection(s, CollectManifestsCommand{OperationRef: "x", TenantRef: "t1", StopRef: "A", ProviderRef: "p1", HandoverChallengeRef: "c", HandoverVerifiedByRef: "v", SourceVersion: 3, Manifests: []ManifestKey{key("MC")}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("collecting a future-stop manifest at A: err = %v, want conflict", err)
	}
	d := collect(t, &s, "B", "p1", 3, "MB1")
	if d.StopStatus != "PARTIALLY_COLLECTED" {
		t.Fatalf("stop B with MB2 outstanding = %s", d.StopStatus)
	}
	collect(t, &s, "B", "p1", 4, "MB2")
	if held := s.HeldBy("p1"); len(held) != 3 {
		t.Fatalf("after B custody = %v, want MA+MB1+MB2", held)
	}
}

func TestDuplicatePickupCallbackDoesNotDuplicateCustody(t *testing.T) {
	s := abcMission()
	collect(t, &s, "A", "p1", 2, "MA")
	d, err := DecideCollection(s, CollectManifestsCommand{OperationRef: "retry-with-new-op", TenantRef: "t1", StopRef: "A", ProviderRef: "p1", HandoverChallengeRef: "ch-A", HandoverVerifiedByRef: "vendor", SourceVersion: 2, Manifests: []ManifestKey{key("MA")}})
	if err != nil || !d.AlreadyHeld {
		t.Fatalf("duplicate callback = %+v %v, want AlreadyHeld", d, err)
	}
	if _, err := DecideCollection(s, CollectManifestsCommand{OperationRef: "other", TenantRef: "t1", StopRef: "A", ProviderRef: "p2", HandoverChallengeRef: "c", HandoverVerifiedByRef: "v", SourceVersion: 2, Manifests: []ManifestKey{key("MA")}}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("second provider acquiring MA: err = %v, want unauthorized", err)
	}
}

func TestStaleSourceVersionIsRejected(t *testing.T) {
	s := abcMission()
	collect(t, &s, "A", "p1", 5, "MA")
	_, err := DecideCollection(s, CollectManifestsCommand{OperationRef: "late", TenantRef: "t1", StopRef: "B", ProviderRef: "p1", HandoverChallengeRef: "c", HandoverVerifiedByRef: "v", SourceVersion: 4, Manifests: []ManifestKey{key("MB1")}})
	if !errors.Is(err, ErrStaleSourceVersion) {
		t.Fatalf("stale event err = %v", err)
	}
}

func TestPartialPickupFailureTransfersHeldAndKeepsFuturePending(t *testing.T) {
	s := abcMission()
	collect(t, &s, "A", "p1", 2, "MA")
	collect(t, &s, "B", "p1", 3, "MB1", "MB2")
	s.Status, s.ReplacementProvider = "TRANSFER_REQUIRED", "p2"
	transfer := TransferManifestsCommand{OperationRef: "tr", TenantRef: "t1", TransferRef: "x", OldProviderRef: "p1", NewProviderRef: "p2", EvidenceRef: "ev",
		OldProviderConfirm: true, NewProviderConfirm: true, NewReceiverChallengeRef: "r2", SourceVersion: 4}

	transfer.Manifests = []ManifestKey{key("MA"), key("MB1")}
	if _, err := DecideTransfer(s, transfer); !errors.Is(err, ErrConflict) {
		t.Fatalf("leaving MB2 behind: err = %v, want conflict", err)
	}
	transfer.Manifests = []ManifestKey{key("MA"), key("MB1"), key("MB2"), key("MC")}
	if _, err := DecideTransfer(s, transfer); !errors.Is(err, ErrConflict) {
		t.Fatalf("transferring uncollected MC: err = %v, want conflict", err)
	}
	transfer.Manifests = []ManifestKey{key("MB2"), key("MA"), key("MB1")}
	items, err := DecideTransfer(s, transfer)
	if err != nil || len(items) != 3 {
		t.Fatalf("exact transfer = %+v %v", items, err)
	}
	for _, item := range items {
		if item.From != "p1" || item.To != "p2" || item.PackageCount != 2 {
			t.Fatalf("item = %+v", item)
		}
	}
	other := transfer
	other.NewProviderRef = "p3"
	if _, err := DecideTransfer(s, other); !errors.Is(err, ErrConflict) {
		t.Fatalf("second replacement provider claiming custody: err = %v, want conflict", err)
	}

	// Apply the transfer, then the replacement continues to C and delivers.
	for _, item := range items {
		m := s.Manifests[item.ManifestKey]
		m.Custodian = "p2"
		s.Manifests[item.ManifestKey] = m
	}
	s.CurrentProvider, s.ReplacementProvider, s.Status = "p2", "", "IN_TRANSIT"
	if s.Manifests[key("MC")].Status != ManifestRegistered {
		t.Fatal("MC must remain a pending future pickup after transfer")
	}
	collect(t, &s, "C", "p2", 5, "MC")
	partial, err := DecideDelivery(s, DeliverManifestsCommand{OperationRef: "d1", TenantRef: "t1", DropoffStopRef: "D", ProviderRef: "p2", DeliveryChallengeRef: "r2", SourceVersion: 6, Manifests: []ManifestKey{key("MA")}})
	if err != nil || partial.MissionComplete {
		t.Fatalf("partial delivery = %+v %v, must not complete", partial, err)
	}
	full, err := DecideDelivery(s, DeliverManifestsCommand{OperationRef: "d2", TenantRef: "t1", DropoffStopRef: "D", ProviderRef: "p2", DeliveryChallengeRef: "r2", SourceVersion: 6, Manifests: []ManifestKey{key("MA"), key("MB1"), key("MB2"), key("MC")}})
	if err != nil || !full.MissionComplete {
		t.Fatalf("full delivery = %+v %v, must complete", full, err)
	}
	if _, err := DecideDelivery(s, DeliverManifestsCommand{OperationRef: "d3", TenantRef: "t1", DropoffStopRef: "D", ProviderRef: "p1", DeliveryChallengeRef: "r2", SourceVersion: 6, Manifests: []ManifestKey{key("MA")}}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("original provider delivering after transfer: err = %v", err)
	}
}

func TestDeliveredManifestCannotRegressAndCompletionNeedsAll(t *testing.T) {
	s := abcMission()
	for k, m := range s.Manifests {
		m.Status = ManifestDelivered
		s.Manifests[k] = m
	}
	if _, err := DecideException(s, ManifestExceptionCommand{OperationRef: "e", TenantRef: "t1", ActorRef: "ops", Manifest: key("MA"), Status: ManifestMissing, SourceVersion: 2}); !errors.Is(err, ErrConflict) {
		t.Fatalf("delivered manifest regressed: err = %v", err)
	}
	if len(s.Unaccounted()) != 0 {
		t.Fatal("all delivered must be fully accounted")
	}
	m := s.Manifests[key("MC")]
	m.Status = ManifestMissing
	s.Manifests[key("MC")] = m
	if got := s.Unaccounted(); len(got) != 1 || got[0] != key("MC") {
		t.Fatalf("missing required manifest must block completion: %v", got)
	}
}

func TestDamageKeepsCustodyButLossReleasesIt(t *testing.T) {
	s := abcMission()
	collect(t, &s, "A", "p1", 2, "MA")
	damaged, err := DecideException(s, ManifestExceptionCommand{OperationRef: "e1", TenantRef: "t1", ActorRef: "p1", Manifest: key("MA"), Status: ManifestDamaged, SourceVersion: 3})
	if err != nil || damaged.ReleaseCustody || damaged.Manifest.Status != ManifestCollected || damaged.PackageStatus != ManifestDamaged {
		t.Fatalf("damaged in transit = %+v %v", damaged, err)
	}
	lost, err := DecideException(s, ManifestExceptionCommand{OperationRef: "e2", TenantRef: "t1", ActorRef: "ops", Manifest: key("MA"), Status: ManifestMissing, SourceVersion: 3})
	if err != nil || !lost.ReleaseCustody || lost.Manifest.Custodian != "" {
		t.Fatalf("lost in transit = %+v %v", lost, err)
	}
}

func TestLegacyMissionsAreNotDrivenByMultiManifestDecisions(t *testing.T) {
	s := abcMission()
	s.Mode = ModeLegacySingle
	if _, err := DecideCollection(s, CollectManifestsCommand{OperationRef: "x", TenantRef: "t1", StopRef: "A", ProviderRef: "p1", HandoverChallengeRef: "c", HandoverVerifiedByRef: "v", SourceVersion: 2, Manifests: []ManifestKey{key("MA")}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("legacy mission via multi-manifest path: err = %v", err)
	}
}
