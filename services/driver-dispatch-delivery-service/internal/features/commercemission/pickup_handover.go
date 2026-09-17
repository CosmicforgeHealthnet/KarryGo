package commercemission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/securehandover"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"cosmicforge/logistics/shared/go/handover"
)

func (s *MultiStopMissionService) WithHandoverSecret(secret []byte) *MultiStopMissionService {
	s.handoverSecret = append([]byte(nil), secret...)
	return s
}

func (s *MultiStopMissionService) ArrivePickup(ctx context.Context, providerID, missionRef, stopRef string, request dispatchcommerce.StopActionRequest) (dispatchcommerce.PickupCredential, error) {
	if len(s.handoverSecret) == 0 {
		return dispatchcommerce.PickupCredential{}, apperrors.New(503, "OTP_DEPENDENCY_UNAVAILABLE", "Pickup handover verification is unavailable.", nil)
	}
	if strings.TrimSpace(request.OperationRef) == "" {
		return dispatchcommerce.PickupCredential{}, apperrors.New(400, "IDEMPOTENCY_KEY_REQUIRED", "operation_ref is required.", nil)
	}
	fingerprint, _ := dispatchcommerce.Fingerprint(struct {
		Mission, Stop, Provider string
		Version                 int64
	}{missionRef, stopRef, providerID, request.ExpectedVersion})
	if raw, stored, found, err := s.store.(*Repository).FindOperation(ctx, "provider:"+providerID, request.OperationRef); err != nil {
		return dispatchcommerce.PickupCredential{}, err
	} else if found {
		if stored != fingerprint {
			return dispatchcommerce.PickupCredential{}, apperrors.New(409, "IDEMPOTENCY_CONFLICT", "This operation key was already used for a different pickup action.", nil)
		}
		var credential dispatchcommerce.PickupCredential
		if err = json.Unmarshal(raw, &credential); err != nil {
			return dispatchcommerce.PickupCredential{}, err
		}
		credential.Code, err = securehandover.CompatibilityCode(s.handoverSecret, handover.BuildChallengeID("dispatch", handover.StepPickup, missionRef+":"+stopRef), credential.Code)
		return credential.WithDirection(), err
	}
	now := s.now().UTC()
	record, err := securehandover.NewCompatibilityCodeRecord(s.handoverSecret, handover.BuildChallengeID("dispatch", handover.StepPickup, missionRef+":"+stopRef), now)
	if err != nil {
		return dispatchcommerce.PickupCredential{}, apperrors.New(503, "OTP_DEPENDENCY_UNAVAILABLE", "Pickup handover verification is unavailable.", err)
	}
	credential, err := s.store.(*Repository).ArrivePickup(ctx, providerID, missionRef, stopRef, request.OperationRef, fingerprint, request.ExpectedVersion, record, now)
	if err != nil {
		return dispatchcommerce.PickupCredential{}, err
	}
	credential.Code, err = securehandover.CompatibilityCode(s.handoverSecret, handover.BuildChallengeID("dispatch", handover.StepPickup, missionRef+":"+stopRef), credential.Code)
	return credential.WithDirection(), err
}

func (s *MultiStopMissionService) VerifyPickup(ctx context.Context, caller, missionRef, stopRef string, request dispatchcommerce.PickupVerificationRequest) (dispatchcommerce.Mission, error) {
	if caller != dispatchcommerce.SignatureCaller {
		return dispatchcommerce.Mission{}, apperrors.Unauthorized("This caller may not verify pickup handovers.", nil)
	}
	if len(s.handoverSecret) == 0 {
		return dispatchcommerce.Mission{}, apperrors.New(503, "OTP_DEPENDENCY_UNAVAILABLE", "Pickup handover verification is unavailable.", nil)
	}
	fingerprint, _ := dispatchcommerce.Fingerprint(struct {
		Mission, Stop, Code string
		Version             int64
	}{missionRef, stopRef, request.Code, request.ExpectedVersion})
	fingerprint = handover.HashCode(s.handoverSecret, "dispatch-fingerprint:pickup", fingerprint)
	return s.store.(*Repository).VerifyPickup(ctx, caller, missionRef, stopRef, request.OperationRef, fingerprint, request.ExpectedVersion, request.Code, s.handoverSecret, s.now().UTC())
}

func (s *MultiStopMissionService) CompletePickup(ctx context.Context, providerID, missionRef, stopRef string, request dispatchcommerce.StopActionRequest) (dispatchcommerce.Mission, error) {
	mission, _, found, err := s.store.FindMultiStopByCanonical(ctx, dispatchcommerce.SignatureCaller, missionRef)
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if !found {
		return dispatchcommerce.Mission{}, apperrors.NotFound("Commerce mission not found.", nil)
	}
	var selected *dispatchcommerce.StopView
	for index := range mission.Stops {
		if mission.Stops[index].StopRef == stopRef {
			selected = &mission.Stops[index]
			break
		}
	}
	if selected == nil || request.ManifestRef != selected.ManifestRef || request.ManifestVersion != selected.ManifestVersion {
		return dispatchcommerce.Mission{}, apperrors.New(409, "MANIFEST_SCOPE_MISMATCH", "Exact stop manifest identity and version are required.", nil)
	}
	if len(request.ParcelRefs) == 0 || request.PackageCount < 1 {
		return dispatchcommerce.Mission{}, apperrors.New(409, "PARCEL_CONFIRMATION_REQUIRED", "Exact parcel ids and package count are required.", nil)
	}
	command := EvidenceCommand{OperationRef: request.OperationRef + ":pickup-evidence", Type: EvidencePickup,
		ParcelRefs: canonicalRefs(request.ParcelRefs), ManifestRef: request.ManifestRef,
		ManifestVersion: request.ManifestVersion, PackageCount: request.PackageCount, CapturedAt: s.now().UTC()}
	if request.Evidence != nil {
		command.CapturedAt = request.Evidence.CapturedAt
		command.Latitude = request.Evidence.Latitude
		command.Longitude = request.Evidence.Longitude
		command.MediaRefs = canonicalRefs(request.Evidence.MediaRefs)
		command.Description = request.Evidence.Description
	}
	if err = command.Validate(s.now().UTC()); err != nil {
		return dispatchcommerce.Mission{}, apperrors.BadRequest("Pickup evidence is invalid.", err)
	}
	if err = s.authorizeEvidence(ctx, missionRef, command); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	fingerprint, _ := dispatchcommerce.Fingerprint(struct {
		Mission, Stop, Provider string
		Request                 dispatchcommerce.StopActionRequest
	}{missionRef, stopRef, providerID, request})
	var delivery handover.CodeRecord
	if len(mission.Stops) > 0 && mission.TripRef != "" {
		delivery, err = securehandover.NewCompatibilityCodeRecord(s.handoverSecret, handover.BuildChallengeID("dispatch", handover.StepDelivery, mission.TripRef), s.now().UTC())
		if err != nil {
			return dispatchcommerce.Mission{}, apperrors.New(503, "OTP_DEPENDENCY_UNAVAILABLE", "Receiver handover verification is unavailable.", err)
		}
	}
	result, err := s.store.(*Repository).CompletePickup(ctx, providerID, missionRef, stopRef, request.OperationRef, fingerprint, request.ExpectedVersion, command, delivery, s.now().UTC())
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	// The receiver credential only exists after the final authoritative pickup
	// transaction starts the Commerce trip. Dispatch owns this policy and sends
	// an opaque typed request through Notification Service. Calling again on a
	// replay is safe because BookingNotifier derives a stable idempotency key.
	if s.notifier != nil && result.State == "IN_TRANSIT" {
		customerID, bookingID, persistedCode, expiresAt, lookupErr := s.store.(*Repository).CommerceNotificationRecipient(ctx, missionRef)
		if lookupErr == nil && customerID != "" && persistedCode != "" {
			s.notifier.NotifyCustomerDeliveryCodeReady(ctx, customerID, bookingID, map[string]interface{}{
				"mission_ref": missionRef, "expires_at": expiresAt,
			})
		}
	}
	return result, nil
}

func (r *Repository) CommerceNotificationRecipient(ctx context.Context, missionRef string) (string, string, string, *time.Time, error) {
	var customerID, bookingID, code string
	var expiresAt *time.Time
	err := r.db.QueryRow(ctx, `SELECT b.customer_id::text,b.id::text,COALESCE(t.delivery_code,''),t.delivery_code_expires_at
		FROM commerce_missions m JOIN bookings b ON b.id=m.booking_id JOIN trips t ON t.id=m.trip_id
		WHERE m.canonical_ref=$1`, missionRef).Scan(&customerID, &bookingID, &code, &expiresAt)
	return customerID, bookingID, code, expiresAt, err
}

func (r *Repository) FindOperation(ctx context.Context, caller, operation string) ([]byte, string, bool, error) {
	var raw []byte
	var fingerprint string
	err := r.db.QueryRow(ctx, `SELECT result_snapshot,request_fingerprint FROM commerce_mission_operations WHERE caller_service=$1 AND operation_ref=$2`, caller, operation).Scan(&raw, &fingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", false, nil
	}
	return raw, fingerprint, err == nil, err
}

func (r *Repository) ArrivePickup(ctx context.Context, providerID, missionRef, stopRef, operation, fingerprint string, expectedVersion int64, code handover.CodeRecord, now time.Time) (dispatchcommerce.PickupCredential, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return dispatchcommerce.PickupCredential{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	missionID, version, err := lockProviderMission(ctx, tx, missionRef, providerID)
	if err != nil {
		return dispatchcommerce.PickupCredential{}, err
	}
	if expectedVersion > 0 && expectedVersion != version {
		return dispatchcommerce.PickupCredential{}, apperrors.New(409, "STALE_MISSION_VERSION", "Mission version changed.", nil)
	}
	var nextStop string
	if err = tx.QueryRow(ctx, `SELECT stop_ref FROM commerce_mission_stops WHERE commerce_mission_id=$1::uuid AND status NOT IN ('PICKED_UP','SKIPPED','RETURNED') ORDER BY sequence LIMIT 1 FOR UPDATE`, missionID).Scan(&nextStop); err != nil {
		return dispatchcommerce.PickupCredential{}, err
	}
	if nextStop != stopRef {
		return dispatchcommerce.PickupCredential{}, apperrors.New(409, "INVALID_SEQUENCE", "This is not the next pickup stop.", nil)
	}
	var readyTo time.Time
	var status string
	if err = tx.QueryRow(ctx, `SELECT ready_to,status FROM commerce_mission_stops WHERE commerce_mission_id=$1::uuid AND stop_ref=$2 FOR UPDATE`, missionID, stopRef).Scan(&readyTo, &status); err != nil {
		return dispatchcommerce.PickupCredential{}, err
	}
	if status != "AWAITING_PICKUP" && status != "ARRIVED" {
		return dispatchcommerce.PickupCredential{}, apperrors.New(409, "INVALID_STOP_STATE", "Pickup stop cannot be arrived now.", nil)
	}
	if now.After(readyTo) {
		return dispatchcommerce.PickupCredential{}, apperrors.New(409, "STOP_WINDOW_EXPIRED", "The pickup ready window has expired.", nil)
	}
	credential := dispatchcommerce.PickupCredential{MissionRef: missionRef, StopRef: stopRef, Code: code.Code, ExpiresAt: code.ExpiresAt, AttemptsRemaining: handover.MaxVerifyAttempts}
	result, _ := json.Marshal(credential)
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_mission_operations(caller_service,operation_ref,operation_type,request_fingerprint,commerce_mission_id,result_snapshot) VALUES($1,$2,'ARRIVE_PICKUP',$3,$4::uuid,$5)`, `provider:`+providerID, operation, fingerprint, missionID, result); err != nil {
		return dispatchcommerce.PickupCredential{}, operationConflict(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_mission_stops SET status='ARRIVED',handover_status='ISSUED',handover_code=$3,handover_code_hash=$4,handover_code_expires_at=$5,handover_code_attempts=0,arrived_at=COALESCE(arrived_at,$6),updated_at=$6 WHERE commerce_mission_id=$1::uuid AND stop_ref=$2`, missionID, stopRef, code.Code, code.Hash, code.ExpiresAt, now); err != nil {
		return dispatchcommerce.PickupCredential{}, err
	}
	version++
	if _, err = tx.Exec(ctx, `UPDATE commerce_missions SET runtime_state='PICKING_UP',queue_state='ASSIGNED',source_version=$2,updated_at=$3 WHERE id=$1::uuid`, missionID, version, now); err != nil {
		return dispatchcommerce.PickupCredential{}, err
	}
	if err = r.emitStopEvent(ctx, tx, missionID, missionRef, stopRef, "ARRIVED", "ISSUED", version, now); err != nil {
		return dispatchcommerce.PickupCredential{}, err
	}
	return credential, tx.Commit(ctx)
}

func (r *Repository) VerifyPickup(ctx context.Context, caller, missionRef, stopRef, operation, fingerprint string, expectedVersion int64, code string, secret []byte, now time.Time) (dispatchcommerce.Mission, error) {
	if raw, stored, found, err := r.FindOperation(ctx, caller, operation); err != nil {
		return dispatchcommerce.Mission{}, err
	} else if found {
		if stored != fingerprint {
			return dispatchcommerce.Mission{}, apperrors.New(409, "IDEMPOTENCY_CONFLICT", "This operation key was already used for another verification.", nil)
		}
		var view dispatchcommerce.Mission
		if err = json.Unmarshal(raw, &view); err != nil {
			return dispatchcommerce.Mission{}, err
		}
		view.Replayed = true
		return view, nil
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var missionID, hash, status string
	var version int64
	var expires time.Time
	var attempts int
	err = tx.QueryRow(ctx, `SELECT m.id::text,m.source_version,s.handover_code_hash,s.handover_code_expires_at,s.handover_code_attempts,s.handover_status FROM commerce_missions m JOIN commerce_mission_stops s ON s.commerce_mission_id=m.id WHERE m.caller_service=$1 AND m.canonical_ref=$2 AND s.stop_ref=$3 FOR UPDATE OF m,s`, caller, missionRef, stopRef).Scan(&missionID, &version, &hash, &expires, &attempts, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return dispatchcommerce.Mission{}, apperrors.NotFound("Commerce pickup stop not found.", nil)
	}
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if expectedVersion > 0 && expectedVersion != version {
		return dispatchcommerce.Mission{}, apperrors.New(409, "STALE_MISSION_VERSION", "Mission version changed.", nil)
	}
	if status == "VERIFIED" || status == "CONSUMED" {
		return r.finishIdempotentMissionOperation(ctx, tx, caller, operation, fingerprint, missionID)
	}
	if attempts >= handover.MaxVerifyAttempts {
		return dispatchcommerce.Mission{}, apperrors.New(429, "OTP_ATTEMPTS_EXCEEDED", "Pickup code attempt limit reached.", nil)
	}
	if now.After(expires) {
		return dispatchcommerce.Mission{}, apperrors.New(409, "OTP_EXPIRED", "Pickup code has expired.", nil)
	}
	if !handover.VerifyCode(secret, handover.BuildChallengeID("dispatch", handover.StepPickup, missionRef+":"+stopRef), strings.TrimSpace(code), hash) {
		_, _ = tx.Exec(ctx, `UPDATE commerce_mission_stops SET handover_code_attempts=handover_code_attempts+1,updated_at=$3 WHERE commerce_mission_id=$1::uuid AND stop_ref=$2`, missionID, stopRef, now)
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return dispatchcommerce.Mission{}, commitErr
		}
		return dispatchcommerce.Mission{}, apperrors.New(400, "OTP_INVALID", "Pickup code is invalid.", nil)
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_mission_stops SET status='READY_FOR_HANDOVER',handover_status='VERIFIED',handover_verified_at=$3,updated_at=$3 WHERE commerce_mission_id=$1::uuid AND stop_ref=$2`, missionID, stopRef, now); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	version++
	if _, err = tx.Exec(ctx, `UPDATE commerce_missions SET source_version=$2,updated_at=$3 WHERE id=$1::uuid`, missionID, version, now); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	view, err := loadMissionViewTx(ctx, tx, missionID)
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	result, _ := json.Marshal(view)
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_mission_operations(caller_service,operation_ref,operation_type,request_fingerprint,commerce_mission_id,result_snapshot) VALUES($1,$2,'VERIFY_PICKUP',$3,$4::uuid,$5)`, caller, operation, fingerprint, missionID, result); err != nil {
		return dispatchcommerce.Mission{}, operationConflict(err)
	}
	if err = r.emitStopEvent(ctx, tx, missionID, missionRef, stopRef, "READY_FOR_HANDOVER", "VERIFIED", version, now); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	return view, tx.Commit(ctx)
}

func (r *Repository) CompletePickup(ctx context.Context, providerID, missionRef, stopRef, operation, fingerprint string, expectedVersion int64, evidence EvidenceCommand, delivery handover.CodeRecord, now time.Time) (dispatchcommerce.Mission, error) {
	caller := "provider:" + providerID
	if raw, stored, found, err := r.FindOperation(ctx, caller, operation); err != nil {
		return dispatchcommerce.Mission{}, err
	} else if found {
		if stored != fingerprint {
			return dispatchcommerce.Mission{}, apperrors.New(409, "IDEMPOTENCY_CONFLICT", "This operation key was already used for another pickup.", nil)
		}
		var view dispatchcommerce.Mission
		if err = json.Unmarshal(raw, &view); err != nil {
			return dispatchcommerce.Mission{}, err
		}
		view.Replayed = true
		return view, nil
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	missionID, version, err := lockProviderMission(ctx, tx, missionRef, providerID)
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if expectedVersion > 0 && expectedVersion != version {
		return dispatchcommerce.Mission{}, apperrors.New(409, "STALE_MISSION_VERSION", "Mission version changed.", nil)
	}
	if err = assertParcelScopeTx(ctx, tx, missionID, stopRef, evidence.ParcelRefs, true); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	var status, handoverStatus string
	var manifestRef string
	var manifestVersion, packageCount int64
	if err = tx.QueryRow(ctx, `SELECT s.status,s.handover_status,s.manifest_ref,s.manifest_version,
		COALESCE((SELECT sum(p.package_count) FROM commerce_mission_parcels p WHERE p.stop_id=s.id),0)
		FROM commerce_mission_stops s WHERE s.commerce_mission_id=$1::uuid AND s.stop_ref=$2 FOR UPDATE`, missionID, stopRef).
		Scan(&status, &handoverStatus, &manifestRef, &manifestVersion, &packageCount); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if evidence.ManifestRef != manifestRef || evidence.ManifestVersion != manifestVersion || evidence.PackageCount != packageCount {
		return dispatchcommerce.Mission{}, apperrors.New(409, "MANIFEST_SCOPE_MISMATCH", "Manifest or package-count confirmation does not match the authoritative stop.", nil)
	}
	if status != "READY_FOR_HANDOVER" || handoverStatus != "VERIFIED" {
		return dispatchcommerce.Mission{}, apperrors.New(409, "OTP_REQUIRED", "Pickup handover must be verified before completion.", nil)
	}
	nextVersion := version + 1
	stopID, err := stopIDTx(ctx, tx, missionID, stopRef)
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	recorded, err := insertEvidenceTx(ctx, tx, missionID, missionRef, stopID, stopRef, "PROVIDER", providerID, evidence, nextVersion, now)
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_mission_stops SET status='PICKED_UP',handover_status='CONSUMED',completed_at=$3,updated_at=$3 WHERE commerce_mission_id=$1::uuid AND stop_ref=$2`, missionID, stopRef, now); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_mission_parcels p SET status='PICKED_UP',custodian_type='PROVIDER',custodian_ref=$3,picked_up_at=$4,updated_at=$4 FROM commerce_mission_stops s WHERE p.stop_id=s.id AND s.commerce_mission_id=$1::uuid AND s.stop_ref=$2`, missionID, stopRef, providerID, now); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_mission_carrier_history(commerce_mission_id,provider_id,trip_id,custody_started_at,source_version)
		SELECT id,current_provider_id,trip_id,$2,$3 FROM commerce_missions m WHERE id=$1::uuid
		AND current_provider_id IS NOT NULL AND trip_id IS NOT NULL
		AND NOT EXISTS (SELECT 1 FROM commerce_mission_carrier_history h WHERE h.commerce_mission_id=m.id AND h.custody_ended_at IS NULL)`, missionID, now, nextVersion); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_mission_stops SET manifest_frozen_at=COALESCE(manifest_frozen_at,$3),handover_evidence=jsonb_build_object('evidence_id',$4::text,'manifest_ref',manifest_ref,'manifest_version',manifest_version) WHERE commerce_mission_id=$1::uuid AND stop_ref=$2`, missionID, stopRef, now, recorded.ID); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_mission_parcels p SET manifest_frozen_at=COALESCE(manifest_frozen_at,$3) FROM commerce_mission_stops s WHERE p.stop_id=s.id AND s.commerce_mission_id=$1::uuid AND s.stop_ref=$2`, missionID, stopRef, now); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if err = r.collectCustodyTx(ctx, tx, missionID, stopRef, providerID, operation, recorded.ID, providerID, nextVersion); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	var remaining int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM commerce_mission_stops WHERE commerce_mission_id=$1::uuid AND status NOT IN ('PICKED_UP','SKIPPED','RETURNED')`, missionID).Scan(&remaining); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	state := "PICKING_UP"
	if remaining == 0 {
		state = "IN_TRANSIT"
		if err = startCommerceTripTx(ctx, tx, missionID, delivery, now); err != nil {
			return dispatchcommerce.Mission{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE commerce_mission_parcels SET status='IN_TRANSIT',updated_at=$2 WHERE commerce_mission_id=$1::uuid AND status='PICKED_UP'`, missionID, now); err != nil {
			return dispatchcommerce.Mission{}, err
		}
	}
	version = nextVersion
	if _, err = tx.Exec(ctx, `UPDATE commerce_missions SET runtime_state=$2,source_version=$3,updated_at=$4 WHERE id=$1::uuid`, missionID, state, version, now); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	view, err := loadMissionViewTx(ctx, tx, missionID)
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	result, _ := json.Marshal(view)
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_mission_operations(caller_service,operation_ref,operation_type,request_fingerprint,commerce_mission_id,result_snapshot) VALUES($1,$2,'COMPLETE_PICKUP',$3,$4::uuid,$5)`, caller, operation, fingerprint, missionID, result); err != nil {
		return dispatchcommerce.Mission{}, operationConflict(err)
	}
	if err = r.emitStopEvent(ctx, tx, missionID, missionRef, stopRef, "PICKED_UP", "CONSUMED", version, now); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if err = r.emitParcelEvent(ctx, tx, missionID, missionRef, stopRef, "PICKED_UP", version, now); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if err = emitOperationalEvent(ctx, r, tx, missionID, missionRef, "commerce.evidence.recorded.v1", version, evidence.ParcelRefs, "", map[string]any{"evidenceId": recorded.ID, "evidenceType": EvidencePickup, "stopRef": stopRef}, now); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	return view, tx.Commit(ctx)
}

func lockProviderMission(ctx context.Context, tx pgx.Tx, missionRef, providerID string) (string, int64, error) {
	var missionID string
	var version int64
	err := tx.QueryRow(ctx, `SELECT id::text,source_version FROM commerce_missions WHERE canonical_ref=$1 AND current_provider_id=$2::uuid AND replacement_state='NONE' FOR UPDATE`, missionRef, providerID).Scan(&missionID, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, apperrors.NotFound("Assigned Commerce mission not found.", nil)
	}
	return missionID, version, err
}

func startCommerceTripTx(ctx context.Context, tx pgx.Tx, missionID string, delivery handover.CodeRecord, now time.Time) error {
	var tripID string
	var from string
	err := tx.QueryRow(ctx, `SELECT t.id::text,t.status FROM trips t JOIN commerce_missions m ON m.booking_id=t.booking_id WHERE m.id=$1::uuid FOR UPDATE OF t`, missionID).Scan(&tripID, &from)
	if err != nil {
		return err
	}
	if from == "in_progress" || from == "proof_submitted" || from == "completed" {
		return nil
	}
	if _, err = tx.Exec(ctx, `UPDATE trips SET status='in_progress',started_at=COALESCE(started_at,$2),delivery_code=$3,delivery_code_hash=$4,delivery_code_expires_at=$5,delivery_code_attempts=0,updated_at=$2 WHERE id=$1::uuid`, tripID, now, delivery.Code, delivery.Hash, delivery.ExpiresAt); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO trip_state_log(trip_id,from_status,to_status,changed_by,notes) VALUES($1::uuid,$2,'in_progress','system','commerce_multi_pickup_completed')`, tripID, from)
	return err
}

func operationConflict(err error) error {
	if err == nil {
		return nil
	}
	return apperrors.New(409, "IDEMPOTENCY_CONFLICT", "This operation key was already used.", err)
}

func (r *Repository) emitStopEvent(ctx context.Context, tx pgx.Tx, missionID, missionRef, stopRef, status, handoverStatus string, version int64, now time.Time) error {
	if r.events == nil {
		return nil
	}
	var operationRef, missionSlotRef string
	if err := tx.QueryRow(ctx, `SELECT operation_ref,mission_slot_ref FROM commerce_missions WHERE id=$1::uuid`, missionID).Scan(&operationRef, &missionSlotRef); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT parcel_ref FROM commerce_mission_parcels p JOIN commerce_mission_stops s ON s.id=p.stop_id WHERE s.commerce_mission_id=$1::uuid AND s.stop_ref=$2 ORDER BY parcel_ref`, missionID, stopRef)
	if err != nil {
		return err
	}
	defer rows.Close()
	var refs []string
	for rows.Next() {
		var ref string
		if err = rows.Scan(&ref); err != nil {
			return err
		}
		refs = append(refs, ref)
	}
	return r.events.EmitTarget(ctx, tx, vendorServiceTarget, "commerce.stop.changed.v1", "COMMERCE_MISSION", missionRef, version, "", "", map[string]any{"missionRef": missionRef, "operationRef": operationRef, "missionSlotRef": missionSlotRef, "stopRef": stopRef, "parcelRefs": refs, "status": status, "handoverStatus": handoverStatus, "sourceVersion": version, "occurredAt": now})
}

func loadMissionViewTx(ctx context.Context, tx pgx.Tx, missionID string) (dispatchcommerce.Mission, error) {
	var record multiStopMissionRecord
	record, err := scanMultiStopMission(tx.QueryRow(ctx, `SELECT `+multiStopMissionColumns+` FROM commerce_missions WHERE id=$1::uuid`, missionID))
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	view := dispatchcommerce.Mission{MissionRef: record.CanonicalRef, OperationRef: record.OperationRef, BookingRef: record.BookingID, TripRef: record.TripID, State: record.RuntimeState, QueueState: record.QueueState, Version: record.SourceVersion, Plan: dispatchcommerce.PlanBinding{PlanRef: record.PlanRef, PlanVersion: record.PlanVersion, PlanFingerprint: record.PlanFingerprint, MissionSlotRef: record.MissionSlotRef}, Quote: dispatchcommerce.QuoteBinding{QuoteID: record.QuoteID, QuoteReference: record.QuoteReference, QuoteRevision: record.QuoteRevision, QuoteFingerprint: record.QuoteFingerprint}, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, ReasonCode: record.ReasonCode}
	rows, err := tx.Query(ctx, `SELECT stop_ref,physical_stop_ref,sequence,vendor_ref,branch_ref,manifest_ref,manifest_version,location_snapshot,ready_from,ready_to,handling_snapshot,status,handover_status FROM commerce_mission_stops WHERE commerce_mission_id=$1::uuid ORDER BY sequence`, missionID)
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	for rows.Next() {
		var s dispatchcommerce.StopView
		var location, handling []byte
		if err = rows.Scan(&s.StopRef, &s.PhysicalStopRef, &s.Sequence, &s.VendorRef, &s.BranchRef, &s.ManifestRef, &s.ManifestVersion, &location, &s.ReadyWindow.From, &s.ReadyWindow.To, &handling, &s.Status, &s.HandoverStatus); err != nil {
			rows.Close()
			return dispatchcommerce.Mission{}, err
		}
		_ = json.Unmarshal(location, &s.Location)
		_ = json.Unmarshal(handling, &s.Handling)
		view.Stops = append(view.Stops, s)
	}
	rows.Close()
	parcelRows, err := tx.Query(ctx, `SELECT p.parcel_ref,p.vendor_ref,p.branch_ref,p.fulfilment_unit_ref,p.vendor_order_ref,p.manifest_ref,p.manifest_version,p.package_count,p.custodian_type,p.custodian_ref,p.status,s.stop_ref FROM commerce_mission_parcels p JOIN commerce_mission_stops s ON s.id=p.stop_id WHERE p.commerce_mission_id=$1::uuid ORDER BY p.created_at,p.parcel_ref`, missionID)
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	for parcelRows.Next() {
		var p dispatchcommerce.ParcelView
		if err = parcelRows.Scan(&p.ParcelRef, &p.VendorRef, &p.BranchRef, &p.FulfilmentUnit, &p.VendorOrderRef, &p.ManifestRef, &p.ManifestVersion, &p.PackageCount, &p.CustodianType, &p.CustodianRef, &p.Status, &p.PickupStopRef); err != nil {
			parcelRows.Close()
			return dispatchcommerce.Mission{}, err
		}
		view.Parcels = append(view.Parcels, p)
	}
	parcelRows.Close()
	for i := range view.Stops {
		for _, p := range view.Parcels {
			if p.PickupStopRef == view.Stops[i].StopRef {
				view.Stops[i].ParcelRefs = append(view.Stops[i].ParcelRefs, p.ParcelRef)
			}
		}
	}
	return view, nil
}

func (r *Repository) finishIdempotentMissionOperation(ctx context.Context, tx pgx.Tx, caller, operation, fingerprint, missionID string) (dispatchcommerce.Mission, error) {
	view, err := loadMissionViewTx(ctx, tx, missionID)
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	result, _ := json.Marshal(view)
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_mission_operations(caller_service,operation_ref,operation_type,request_fingerprint,commerce_mission_id,result_snapshot) VALUES($1,$2,'VERIFY_PICKUP',$3,$4::uuid,$5)`, caller, operation, fingerprint, missionID, result); err != nil {
		return dispatchcommerce.Mission{}, operationConflict(err)
	}
	return view, tx.Commit(ctx)
}

var _ = fmt.Sprintf
