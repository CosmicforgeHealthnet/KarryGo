package events

import "strings"

// CanonicalEvent is a Section 26.2 core event derived from a feature's own
// domain event.
type CanonicalEvent struct {
	EventType     string
	AggregateType string
	Status        string
}

// organizationStatusEvents are the organization lifecycle domain events
// whose suffix is the new status (Section 58.1).
var organizationStatusEvents = map[string]bool{
	"contact_verified": true, "business_submitted": true, "submitted": true, "resubmitted": true,
	"under_review": true, "changes_required": true, "rejected": true, "activated": true,
	"restricted": true, "reactivated": true, "suspended": true, "reinstated": true,
	"closure_requested": true, "closing_started": true, "closed": true,
}

// providerMembershipEvents are the provider-membership lifecycle domain
// events whose suffix is the new membership status (Section 58.2).
var providerMembershipEvents = map[string]bool{
	"invited": true, "accepted": true, "activated": true, "suspended": true, "reactivated": true,
	"removed": true, "ending": true, "ended": true, "expired": true, "cancelled": true, "declined": true,
}

// CanonicalAliases returns the Section 26.2 core events a domain event
// implies. Features keep emitting their detailed domain events; the
// transactional publisher appends these canonical events in the same
// transaction so a core event exists if and only if the mutation committed
// (spec 26.1, 26.2, 61). Before this, six core events offered to
// organization webhooks had no producer at all.
func CanonicalAliases(eventType string) []CanonicalEvent {
	name, ok := strings.CutSuffix(eventType, ".v1")
	if !ok {
		return nil
	}
	switch {
	case name == "invoice.issued":
		return []CanonicalEvent{{EventType: "corporate.invoice.issued.v1", AggregateType: "invoice", Status: "ISSUED"}}
	case name == "asset.blocked":
		return []CanonicalEvent{{EventType: "organization.asset.blocked.v1", AggregateType: "asset", Status: "BLOCKED"}}
	case name == "organization.orders.order.confirmed":
		return []CanonicalEvent{{EventType: "corporate.order.confirmed.v1", AggregateType: "order", Status: "CONFIRMED"}}
	case strings.HasPrefix(name, "organization.shipments.shipment."):
		status := strings.TrimPrefix(name, "organization.shipments.shipment.")
		if status == "" || strings.Contains(status, ".") {
			return nil
		}
		return []CanonicalEvent{{EventType: "corporate.shipment.status_changed.v1", AggregateType: "shipment", Status: strings.ToUpper(status)}}
	case strings.HasPrefix(name, "organization.provider."):
		status := strings.TrimPrefix(name, "organization.provider.")
		if !providerMembershipEvents[status] {
			return nil
		}
		out := []CanonicalEvent{{EventType: "organization.provider.membership_changed.v1", AggregateType: "provider_membership", Status: strings.ToUpper(status)}}
		if status == "invited" {
			out = append([]CanonicalEvent{{EventType: "organization.provider.provisioned.v1", AggregateType: "provider", Status: "PROVISIONED"}}, out...)
		}
		return out
	case strings.HasPrefix(name, "organization."):
		status := strings.TrimPrefix(name, "organization.")
		if organizationStatusEvents[status] {
			return []CanonicalEvent{{EventType: "organization.status_changed.v1", AggregateType: "organization", Status: strings.ToUpper(status)}}
		}
	}
	return nil
}
