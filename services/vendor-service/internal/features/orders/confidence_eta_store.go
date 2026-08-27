package orders

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// DeliveryETA is the bounded public/read-model shape. Internal source
// snapshots remain in the immutable history table and are not exposed here.
type DeliveryETA struct {
	ID                string         `json:"eta_id"`
	ParentOrderID     string         `json:"parent_order_ref"`
	FulfilmentGroupID string         `json:"fulfilment_group_ref,omitempty"`
	MissionSlotRef    string         `json:"mission_ref"`
	PlanVersion       int64          `json:"fulfilment_plan_version"`
	ETAVersion        int64          `json:"eta_version"`
	Status            ETAStatus      `json:"status"`
	EarliestAt        *time.Time     `json:"earliest_at,omitempty"`
	LatestAt          *time.Time     `json:"latest_at,omitempty"`
	ActualAt          *time.Time     `json:"actual_at,omitempty"`
	Confidence        ETAConfidence  `json:"confidence"`
	PolicyVersion     string         `json:"policy_version"`
	Trigger           ETATrigger     `json:"trigger"`
	PromisedLatestAt  *time.Time     `json:"promised_latest_at,omitempty"`
	DelayState        ETADelayState  `json:"delay_state"`
	Components        []ETAComponent `json:"components,omitempty"`
	SourceVersion     int64          `json:"source_version"`
	SourceAuthority   string         `json:"source_authority"`
	SourceOccurredAt  time.Time      `json:"source_occurred_at"`
	CreatedAt         time.Time      `json:"created_at"`
}

const etaReadColumns = `e.id,e.parent_order_id,COALESCE(e.fulfilment_group_id,''),e.mission_slot_ref,
	e.plan_version,e.eta_version,e.status,e.earliest_at,e.latest_at,e.actual_at,e.confidence,
	e.policy_version,e.trigger_code,e.promised_latest_at,e.delay_state,e.components,
	e.source_version,e.source_authority,e.source_occurred_at,e.created_at`

const etaVersionColumns = `id,parent_order_id,COALESCE(fulfilment_group_id,''),mission_slot_ref,
	plan_version,eta_version,status,earliest_at,latest_at,actual_at,confidence,
	policy_version,trigger_code,promised_latest_at,delay_state,components,
	source_version,source_authority,source_occurred_at,created_at`

func scanDeliveryETA(row scanner) (DeliveryETA, error) {
	var out DeliveryETA
	var components []byte
	err := row.Scan(&out.ID, &out.ParentOrderID, &out.FulfilmentGroupID, &out.MissionSlotRef,
		&out.PlanVersion, &out.ETAVersion, &out.Status, &out.EarliestAt, &out.LatestAt,
		&out.ActualAt, &out.Confidence, &out.PolicyVersion, &out.Trigger,
		&out.PromisedLatestAt, &out.DelayState, &components, &out.SourceVersion, &out.SourceAuthority,
		&out.SourceOccurredAt, &out.CreatedAt)
	if err == nil {
		err = json.Unmarshal(components, &out.Components)
	}
	return out, err
}

// applyConfidenceETATx appends one immutable version and moves the current
// pointer in the same transaction as the source event inbox/order projection.
func (s *Store) applyConfidenceETATx(ctx context.Context, tx pgx.Tx, in ETAInput) (DeliveryETA, bool, error) {
	// A current-pointer row does not exist for the first version, so FOR UPDATE
	// alone cannot serialize two simultaneous first calculations. A stable
	// transaction advisory lock closes that gap without creating lock rows or a
	// second authority table.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, in.ParentOrderID+":"+in.MissionSlotRef); err != nil {
		return DeliveryETA{}, false, err
	}
	duplicate, duplicateErr := scanDeliveryETA(tx.QueryRow(ctx, `SELECT `+etaReadColumns+`
		FROM commerce_delivery_eta_versions e WHERE parent_order_id=$1 AND mission_slot_ref=$2 AND source_event_id=$3`,
		in.ParentOrderID, in.MissionSlotRef, in.SourceEventID))
	if duplicateErr == nil {
		return duplicate, false, nil
	}
	if !errors.Is(duplicateErr, pgx.ErrNoRows) {
		return DeliveryETA{}, false, duplicateErr
	}
	var previous DeliveryETA
	var currentProvider, currentAuthority string
	err := tx.QueryRow(ctx, `SELECT `+etaReadColumns+`,c.provider_ref,c.source_authority
		FROM commerce_delivery_eta_current c JOIN commerce_delivery_eta_versions e ON e.id=c.eta_id
		WHERE c.parent_order_id=$1 AND c.mission_slot_ref=$2 FOR UPDATE OF c`, in.ParentOrderID, in.MissionSlotRef).
		Scan(&previous.ID, &previous.ParentOrderID, &previous.FulfilmentGroupID, &previous.MissionSlotRef,
			&previous.PlanVersion, &previous.ETAVersion, &previous.Status, &previous.EarliestAt,
			&previous.LatestAt, &previous.ActualAt, &previous.Confidence, &previous.PolicyVersion,
			&previous.Trigger, &previous.PromisedLatestAt, &previous.DelayState, new([]byte),
			&previous.SourceVersion, &previous.SourceAuthority, &previous.SourceOccurredAt, &previous.CreatedAt, &currentProvider, &currentAuthority)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return DeliveryETA{}, false, err
	}
	if err == nil {
		if in.PlanVersion < previous.PlanVersion || (in.PlanVersion == previous.PlanVersion && currentAuthority == in.SourceAuthority && in.SourceVersion < previous.SourceVersion) {
			return previous, false, nil
		}
		if currentProvider != "" && in.ProviderRef != "" && currentProvider != in.ProviderRef && in.Trigger != TriggerProviderReassigned {
			return previous, false, nil
		}
		in.PreviousDelay, in.PreviousLatest = previous.DelayState, previous.LatestAt
	}
	calculation, err := CalculateConfidenceETA(in)
	if err != nil {
		return DeliveryETA{}, false, err
	}
	// Semantic duplicate: a different at-least-once source envelope with the
	// same authoritative inputs does not manufacture another ETA version.
	existing, getErr := scanDeliveryETA(tx.QueryRow(ctx, `SELECT `+etaReadColumns+`
		FROM commerce_delivery_eta_versions e WHERE parent_order_id=$1 AND mission_slot_ref=$2 AND input_fingerprint=$3`,
		in.ParentOrderID, in.MissionSlotRef, calculation.InputFingerprint))
	if getErr == nil {
		return existing, false, nil
	}
	if !errors.Is(getErr, pgx.ErrNoRows) {
		return DeliveryETA{}, false, getErr
	}

	nextVersion := previous.ETAVersion + 1
	if nextVersion < 1 {
		nextVersion = 1
	}
	components, _ := json.Marshal(calculation.Components)
	source, _ := json.Marshal(map[string]any{
		"source_event_id": in.SourceEventID, "source_authority": in.SourceAuthority, "source_version": in.SourceVersion,
		"source_occurred_at": in.SourceOccurredAt, "mission_state": in.MissionState,
		"mode": in.Mode, "provider_ref": in.ProviderRef,
		"route_provider": in.RouteProvider, "route_evidence_version": in.RouteEvidenceVersion,
		"route_observed_at": in.RouteObservedAt, "route_seconds": in.RouteSeconds,
		"pickup_stop_count": in.PickupStopCount, "completed_pickup_stops": in.CompletedPickupStops,
		"promised_latest": in.PromisedLatest, "ready_from": in.ReadyFrom, "ready_to": in.ReadyTo,
		"actual_ready": in.ActualReady, "provider_assigned": in.ProviderAssigned,
		"provider_arrived": in.ProviderArrived, "picked_up": in.PickedUp,
		"operational_exception": in.OperationalException, "domain": in.Domain,
	})
	id := "ceta_" + uuid.NewString()
	createdAt := time.Now().UTC()
	var out DeliveryETA
	var persistedComponents []byte
	err = tx.QueryRow(ctx, `INSERT INTO commerce_delivery_eta_versions(
		id,parent_order_id,fulfilment_group_id,mission_slot_ref,plan_version,provider_ref,
		eta_version,status,earliest_at,latest_at,actual_at,confidence,policy_version,
		trigger_code,promised_latest_at,delay_state,components,source_snapshot,input_fingerprint,
		source_event_id,source_authority,source_version,source_occurred_at,created_at)
		VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
		RETURNING `+etaVersionColumns, id, in.ParentOrderID, in.FulfilmentGroupID, in.MissionSlotRef,
		in.PlanVersion, in.ProviderRef, nextVersion, calculation.Status, calculation.EarliestAt,
		calculation.LatestAt, calculation.ActualAt, calculation.Confidence, calculation.PolicyVersion,
		in.Trigger, in.PromisedLatest, calculation.DelayState, components, source,
		calculation.InputFingerprint, in.SourceEventID, in.SourceAuthority, in.SourceVersion, in.SourceOccurredAt, createdAt).
		Scan(&out.ID, &out.ParentOrderID, &out.FulfilmentGroupID, &out.MissionSlotRef,
			&out.PlanVersion, &out.ETAVersion, &out.Status, &out.EarliestAt, &out.LatestAt,
			&out.ActualAt, &out.Confidence, &out.PolicyVersion, &out.Trigger,
			&out.PromisedLatestAt, &out.DelayState, &persistedComponents, &out.SourceVersion, &out.SourceAuthority,
			&out.SourceOccurredAt, &out.CreatedAt)
	if err != nil {
		return DeliveryETA{}, false, err
	}
	if err = json.Unmarshal(persistedComponents, &out.Components); err != nil {
		return DeliveryETA{}, false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO commerce_delivery_eta_current(
		parent_order_id,mission_slot_ref,eta_id,eta_version,plan_version,provider_ref,source_authority,source_version,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT(parent_order_id,mission_slot_ref) DO UPDATE SET
		eta_id=EXCLUDED.eta_id,eta_version=EXCLUDED.eta_version,plan_version=EXCLUDED.plan_version,
		provider_ref=EXCLUDED.provider_ref,source_authority=EXCLUDED.source_authority,source_version=EXCLUDED.source_version,updated_at=EXCLUDED.updated_at
		WHERE commerce_delivery_eta_current.plan_version < EXCLUDED.plan_version OR
		 (commerce_delivery_eta_current.plan_version = EXCLUDED.plan_version AND
		  (commerce_delivery_eta_current.source_authority <> EXCLUDED.source_authority OR
		   commerce_delivery_eta_current.source_version <= EXCLUDED.source_version))`,
		in.ParentOrderID, in.MissionSlotRef, out.ID, out.ETAVersion, in.PlanVersion,
		in.ProviderRef, in.SourceAuthority, in.SourceVersion, createdAt)
	if err != nil {
		return DeliveryETA{}, false, err
	}
	if s.outbox != nil {
		var customerRef string
		if err = tx.QueryRow(ctx, `SELECT customer_id FROM commerce_parent_orders WHERE id=$1`, in.ParentOrderID).Scan(&customerRef); err != nil {
			return DeliveryETA{}, false, err
		}
		payload := map[string]any{"customer_ref": customerRef, "parent_order_ref": in.ParentOrderID,
			"mission_ref": in.MissionSlotRef, "mission_slot_ref": in.MissionSlotRef,
			"eta_version": out.ETAVersion, "earliest_at": out.EarliestAt, "latest_at": out.LatestAt,
			"confidence": out.Confidence, "delay_state": out.DelayState, "source_version": in.SourceVersion}
		etaAggregateID := in.ParentOrderID + ":" + in.MissionSlotRef
		if err = s.outbox.Publish(ctx, tx, "commerce.eta.updated", "CommerceDeliveryMissionETA", etaAggregateID,
			out.ETAVersion, "", in.SourceEventID, in.SourceEventID, payload); err != nil {
			return DeliveryETA{}, false, err
		}
		if calculation.NotifyDelay {
			if err = s.outbox.Publish(ctx, tx, "commerce.delivery.delay_detected", "CommerceDeliveryMissionETA", etaAggregateID,
				out.ETAVersion, "", in.SourceEventID, in.SourceEventID, payload); err != nil {
				return DeliveryETA{}, false, err
			}
		}
	}
	return out, true, nil
}

func (s *Store) ListCurrentDeliveryETAForCustomer(ctx context.Context, customerID, parentOrderID string) ([]DeliveryETA, error) {
	rows, err := s.db.Query(ctx, `SELECT `+etaReadColumns+` FROM commerce_delivery_eta_current c
		JOIN commerce_delivery_eta_versions e ON e.id=c.eta_id
		JOIN commerce_parent_orders p ON p.id=e.parent_order_id
		WHERE p.customer_id=$1 AND p.id=$2 ORDER BY e.mission_slot_ref`, customerID, parentOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []DeliveryETA{}
	for rows.Next() {
		item, scanErr := scanDeliveryETA(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) GetCurrentDeliveryETAForVendorOrder(ctx context.Context, businessID, vendorOrderID string) (*DeliveryETA, error) {
	item, err := scanDeliveryETA(s.db.QueryRow(ctx, `SELECT `+etaReadColumns+`
		FROM commerce_delivery_eta_current c JOIN commerce_delivery_eta_versions e ON e.id=c.eta_id
		JOIN commerce_vendor_orders vo ON vo.parent_order_id=e.parent_order_id
		JOIN commerce_fulfilment_groups g ON g.parent_order_id=e.parent_order_id
		JOIN commerce_fulfilment_plan_versions pv ON pv.id=g.current_plan_id
		JOIN commerce_fulfilment_plan_missions pm ON pm.plan_id=pv.id AND pm.slot_ref=e.mission_slot_ref
		JOIN commerce_fulfilment_plan_mission_parcels mp ON mp.plan_mission_id=pm.id
		JOIN commerce_fulfilment_parcels fp ON fp.id=mp.parcel_id AND fp.vendor_order_id=vo.id
		WHERE vo.business_id=$1 AND vo.id=$2 LIMIT 1`, businessID, vendorOrderID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &item, err
}
