package capabilityfit

import (
	"strings"
	"testing"
)

// The zero value of a capability fact must be Unknown, not Yes. This is the
// single property the whole package rests on: a struct field nobody populated
// has to cost a match, never a delivery.
func TestTriZeroValueIsUnknown(t *testing.T) {
	var fact Tri
	if fact != Unknown {
		t.Fatalf("zero Tri = %v, want Unknown", fact)
	}
	if fact.Known() {
		t.Fatal("zero Tri reports Known")
	}
}

func TestTriFromPointerKeepsNullDistinctFromFalse(t *testing.T) {
	no := false
	yes := true
	if got := TriFromPointer(nil); got != Unknown {
		t.Fatalf("nil -> %v, want Unknown", got)
	}
	if got := TriFromPointer(&no); got != No {
		t.Fatalf("false -> %v, want No", got)
	}
	if got := TriFromPointer(&yes); got != Yes {
		t.Fatalf("true -> %v, want Yes", got)
	}
}

func TestTriPointerRoundTrip(t *testing.T) {
	for _, fact := range []Tri{Unknown, Yes, No} {
		if got := TriFromPointer(fact.Pointer()); got != fact {
			t.Fatalf("round trip %v -> %v", fact, got)
		}
	}
}

// An unknown capability must refuse with CAPABILITY_UNKNOWN, and a declared
// negative with REQUIRED_CAPABILITY_UNAVAILABLE. Collapsing the two would make
// a missing measurement indistinguishable from a proven incompatibility.
func TestRequireCapabilityDistinguishesUnknownFromNo(t *testing.T) {
	unknownCase := NewDecision()
	unknownCase.RequireCapability(true, Unknown, "thermal bag never recorded")
	if unknownCase.Eligible {
		t.Fatal("unknown critical capability was accepted")
	}
	if got := unknownCase.PrimaryReason(); got != ReasonCapabilityUnknown {
		t.Fatalf("unknown reason = %s, want %s", got, ReasonCapabilityUnknown)
	}

	refusedCase := NewDecision()
	refusedCase.RequireCapability(true, No, "no thermal bag fitted")
	if refusedCase.Eligible {
		t.Fatal("declared-incapable asset was accepted")
	}
	if got := refusedCase.PrimaryReason(); got != ReasonRequiredCapabilityUnavailable {
		t.Fatalf("no reason = %s, want %s", got, ReasonRequiredCapabilityUnavailable)
	}
}

// An unknown fact must only disqualify when the requirement actually needs it.
// Otherwise every gap in capability data would disqualify every asset from
// every job, which is fail-closed taken past the point of usefulness.
func TestRequireCapabilityIgnoresUnrequestedFacts(t *testing.T) {
	decision := NewDecision()
	decision.RequireCapability(false, Unknown, "thermal bag never recorded")
	decision.RequireCapability(false, No, "no thermal bag fitted")
	if !decision.Eligible {
		t.Fatalf("unrequested capabilities refused: %v", decision.Reasons)
	}
	if got := decision.PrimaryReason(); got != ReasonEligible {
		t.Fatalf("primary reason = %s, want %s", got, ReasonEligible)
	}
}

func TestRequireCapabilityAcceptsDeclaredYes(t *testing.T) {
	decision := NewDecision()
	decision.RequireCapability(true, Yes, "thermal bag fitted")
	if !decision.Eligible {
		t.Fatalf("declared-capable asset refused: %v", decision.Reasons)
	}
}

func TestRequireBoundBoundaryIsInclusive(t *testing.T) {
	exact := NewDecision()
	exact.RequireBound(20_000, 20_000, true, ReasonCapacityExceeded, "payload")
	if !exact.Eligible {
		t.Fatalf("exact-capacity load refused: %v", exact.Reasons)
	}

	over := NewDecision()
	over.RequireBound(20_001, 20_000, true, ReasonCapacityExceeded, "payload")
	if over.Eligible {
		t.Fatal("over-capacity load accepted")
	}
	if got := over.PrimaryReason(); got != ReasonCapacityExceeded {
		t.Fatalf("reason = %s, want %s", got, ReasonCapacityExceeded)
	}
}

// An undeclared bound that the requirement actually depends on is unknown, not
// unlimited. A bike whose payload was never measured must not carry 40kg.
func TestRequireBoundUnknownDeclarationFailsClosed(t *testing.T) {
	decision := NewDecision()
	decision.RequireBound(20_000, 0, false, ReasonCapacityExceeded, "payload never measured")
	if decision.Eligible {
		t.Fatal("unmeasured bound accepted a real load")
	}
	if got := decision.PrimaryReason(); got != ReasonCapabilityUnknown {
		t.Fatalf("reason = %s, want %s", got, ReasonCapabilityUnknown)
	}
}

func TestRequireBoundSkipsWhenNothingRequired(t *testing.T) {
	decision := NewDecision()
	decision.RequireBound(0, 0, false, ReasonCapacityExceeded, "payload never measured")
	if !decision.Eligible {
		t.Fatalf("absent requirement refused: %v", decision.Reasons)
	}
}

// Once refused, a decision must stay refused. Ranking runs after eligibility
// and must not be able to talk an ineligible asset back into the pool.
func TestDecisionCannotBecomeEligibleAgain(t *testing.T) {
	decision := NewDecision()
	decision.Refuse(ReasonCargoFitFailed, "too long")
	decision.RequireCapability(true, Yes, "fine")
	decision.RequireBound(1, 100, true, ReasonCapacityExceeded, "fine")
	if decision.Eligible {
		t.Fatal("refused decision became eligible after passing checks")
	}
}

func TestDecisionCollectsEveryDistinctReasonOnce(t *testing.T) {
	decision := NewDecision()
	decision.Refuse(ReasonCargoFitFailed, "too long")
	decision.Refuse(ReasonAssetNotCertified, "no product eligibility")
	decision.Refuse(ReasonCargoFitFailed, "too wide")

	if len(decision.Reasons) != 2 {
		t.Fatalf("reasons = %v, want 2 distinct", decision.Reasons)
	}
	joined := decision.ReasonString()
	if !strings.Contains(joined, ReasonCargoFitFailed) || !strings.Contains(joined, ReasonAssetNotCertified) {
		t.Fatalf("reason string %q lost a reason", joined)
	}
	// Both details for the repeated reason survive, because an operator
	// fixing a truck needs to know it failed on length AND width.
	if detail := decision.Details[ReasonCargoFitFailed]; !strings.Contains(detail, "too long") || !strings.Contains(detail, "too wide") {
		t.Fatalf("detail %q lost a repeat", detail)
	}
}

func TestEmptyDecisionReportsEligible(t *testing.T) {
	decision := NewDecision()
	if got := decision.PrimaryReason(); got != ReasonEligible {
		t.Fatalf("primary = %s, want %s", got, ReasonEligible)
	}
	if got := decision.ReasonString(); got != ReasonEligible {
		t.Fatalf("string = %s, want %s", got, ReasonEligible)
	}
}
