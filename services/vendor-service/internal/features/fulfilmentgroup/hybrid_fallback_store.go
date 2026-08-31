package fulfilmentgroup

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const hybridFallbackColumns = `id,operation_ref,request_fingerprint,business_id,branch_id,vendor_mission_id,group_id,source_plan_id,source_plan_version,source_plan_mission_id,COALESCE(target_plan_id,''),COALESCE(target_plan_version,0),COALESCE(target_plan_mission_id,''),COALESCE(platform_mission_ref_id,''),wallet_transition_ref,reason_code,state,failure_reason,version,created_at,updated_at,completed_at`

func scanHybridFallback(row pgx.Row) (HybridFallback, error) {
	var v HybridFallback
	err := row.Scan(&v.ID, &v.OperationRef, &v.RequestFingerprint, &v.BusinessID, &v.BranchID, &v.VendorMissionID, &v.GroupID, &v.SourcePlanID, &v.SourcePlanVersion, &v.SourcePlanMissionID, &v.TargetPlanID, &v.TargetPlanVersion, &v.TargetPlanMissionID, &v.PlatformMissionRefID, &v.WalletTransitionRef, &v.ReasonCode, &v.State, &v.FailureReason, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.CompletedAt)
	return v, err
}

func (s *Store) EnsureHybridFallback(ctx context.Context, businessID, actorStaffID, missionID string, request HybridFallbackRequest, now time.Time) (HybridFallback, error) {
	fingerprint := actionFingerprint(request)
	tx, err := s.begin(ctx)
	if err != nil {
		return HybridFallback{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	existing, loadErr := scanHybridFallback(tx.QueryRow(ctx, `SELECT `+hybridFallbackColumns+` FROM commerce_vendor_hybrid_fallbacks WHERE operation_ref=$1 FOR UPDATE`, request.OperationRef))
	if loadErr == nil {
		if existing.RequestFingerprint != fingerprint || existing.VendorMissionID != missionID {
			return HybridFallback{}, ErrConflict
		}
		return existing, tx.Commit(ctx)
	}
	if !errors.Is(loadErr, pgx.ErrNoRows) {
		return HybridFallback{}, loadErr
	}
	mission, err := scanVendorMission(tx.QueryRow(ctx, `SELECT `+vendorMissionColumns+` FROM commerce_vendor_missions WHERE id=$1 FOR UPDATE`, missionID))
	if errors.Is(err, pgx.ErrNoRows) || mission.BusinessID != businessID {
		return HybridFallback{}, ErrNotFound
	}
	if err != nil {
		return HybridFallback{}, err
	}
	// Resolve a simultaneous same-key fallback after the mission-lock wait.
	existing, loadErr = scanHybridFallback(tx.QueryRow(ctx, `SELECT `+hybridFallbackColumns+` FROM commerce_vendor_hybrid_fallbacks WHERE operation_ref=$1`, request.OperationRef))
	if loadErr == nil {
		if existing.RequestFingerprint != fingerprint || existing.VendorMissionID != missionID {
			return HybridFallback{}, ErrConflict
		}
		return existing, tx.Commit(ctx)
	}
	if !errors.Is(loadErr, pgx.ErrNoRows) {
		return HybridFallback{}, loadErr
	}
	if mission.Version != request.ExpectedVersion {
		return HybridFallback{}, ErrVersionConflict
	}
	if mission.Status.InCustody() || mission.Status == VendorMissionOutForDelivery || mission.Status == VendorMissionDelivered || mission.Status == VendorMissionReturnRequired || mission.Status == VendorMissionReturned {
		return HybridFallback{}, ErrConflict
	}
	var custodyCount int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM commerce_parcel_custody_events WHERE vendor_mission_id=$1 AND event_type='HANDED_OVER'`, missionID).Scan(&custodyCount); err != nil {
		return HybridFallback{}, err
	}
	if custodyCount > 0 {
		return HybridFallback{}, ErrConflict
	}
	var sourceVersion int64
	if err = tx.QueryRow(ctx, `SELECT version FROM commerce_fulfilment_plan_versions WHERE id=$1 AND group_id=$2 AND status='ACTIVE' FOR UPDATE`, mission.PlanID, mission.GroupID).Scan(&sourceVersion); errors.Is(err, pgx.ErrNoRows) {
		return HybridFallback{}, ErrStalePlan
	}
	if err != nil {
		return HybridFallback{}, err
	}
	id := "cvhf_" + uuid.NewString()
	existing, err = scanHybridFallback(tx.QueryRow(ctx, `INSERT INTO commerce_vendor_hybrid_fallbacks(id,operation_ref,request_fingerprint,business_id,branch_id,vendor_mission_id,group_id,source_plan_id,source_plan_version,source_plan_mission_id,reason_code,state,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'STARTED',$12,$12) RETURNING `+hybridFallbackColumns, id, request.OperationRef, fingerprint, businessID, mission.BranchID, missionID, mission.GroupID, mission.PlanID, sourceVersion, mission.PlanMissionID, request.ReasonCode, now))
	if err != nil {
		return HybridFallback{}, translateConstraint(err)
	}
	_, err = tx.Exec(ctx, `UPDATE commerce_vendor_mission_runtime SET stage='FALLBACK_STARTED',version=version+1,updated_at=$2 WHERE vendor_mission_id=$1`, missionID, now)
	if err != nil {
		return HybridFallback{}, err
	}
	return existing, tx.Commit(ctx)
}

func (s *Store) MarkFallbackPlan(ctx context.Context, id string, plan Plan, mission PlanMission, now time.Time) (HybridFallback, error) {
	v, err := scanHybridFallback(s.db.QueryRow(ctx, `UPDATE commerce_vendor_hybrid_fallbacks SET target_plan_id=$2,target_plan_version=$3,target_plan_mission_id=$4,state='PLAN_ACTIVATED',version=version+1,updated_at=$5 WHERE id=$1 AND state IN ('STARTED','PLAN_ACTIVATED') RETURNING `+hybridFallbackColumns, id, plan.ID, plan.Version, mission.ID, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrConflict
	}
	return v, err
}
func (s *Store) MarkFallbackFunding(ctx context.Context, id, refs string, now time.Time) (HybridFallback, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return HybridFallback{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	v, err := scanHybridFallback(tx.QueryRow(ctx, `UPDATE commerce_vendor_hybrid_fallbacks SET wallet_transition_ref=$2,state='FUNDING_TRANSITIONED',version=version+1,updated_at=$3 WHERE id=$1 AND state IN ('PLAN_ACTIVATED','FUNDING_TRANSITIONED') RETURNING `+hybridFallbackColumns, id, refs, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrConflict
	}
	if err != nil {
		return v, err
	}
	_, err = tx.Exec(ctx, `UPDATE commerce_vendor_orders vo SET
		effective_delivery_responsibility='PLATFORM_LOGISTICS',
		effective_delivery_fee_beneficiary='PLATFORM_PROVIDER',
		effective_logistics_mission_ref=pm.slot_ref,version=vo.version+1,updated_at=$2
		FROM commerce_fulfilment_plan_stops st
		JOIN commerce_fulfilment_units u ON u.id=st.unit_id
		JOIN commerce_fulfilment_plan_missions pm ON pm.id=st.plan_mission_id
		WHERE st.plan_mission_id=$1 AND st.stop_type='PICKUP' AND vo.id=u.vendor_order_id`, v.TargetPlanMissionID, now)
	if err != nil {
		return v, err
	}
	return v, tx.Commit(ctx)
}
func (s *Store) CompleteHybridFallback(ctx context.Context, id string, result FallbackMissionResult, now time.Time) (HybridFallback, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return HybridFallback{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	fallback, err := scanHybridFallback(tx.QueryRow(ctx, `SELECT `+hybridFallbackColumns+` FROM commerce_vendor_hybrid_fallbacks WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return fallback, err
	}
	if fallback.State == FallbackCompleted {
		return fallback, tx.Commit(ctx)
	}
	if result.PlatformMissionRefID == "" || result.ExternalRef == "" {
		return fallback, ErrDependencyMissing
	}
	fallback, err = scanHybridFallback(tx.QueryRow(ctx, `UPDATE commerce_vendor_hybrid_fallbacks SET platform_mission_ref_id=$2,state='COMPLETED',version=version+1,updated_at=$3,completed_at=$3 WHERE id=$1 RETURNING `+hybridFallbackColumns, id, result.PlatformMissionRefID, now))
	if err != nil {
		return fallback, err
	}
	var staffID string
	_ = tx.QueryRow(ctx, `SELECT COALESCE(assigned_staff_id,'') FROM commerce_vendor_missions WHERE id=$1`, fallback.VendorMissionID).Scan(&staffID)
	_, err = tx.Exec(ctx, `UPDATE commerce_vendor_missions SET status='FAILED',failure_reason='HYBRID_FALLBACK:'||$2,version=version+1,updated_at=$3 WHERE id=$1 AND status IN ('PLANNED','ASSIGNED','READY_FOR_PICKUP')`, fallback.VendorMissionID, fallback.ReasonCode, now)
	if err != nil {
		return fallback, err
	}
	_, err = tx.Exec(ctx, `UPDATE commerce_vendor_mission_runtime SET stage='FALLBACK_COMPLETED',version=version+1,updated_at=$2 WHERE vendor_mission_id=$1`, fallback.VendorMissionID, now)
	if err != nil {
		return fallback, err
	}
	_, err = tx.Exec(ctx, `UPDATE commerce_vendor_mission_assignments SET state='RELEASED',ended_at=$2,end_reason='HYBRID_FALLBACK' WHERE vendor_mission_id=$1 AND state='ACTIVE'`, fallback.VendorMissionID, now)
	if err != nil {
		return fallback, err
	}
	if staffID != "" {
		_, err = tx.Exec(ctx, `UPDATE commerce_vendor_rider_availability SET state='AVAILABLE',current_vendor_mission_id=NULL,version=version+1,updated_at=$2 WHERE staff_id=$1 AND current_vendor_mission_id=$3`, staffID, now, fallback.VendorMissionID)
		if err != nil {
			return fallback, err
		}
	}
	if err = s.publish(ctx, tx, "vendor.delivery.fallback_completed", fallback.ID, fallback.Version, fallback.BusinessID, map[string]string{"vendor_mission_id": fallback.VendorMissionID, "platform_mission_ref_id": result.PlatformMissionRefID, "operation_ref": fallback.OperationRef}); err != nil {
		return fallback, err
	}
	return fallback, tx.Commit(ctx)
}
func (s *Store) GetHybridFallback(ctx context.Context, operationRef string) (HybridFallback, error) {
	v, err := scanHybridFallback(s.db.QueryRow(ctx, `SELECT `+hybridFallbackColumns+` FROM commerce_vendor_hybrid_fallbacks WHERE operation_ref=$1`, operationRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}
func (s *Store) VendorOrdersForMission(ctx context.Context, planMissionID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT DISTINCT u.vendor_order_id FROM commerce_fulfilment_plan_stops st JOIN commerce_fulfilment_units u ON u.id=st.unit_id WHERE st.plan_mission_id=$1 AND st.stop_type='PICKUP' ORDER BY u.vendor_order_id`, planMissionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
