package providercompat

import "strings"

// This file gives Fleet's membership status a name and a meaning that every
// execution service reads the same way.
//
// The gate itself has always been "ACTIVE, or refuse", which is correct and
// is not changed here. What was missing is the ability to say WHICH refusal
// happened: a suspended provider, an expired membership and a terminated one
// are the same "not active" to a boolean, but they are three different things
// to the person on the other end of the refusal, and Section 16 asks for all
// three to be handled distinctly (IAM-008 through IAM-011).

// MembershipStatus is one of Fleet's persisted membership states.
//
// These are the exact values in fleet_partner_provider_memberships' CHECK
// constraint. They are restated as a type rather than reinvented as a tidier
// set, because a vocabulary that disagrees with the rows in the database is
// worse than no vocabulary at all.
type MembershipStatus string

const (
	// MembershipInvited is an offer Fleet has extended and nobody has
	// answered yet.
	MembershipInvited MembershipStatus = "INVITED"
	// MembershipPendingProviderAcceptance is awaiting the provider's answer.
	MembershipPendingProviderAcceptance MembershipStatus = "PENDING_PROVIDER_ACCEPTANCE"
	// MembershipPendingSetup is accepted but not yet ready to work.
	MembershipPendingSetup MembershipStatus = "PENDING_SETUP"
	// MembershipActive is the only status that authorizes new managed work.
	MembershipActive MembershipStatus = "ACTIVE"
	// MembershipDeclined is an offer the provider refused.
	MembershipDeclined MembershipStatus = "DECLINED"
	// MembershipExpired is a membership whose term ran out.
	MembershipExpired MembershipStatus = "EXPIRED"
	// MembershipCancelled is an offer or membership the organization withdrew.
	MembershipCancelled MembershipStatus = "CANCELLED"
	// MembershipSuspended is a live membership the organization has paused.
	MembershipSuspended MembershipStatus = "SUSPENDED"
	// MembershipEnding is serving notice. It is NOT active: the organization
	// has decided the relationship is over and only the date is outstanding,
	// so no new managed work may be started under it.
	MembershipEnding MembershipStatus = "ENDING"
	// MembershipRemoved is Fleet's terminated state.
	MembershipRemoved MembershipStatus = "REMOVED"
)

// MembershipClass groups statuses by what they mean for authorization, so a
// caller can answer "is this suspended, expired or terminated" without
// enumerating ten strings.
type MembershipClass string

const (
	// ClassActive can take new managed work.
	ClassActive MembershipClass = "ACTIVE"
	// ClassPending has not started yet.
	ClassPending MembershipClass = "PENDING"
	// ClassSuspended is paused and may come back.
	ClassSuspended MembershipClass = "SUSPENDED"
	// ClassExpired ran out of time.
	ClassExpired MembershipClass = "EXPIRED"
	// ClassTerminated is over: declined, cancelled, ending or removed. The
	// section calls this TERMINATED; Fleet spells it four different ways
	// depending on who ended it and when.
	ClassTerminated MembershipClass = "TERMINATED"
	// ClassUnknown is a status this build does not recognise. It authorizes
	// nothing, because a status nobody can classify is not evidence that a
	// provider may work.
	ClassUnknown MembershipClass = "UNKNOWN"
)

// ParseMembershipStatus normalizes a status Fleet reported.
func ParseMembershipStatus(value string) MembershipStatus {
	return MembershipStatus(strings.ToUpper(strings.TrimSpace(value)))
}

// Class groups the status.
func (s MembershipStatus) Class() MembershipClass {
	switch s {
	case MembershipActive:
		return ClassActive
	case MembershipInvited, MembershipPendingProviderAcceptance, MembershipPendingSetup:
		return ClassPending
	case MembershipSuspended:
		return ClassSuspended
	case MembershipExpired:
		return ClassExpired
	case MembershipDeclined, MembershipCancelled, MembershipEnding, MembershipRemoved:
		return ClassTerminated
	default:
		return ClassUnknown
	}
}

// AllowsNewManagedWork reports whether a provider may START managed work
// under this membership.
//
// Only ACTIVE does. Everything else — including a status this build has never
// heard of — is refused, which is the fail-closed direction: a membership
// nobody can classify is not permission to work.
func (s MembershipStatus) AllowsNewManagedWork() bool {
	return s.Class() == ClassActive
}

// Terminal reports whether the membership is over and will not come back on
// its own. A suspended membership can be reinstated; a removed one cannot.
func (s MembershipStatus) Terminal() bool {
	return s.Class() == ClassTerminated
}

// AllowsActiveWorkCompletion reports whether work the provider is ALREADY
// executing may be carried to completion under this membership.
//
// The answer is yes for every status, and that is deliberate (IAM-012).
//
// A provider suspended while holding a customer's goods is a corporate event,
// not a physical one: the parcel is still in their vehicle, the receiver is
// still waiting, and refusing the delivery step would strand the goods with
// nobody accountable for them. The organization's decision takes effect where
// it belongs — no new assignment, no new offer, no new execution — while the
// work in hand finishes and is settled against the membership snapshot taken
// when it started.
//
// This is a policy statement rather than a gate. It is here so the rule is
// stated in one place and can be asserted by a test, instead of being an
// emergent property of which call sites happen to check membership.
func (s MembershipStatus) AllowsActiveWorkCompletion() bool { return true }

// membershipFailureDetail explains a refusal in terms of the class, so the
// three cases Section 16 separates are separable by the caller too.
func membershipFailureDetail(status MembershipStatus) string {
	switch status.Class() {
	case ClassSuspended:
		return "membership is suspended and cannot take new managed work"
	case ClassExpired:
		return "membership has expired and cannot begin new managed work"
	case ClassTerminated:
		return "membership is terminated and cannot accept new organization work"
	case ClassPending:
		return "membership has not started and cannot take managed work yet"
	case ClassUnknown:
		return "membership status is not recognised, so it authorizes nothing"
	default:
		return "membership is not active"
	}
}
