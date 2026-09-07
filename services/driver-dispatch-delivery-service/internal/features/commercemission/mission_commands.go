package commercemission

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
)

const (
	commandCancel              = "CANCEL"
	commandReceiverUnavailable = "RECEIVER_UNAVAILABLE"
	commandReschedule          = "RESCHEDULE"
	commandReturn              = "RETURN"
	commandReplaceProvider     = "REPLACE_PROVIDER"
)

func (s *MultiStopMissionService) Cancel(ctx context.Context, caller, missionRef string, request dispatchcommerce.CommandRequest) (dispatchcommerce.Mission, error) {
	return s.command(ctx, caller, missionRef, commandCancel, request)
}
func (s *MultiStopMissionService) Reschedule(ctx context.Context, caller, missionRef string, request dispatchcommerce.CommandRequest) (dispatchcommerce.Mission, error) {
	return s.command(ctx, caller, missionRef, commandReschedule, request)
}
func (s *MultiStopMissionService) RequireReturn(ctx context.Context, caller, missionRef string, request dispatchcommerce.CommandRequest) (dispatchcommerce.Mission, error) {
	return s.command(ctx, caller, missionRef, commandReturn, request)
}
func (s *MultiStopMissionService) ReplaceProvider(ctx context.Context, caller, missionRef string, request dispatchcommerce.CommandRequest) (dispatchcommerce.Mission, error) {
	mission, err := s.command(ctx, caller, missionRef, commandReplaceProvider, request)
	if err != nil {
		return mission, err
	}
	if mission.State == "SEARCHING" || mission.State == "RETRYING" || mission.State == "REPLACEMENT_REQUIRED" {
		if repo, ok := s.store.(*Repository); ok {
			snapshot, fee, currency, readErr := repo.MissionExecutionSnapshot(ctx, caller, missionRef)
			if readErr != nil {
				return dispatchcommerce.Mission{}, readErr
			}
			if err = s.ensureBroadcast(ctx, snapshot, mission, fee, currency, false); err != nil {
				return dispatchcommerce.Mission{}, apperrors.Unavailable("Replacement was committed but marketplace broadcast is unavailable.", err)
			}
		}
	}
	return mission, nil
}
func (s *MultiStopMissionService) ReceiverUnavailable(ctx context.Context, providerID, missionRef string, request dispatchcommerce.CommandRequest) (dispatchcommerce.Mission, error) {
	return s.command(ctx, "provider:"+providerID, missionRef, commandReceiverUnavailable, request)
}

func (s *MultiStopMissionService) command(ctx context.Context, caller, missionRef, kind string, request dispatchcommerce.CommandRequest) (dispatchcommerce.Mission, error) {
	if strings.TrimSpace(request.OperationRef) == "" {
		return dispatchcommerce.Mission{}, apperrors.New(400, "IDEMPOTENCY_KEY_REQUIRED", "operation_ref is required.", nil)
	}
	if kind != commandReceiverUnavailable && caller != dispatchcommerce.SignatureCaller {
		return dispatchcommerce.Mission{}, apperrors.Unauthorized("This caller may not change Commerce missions.", nil)
	}
	fingerprint, _ := dispatchcommerce.Fingerprint(struct {
		Kind, Mission, Reason, Transfer string
		ReadyAt                         *time.Time
		Version                         int64
	}{kind, missionRef, request.ReasonCode, request.TransferEvidenceRef, request.ReadyAt, request.ExpectedVersion})
	repo, ok := s.store.(*Repository)
	if !ok {
		return dispatchcommerce.Mission{}, apperrors.Unavailable("Commerce mission commands are unavailable.", nil)
	}
	if raw, stored, found, err := repo.FindOperation(ctx, caller, request.OperationRef); err != nil {
		return dispatchcommerce.Mission{}, err
	} else if found {
		if stored != fingerprint {
			return dispatchcommerce.Mission{}, apperrors.New(409, "IDEMPOTENCY_CONFLICT", "This operation key was already used for a different command.", nil)
		}
		var view dispatchcommerce.Mission
		if err = json.Unmarshal(raw, &view); err != nil {
			return dispatchcommerce.Mission{}, err
		}
		view.Replayed = true
		return view, nil
	}
	return repo.ApplyCommand(ctx, caller, missionRef, kind, request, fingerprint, s.now().UTC())
}

func (r *Repository) ApplyCommand(ctx context.Context, caller, missionRef, kind string, request dispatchcommerce.CommandRequest, fingerprint string, now time.Time) (dispatchcommerce.Mission, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var record multiStopMissionRecord
	if strings.HasPrefix(caller, "provider:") {
		providerID := strings.TrimPrefix(caller, "provider:")
		record, err = scanMultiStopMission(tx.QueryRow(ctx, `SELECT `+multiStopMissionColumns+` FROM commerce_missions WHERE canonical_ref=$1 AND booking_id IN (SELECT booking_id FROM trips WHERE provider_id=$2::uuid) FOR UPDATE`, missionRef, providerID))
	} else {
		record, err = scanMultiStopMission(tx.QueryRow(ctx, `SELECT `+multiStopMissionColumns+` FROM commerce_missions WHERE caller_service=$1 AND canonical_ref=$2 FOR UPDATE`, caller, missionRef))
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return dispatchcommerce.Mission{}, apperrors.NotFound("Commerce mission not found.", nil)
	}
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if request.ExpectedVersion > 0 && request.ExpectedVersion != record.SourceVersion {
		return dispatchcommerce.Mission{}, apperrors.New(409, "STALE_MISSION_VERSION", "Mission version changed.", nil)
	}
	if record.RuntimeState == "COMPLETED" {
		return dispatchcommerce.Mission{}, apperrors.New(409, "MISSION_ALREADY_COMPLETED", "Completed mission cannot be changed.", nil)
	}
	var custody int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM commerce_mission_parcels WHERE commerce_mission_id=$1::uuid AND status IN ('PICKED_UP','IN_TRANSIT','RETURN_REQUIRED','TRANSFER_REQUIRED')`, record.ID).Scan(&custody); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	state, queue, reason := record.RuntimeState, record.QueueState, strings.TrimSpace(request.ReasonCode)
	parcelChanged := false
	parcelStatus := ""
	switch kind {
	case commandCancel:
		if custody > 0 {
			return dispatchcommerce.Mission{}, apperrors.New(409, "MISSION_CANCEL_NOT_ALLOWED", "Parcels are in custody; return or transfer is required.", nil)
		}
		state, queue = "CANCELLED", "CANCELLED"
		if reason == "" {
			reason = "COMMERCE_CANCELLED"
		}
		if err = cancelExecutionTx(ctx, tx, record, reason, now); err != nil {
			return dispatchcommerce.Mission{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE commerce_mission_parcels SET status='CANCELLED',updated_at=$2 WHERE commerce_mission_id=$1::uuid AND status='AWAITING_PICKUP'`, record.ID, now); err != nil {
			return dispatchcommerce.Mission{}, err
		}
		parcelChanged = true
		parcelStatus = "CANCELLED"
	case commandReceiverUnavailable:
		if record.RuntimeState != "IN_TRANSIT" {
			return dispatchcommerce.Mission{}, apperrors.New(409, "INVALID_MISSION_STATE", "Receiver-unavailable can only be reported in transit.", nil)
		}
		state, queue = "RECEIVER_UNAVAILABLE", "ASSIGNED"
		if reason == "" {
			reason = "RECEIVER_UNAVAILABLE"
		}
	case commandReschedule:
		if request.ReadyAt == nil || !request.ReadyAt.After(now) {
			return dispatchcommerce.Mission{}, apperrors.New(400, "INVALID_RESCHEDULE_TIME", "ready_at must be in the future.", nil)
		}
		if record.RuntimeState != "RECEIVER_UNAVAILABLE" && record.RuntimeState != "QUEUED" {
			return dispatchcommerce.Mission{}, apperrors.New(409, "INVALID_MISSION_STATE", "Mission is not eligible for reschedule.", nil)
		}
		state, queue, reason = "RESCHEDULED", "QUEUED", "RECEIVER_RESCHEDULED"
	case commandReturn:
		if custody == 0 {
			return dispatchcommerce.Mission{}, apperrors.New(409, "RETURN_NOT_REQUIRED", "No parcel is in provider custody.", nil)
		}
		state, queue = "RETURN_REQUIRED", "ASSIGNED"
		if reason == "" {
			reason = "RETURN_REQUIRED"
		}
		if _, err = tx.Exec(ctx, `UPDATE commerce_mission_parcels SET status='RETURN_REQUIRED',updated_at=$2 WHERE commerce_mission_id=$1::uuid AND status IN ('PICKED_UP','IN_TRANSIT','TRANSFER_REQUIRED')`, record.ID, now); err != nil {
			return dispatchcommerce.Mission{}, err
		}
		parcelChanged = true
		parcelStatus = "RETURN_REQUIRED"
		if err = insertMissionLeg(ctx, tx, record.ID, "RETURN", request.TransferEvidenceRef, now); err != nil {
			return dispatchcommerce.Mission{}, err
		}
	case commandReplaceProvider:
		if custody > 0 {
			// Post-custody replacement starts matching without changing the
			// active custodian. A real two-provider challenge is required later;
			// a caller-supplied evidence reference is never a custody transfer.
			if err = replaceExecutionTx(ctx, tx, &record, now, true); err != nil {
				return dispatchcommerce.Mission{}, err
			}
			state, queue, reason = "REPLACEMENT_REQUIRED", "RETRYING", "PROVIDER_REPLACEMENT"
			if _, err = tx.Exec(ctx, `UPDATE commerce_missions SET replacement_state='MATCHING',replacement_reason=$2 WHERE id=$1::uuid`, record.ID, reason); err != nil {
				return dispatchcommerce.Mission{}, err
			}
		} else {
			if err = replaceExecutionTx(ctx, tx, &record, now, false); err != nil {
				return dispatchcommerce.Mission{}, err
			}
			state, queue, reason = "RETRYING", "RETRYING", "PROVIDER_REPLACEMENT"
		}
	default:
		return dispatchcommerce.Mission{}, apperrors.BadRequest("Unknown Commerce mission command.", nil)
	}
	version := record.SourceVersion + 1
	var readyAt any
	if request.ReadyAt != nil {
		readyAt = *request.ReadyAt
	}
	_, err = tx.Exec(ctx, `UPDATE commerce_missions SET runtime_state=$2,queue_state=$3,reason_code=$4,source_version=$5,updated_at=$6,next_retry_at=COALESCE($7::timestamptz,next_retry_at),cancelled_at=CASE WHEN $2='CANCELLED' THEN $6 ELSE cancelled_at END WHERE id=$1::uuid`, record.ID, state, queue, reason, version, now, readyAt)
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	view, err := loadMissionViewTx(ctx, tx, record.ID)
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	result, _ := json.Marshal(view)
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_mission_operations(caller_service,operation_ref,operation_type,request_fingerprint,commerce_mission_id,result_snapshot) VALUES($1,$2,$3,$4,$5::uuid,$6)`, caller, request.OperationRef, kind, fingerprint, record.ID, result); err != nil {
		return dispatchcommerce.Mission{}, operationConflict(err)
	}
	if r.events != nil {
		if err = r.events.EmitTarget(ctx, tx, vendorServiceTarget, "commerce.mission.changed.v1", "COMMERCE_MISSION", missionRef, version, request.CorrelationID, "", map[string]any{"missionRef": missionRef, "operationRef": record.OperationRef, "missionSlotRef": record.MissionSlotRef, "planVersion": record.PlanVersion, "state": state, "queueState": queue, "reasonCode": reason, "parcelRefs": dispatchcommerce.SortedParcelRefs(view.Parcels), "sourceVersion": version, "occurredAt": now}); err != nil {
			return dispatchcommerce.Mission{}, err
		}
		if parcelChanged {
			if err = r.emitParcelEvent(ctx, tx, record.ID, missionRef, "", parcelStatus, version, now); err != nil {
				return dispatchcommerce.Mission{}, err
			}
		}
	}
	return view, tx.Commit(ctx)
}

func cancelExecutionTx(ctx context.Context, tx pgx.Tx, record multiStopMissionRecord, reason string, now time.Time) error {
	if _, err := tx.Exec(ctx, `UPDATE request_broadcasts SET status='cancelled',updated_at=$2 WHERE booking_id=$1::uuid AND status='broadcasting'`, record.BookingID, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE provider_request_inbox SET status='expired',responded_at=$2 WHERE booking_id=$1::uuid AND status='pending'`, record.BookingID, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE dispatch_bookings SET status='cancelled',cancel_reason=$2,cancelled_at=$3,updated_at=$3 WHERE id=$1::uuid AND status NOT IN ('completed','cancelled','failed')`, record.BookingID, reason, now); err != nil {
		return err
	}
	if record.TripID != "" {
		if _, err := tx.Exec(ctx, `UPDATE trips SET status='cancelled',cancelled_at=$2,updated_at=$2 WHERE id=$1::uuid AND status NOT IN ('completed','cancelled','failed')`, record.TripID, now); err != nil {
			return err
		}
	}
	return nil
}

func replaceExecutionTx(ctx context.Context, tx pgx.Tx, record *multiStopMissionRecord, now time.Time, preserveCustody bool) error {
	if !preserveCustody {
		if err := cancelExecutionTx(ctx, tx, *record, "provider_replacement", now); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `UPDATE request_broadcasts SET status='cancelled',updated_at=$2 WHERE booking_id=$1::uuid AND status='broadcasting'`, record.BookingID, now); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE provider_request_inbox SET status='expired',responded_at=$2 WHERE booking_id=$1::uuid AND status='pending'`, record.BookingID, now); err != nil {
			return err
		}
	}
	var snapshot dispatchcommerce.MissionCreateRequest
	var raw []byte
	var fee int64
	var currency string
	if err := tx.QueryRow(ctx, `SELECT request_snapshot,agreed_fee_minor,currency FROM commerce_missions WHERE id=$1::uuid`, record.ID).Scan(&raw, &fee, &currency); err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return err
	}
	first := snapshot.PickupStops[0]
	var bookingID string
	err := tx.QueryRow(ctx, `INSERT INTO dispatch_bookings(customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,currency,payment_method,payment_status,payment_ref,receiver_name,receiver_phone,package_desc,package_size,is_fragile,service_tier,rollout_protection_mode,rollout_policy_version,handover_required,pickup_evidence_required,delivery_evidence_required,provider_transfer_required)
		SELECT $1::uuid,'searching',$2,$3,$4,$5,$6,$7,$8,$9,'wallet','paid',$10,$11,$12,$13,'large',$14,$15,
			b.rollout_protection_mode,b.rollout_policy_version,b.handover_required,b.pickup_evidence_required,b.delivery_evidence_required,b.provider_transfer_required
		FROM dispatch_bookings b WHERE b.id=$16::uuid RETURNING id::text`, snapshot.CustomerRef, first.Location.Address, first.Location.Latitude, first.Location.Longitude, snapshot.Receiver.Address, snapshot.Receiver.Latitude, snapshot.Receiver.Longitude, fee, currency, snapshot.FundingReference, receiverDisplay(snapshot.Receiver), snapshot.Receiver.Phone, "Commerce provider replacement", hasFragile(snapshot.Parcels), serviceTier(snapshot.Mode), record.BookingID).Scan(&bookingID)
	if err != nil {
		return err
	}
	var attempt int
	if preserveCustody {
		err = tx.QueryRow(ctx, `UPDATE commerce_missions SET booking_id=$2::uuid,assignment_attempt=assignment_attempt+1 WHERE id=$1::uuid RETURNING assignment_attempt`, record.ID, bookingID).Scan(&attempt)
	} else {
		err = tx.QueryRow(ctx, `UPDATE commerce_missions SET booking_id=$2::uuid,trip_id=NULL,current_provider_id=NULL,assignment_attempt=assignment_attempt+1 WHERE id=$1::uuid RETURNING assignment_attempt`, record.ID, bookingID).Scan(&attempt)
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_mission_assignment_attempts SET ended_at=$2,state='REPLACED' WHERE commerce_mission_id=$1::uuid AND ended_at IS NULL`, record.ID, now); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_mission_assignment_attempts(commerce_mission_id,attempt_number,booking_id,state) VALUES($1::uuid,$2,$3::uuid,'SEARCHING')`, record.ID, attempt, bookingID); err != nil {
		return err
	}
	record.BookingID = bookingID
	if !preserveCustody {
		record.TripID = ""
	}
	return nil
}

func insertMissionLeg(ctx context.Context, tx pgx.Tx, missionID, kind, evidence string, now time.Time) error {
	rows, err := tx.Query(ctx, `SELECT parcel_ref FROM commerce_mission_parcels WHERE commerce_mission_id=$1::uuid ORDER BY parcel_ref`, missionID)
	if err != nil {
		return err
	}
	var refs []string
	for rows.Next() {
		var ref string
		if err = rows.Scan(&ref); err != nil {
			rows.Close()
			return err
		}
		refs = append(refs, ref)
	}
	rows.Close()
	_, err = tx.Exec(ctx, `INSERT INTO commerce_mission_legs(commerce_mission_id,leg_ref,leg_type,state,parcel_refs,evidence_ref,created_at) VALUES($1::uuid,$2,$3,'REQUIRED',$4,$5,$6)`, missionID, "leg_"+strings.ReplaceAll(uuid.NewString(), "-", ""), kind, refs, evidence, now)
	return err
}

func (r *Repository) MissionExecutionSnapshot(ctx context.Context, caller, missionRef string) (dispatchcommerce.MissionCreateRequest, int64, string, error) {
	var raw []byte
	var request dispatchcommerce.MissionCreateRequest
	var fee int64
	var currency string
	err := r.db.QueryRow(ctx, `SELECT request_snapshot,agreed_fee_minor,currency FROM commerce_missions WHERE caller_service=$1 AND canonical_ref=$2`, caller, missionRef).Scan(&raw, &fee, &currency)
	if err != nil {
		return request, 0, "", err
	}
	err = json.Unmarshal(raw, &request)
	return request, fee, currency, err
}
