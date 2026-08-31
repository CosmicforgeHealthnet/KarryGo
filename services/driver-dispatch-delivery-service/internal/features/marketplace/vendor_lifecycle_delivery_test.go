package marketplace

import (
	"context"
	"encoding/json"
	"testing"
)

type lifecycleTransport struct{ events []Envelope }

func (d *lifecycleTransport) Configured() bool { return true }
func (d *lifecycleTransport) Deliver(_ context.Context, e Envelope) (DeliveryResult, error) {
	d.events = append(d.events, e)
	return DeliveryResult{Outcome: DeliveryAccepted}, nil
}
func TestVendorAdapterAllowlistsAndMinimizesWithoutChangingLegacy(t *testing.T) {
	transport := &lifecycleTransport{}
	d := VendorLifecycleDeliverer{Transport: transport}
	e := Envelope{EventID: "event", EventType: "commerce.mission.changed.v1", Payload: map[string]any{"missionRef": "legacy"}}
	if _, err := d.Deliver(context.Background(), e); err != nil || transport.events[0].Payload["missionRef"] != "legacy" {
		t.Fatal("legacy changed")
	}
	e.EventType = "dispatch.offer.created.v1"
	if r, _ := d.Deliver(context.Background(), e); r.Outcome != DeliveryRejected {
		t.Fatal("offer internals leaked")
	}
	var p map[string]any
	_ = json.Unmarshal([]byte(`{"source_ref":"operation","source_version":1,"group_ref":"group","missions":[{"mission_ref":"mission","version":1,"state":"ASSIGNED","parcel_refs":["parcel"]}]}`), &p)
	e.EventType = "dispatch.mission.assigned.v1"
	e.Payload = map[string]any{"delivery_projection": p, "provider_id": "do-not-forward", "candidate_snapshot": "private"}
	if r, err := d.Deliver(context.Background(), e); err != nil || r.Outcome != DeliveryAccepted {
		t.Fatal(r, err)
	}
	if len(transport.events[1].Payload) != 4 {
		t.Fatal("unapproved fields forwarded")
	}
}
