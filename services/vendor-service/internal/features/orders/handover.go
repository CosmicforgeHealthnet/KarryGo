package orders

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	handover "cosmicforge/logistics/services/vendor-service/internal/integrations/handover"
	"github.com/jackc/pgx/v5"
)

func (s *Service) SetHandoverAuthorities(authorities handover.Router) { s.handover = authorities }

func (s *Service) handoverBinding(ctx context.Context, actor Actor, orderID, missionID string, confirm bool) (handover.Binding, error) {
	permission := PermOrdersRead
	if confirm {
		permission = PermOrdersManage
	}
	scope, err := s.authorize(ctx, actor, permission)
	if err != nil {
		return handover.Binding{}, err
	}
	order, err := s.store.GetVendorOrderForVendor(ctx, actor.VendorID, orderID)
	if err != nil {
		return handover.Binding{}, err
	}
	if !scope.Covers(order.BranchID) {
		return handover.Binding{}, ErrOrderNotFound
	}
	if confirm && order.Status != VendorStatusReady && order.Status != VendorStatusReadyForPickup && order.Status != VendorStatusHandedOver {
		return handover.Binding{}, ErrOrderConflict
	}
	var vendor handover.Binding
	var parcelJSON []byte
	err = s.store.db.QueryRow(ctx, `
		SELECT vm.id,vm.id,vm.business_id,vm.branch_id,COALESCE(vm.assigned_staff_id,''),
		       po.customer_id,COALESCE(COALESCE(po.operational_receiver_snapshot,po.receiver_snapshot)->>'receiver_phone',''),
		       vm.plan_id||':'||vm.plan_mission_id,pv.version,jsonb_agg(p.id ORDER BY p.id)
		  FROM commerce_vendor_missions vm
		  JOIN commerce_fulfilment_groups g ON g.id=vm.group_id
		  JOIN commerce_parent_orders po ON po.id=g.parent_order_id
		  JOIN commerce_fulfilment_plan_versions pv ON pv.id=vm.plan_id
		  JOIN commerce_fulfilment_plan_mission_parcels mp ON mp.plan_mission_id=vm.plan_mission_id
		  JOIN commerce_fulfilment_parcels p ON p.id=mp.parcel_id
		  JOIN commerce_fulfilment_units u ON u.id=p.unit_id AND u.vendor_order_id=$2
		 WHERE vm.id=$1 AND vm.business_id=$3
		 GROUP BY vm.id,po.customer_id,po.receiver_snapshot,pv.version`, missionID, order.ID, actor.VendorID).
		Scan(&vendor.MissionRef, &vendor.FulfilmentRef, &vendor.BusinessID, &vendor.BranchID,
			&vendor.ProviderRef, &vendor.ReceiverRef, &vendor.ReceiverPhone, &vendor.ManifestRef,
			&vendor.ManifestVersion, &parcelJSON)
	if err == nil {
		if vendor.ProviderRef == "" || vendor.BranchID != order.BranchID || json.Unmarshal(parcelJSON, &vendor.ParcelRefs) != nil {
			return handover.Binding{}, ErrOrderConflict
		}
		vendor.Authority = "VENDOR_OWN_DELIVERY"
		return vendor, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return handover.Binding{}, err
	}
	m, err := loadMissionBy(ctx, s.store.db, `m.id=$1 AND EXISTS (SELECT 1 FROM commerce_order_mission_allocations a WHERE a.mission_id=m.id AND a.vendor_order_id=$2)`, missionID, order.ID)
	if err != nil {
		return handover.Binding{}, err
	}
	if m.Status != MissionAccepted || m.ExternalRef == "" {
		return handover.Binding{}, ErrOrderConflict
	}
	binding := handover.Binding{Authority: string(m.Service), MissionRef: m.MissionRef, FulfilmentRef: m.ID}
	if m.Service == LogisticsDispatch {
		// Dispatch pickup challenges are scoped to the exact immutable plan
		// stop and manifest. Resolve the stop through this vendor order's unit;
		// accepting either value from the HTTP caller would permit cross-stop
		// confirmation in a consolidated mission.
		var planRef string
		err = s.store.db.QueryRow(ctx, `
			SELECT ps.id,pv.id,pv.version
			  FROM commerce_fulfilment_groups g
			  JOIN commerce_fulfilment_plan_versions pv ON pv.id=g.current_plan_id
			  JOIN commerce_fulfilment_plan_missions pm ON pm.plan_id=pv.id AND pm.slot_ref=$1
			  JOIN commerce_fulfilment_plan_stops ps ON ps.plan_mission_id=pm.id AND ps.stop_type='PICKUP'
			  JOIN commerce_fulfilment_units u ON u.id=ps.unit_id AND u.vendor_order_id=$2
			 WHERE g.parent_order_id=$3`, m.PlanMissionRef, order.ID, m.ParentOrderID).Scan(&binding.StopRef, &planRef, &binding.ManifestVersion)
		if err != nil {
			return handover.Binding{}, handover.ErrUnavailable
		}
		binding.ManifestRef = planRef + "-" + binding.StopRef
	}
	return binding, nil
}

func (s *Service) ReadHandover(ctx context.Context, actor Actor, orderID, missionID, stage string) (handover.Observation, error) {
	b, err := s.handoverBinding(ctx, actor, orderID, missionID, false)
	if err != nil {
		return handover.Observation{}, err
	}
	o, err := s.handover.Read(ctx, b, stage, actor.CorrelationID)
	if err != nil {
		return handover.Observation{}, err
	}
	return o, s.observeHandover(ctx, o)
}

func (s *Service) ConfirmPickupHandover(ctx context.Context, actor Actor, orderID, missionID string, input handover.Confirmation) (handover.Observation, error) {
	b, err := s.handoverBinding(ctx, actor, orderID, missionID, true)
	if err != nil {
		return handover.Observation{}, err
	}
	input.Binding = b
	input.CorrelationID = actor.CorrelationID
	// Namespace the caller's operation by physical mission and actor; no
	// credential or credential-derived fingerprint is persisted by Vendor.
	if strings.TrimSpace(input.OperationRef) == "" || len(input.OperationRef) > 128 {
		return handover.Observation{}, handover.ErrInvalid
	}
	input.OperationRef = "vendor-pickup:" + b.FulfilmentRef + ":" + actor.StaffID + ":" + input.OperationRef
	o, err := s.handover.ConfirmPickup(ctx, input)
	if err != nil {
		return handover.Observation{}, err
	}
	return o, s.observeHandover(ctx, o)
}

func (s *Service) observeHandover(ctx context.Context, o handover.Observation) error {
	// Authority state is observed, not interpreted as a local custody command.
	// Existing authenticated logistics events continue to own order progress.
	missionColumn := "mission_id"
	if o.Authority == "VENDOR_OWN_DELIVERY" {
		missionColumn = "vendor_mission_id"
	}
	tag, err := s.store.db.Exec(ctx, `INSERT INTO commerce_handover_observations
	 (`+missionColumn+`,authority,authority_mission_ref,handover_ref,stage,status,source_version)
	 VALUES($1,$2,$3,$4,$5,$6,$7)
	 ON CONFLICT(authority,handover_ref) DO UPDATE SET status=EXCLUDED.status,
	 source_version=EXCLUDED.source_version,observed_at=now()
	 WHERE COALESCE(commerce_handover_observations.mission_id,commerce_handover_observations.vendor_mission_id)
	       = COALESCE(EXCLUDED.mission_id,EXCLUDED.vendor_mission_id)
	 AND commerce_handover_observations.stage=EXCLUDED.stage
	 AND commerce_handover_observations.authority_mission_ref=EXCLUDED.authority_mission_ref
	 AND commerce_handover_observations.source_version<EXCLUDED.source_version`, o.FulfilmentRef, o.Authority, o.MissionRef, o.HandoverRef, o.Stage, o.Status, o.SourceVersion)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var mission, stage, ref, status string
		var version int64
		err = s.store.db.QueryRow(ctx, `SELECT COALESCE(mission_id,vendor_mission_id),stage,authority_mission_ref,status,source_version FROM commerce_handover_observations WHERE authority=$1 AND handover_ref=$2`, o.Authority, o.HandoverRef).Scan(&mission, &stage, &ref, &status, &version)
		if err != nil {
			return err
		}
		if mission != o.FulfilmentRef || stage != o.Stage || ref != o.MissionRef || (version == o.SourceVersion && status != o.Status) {
			return handover.ErrInvalid
		}
	}
	return nil
}
