package dispatchcommerce

import (
	"bytes"
	"encoding/json"
	"errors"
)

type DeliveryLifecycleMission struct {
	MissionRef string   `json:"mission_ref"`
	Version    int64    `json:"version"`
	State      string   `json:"state"`
	ParcelRefs []string `json:"parcel_refs"`
}

// This is an opaque delivery read model, not a second CommerceMission command.
type DeliveryLifecycleProjection struct {
	SourceRef     string                     `json:"source_ref"`
	SourceVersion int64                      `json:"source_version"`
	GroupRef      string                     `json:"group_ref"`
	Missions      []DeliveryLifecycleMission `json:"missions"`
}

func VendorLifecycleEvent(name string) bool {
	switch name {
	case "dispatch.delivery_plan.created.v1", "dispatch.delivery_plan.superseded.v1", "dispatch.mission.assigned.v1", "dispatch.pickup.verified.v1", "dispatch.receiver.verified.v1", "dispatch.return.completed.v1", "dispatch.return.started.v1", "dispatch.delivery_recovery.selected.v1", "dispatch.provider_replacement.started.v1":
		return true
	}
	return false
}
func DecodeLifecycleProjection(raw []byte) (DeliveryLifecycleProjection, error) {
	var p DeliveryLifecycleProjection
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&p) != nil || p.SourceRef == "" || p.SourceVersion < 1 || p.GroupRef == "" || len(p.Missions) == 0 || len(p.Missions) > 100 {
		return p, errors.New("invalid delivery lifecycle projection")
	}
	for _, m := range p.Missions {
		if m.MissionRef == "" || m.Version < 1 || len(m.ParcelRefs) == 0 {
			return p, errors.New("invalid lifecycle mission")
		}
		switch m.State {
		case "PLANNED", "OFFERING", "QUEUED", "ASSIGNED", "EN_ROUTE_PICKUP", "AT_PICKUP", "PICKUP_VERIFICATION", "PICKED_UP", "IN_TRANSIT", "AT_DELIVERY", "DELIVERY_VERIFICATION", "DELIVERED", "COMPLETED", "CANCELLED", "FAILED", "REASSIGNING", "RETURNING", "RETURNED", "HUB_HOLD":
		default:
			return p, errors.New("invalid lifecycle state")
		}
	}
	return p, nil
}
