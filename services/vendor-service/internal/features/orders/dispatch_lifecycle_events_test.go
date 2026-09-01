package orders

import (
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"encoding/json"
	"testing"
	"time"
)

func lifecycleEvent() DispatchCommerceEvent {
	p := dispatchcommerce.DeliveryLifecycleProjection{SourceRef: "reserved-operation", SourceVersion: 1, GroupRef: "group", Missions: []dispatchcommerce.DeliveryLifecycleMission{{MissionRef: "mission", Version: 2, State: "ASSIGNED", ParcelRefs: []string{"parcel"}}}}
	raw, _ := json.Marshal(p)
	return DispatchCommerceEvent{EventID: "event", EventType: "dispatch.mission.assigned.v1", SchemaVersion: 1, SourceService: DispatchServiceCaller, AggregateID: "mission", AggregateVersion: 2, OccurredAt: time.Now(), Payload: raw, PayloadHash: hashDispatchPayload(raw)}
}
func TestDispatchLifecycleAllowlistSecretsAndLegacyCompatibility(t *testing.T) {
	e := lifecycleEvent()
	if _, err := validateDispatchLifecycle(e); err != nil {
		t.Fatal(err)
	}
	e.EventType = "dispatch.offer.created.v1"
	if _, err := validateDispatchLifecycle(e); err == nil {
		t.Fatal("internal offer accepted")
	}
	e = lifecycleEvent()
	var raw map[string]any
	_ = json.Unmarshal(e.Payload, &raw)
	raw["otp_hash"] = "secret"
	e.Payload, _ = json.Marshal(raw)
	e.PayloadHash = hashDispatchPayload(e.Payload)
	if _, err := validateDispatchLifecycle(e); err == nil {
		t.Fatal("secret accepted")
	}
	if !validDispatchCommerceEventType("commerce.mission.changed.v1") {
		t.Fatal("legacy removed")
	}
}
func TestDispatchLifecycleDuplicateAndStaleVersionsDoNotRegress(t *testing.T) {
	p, err := validateDispatchLifecycle(lifecycleEvent())
	if err != nil {
		t.Fatal(err)
	}
	same, err := mergeDispatchLifecycle(p, p)
	if err != nil || len(same.Missions) != 1 {
		t.Fatal(same, err)
	}
	stale := p
	stale.Missions = append([]dispatchcommerce.DeliveryLifecycleMission(nil), p.Missions...)
	stale.Missions[0].Version = 1
	stale.Missions[0].State = "PLANNED"
	result, err := mergeDispatchLifecycle(p, stale)
	if err != nil || result.Missions[0].State != "ASSIGNED" {
		t.Fatal(result, err)
	}
	stale.GroupRef = "foreign"
	if _, err = mergeDispatchLifecycle(p, stale); err == nil {
		t.Fatal("source group replaced without version change")
	}
}
