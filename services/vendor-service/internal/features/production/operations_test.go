package production

import (
	"testing"
	"time"
)

func TestQueuePriorityRank(t *testing.T) {
	if QueuePriorityRank("URGENT") != 0 || QueuePriorityRank("HIGH") != 1 || QueuePriorityRank("NORMAL") != 2 || QueuePriorityRank("unknown") != 2 {
		t.Fatal("priority ordering must be URGENT, HIGH, NORMAL")
	}
}

func TestProductionCapabilityForOrderDomain(t *testing.T) {
	for _, domain := range []string{"FOOD", "RESTAURANT", "FAST_FOOD", "CLOUD_KITCHEN", "PASTRY_BAKERY"} {
		if _, ok := ProductionCapabilityForOrderDomain(domain); !ok {
			t.Fatalf("%s should be production-eligible", domain)
		}
	}
	if _, ok := ProductionCapabilityForOrderDomain("SHOP"); ok {
		t.Fatal("grocery-only Shop orders must not activate kitchen work")
	}
}

func TestQueueCursorRoundTripAndValidation(t *testing.T) {
	ready := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	created := ready.Add(-time.Hour)
	raw := encodeQueueCursor(QueueItem{ID: "job_1", Priority: "URGENT", PromisedReadyAt: &ready, CreatedAt: created})
	got, err := decodeQueueCursor(raw)
	if err != nil || got.ID != "job_1" || got.PriorityRank != 0 || got.PromisedReadyAt == nil || !got.PromisedReadyAt.Equal(ready) || !got.CreatedAt.Equal(created) {
		t.Fatalf("cursor round trip failed: %+v, %v", got, err)
	}
	if _, err = decodeQueueCursor("not-a-cursor"); err == nil {
		t.Fatal("malformed cursor accepted")
	}
	if _, err = decodeQueueCursor(encodeQueueCursor(QueueItem{ID: "", Priority: "NORMAL", CreatedAt: created})); err == nil {
		t.Fatal("cursor without stable id accepted")
	}
}
