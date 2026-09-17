package movementmodels

import (
	"encoding/json"
	"errors"
	"testing"

	"cosmicforge/logistics/shared/go/events"
)

func TestApplyKeepsDescriptiveContextAndReplacementCount(t *testing.T) {
	existing := Movement{CustomerID: "c1", Authority: "TAXI", SourceRef: "m1", SourceVersion: 2, State: events.MovementStateAssigned,
		ProductCode: text("TAXI_GOODS_STANDARD"), ParentContextRef: text("order-1"), OriginSummary: json.RawMessage(`{"latitude":1,"longitude":2}`), ProviderReplacementCount: 1}
	incoming := Movement{CustomerID: "c1", Authority: "TAXI", SourceRef: "m1", SourceVersion: 3, State: events.MovementStateInProgress}
	next, outcome, err := Apply(&existing, incoming)
	if err != nil || outcome != OutcomeApplied {
		t.Fatalf("apply = %s %v", outcome, err)
	}
	if next.ProductCode == nil || *next.ProductCode != "TAXI_GOODS_STANDARD" || next.ParentContextRef == nil || len(next.OriginSummary) == 0 || next.ProviderReplacementCount != 1 {
		t.Fatalf("a lifecycle event without context must not erase it: %+v", next)
	}
}

func TestApplyRejectsCustomerRebinding(t *testing.T) {
	existing := Movement{CustomerID: "c1", SourceVersion: 1, State: events.MovementStateSearching}
	if _, _, err := Apply(&existing, Movement{CustomerID: "c2", SourceVersion: 2, State: events.MovementStateAssigned}); !errors.Is(err, ErrCustomerMismatch) {
		t.Fatalf("customer rebinding err = %v", err)
	}
}
