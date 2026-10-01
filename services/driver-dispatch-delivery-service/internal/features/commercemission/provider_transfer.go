package commercemission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/securehandover"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"cosmicforge/logistics/shared/go/handover"
)

type StartTransferCommand struct {
	OperationRef    string   `json:"operation_ref"`
	ExpectedVersion int64    `json:"expected_version,omitempty"`
	ParcelRefs      []string `json:"parcel_refs"`
	PackageCount    int64    `json:"package_count"`
	Latitude        float64  `json:"latitude"`
	Longitude       float64  `json:"longitude"`
}

type TransferCredential struct {
	TransferRef      string    `json:"transfer_ref"`
	MissionRef       string    `json:"mission_ref"`
	OldProviderID    string    `json:"old_provider_id"`
	NewProviderID    string    `json:"new_provider_id"`
	ParcelRefs       []string  `json:"parcel_refs"`
	PackageCount     int64     `json:"package_count"`
	ChallengeCode    string    `json:"challenge_code"`
	ChallengeExpires time.Time `json:"challenge_expires_at"`
	SourceVersion    int64     `json:"source_version"`
	Replayed         bool      `json:"replayed,omitempty"`
}

type CompleteTransferCommand struct {
	OperationRef    string                         `json:"operation_ref"`
	ExpectedVersion int64                          `json:"expected_version,omitempty"`
	Code            string                         `json:"code"`
	ParcelRefs      []string                       `json:"parcel_refs"`
	PackageCount    int64                          `json:"package_count"`
	Latitude        float64                        `json:"latitude"`
	Longitude       float64                        `json:"longitude"`
	Evidence        dispatchcommerce.EvidenceInput `json:"evidence"`
}

type transferBinding struct {
	ParcelRef       string `json:"parcel_ref"`
	ManifestRef     string `json:"manifest_ref"`
	ManifestVersion int64  `json:"manifest_version"`
	PackageCount    int64  `json:"package_count"`
}

func (s *OperationalService) StartProviderTransfer(ctx context.Context, providerID, missionRef string, command StartTransferCommand) (TransferCredential, error) {
	if s == nil || s.repo == nil || len(s.handoverSecret) == 0 {
		return TransferCredential{}, apperrors.New(503, "TRANSFER_HANDOVER_UNAVAILABLE", "Provider transfer handover is unavailable.", nil)
	}
	command.OperationRef = strings.TrimSpace(command.OperationRef)
	command.ParcelRefs = canonicalRefs(command.ParcelRefs)
	if command.OperationRef == "" || len(command.ParcelRefs) == 0 || command.PackageCount < 1 ||
		command.Latitude < -90 || command.Latitude > 90 || command.Longitude < -180 || command.Longitude > 180 {
		return TransferCredential{}, apperrors.BadRequest("Provider transfer request is invalid.", nil)
	}
	fingerprint, _ := dispatchcommerce.Fingerprint(struct {
		Mission, Provider string
		Command           StartTransferCommand
	}{missionRef, providerID, command})
	if raw, stored, found, err := s.repo.FindOperation(ctx, "provider:"+providerID, command.OperationRef); err != nil {
		return TransferCredential{}, err
	} else if found {
		if stored != fingerprint {
			return TransferCredential{}, apperrors.New(409, "IDEMPOTENCY_CONFLICT", "Transfer operation was reused with different semantics.", nil)
		}
		var result TransferCredential
		if err = json.Unmarshal(raw, &result); err != nil {
			return TransferCredential{}, err
		}
		result.Replayed = true
		return s.transferCredentialForProvider(ctx, providerID, result)
	}
	result, err := s.repo.StartProviderTransfer(ctx, providerID, missionRef, command, fingerprint, s.handoverSecret, s.now().UTC())
	if err != nil {
		return TransferCredential{}, err
	}
	return s.transferCredentialForProvider(ctx, providerID, result)
}

func (s *OperationalService) transferCredentialForProvider(ctx context.Context, providerID string, result TransferCredential) (TransferCredential, error) {
	if result.OldProviderID != providerID {
		return TransferCredential{}, apperrors.NotFound("Transfer not found.", nil)
	}
	if !strings.HasPrefix(result.ChallengeCode, "handover_") {
		return result, nil
	} // pre-upgrade active credential
	var raw []byte
	if err := s.repo.db.QueryRow(ctx, `SELECT manifest_binding FROM commerce_provider_transfers WHERE transfer_ref=$1 AND old_provider_id=$2::uuid`, result.TransferRef, providerID).Scan(&raw); err != nil {
		return TransferCredential{}, err
	}
	var bindings []transferBinding
	if err := json.Unmarshal(raw, &bindings); err != nil {
		return TransferCredential{}, err
	}
	fp, err := dispatchcommerce.Fingerprint(bindings)
	if err != nil {
		return TransferCredential{}, err
	}
	result.ChallengeCode, err = securehandover.CompatibilityCode(s.handoverSecret, transferChallengeID(result.MissionRef, result.TransferRef, result.OldProviderID, result.NewProviderID, fp), result.ChallengeCode)
	return result, err
}

func (r *Repository) StartProviderTransfer(ctx context.Context, providerID, missionRef string, command StartTransferCommand, fingerprint string, secret []byte, now time.Time) (TransferCredential, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return TransferCredential{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	missionID, version, err := lockOperationalMission(ctx, tx, missionRef, providerID, false)
	if err != nil {
		return TransferCredential{}, err
	}
	if command.ExpectedVersion > 0 && command.ExpectedVersion != version {
		return TransferCredential{}, apperrors.New(409, "STALE_MISSION_VERSION", "Mission version changed.", nil)
	}
	var replacementState, oldProvider, newProvider, oldTrip, newTrip string
	err = tx.QueryRow(ctx, `SELECT replacement_state,COALESCE(current_provider_id::text,''),COALESCE(replacement_provider_id::text,''),
		COALESCE(trip_id::text,''),COALESCE(replacement_trip_id::text,'') FROM commerce_missions WHERE id=$1::uuid`, missionID).
		Scan(&replacementState, &oldProvider, &newProvider, &oldTrip, &newTrip)
	if err != nil {
		return TransferCredential{}, err
	}
	if oldProvider != providerID || newProvider == "" || newTrip == "" || (replacementState != "ASSIGNED" && replacementState != "TRANSFER_PENDING") {
		return TransferCredential{}, apperrors.New(409, "REPLACEMENT_NOT_READY", "A compatible replacement provider must be assigned before transfer.", nil)
	}
	bindings, packageCount, err := loadCustodyBindingsTx(ctx, tx, missionID, oldProvider)
	if err != nil {
		return TransferCredential{}, err
	}
	expectedRefs := make([]string, 0, len(bindings))
	for _, item := range bindings {
		expectedRefs = append(expectedRefs, item.ParcelRef)
	}
	expectedRefs = canonicalRefs(expectedRefs)
	if strings.Join(expectedRefs, "\x00") != strings.Join(command.ParcelRefs, "\x00") || packageCount != command.PackageCount {
		return TransferCredential{}, apperrors.New(409, "TRANSFER_MANIFEST_MISMATCH", "Transfer must confirm every parcel and package currently in custody.", nil)
	}
	bindingRaw, _ := json.Marshal(bindings)
	bindingFingerprint, _ := dispatchcommerce.Fingerprint(bindings)
	transferRef := "dpt_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	challengeID := transferChallengeID(missionRef, transferRef, oldProvider, newProvider, bindingFingerprint)
	challenge, err := securehandover.NewCompatibilityCodeRecord(secret, challengeID, now)
	if err != nil {
		return TransferCredential{}, apperrors.New(503, "TRANSFER_HANDOVER_UNAVAILABLE", "Provider transfer handover is unavailable.", err)
	}
	nextVersion := version + 1
	var transferID string
	err = tx.QueryRow(ctx, `INSERT INTO commerce_provider_transfers(
		transfer_ref,commerce_mission_id,operation_ref,state,old_provider_id,new_provider_id,old_trip_id,new_trip_id,
		parcel_refs,manifest_binding,package_count,latitude,longitude,challenge_hash,challenge_expires_at,source_version)
		VALUES($1,$2::uuid,$3,'CHALLENGE_ISSUED',$4::uuid,$5::uuid,$6::uuid,$7::uuid,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING id::text`, transferRef, missionID, command.OperationRef, oldProvider, newProvider, oldTrip, newTrip,
		command.ParcelRefs, bindingRaw, command.PackageCount, command.Latitude, command.Longitude,
		challenge.Hash, challenge.ExpiresAt, nextVersion).Scan(&transferID)
	if err != nil {
		return TransferCredential{}, operationConflict(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_mission_parcels SET status='TRANSFER_REQUIRED',updated_at=$3
		WHERE commerce_mission_id=$1::uuid AND parcel_ref=ANY($2) AND custodian_type='PROVIDER' AND custodian_ref=$4`, missionID, command.ParcelRefs, now, oldProvider); err != nil {
		return TransferCredential{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_missions SET replacement_state='TRANSFER_IN_PROGRESS',runtime_state='TRANSFER_PENDING',
		reason_code='PROVIDER_TRANSFER_REQUIRED',source_version=$2,updated_at=$3 WHERE id=$1::uuid`, missionID, nextVersion, now); err != nil {
		return TransferCredential{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_mission_legs(commerce_mission_id,leg_ref,leg_type,state,parcel_refs,evidence_ref,created_at)
		VALUES($1::uuid,$2,'TRANSFER','CHALLENGE_ISSUED',$3,'',$4)`, missionID, transferRef, command.ParcelRefs, now); err != nil {
		return TransferCredential{}, err
	}
	result := TransferCredential{TransferRef: transferRef, MissionRef: missionRef, OldProviderID: oldProvider,
		NewProviderID: newProvider, ParcelRefs: command.ParcelRefs, PackageCount: command.PackageCount,
		ChallengeCode: challenge.Code, ChallengeExpires: challenge.ExpiresAt, SourceVersion: nextVersion}
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_mission_operations(caller_service,operation_ref,operation_type,request_fingerprint,commerce_mission_id,result_snapshot)
		VALUES($1,$2,'START_PROVIDER_TRANSFER',$3,$4::uuid,$5)`, "provider:"+providerID, command.OperationRef, fingerprint, missionID, jsonResult(result)); err != nil {
		return TransferCredential{}, operationConflict(err)
	}
	if err = emitOperationalEvent(ctx, r, tx, missionID, missionRef, "commerce.provider_transfer.started.v1", nextVersion, command.ParcelRefs, "PROVIDER_TRANSFER_REQUIRED", map[string]any{
		"transferRef": transferRef, "oldProviderRef": oldProvider, "newProviderRef": newProvider,
		"packageCount": command.PackageCount, "manifestBinding": bindings,
	}, now); err != nil {
		return TransferCredential{}, err
	}
	_ = transferID
	return result, tx.Commit(ctx)
}

func (s *OperationalService) CompleteProviderTransfer(ctx context.Context, providerID, missionRef, transferRef string, command CompleteTransferCommand) (dispatchcommerce.Mission, error) {
	if s == nil || s.repo == nil || len(s.handoverSecret) == 0 {
		return dispatchcommerce.Mission{}, apperrors.New(503, "TRANSFER_HANDOVER_UNAVAILABLE", "Provider transfer handover is unavailable.", nil)
	}
	command.OperationRef = strings.TrimSpace(command.OperationRef)
	command.ParcelRefs = canonicalRefs(command.ParcelRefs)
	if command.OperationRef == "" || strings.TrimSpace(command.Code) == "" || len(command.ParcelRefs) == 0 || command.PackageCount < 1 ||
		command.Latitude < -90 || command.Latitude > 90 || command.Longitude < -180 || command.Longitude > 180 {
		return dispatchcommerce.Mission{}, apperrors.BadRequest("Provider transfer completion is invalid.", nil)
	}
	transferEvidence := EvidenceCommand{
		ParcelRefs: command.ParcelRefs,
		MediaRefs:  canonicalRefs(command.Evidence.MediaRefs),
	}
	if err := s.authorizeEvidence(ctx, missionRef, transferEvidence); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	fingerprint, _ := dispatchcommerce.Fingerprint(struct {
		Mission, Transfer, Provider string
		Command                     CompleteTransferCommand
	}{missionRef, transferRef, providerID, command})
	fingerprint = handover.HashCode(s.handoverSecret, "dispatch-fingerprint:provider-transfer", fingerprint)
	if raw, stored, found, err := s.repo.FindOperation(ctx, "provider:"+providerID, command.OperationRef); err != nil {
		return dispatchcommerce.Mission{}, err
	} else if found {
		if stored != fingerprint {
			return dispatchcommerce.Mission{}, apperrors.New(409, "IDEMPOTENCY_CONFLICT", "Transfer operation was reused with different semantics.", nil)
		}
		var result dispatchcommerce.Mission
		if err = json.Unmarshal(raw, &result); err != nil {
			return dispatchcommerce.Mission{}, err
		}
		result.Replayed = true
		return result, nil
	}
	return s.repo.CompleteProviderTransfer(ctx, providerID, missionRef, transferRef, command, fingerprint, s.handoverSecret, s.now().UTC())
}

func (r *Repository) CompleteProviderTransfer(ctx context.Context, providerID, missionRef, transferRef string, command CompleteTransferCommand, fingerprint string, secret []byte, now time.Time) (dispatchcommerce.Mission, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	missionID, version, err := lockOperationalMission(ctx, tx, missionRef, providerID, true)
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if command.ExpectedVersion > 0 && command.ExpectedVersion != version {
		return dispatchcommerce.Mission{}, apperrors.New(409, "STALE_MISSION_VERSION", "Mission version changed.", nil)
	}
	var transferID, state, oldProvider, newProvider, oldTrip, newTrip, hash string
	var refs []string
	var bindingRaw []byte
	var packageCount int64
	var latitude, longitude float64
	var expires time.Time
	var attempts int
	err = tx.QueryRow(ctx, `SELECT id::text,state,old_provider_id::text,new_provider_id::text,old_trip_id::text,new_trip_id::text,
		parcel_refs,manifest_binding,package_count,latitude,longitude,challenge_hash,challenge_expires_at,challenge_attempts
		FROM commerce_provider_transfers WHERE commerce_mission_id=$1::uuid AND transfer_ref=$2 FOR UPDATE`, missionID, transferRef).Scan(
		&transferID, &state, &oldProvider, &newProvider, &oldTrip, &newTrip, &refs, &bindingRaw,
		&packageCount, &latitude, &longitude, &hash, &expires, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return dispatchcommerce.Mission{}, apperrors.NotFound("Provider transfer was not found.", nil)
	}
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if state == "COMPLETED" {
		return loadMissionViewTx(ctx, tx, missionID)
	}
	if providerID != newProvider {
		return dispatchcommerce.Mission{}, apperrors.Forbidden("Only the assigned replacement provider may accept custody.", nil)
	}
	if strings.Join(canonicalRefs(refs), "\x00") != strings.Join(command.ParcelRefs, "\x00") || packageCount != command.PackageCount ||
		math.Abs(latitude-command.Latitude) > 0.001 || math.Abs(longitude-command.Longitude) > 0.001 {
		return dispatchcommerce.Mission{}, apperrors.New(409, "TRANSFER_MANIFEST_MISMATCH", "Transfer parcel, package or location confirmation does not match.", nil)
	}
	if attempts >= handover.MaxVerifyAttempts {
		return dispatchcommerce.Mission{}, apperrors.New(429, "OTP_ATTEMPTS_EXCEEDED", "Transfer challenge attempt limit reached.", nil)
	}
	if now.After(expires) {
		return dispatchcommerce.Mission{}, apperrors.New(409, "OTP_EXPIRED", "Transfer challenge has expired.", nil)
	}
	var bindings []transferBinding
	if err = json.Unmarshal(bindingRaw, &bindings); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	bindingFingerprint, _ := dispatchcommerce.Fingerprint(bindings)
	challengeID := transferChallengeID(missionRef, transferRef, oldProvider, newProvider, bindingFingerprint)
	if !handover.VerifyCode(secret, challengeID, strings.TrimSpace(command.Code), hash) {
		if _, err = tx.Exec(ctx, `UPDATE commerce_provider_transfers SET challenge_attempts=challenge_attempts+1 WHERE id=$1::uuid`, transferID); err != nil {
			return dispatchcommerce.Mission{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return dispatchcommerce.Mission{}, err
		}
		return dispatchcommerce.Mission{}, apperrors.New(400, "OTP_INVALID", "Transfer challenge is invalid.", nil)
	}
	maxManifestVersion := int64(1)
	for _, item := range bindings {
		if item.ManifestVersion > maxManifestVersion {
			maxManifestVersion = item.ManifestVersion
		}
	}
	bindingRef := "manifest-binding:" + bindingFingerprint
	evidenceCommand := EvidenceCommand{OperationRef: command.OperationRef + ":evidence", Type: EvidenceProviderTransfer,
		ParcelRefs: command.ParcelRefs, ManifestRef: bindingRef, ManifestVersion: maxManifestVersion,
		PackageCount: command.PackageCount, CapturedAt: command.Evidence.CapturedAt,
		Latitude: &command.Latitude, Longitude: &command.Longitude, MediaRefs: canonicalRefs(command.Evidence.MediaRefs),
		Description: command.Evidence.Description, ReasonCode: "PROVIDER_REPLACEMENT"}
	if err = evidenceCommand.Validate(now); err != nil || len(evidenceCommand.MediaRefs) == 0 {
		return dispatchcommerce.Mission{}, apperrors.New(409, "TRANSFER_EVIDENCE_REQUIRED", "Transfer requires valid evidence and an opaque media reference.", err)
	}
	nextVersion := version + 1
	// The shared custody transfer needs its own immutable evidence record.
	// It is written before the transfer itself, so a failed transfer leaves
	// evidence of an attempt and never a custody change nobody performed.
	custodyEvidenceRef, err := r.recordCustodyTransferEvidence(ctx, missionRef, command.OperationRef, providerID, firstRef(evidenceCommand.MediaRefs), command.ParcelRefs, now)
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	evidence, err := insertEvidenceTx(ctx, tx, missionID, missionRef, "", "", "PROVIDER", providerID, evidenceCommand, nextVersion, now)
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	delivery, err := securehandover.NewCompatibilityCodeRecord(secret, handover.BuildChallengeID("dispatch", handover.StepDelivery, newTrip), now)
	if err != nil {
		return dispatchcommerce.Mission{}, apperrors.New(503, "OTP_DEPENDENCY_UNAVAILABLE", "Receiver handover regeneration is unavailable.", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE trips SET status='cancelled',delivery_code='',delivery_code_hash='',delivery_code_expires_at=NULL,
		cancelled_at=$2,updated_at=$2 WHERE id=$1::uuid AND status NOT IN ('completed','cancelled','failed')`, oldTrip, now); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if err = startCommerceTripTx(ctx, tx, missionID, delivery, now); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_mission_carrier_history SET custody_ended_at=$2,end_reason='PROVIDER_TRANSFER',source_version=$3
		WHERE commerce_mission_id=$1::uuid AND custody_ended_at IS NULL`, missionID, now, nextVersion); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_mission_carrier_history(commerce_mission_id,transfer_id,provider_id,trip_id,custody_started_at,source_version)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6)`, missionID, transferID, newProvider, newTrip, now, nextVersion); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_mission_parcels SET status='IN_TRANSIT',custodian_type='PROVIDER',custodian_ref=$3,updated_at=$4
		WHERE commerce_mission_id=$1::uuid AND parcel_ref=ANY($2) AND custodian_ref=$5`, missionID, command.ParcelRefs, newProvider, now, oldProvider); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_provider_transfers SET state='COMPLETED',evidence_id=$2::uuid,source_version=$3,completed_at=$4 WHERE id=$1::uuid`, transferID, evidence.ID, nextVersion, now); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	// Shared custody moves in the same transaction, recording exactly which
	// manifests changed hands.
	if err = r.transferCustodyTx(ctx, tx, missionID, transferRef, oldProvider, newProvider, custodyEvidenceRef, command.OperationRef, nextVersion); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_mission_legs SET state='COMPLETED',evidence_ref=$3,completed_at=$4 WHERE commerce_mission_id=$1::uuid AND leg_ref=$2`, missionID, transferRef, evidence.ID, now); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_missions SET trip_id=$2::uuid,current_provider_id=$3::uuid,replacement_state='NONE',
		replacement_provider_id=NULL,replacement_trip_id=NULL,runtime_state='IN_TRANSIT',queue_state='ASSIGNED',
		reason_code='',receiver_challenge_generation=receiver_challenge_generation+1,source_version=$4,updated_at=$5
		WHERE id=$1::uuid`, missionID, newTrip, newProvider, nextVersion, now); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	view, err := loadMissionViewTx(ctx, tx, missionID)
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_mission_operations(caller_service,operation_ref,operation_type,request_fingerprint,commerce_mission_id,result_snapshot)
		VALUES($1,$2,'COMPLETE_PROVIDER_TRANSFER',$3,$4::uuid,$5)`, "provider:"+providerID, command.OperationRef, fingerprint, missionID, jsonResult(view)); err != nil {
		return dispatchcommerce.Mission{}, operationConflict(err)
	}
	if err = r.emitParcelEvent(ctx, tx, missionID, missionRef, "", "IN_TRANSIT", nextVersion, now); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if err = emitOperationalEvent(ctx, r, tx, missionID, missionRef, "commerce.provider_transfer.completed.v1", nextVersion, command.ParcelRefs, "PROVIDER_REPLACEMENT", map[string]any{
		"transferRef": transferRef, "oldProviderRef": oldProvider, "newProviderRef": newProvider,
		"evidenceId": evidence.ID, "receiverChallengeRegenerated": true,
	}, now); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	return view, tx.Commit(ctx)
}

func loadCustodyBindingsTx(ctx context.Context, tx pgx.Tx, missionID, providerID string) ([]transferBinding, int64, error) {
	rows, err := tx.Query(ctx, `SELECT parcel_ref,manifest_ref,manifest_version,package_count FROM commerce_mission_parcels
		WHERE commerce_mission_id=$1::uuid AND custodian_type='PROVIDER' AND custodian_ref=$2 AND status IN ('PICKED_UP','IN_TRANSIT','TRANSFER_REQUIRED') ORDER BY parcel_ref`, missionID, providerID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var result []transferBinding
	var total int64
	for rows.Next() {
		var item transferBinding
		if err = rows.Scan(&item.ParcelRef, &item.ManifestRef, &item.ManifestVersion, &item.PackageCount); err != nil {
			return nil, 0, err
		}
		result = append(result, item)
		total += item.PackageCount
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	if len(result) == 0 {
		return nil, 0, apperrors.New(409, "TRANSFER_CUSTODY_REQUIRED", "No parcel is in the old provider's custody.", nil)
	}
	return result, total, nil
}

func transferChallengeID(missionRef, transferRef, oldProvider, newProvider, manifestFingerprint string) string {
	entity := fmt.Sprintf("%s:%s:%s:%s:%s", missionRef, transferRef, oldProvider, newProvider, manifestFingerprint)
	return handover.BuildChallengeID("dispatch", handover.StepProviderTransfer, entity)
}

func firstRef(refs []string) string {
	if len(refs) == 0 {
		return ""
	}
	return refs[0]
}
