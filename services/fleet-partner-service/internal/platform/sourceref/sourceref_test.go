package sourceref

import (
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	valid := SourceReference{SourceService: ServiceCustomer, ExternalID: "ext-1"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid reference rejected: %v", err)
	}
	if err := (SourceReference{SourceService: "rogue-service", ExternalID: "x"}).Validate(); err == nil {
		t.Fatal("unknown service must be rejected")
	}
	if err := (SourceReference{SourceService: ServicePayment}).Validate(); err == nil {
		t.Fatal("missing external id must be rejected")
	}
}

func TestFreshness(t *testing.T) {
	now := time.Now()
	unsynced := SourceReference{}
	if got := unsynced.Freshness(now, time.Hour); got != SyncStatusUnsynced {
		t.Fatalf("unsynced = %s", got)
	}
	fresh := SourceReference{SyncedAt: now.Add(-time.Minute)}
	if got := fresh.Freshness(now, time.Hour); got != SyncStatusFresh {
		t.Fatalf("fresh = %s", got)
	}
	stale := SourceReference{SyncedAt: now.Add(-2 * time.Hour)}
	if got := stale.Freshness(now, time.Hour); got != SyncStatusStale {
		t.Fatalf("stale = %s", got)
	}
}

func TestCompareVersionDetectsOutOfOrder(t *testing.T) {
	ref := SourceReference{SourceVersion: 5}
	if ref.CompareVersion(6) != 1 {
		t.Fatal("newer version must classify as apply")
	}
	if ref.CompareVersion(5) != 0 {
		t.Fatal("same version must classify as duplicate")
	}
	if ref.CompareVersion(4) != -1 {
		t.Fatal("older version must classify as out-of-order")
	}
}
