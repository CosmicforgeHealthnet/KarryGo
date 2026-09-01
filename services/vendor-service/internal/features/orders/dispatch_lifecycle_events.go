package orders

import (
	"context"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"encoding/json"
	"sort"
	"strings"
)

func validateDispatchLifecycle(e DispatchCommerceEvent) (dispatchcommerce.DeliveryLifecycleProjection, error) {
	if !dispatchcommerce.VendorLifecycleEvent(e.EventType) || e.SourceService != DispatchServiceCaller || e.EventID == "" || e.AggregateID == "" || e.AggregateVersion < 1 || e.SchemaVersion != 1 || e.OccurredAt.IsZero() || len(e.Payload) > 256<<10 || !strings.EqualFold(e.PayloadHash, hashDispatchPayload(e.Payload)) {
		return dispatchcommerce.DeliveryLifecycleProjection{}, ErrOrderConflict
	}
	return dispatchcommerce.DecodeLifecycleProjection(e.Payload)
}

func (s *Service) applyDispatchLifecycle(ctx context.Context, e DispatchCommerceEvent) error {
	p, err := validateDispatchLifecycle(e)
	if err != nil {
		return err
	}
	tx, err := s.store.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var refID, slot string
	var currentRaw []byte
	// SourceRef must name OUR previously reserved operation, not a caller tenant.
	err = tx.QueryRow(ctx, `SELECT r.id,pm.slot_ref,r.dispatch_domain_projection FROM commerce_platform_mission_refs r JOIN commerce_fulfilment_plan_missions pm ON pm.id=r.plan_mission_id WHERE r.mission_ref=$1 AND r.authority='DISPATCH' FOR UPDATE OF r`, p.SourceRef).Scan(&refID, &slot, &currentRaw)
	if err != nil {
		return ErrOrderNotFound
	}
	var previous dispatchcommerce.DeliveryLifecycleProjection
	if json.Unmarshal(currentRaw, &previous) != nil {
		return ErrOrderConflict
	}
	for _, mission := range p.Missions {
		var valid bool
		err = tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM unnest($2::text[]) supplied WHERE NOT EXISTS(SELECT 1 FROM commerce_platform_mission_refs r JOIN commerce_fulfilment_plan_mission_parcels mp ON mp.plan_mission_id=r.plan_mission_id JOIN commerce_fulfilment_parcels p ON p.id=mp.parcel_id WHERE r.id=$1 AND p.parcel_ref=supplied))`, refID, mission.ParcelRefs).Scan(&valid)
		if err != nil {
			return err
		}
		if !valid {
			return ErrOrderConflict
		}
	}
	tag, err := tx.Exec(ctx, `INSERT INTO commerce_platform_operational_events(id,event_id,canonical_mission_ref,operation_ref,mission_slot_ref,event_type,source_version,payload_hash,payload,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(event_id) DO NOTHING`, "cpoe_"+e.EventID, e.EventID, p.GroupRef, p.SourceRef, slot, e.EventType, p.SourceVersion, e.PayloadHash, e.Payload, e.OccurredAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var hash string
		if err = tx.QueryRow(ctx, `SELECT payload_hash FROM commerce_platform_operational_events WHERE event_id=$1 AND operation_ref=$2 AND event_type=$3`, e.EventID, p.SourceRef, e.EventType).Scan(&hash); err != nil || !strings.EqualFold(hash, e.PayloadHash) {
			return ErrOrderConflict
		}
		return tx.Commit(ctx)
	}
	merged, err := mergeDispatchLifecycle(previous, p)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(merged)
	if _, err = tx.Exec(ctx, `UPDATE commerce_platform_mission_refs SET dispatch_domain_projection=$2 WHERE id=$1`, refID, raw); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func mergeDispatchLifecycle(old, in dispatchcommerce.DeliveryLifecycleProjection) (dispatchcommerce.DeliveryLifecycleProjection, error) {
	if old.SourceVersion > in.SourceVersion {
		return old, nil
	}
	if old.SourceVersion == in.SourceVersion && old.GroupRef != "" && old.GroupRef != in.GroupRef {
		return old, ErrOrderConflict
	}
	if old.SourceVersion < in.SourceVersion {
		return in, nil
	}
	merged := map[string]dispatchcommerce.DeliveryLifecycleMission{}
	for _, m := range old.Missions {
		merged[m.MissionRef] = m
	}
	for _, m := range in.Missions {
		if prev, ok := merged[m.MissionRef]; !ok || m.Version > prev.Version {
			merged[m.MissionRef] = m
		}
	}
	in.Missions = nil
	for _, m := range merged {
		in.Missions = append(in.Missions, m)
	}
	sort.Slice(in.Missions, func(i, j int) bool { return in.Missions[i].MissionRef < in.Missions[j].MissionRef })
	return in, nil
}
