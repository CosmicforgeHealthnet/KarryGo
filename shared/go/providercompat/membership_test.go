package providercompat

import (
	"context"
	"strings"
	"testing"
)

// IAM-009, IAM-010, IAM-011: a suspended, expired or terminated membership
// cannot take new managed work, and the three are distinguishable rather than
// collapsed into one "not active".
func TestSuspendedExpiredAndTerminatedAreRefusedAndDistinct(t *testing.T) {
	cases := []struct {
		status MembershipStatus
		class  MembershipClass
	}{
		{MembershipSuspended, ClassSuspended},
		{MembershipExpired, ClassExpired},
		{MembershipRemoved, ClassTerminated},
		{MembershipCancelled, ClassTerminated},
		{MembershipDeclined, ClassTerminated},
		{MembershipEnding, ClassTerminated},
	}
	seen := map[string]bool{}
	for _, tc := range cases {
		if tc.status.AllowsNewManagedWork() {
			t.Fatalf("%s authorized new managed work", tc.status)
		}
		if got := tc.status.Class(); got != tc.class {
			t.Fatalf("%s classified as %s; want %s", tc.status, got, tc.class)
		}
		err := VerifyManagedContext(CorporateContext{
			IsManagedProvider: true, MembershipID: "mem_1", OrganizationID: "org_1",
			MembershipStatus: string(tc.status),
		}, ManagedRequest{Service: "TAXI", ProviderID: "prv_1", RequireManaged: true})
		if err == nil {
			t.Fatalf("%s was authorized", tc.status)
		}
		kind, _ := KindOf(err)
		if kind != FailureMembershipInvalid {
			t.Fatalf("%s failed as %s; want %s", tc.status, kind, FailureMembershipInvalid)
		}
		if kind.Retryable() {
			t.Fatalf("%s was reported as retryable; an authoritative refusal is final", tc.status)
		}
		if !strings.Contains(err.Error(), string(tc.class)) {
			t.Fatalf("%s refusal does not name its class: %v", tc.status, err)
		}
		seen[string(tc.class)] = true
	}
	// Suspended, expired and terminated must each be reachable as their own
	// answer; if two of them collapsed the caller could not tell them apart.
	for _, class := range []MembershipClass{ClassSuspended, ClassExpired, ClassTerminated} {
		if !seen[string(class)] {
			t.Fatalf("no status produced the %s class", class)
		}
	}
}

// A status this build does not recognise authorizes nothing. Treating an
// unknown status as permissive is how a new Fleet state silently becomes a
// bypass.
func TestUnrecognisedMembershipStatusAuthorizesNothing(t *testing.T) {
	status := ParseMembershipStatus("PROBATION")
	if status.Class() != ClassUnknown || status.AllowsNewManagedWork() {
		t.Fatalf("an unknown status was treated as %s", status.Class())
	}
	if status.Terminal() {
		t.Fatal("an unknown status was reported as definitely over")
	}
}

func TestOnlyActiveTakesNewManagedWork(t *testing.T) {
	all := []MembershipStatus{
		MembershipInvited, MembershipPendingProviderAcceptance, MembershipPendingSetup,
		MembershipActive, MembershipDeclined, MembershipExpired, MembershipCancelled,
		MembershipSuspended, MembershipEnding, MembershipRemoved,
	}
	allowed := 0
	for _, status := range all {
		if status.AllowsNewManagedWork() {
			allowed++
			if status != MembershipActive {
				t.Fatalf("%s authorized new managed work", status)
			}
		}
	}
	if allowed != 1 {
		t.Fatalf("%d statuses authorize new work; exactly ACTIVE should", allowed)
	}
	if !ParseMembershipStatus(" active ").AllowsNewManagedWork() {
		t.Fatal("a valid status in the wrong case was refused")
	}
}

// IAM-012: a membership that changes while work is already in the provider's
// custody must not strand the goods. Every status permits finishing work
// already in hand; the organization's decision takes effect on NEW work.
func TestMembershipChangeNeverStrandsWorkAlreadyInCustody(t *testing.T) {
	for _, status := range []MembershipStatus{
		MembershipSuspended, MembershipExpired, MembershipRemoved,
		MembershipCancelled, MembershipEnding, ParseMembershipStatus("PROBATION"),
	} {
		if !status.AllowsActiveWorkCompletion() {
			t.Fatalf("%s would strand a parcel the provider is already carrying", status)
		}
		if status.AllowsNewManagedWork() {
			t.Fatalf("%s would also let the provider take NEW work", status)
		}
	}
}

// The gate must never turn a Fleet outage into a membership refusal: an
// unreachable Fleet is retryable, a real status is final.
func TestFleetOutageIsNotAMembershipRefusal(t *testing.T) {
	_, err := AuthorizeManagedOperation(context.Background(), failingReader{},
		ManagedRequest{Service: "TAXI", ProviderID: "prv_1", RequireManaged: true})
	if err == nil {
		t.Fatal("an unreachable Fleet authorized the operation")
	}
	kind, _ := KindOf(err)
	if kind != FailureDependency || !kind.Retryable() {
		t.Fatalf("outage reported as %s (retryable=%v)", kind, kind.Retryable())
	}
}

type failingReader struct{}

func (failingReader) ReadCorporateContext(context.Context, string, string) (CorporateContext, error) {
	return CorporateContext{}, context.DeadlineExceeded
}
