package events

import "testing"

// Every Section 26.2 core event registered as produced by this service has
// a derivation, so none is offered to subscribers while never being emitted.
func TestEverySection26ProducedEventHasAProducer(t *testing.T) {
	derived := map[string]bool{"organization.created.v1": true, "organization.request.accepted.v1": true,
		"organization.assignment.created.v1": true, "organization.assignment.accepted.v1": true,
		"organization.assignment.declined.v1": true, "organization.assignment.expired.v1": true}
	for _, source := range []string{"invoice.issued.v1", "asset.blocked.v1", "organization.orders.order.confirmed.v1",
		"organization.shipments.shipment.ready.v1", "organization.provider.invited.v1", "organization.activated.v1"} {
		for _, a := range CanonicalAliases(source) {
			derived[a.EventType] = true
		}
	}
	for _, def := range Registered() {
		if def.Direction == Produced && def.Description == "Section 26 minimized contract" && !derived[def.Type] {
			t.Errorf("Section 26 produced event %s has no producer", def.Type)
		}
	}
}

func TestCanonicalAliases(t *testing.T) {
	cases := map[string][]string{
		"organization.activated.v1":                    {"organization.status_changed.v1:ACTIVATED"},
		"organization.suspended.v1":                    {"organization.status_changed.v1:SUSPENDED"},
		"organization.updated.v1":                      nil,
		"organization.settings_updated.v1":             nil,
		"organization.provider.invited.v1":             {"organization.provider.provisioned.v1:PROVISIONED", "organization.provider.membership_changed.v1:INVITED"},
		"organization.provider.removed.v1":             {"organization.provider.membership_changed.v1:REMOVED"},
		"organization.provider.policy_changed.v1":      nil,
		"asset.blocked.v1":                             {"organization.asset.blocked.v1:BLOCKED"},
		"organization.orders.order.confirmed.v1":       {"corporate.order.confirmed.v1:CONFIRMED"},
		"organization.orders.order.created.v1":         nil,
		"organization.shipments.shipment.delivered.v1": {"corporate.shipment.status_changed.v1:DELIVERED"},
		"invoice.issued.v1":                            {"corporate.invoice.issued.v1:ISSUED"},
		"invoice.approved.v1":                          nil,
		"corporate.invoice.issued.v1":                  nil,
	}
	for source, want := range cases {
		got := CanonicalAliases(source)
		if len(got) != len(want) {
			t.Errorf("%s -> %+v, want %v", source, got, want)
			continue
		}
		for i, a := range got {
			if a.EventType+":"+a.Status != want[i] {
				t.Errorf("%s[%d] = %s:%s, want %s", source, i, a.EventType, a.Status, want[i])
			}
			if def, ok := Lookup(a.EventType); !ok || def.AggregateType != a.AggregateType {
				t.Errorf("%s aggregate %s does not match registry %+v", a.EventType, a.AggregateType, def)
			}
		}
	}
}
