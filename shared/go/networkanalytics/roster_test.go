package networkanalytics

import (
	"testing"
	"time"
)

func TestSupplyRosterMemberIsIndividualAndDeterministic(t *testing.T) {
	bucket := BucketStart(time.Now())
	m := SupplyRosterMember{Authority: AuthorityHauling, ProductCode: ProductHaulage, ProviderID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Eligible: Bool(true), BucketStart: bucket, PolicyVersion: "v1"}
	if !m.Valid() || m.EventID() != m.EventID() || m.Version() != bucket.Unix() {
		t.Fatal("roster member identity")
	}
	other := m
	other.BucketStart = bucket.Add(SupplyBucket)
	if other.EventID() == m.EventID() {
		t.Fatal("bucket not in identity")
	}
	for name, bad := range map[string]SupplyRosterMember{
		"non-canonical id": {Authority: AuthorityHauling, ProductCode: ProductHaulage, ProviderID: "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA", BucketStart: bucket, PolicyVersion: "v1"},
		"vendor authority": {Authority: AuthorityVendor, ProductCode: "X", ProviderID: m.ProviderID, BucketStart: bucket, PolicyVersion: "v1"},
		"unaligned bucket": {Authority: AuthorityHauling, ProductCode: ProductHaulage, ProviderID: m.ProviderID, BucketStart: bucket.Add(time.Second), PolicyVersion: "v1"},
	} {
		if bad.Valid() {
			t.Fatal(name)
		}
	}
	if authority, kind, ok := Classify(EventSupplyRoster, "HAULING"); !ok || authority != AuthorityHauling || kind != KindSupplyRoster {
		t.Fatal("roster classification")
	}
	if _, _, ok := Classify(EventSupplyRoster, ""); ok {
		t.Fatal("roster without authority classified")
	}
	envelope, err := RosterEnvelope("driver-hauling-service", m)
	if err != nil || envelope.Validate() != nil || envelope.SourceEntityType != SupplyRosterEntityType {
		t.Fatal(err)
	}
}

func TestSameSlotMeanTrustsOnlyMinimumHistory(t *testing.T) {
	if mean, trusted := SameSlotMean([]int64{5, 6, 7, 8}); mean != 6.5 || !trusted {
		t.Fatal(mean, trusted)
	}
	if _, trusted := SameSlotMean([]int64{5, 6}); trusted {
		t.Fatal("short history trusted")
	}
	if _, trusted := SameSlotMean(nil); trusted {
		t.Fatal("no history trusted")
	}
}
