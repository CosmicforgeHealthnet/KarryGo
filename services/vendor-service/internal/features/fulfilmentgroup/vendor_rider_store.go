package fulfilmentgroup

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const riderAvailabilityColumns = `staff_id,business_id,branch_id,state,COALESCE(current_vendor_mission_id,''),version,updated_at`

func scanRiderAvailability(row pgx.Row) (RiderAvailability, error) {
	var value RiderAvailability
	err := row.Scan(&value.StaffID, &value.BusinessID, &value.BranchID, &value.State, &value.CurrentVendorMissionID, &value.Version, &value.UpdatedAt)
	return value, err
}

func (s *Store) SetRiderAvailability(ctx context.Context, businessID, staffID, branchID string, state RiderAvailabilityState, expectedVersion int64, operationRef string, now time.Time) (RiderAvailability, error) {
	if !s.Ready() {
		return RiderAvailability{}, ErrDependencyMissing
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return RiderAvailability{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	fingerprint := actionFingerprint(map[string]any{"staff_id": staffID, "branch_id": branchID, "state": state, "expected_version": expectedVersion})
	var priorFingerprint string
	loadOperationErr := tx.QueryRow(ctx, `SELECT request_fingerprint FROM commerce_vendor_rider_availability_operations WHERE business_id=$1 AND operation_ref=$2`, businessID, operationRef).Scan(&priorFingerprint)
	if loadOperationErr == nil {
		if priorFingerprint != fingerprint {
			return RiderAvailability{}, ErrConflict
		}
		value, readErr := scanRiderAvailability(tx.QueryRow(ctx, `SELECT `+riderAvailabilityColumns+` FROM commerce_vendor_rider_availability WHERE staff_id=$1`, staffID))
		if readErr != nil {
			return RiderAvailability{}, readErr
		}
		return value, tx.Commit(ctx)
	}
	if !errors.Is(loadOperationErr, pgx.ErrNoRows) {
		return RiderAvailability{}, loadOperationErr
	}
	if err = assertEligibleRider(ctx, tx, businessID, staffID, branchID); err != nil {
		return RiderAvailability{}, err
	}
	existing, loadErr := scanRiderAvailability(tx.QueryRow(ctx, `SELECT `+riderAvailabilityColumns+` FROM commerce_vendor_rider_availability WHERE staff_id=$1 FOR UPDATE`, staffID))
	if errors.Is(loadErr, pgx.ErrNoRows) {
		if expectedVersion != 0 {
			return RiderAvailability{}, ErrVersionConflict
		}
		existing, err = scanRiderAvailability(tx.QueryRow(ctx, `INSERT INTO commerce_vendor_rider_availability(staff_id,business_id,branch_id,state,version,updated_at) VALUES($1,$2,$3,$4,1,$5) RETURNING `+riderAvailabilityColumns, staffID, businessID, branchID, state, now))
		if err != nil {
			return RiderAvailability{}, translateConstraint(err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO commerce_vendor_rider_availability_operations(id,staff_id,business_id,operation_ref,request_fingerprint,result_version) VALUES($1,$2,$3,$4,$5,$6)`, "cvrao_"+uuid.NewString(), staffID, businessID, operationRef, fingerprint, existing.Version); err != nil {
			return RiderAvailability{}, translateConstraint(err)
		}
		return existing, tx.Commit(ctx)
	}
	if loadErr != nil {
		return RiderAvailability{}, loadErr
	}
	if existing.BusinessID != businessID {
		return RiderAvailability{}, ErrNotFound
	}
	if expectedVersion != existing.Version {
		return RiderAvailability{}, ErrVersionConflict
	}
	if existing.State == RiderBusy || existing.CurrentVendorMissionID != "" {
		return RiderAvailability{}, ErrConflict
	}
	existing, err = scanRiderAvailability(tx.QueryRow(ctx, `UPDATE commerce_vendor_rider_availability SET branch_id=$2,state=$3,version=version+1,updated_at=$4 WHERE staff_id=$1 AND version=$5 RETURNING `+riderAvailabilityColumns, staffID, branchID, state, now, expectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return RiderAvailability{}, ErrVersionConflict
	}
	if err != nil {
		return RiderAvailability{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_vendor_rider_availability_operations(id,staff_id,business_id,operation_ref,request_fingerprint,result_version) VALUES($1,$2,$3,$4,$5,$6)`, "cvrao_"+uuid.NewString(), staffID, businessID, operationRef, fingerprint, existing.Version); err != nil {
		return RiderAvailability{}, translateConstraint(err)
	}
	return existing, tx.Commit(ctx)
}

func (s *Store) GetRiderAvailability(ctx context.Context, businessID, staffID string) (RiderAvailability, error) {
	v, err := scanRiderAvailability(s.db.QueryRow(ctx, `SELECT `+riderAvailabilityColumns+` FROM commerce_vendor_rider_availability WHERE staff_id=$1 AND business_id=$2`, staffID, businessID))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}

func assertEligibleRider(ctx context.Context, q querier, businessID, staffID, branchID string) error {
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM vendor_staff s
		JOIN vendor_staff_role_assignments a ON a.staff_id=s.id AND a.revoked_at IS NULL
		WHERE s.id=$1 AND s.vendor_id=$2 AND s.status='active' AND a.role_id='role_vendor_rider'
		AND (a.branch_id IS NULL OR a.branch_id=$3))`, staffID, businessID, branchID).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}

func (s *Store) AssignEligibleVendorRider(ctx context.Context, businessID, missionID, staffID, assignedBy, operationRef string, expectedVersion int64, now time.Time) (VendorMission, error) {
	if !s.Ready() {
		return VendorMission{}, ErrDependencyMissing
	}
	if staffID == "" || assignedBy == "" || operationRef == "" {
		return VendorMission{}, ErrInvalid
	}
	fingerprint := actionFingerprint(map[string]any{"mission_id": missionID, "staff_id": staffID, "expected_version": expectedVersion})
	tx, err := s.begin(ctx)
	if err != nil {
		return VendorMission{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var previousFingerprint, previousMission, previousStaff string
	err = tx.QueryRow(ctx, `SELECT request_fingerprint,vendor_mission_id,staff_id FROM commerce_vendor_mission_assignments WHERE business_id=$1 AND operation_ref=$2`, businessID, operationRef).Scan(&previousFingerprint, &previousMission, &previousStaff)
	if err == nil {
		if previousFingerprint != fingerprint || previousMission != missionID || previousStaff != staffID {
			return VendorMission{}, ErrConflict
		}
		mission, getErr := scanVendorMission(tx.QueryRow(ctx, `SELECT `+vendorMissionColumns+` FROM commerce_vendor_missions WHERE id=$1`, missionID))
		if getErr != nil {
			return VendorMission{}, getErr
		}
		return mission, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return VendorMission{}, err
	}
	mission, err := scanVendorMission(tx.QueryRow(ctx, `SELECT `+vendorMissionColumns+` FROM commerce_vendor_missions WHERE id=$1 FOR UPDATE`, missionID))
	if errors.Is(err, pgx.ErrNoRows) || mission.BusinessID != businessID {
		return VendorMission{}, ErrNotFound
	}
	if err != nil {
		return VendorMission{}, err
	}
	// The same operation may have committed while this transaction waited for
	// the mission lock. Read it back before treating the now-advanced mission
	// version as a conflicting assignment.
	err = tx.QueryRow(ctx, `SELECT request_fingerprint,vendor_mission_id,staff_id FROM commerce_vendor_mission_assignments WHERE business_id=$1 AND operation_ref=$2`, businessID, operationRef).Scan(&previousFingerprint, &previousMission, &previousStaff)
	if err == nil {
		if previousFingerprint != fingerprint || previousMission != missionID || previousStaff != staffID {
			return VendorMission{}, ErrConflict
		}
		return mission, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return VendorMission{}, err
	}
	if mission.Version != expectedVersion {
		return VendorMission{}, ErrVersionConflict
	}
	if mission.Status.InCustody() || mission.Status == VendorMissionDelivered || mission.Status == VendorMissionReturned {
		return VendorMission{}, ErrConflict
	}
	if err = assertEligibleRider(ctx, tx, businessID, staffID, mission.BranchID); err != nil {
		return VendorMission{}, err
	}
	availability, err := scanRiderAvailability(tx.QueryRow(ctx, `SELECT `+riderAvailabilityColumns+` FROM commerce_vendor_rider_availability WHERE staff_id=$1 FOR UPDATE`, staffID))
	if errors.Is(err, pgx.ErrNoRows) || availability.BusinessID != businessID || availability.BranchID != mission.BranchID || availability.State != RiderAvailable || availability.CurrentVendorMissionID != "" {
		return VendorMission{}, ErrConflict
	}
	if err != nil {
		return VendorMission{}, err
	}
	if mission.AssignedStaffID != "" && mission.AssignedStaffID != staffID {
		_, err = tx.Exec(ctx, `UPDATE commerce_vendor_mission_assignments SET state='REPLACED',ended_at=$2,end_reason='PRE_CUSTODY_REPLACEMENT' WHERE vendor_mission_id=$1 AND state='ACTIVE'`, missionID, now)
		if err != nil {
			return VendorMission{}, err
		}
		_, err = tx.Exec(ctx, `UPDATE commerce_vendor_rider_availability SET state='AVAILABLE',current_vendor_mission_id=NULL,version=version+1,updated_at=$2 WHERE staff_id=$1 AND current_vendor_mission_id=$3`, mission.AssignedStaffID, now, missionID)
		if err != nil {
			return VendorMission{}, err
		}
	}
	next := mission.Status
	if next == VendorMissionPlanned {
		next = VendorMissionAssigned
	}
	mission, err = scanVendorMission(tx.QueryRow(ctx, `UPDATE commerce_vendor_missions SET assigned_staff_id=$1,assigned_at=$2,status=$3,version=version+1,updated_at=$2 WHERE id=$4 AND version=$5 RETURNING `+vendorMissionColumns, staffID, now, next, missionID, expectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return VendorMission{}, ErrVersionConflict
	}
	if err != nil {
		return VendorMission{}, translateConstraint(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO commerce_vendor_mission_assignments(id,vendor_mission_id,business_id,branch_id,staff_id,assigned_by_staff_id,operation_ref,request_fingerprint,state,assigned_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'ACTIVE',$9)`, `cvma_`+uuid.NewString(), missionID, businessID, mission.BranchID, staffID, assignedBy, operationRef, fingerprint, now)
	if err != nil {
		return VendorMission{}, translateConstraint(err)
	}
	tag, updateErr := tx.Exec(ctx, `UPDATE commerce_vendor_rider_availability SET state='BUSY',current_vendor_mission_id=$2,version=version+1,updated_at=$3 WHERE staff_id=$1 AND state='AVAILABLE' AND current_vendor_mission_id IS NULL`, staffID, missionID, now)
	err = updateErr
	if err != nil {
		return VendorMission{}, err
	}
	if tag.RowsAffected() != 1 {
		return VendorMission{}, ErrConflict
	}
	_, err = tx.Exec(ctx, `INSERT INTO commerce_vendor_mission_runtime(vendor_mission_id,stage,version,updated_at) VALUES($1,'ASSIGNED',1,$2) ON CONFLICT(vendor_mission_id) DO UPDATE SET stage='ASSIGNED',version=commerce_vendor_mission_runtime.version+1,updated_at=$2`, missionID, now)
	if err != nil {
		return VendorMission{}, err
	}
	if err = s.publish(ctx, tx, EventVendorMission, mission.ID, mission.Version, businessID, map[string]string{"status": string(mission.Status), "staff_id": staffID}); err != nil {
		return VendorMission{}, err
	}
	return mission, tx.Commit(ctx)
}

func (s *Store) GetVendorMissionRuntime(ctx context.Context, businessID, missionID string) (VendorMissionRuntime, error) {
	mission, err := s.GetVendorMission(ctx, businessID, missionID)
	if err != nil {
		return VendorMissionRuntime{}, err
	}
	var runtime VendorMissionRuntime
	runtime.Mission = mission
	err = s.db.QueryRow(ctx, `SELECT stage,version,pickup_arrived_at,custody_verified_at,receiver_arrived_at,receiver_unavailable_reason FROM commerce_vendor_mission_runtime WHERE vendor_mission_id=$1`, missionID).Scan(&runtime.Stage, &runtime.RuntimeVersion, &runtime.PickupArrivedAt, &runtime.CustodyVerifiedAt, &runtime.ReceiverArrivedAt, &runtime.ReceiverUnavailableReason)
	if errors.Is(err, pgx.ErrNoRows) {
		return VendorMissionRuntime{}, ErrNotFound
	}
	return runtime, err
}

func (s *Store) GetRiderActionResult(ctx context.Context, businessID, staffID, missionID string, request RiderActionRequest) (VendorMissionRuntime, bool, error) {
	var fingerprint string
	err := s.db.QueryRow(ctx, `SELECT request_fingerprint FROM commerce_vendor_mission_operations WHERE business_id=$1 AND operation_ref=$2 AND vendor_mission_id=$3 AND staff_id=$4`, businessID, request.OperationRef, missionID, staffID).Scan(&fingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return VendorMissionRuntime{}, false, nil
	}
	if err != nil {
		return VendorMissionRuntime{}, false, err
	}
	if fingerprint != actionFingerprint(request) {
		return VendorMissionRuntime{}, false, ErrConflict
	}
	runtime, err := s.GetVendorMissionRuntime(ctx, businessID, missionID)
	return runtime, true, err
}

func (s *Store) VendorOrdersEligibleForRiderProjection(ctx context.Context, planMissionID string, action RiderAction) ([]string, error) {
	condition := "TRUE"
	if action == ActionCompletePickup {
		condition = "NOT EXISTS (SELECT 1 FROM commerce_fulfilment_plan_mission_parcels mp2 JOIN commerce_fulfilment_parcels p2 ON p2.id=mp2.parcel_id WHERE mp2.plan_mission_id=mp.plan_mission_id AND p2.unit_id=u.id AND p2.status<>'IN_TRANSIT')"
	}
	rows, err := s.db.Query(ctx, `SELECT DISTINCT u.vendor_order_id FROM commerce_fulfilment_plan_mission_parcels mp JOIN commerce_fulfilment_parcels p ON p.id=mp.parcel_id JOIN commerce_fulfilment_units u ON u.id=p.unit_id WHERE mp.plan_mission_id=$1 AND `+condition+` ORDER BY u.vendor_order_id`, planMissionID)
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

func (s *Store) ListRiderMissionRuntime(ctx context.Context, businessID, staffID string, limit int) ([]VendorMissionRuntime, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `SELECT m.id FROM commerce_vendor_missions m WHERE m.business_id=$1 AND m.assigned_staff_id=$2 ORDER BY m.updated_at DESC,m.id LIMIT $3`, businessID, staffID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out := make([]VendorMissionRuntime, 0, len(ids))
	for _, id := range ids {
		value, getErr := s.GetVendorMissionRuntime(ctx, businessID, id)
		if getErr != nil {
			return nil, getErr
		}
		out = append(out, value)
	}
	return out, nil
}

var riderActionNext = map[VendorMissionStage]map[RiderAction]VendorMissionStage{
	StageAssigned: {ActionAcknowledge: StageAcknowledged}, StageAcknowledged: {ActionStartPickup: StageEnRoutePickup},
	StageEnRoutePickup: {ActionArrivePickup: StageArrivedPickup}, StageArrivedPickup: {ActionCompletePickup: StageInCustody},
	StageInCustody: {ActionStartReceiver: StageEnRouteReceiver}, StageEnRouteReceiver: {ActionArriveReceiver: StageArrivedReceiver},
	StageArrivedReceiver:     {ActionCompleteDelivery: StageCompleted, ActionReceiverUnavailable: StageReceiverUnavailable},
	StageReceiverUnavailable: {ActionStartReceiver: StageEnRouteReceiver, ActionReturn: StageReturned},
}

func (s *Store) ApplyRiderAction(ctx context.Context, businessID, staffID, missionID string, request RiderActionRequest, now time.Time) (VendorMissionRuntime, error) {
	if request.OperationRef == "" || request.ExpectedVersion < 1 {
		return VendorMissionRuntime{}, ErrInvalid
	}
	fingerprint := actionFingerprint(request)
	evidenceRef, proofRef := "", ""
	if request.verifiedRef != "" {
		evidenceRef, proofRef = request.verifiedRef, request.verifiedProof
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return VendorMissionRuntime{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var oldFingerprint, resultStage string
	err = tx.QueryRow(ctx, `SELECT request_fingerprint,result_stage FROM commerce_vendor_mission_operations WHERE business_id=$1 AND operation_ref=$2`, businessID, request.OperationRef).Scan(&oldFingerprint, &resultStage)
	if err == nil {
		if oldFingerprint != fingerprint {
			return VendorMissionRuntime{}, ErrConflict
		}
		return scanVendorMissionRuntimeTx(ctx, tx, businessID, missionID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return VendorMissionRuntime{}, err
	}
	mission, err := scanVendorMission(tx.QueryRow(ctx, `SELECT `+vendorMissionColumns+` FROM commerce_vendor_missions WHERE id=$1 FOR UPDATE`, missionID))
	if errors.Is(err, pgx.ErrNoRows) || mission.BusinessID != businessID || mission.AssignedStaffID != staffID {
		return VendorMissionRuntime{}, ErrNotFound
	}
	if err != nil {
		return VendorMissionRuntime{}, err
	}
	// Resolve a simultaneous same-key retry after the mission-lock wait.
	err = tx.QueryRow(ctx, `SELECT request_fingerprint,result_stage FROM commerce_vendor_mission_operations WHERE business_id=$1 AND operation_ref=$2`, businessID, request.OperationRef).Scan(&oldFingerprint, &resultStage)
	if err == nil {
		if oldFingerprint != fingerprint {
			return VendorMissionRuntime{}, ErrConflict
		}
		return scanVendorMissionRuntimeTx(ctx, tx, businessID, missionID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return VendorMissionRuntime{}, err
	}
	if mission.Version != request.ExpectedVersion {
		return VendorMissionRuntime{}, ErrVersionConflict
	}
	var stage VendorMissionStage
	var runtimeVersion int64
	err = tx.QueryRow(ctx, `SELECT stage,version FROM commerce_vendor_mission_runtime WHERE vendor_mission_id=$1 FOR UPDATE`, missionID).Scan(&stage, &runtimeVersion)
	if err != nil {
		return VendorMissionRuntime{}, err
	}
	next, ok := riderActionNext[stage][request.Action]
	if !ok {
		return VendorMissionRuntime{}, ErrConflict
	}
	canonical := mission.Status
	switch request.Action {
	case ActionArrivePickup:
		canonical = VendorMissionReadyForPickup
	case ActionCompletePickup:
		canonical = VendorMissionCustodyTransferred
	case ActionStartReceiver:
		if stage == StageInCustody {
			canonical = VendorMissionOutForDelivery
		}
	case ActionCompleteDelivery:
		canonical = VendorMissionDelivered
	case ActionReceiverUnavailable:
		if strings.TrimSpace(request.ReasonCode) == "" {
			return VendorMissionRuntime{}, ErrInvalid
		}
		canonical = VendorMissionReturnRequired
	case ActionReturn:
		canonical = VendorMissionReturned
	}
	if canonical != mission.Status {
		if err = TransitionVendorMission(mission.Status, canonical); err != nil {
			return VendorMissionRuntime{}, err
		}
	}
	parcels, err := missionParcelIDs(ctx, tx, mission.PlanMissionID)
	if err != nil {
		return VendorMissionRuntime{}, err
	}
	selected := parcels
	partialPickup := false
	if request.Action == ActionCompletePickup && len(request.ParcelRefs) > 0 {
		selected = nil
		allowed := map[string]bool{}
		for _, id := range parcels {
			allowed[id] = true
		}
		seen := map[string]bool{}
		for _, id := range request.ParcelRefs {
			if !allowed[id] || seen[id] {
				return VendorMissionRuntime{}, ErrInvalid
			}
			seen[id] = true
			selected = append(selected, id)
		}
		if len(selected) < len(parcels) {
			partialPickup = true
		}
	}
	eventStatus := canonical
	if partialPickup {
		eventStatus = VendorMissionCustodyTransferred
	}
	if eventType, moves := vendorCustodyEventFor(eventStatus); moves && (canonical != mission.Status || partialPickup) {
		for _, parcelID := range selected {
			event := CustodyEvent{ParcelID: parcelID, GroupID: mission.GroupID, EventType: eventType, FromCustodianType: CustodianVendor, FromCustodianRef: businessID, ToCustodianType: vendorCustodianFor(eventType), ToCustodianRef: staffID, Source: SourceVendor, SourceEventID: fmt.Sprintf("rider-action-%s-%s", request.OperationRef, parcelID), SourceVersion: mission.Version + 1, VendorMissionID: missionID, HandoverRef: evidenceRef, ProofRef: proofRef, ReasonCode: request.ReasonCode, OccurredAt: now}
			if err = recordCustodyTx(ctx, tx, event, now); err != nil {
				return VendorMissionRuntime{}, err
			}
		}
	}
	if partialPickup {
		var remaining int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM commerce_fulfilment_plan_mission_parcels mp JOIN commerce_fulfilment_parcels p ON p.id=mp.parcel_id WHERE mp.plan_mission_id=$1 AND p.status<>'IN_TRANSIT'`, mission.PlanMissionID).Scan(&remaining); err != nil {
			return VendorMissionRuntime{}, err
		}
		if remaining > 0 {
			canonical = mission.Status
			next = stage
		}
	}
	mission, err = scanVendorMission(tx.QueryRow(ctx, `UPDATE commerce_vendor_missions SET status=$2,handover_ref=CASE WHEN $3='' THEN handover_ref ELSE $3 END,proof_ref=CASE WHEN $4='' THEN proof_ref ELSE $4 END,failure_reason=$5,version=version+1,updated_at=$6 WHERE id=$1 AND version=$7 RETURNING `+vendorMissionColumns, missionID, canonical, evidenceRef, proofRef, request.ReasonCode, now, request.ExpectedVersion))
	if err != nil {
		return VendorMissionRuntime{}, translateConstraint(err)
	}
	_, err = tx.Exec(ctx, `UPDATE commerce_vendor_mission_runtime SET stage=$2,pickup_arrived_at=CASE WHEN $2='ARRIVED_PICKUP' THEN $3 ELSE pickup_arrived_at END,custody_verified_at=CASE WHEN $2='IN_CUSTODY' THEN $3 ELSE custody_verified_at END,receiver_arrived_at=CASE WHEN $2='ARRIVED_RECEIVER' THEN $3 ELSE receiver_arrived_at END,receiver_unavailable_reason=CASE WHEN $2='RECEIVER_UNAVAILABLE' THEN $4 ELSE receiver_unavailable_reason END,version=version+1,updated_at=$3 WHERE vendor_mission_id=$1`, missionID, next, now, request.ReasonCode)
	if err != nil {
		return VendorMissionRuntime{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO commerce_vendor_mission_operations(id,vendor_mission_id,business_id,staff_id,operation_ref,action,request_fingerprint,result_stage,result_mission_version,evidence_ref) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, `cvmo_`+uuid.NewString(), missionID, businessID, staffID, request.OperationRef, request.Action, fingerprint, next, mission.Version, evidenceRef)
	if err != nil {
		return VendorMissionRuntime{}, translateConstraint(err)
	}
	if next == StageCompleted || next == StageReturned {
		_, err = tx.Exec(ctx, `UPDATE commerce_vendor_mission_assignments SET state='RELEASED',ended_at=$2,end_reason=$3 WHERE vendor_mission_id=$1 AND state='ACTIVE'`, missionID, now, next)
		if err != nil {
			return VendorMissionRuntime{}, err
		}
		_, err = tx.Exec(ctx, `UPDATE commerce_vendor_rider_availability SET state='AVAILABLE',current_vendor_mission_id=NULL,version=version+1,updated_at=$2 WHERE staff_id=$1 AND current_vendor_mission_id=$3`, staffID, now, missionID)
		if err != nil {
			return VendorMissionRuntime{}, err
		}
	}
	if err = s.publish(ctx, tx, EventVendorMission, missionID, mission.Version, businessID, map[string]string{"status": string(mission.Status), "stage": string(next), "staff_id": staffID}); err != nil {
		return VendorMissionRuntime{}, err
	}
	return scanVendorMissionRuntimeTx(ctx, tx, businessID, missionID)
}

func scanVendorMissionRuntimeTx(ctx context.Context, tx pgx.Tx, businessID, missionID string) (VendorMissionRuntime, error) {
	mission, err := scanVendorMission(tx.QueryRow(ctx, `SELECT `+vendorMissionColumns+` FROM commerce_vendor_missions WHERE id=$1 AND business_id=$2`, missionID, businessID))
	if err != nil {
		return VendorMissionRuntime{}, err
	}
	var value VendorMissionRuntime
	value.Mission = mission
	err = tx.QueryRow(ctx, `SELECT stage,version,pickup_arrived_at,custody_verified_at,receiver_arrived_at,receiver_unavailable_reason FROM commerce_vendor_mission_runtime WHERE vendor_mission_id=$1`, missionID).Scan(&value.Stage, &value.RuntimeVersion, &value.PickupArrivedAt, &value.CustodyVerifiedAt, &value.ReceiverArrivedAt, &value.ReceiverUnavailableReason)
	if err != nil {
		return value, err
	}
	return value, tx.Commit(ctx)
}
