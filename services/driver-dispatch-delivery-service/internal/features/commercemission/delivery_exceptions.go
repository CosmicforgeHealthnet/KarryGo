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

type ExceptionCommand struct {
	OperationRef                  string          `json:"operation_ref"`
	ExpectedVersion               int64           `json:"expected_version,omitempty"`
	Reason                        ExceptionReason `json:"reason_code"`
	Recovery                      RecoveryAction  `json:"recovery_action"`
	ParcelRefs                    []string        `json:"parcel_refs"`
	Evidence                      EvidenceCommand `json:"evidence"`
	AlternativeReceiverAuthorized bool            `json:"alternative_receiver_authorized,omitempty"`
	AdditionalFeeMinor            int64           `json:"additional_fee_minor,omitempty"`
	CommercialApprovalRef         string          `json:"commercial_approval_ref,omitempty"`
}

type ExceptionRecord struct {
	ID            string          `json:"exception_id"`
	MissionRef    string          `json:"mission_ref"`
	OperationRef  string          `json:"operation_ref"`
	Reason        ExceptionReason `json:"reason_code"`
	State         string          `json:"state"`
	Recovery      RecoveryAction  `json:"recovery_action"`
	ParcelRefs    []string        `json:"parcel_refs"`
	EvidenceID    string          `json:"evidence_id"`
	SourceVersion int64           `json:"source_version"`
	CreatedAt     time.Time       `json:"created_at"`
	Replayed      bool            `json:"replayed,omitempty"`
}

func (s *OperationalService) ReportException(ctx context.Context, providerID, missionRef string, command ExceptionCommand) (ExceptionRecord, error) {
	if s == nil || s.repo == nil {
		return ExceptionRecord{}, apperrors.Unavailable("Delivery exception authority is unavailable.", nil)
	}
	command.OperationRef = strings.TrimSpace(command.OperationRef)
	command.ParcelRefs = canonicalRefs(command.ParcelRefs)
	// A provider-supplied boolean is not an authority to redirect a delivery to
	// another receiver. Until an accepted receiver-authorization authority is
	// connected, the provider API fails that recovery closed.
	if command.Recovery == RecoveryAlternativeReceiver {
		return ExceptionRecord{}, apperrors.New(409, "ALTERNATIVE_RECEIVER_AUTHORITY_UNAVAILABLE", "Alternative receiver authorization is unavailable.", nil)
	}
	if command.OperationRef == "" || !command.Reason.Valid() || !RecoveryAllowed(command.Reason, command.Recovery, false) {
		return ExceptionRecord{}, apperrors.BadRequest("Exception or recovery action is invalid.", nil)
	}
	if command.AdditionalFeeMinor < 0 {
		return ExceptionRecord{}, apperrors.BadRequest("Additional fee is invalid.", nil)
	}
	// Dispatch has no Section 24 commercial-approval read authority. A caller
	// supplied reference is not proof of approval, so every fee-bearing
	// recovery remains fail-closed until that authority is explicitly wired.
	if command.AdditionalFeeMinor > 0 {
		if strings.TrimSpace(command.CommercialApprovalRef) == "" {
			return ExceptionRecord{}, apperrors.New(409, "COMMERCIAL_APPROVAL_REQUIRED", "An additional customer fee requires an approved commercial authority.", nil)
		}
		return ExceptionRecord{}, apperrors.New(409, "COMMERCIAL_APPROVAL_AUTHORITY_UNAVAILABLE", "The commercial approval reference cannot be verified by Dispatch.", nil)
	}
	command.Evidence.OperationRef = command.OperationRef + ":evidence"
	command.Evidence.Type = command.Reason.EvidenceType()
	command.Evidence.ParcelRefs = command.ParcelRefs
	command.Evidence.ReasonCode = string(command.Reason)
	if err := command.Evidence.Validate(s.now().UTC()); err != nil {
		return ExceptionRecord{}, apperrors.BadRequest("Exception evidence is invalid.", err)
	}
	if (command.Reason == ReasonDamagedPackage || command.Reason == ReasonPackageMismatch) && len(command.Evidence.MediaRefs) == 0 {
		return ExceptionRecord{}, apperrors.New(409, "EVIDENCE_REQUIRED", "Damage and discrepancy exceptions require an opaque media reference.", nil)
	}
	if err := s.authorizeEvidence(ctx, missionRef, command.Evidence); err != nil {
		return ExceptionRecord{}, err
	}
	record, err := s.repo.ReportProviderException(ctx, providerID, missionRef, command, s.now().UTC())
	if err != nil {
		return record, err
	}
	if command.Reason.RequiresReplacement() && command.Recovery == RecoveryProviderReplacement {
		if s.missions == nil {
			return record, apperrors.Unavailable("Replacement matching authority is unavailable.", nil)
		}
		_, err = s.missions.ReplaceProvider(ctx, dispatchcommerce.SignatureCaller, missionRef, dispatchcommerce.CommandRequest{
			OperationRef: command.OperationRef + ":replacement", ExpectedVersion: record.SourceVersion,
			ReasonCode: string(command.Reason), CorrelationID: command.OperationRef,
		})
		if err != nil {
			return record, err
		}
	}
	return record, nil
}

func (r *Repository) ReportProviderException(ctx context.Context, providerID, missionRef string, command ExceptionCommand, now time.Time) (ExceptionRecord, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ExceptionRecord{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	missionID, version, err := lockOperationalMission(ctx, tx, missionRef, providerID, false)
	if err != nil {
		return ExceptionRecord{}, err
	}
	if existing, found, err := readExceptionTx(ctx, tx, missionID, command.OperationRef); err != nil {
		return ExceptionRecord{}, err
	} else if found {
		existing.Replayed = true
		return existing, tx.Commit(ctx)
	}
	if command.ExpectedVersion > 0 && command.ExpectedVersion != version {
		return ExceptionRecord{}, apperrors.New(409, "STALE_MISSION_VERSION", "Mission version changed.", nil)
	}
	var state string
	if err = tx.QueryRow(ctx, `SELECT runtime_state FROM commerce_missions WHERE id=$1::uuid`, missionID).Scan(&state); err != nil {
		return ExceptionRecord{}, err
	}
	if state == "COMPLETED" || state == "CANCELLED" {
		return ExceptionRecord{}, apperrors.New(409, "INVALID_MISSION_STATE", "A terminal mission cannot receive a new exception.", nil)
	}
	if command.Reason == ReasonVendorNotReady && state != "ASSIGNED" && state != "PICKING_UP" {
		return ExceptionRecord{}, apperrors.New(409, "INVALID_MISSION_STATE", "Vendor-not-ready is only valid before pickup.", nil)
	}
	if err = assertParcelScopeTx(ctx, tx, missionID, "", command.ParcelRefs, false); err != nil {
		return ExceptionRecord{}, err
	}
	nextVersion := version + 1
	evidence, err := insertEvidenceTx(ctx, tx, missionID, missionRef, "", "", "PROVIDER", providerID, command.Evidence, nextVersion, now)
	if err != nil {
		return ExceptionRecord{}, err
	}
	id := uuid.NewString()
	status := "RECOVERY_SELECTED"
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_delivery_exceptions(
		id,commerce_mission_id,operation_ref,reason_code,state,recovery_action,actor_type,actor_ref,
		parcel_refs,evidence_id,source_version,created_at)
		VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,'PROVIDER',$7,$8,$9::uuid,$10,$11)`,
		id, missionID, command.OperationRef, string(command.Reason), status, string(command.Recovery),
		providerID, command.ParcelRefs, evidence.ID, nextVersion, now); err != nil {
		return ExceptionRecord{}, operationConflict(err)
	}
	runtimeState := "RECOVERY_REQUIRED"
	replacementState := "NONE"
	if command.Reason.RequiresReplacement() {
		runtimeState, replacementState = "REPLACEMENT_REQUIRED", "REQUIRED"
	} else if command.Reason.FailClosedBeforeCustody() {
		runtimeState = "PICKUP_BLOCKED"
	} else if command.Reason == ReasonReceiverUnavailable || command.Reason == ReasonReceiverRefused {
		runtimeState = "DELIVERY_EXCEPTION"
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_missions SET runtime_state=$2,replacement_state=CASE WHEN $3='NONE' THEN replacement_state ELSE $3 END,
		replacement_reason=CASE WHEN $3='NONE' THEN replacement_reason ELSE $4 END,reason_code=$4,
		source_version=$5,updated_at=$6 WHERE id=$1::uuid`, missionID, runtimeState, replacementState, string(command.Reason), nextVersion, now); err != nil {
		return ExceptionRecord{}, err
	}
	record := ExceptionRecord{ID: id, MissionRef: missionRef, OperationRef: command.OperationRef,
		Reason: command.Reason, State: status, Recovery: command.Recovery, ParcelRefs: command.ParcelRefs,
		EvidenceID: evidence.ID, SourceVersion: nextVersion, CreatedAt: now}
	if err = emitOperationalEvent(ctx, r, tx, missionID, missionRef, "commerce.evidence.recorded.v1", nextVersion, command.ParcelRefs, string(command.Reason), map[string]any{"evidenceId": evidence.ID, "evidenceType": command.Evidence.Type}, now); err != nil {
		return ExceptionRecord{}, err
	}
	if err = emitOperationalEvent(ctx, r, tx, missionID, missionRef, "commerce.exception.reported.v1", nextVersion, command.ParcelRefs, string(command.Reason), map[string]any{"exceptionId": id, "recoveryAction": command.Recovery, "state": runtimeState}, now); err != nil {
		return ExceptionRecord{}, err
	}
	return record, tx.Commit(ctx)
}

func readExceptionTx(ctx context.Context, tx pgx.Tx, missionID, operation string) (ExceptionRecord, bool, error) {
	var item ExceptionRecord
	err := tx.QueryRow(ctx, `SELECT x.id::text,m.canonical_ref,x.operation_ref,x.reason_code,x.state,x.recovery_action,
		x.parcel_refs,COALESCE(x.evidence_id::text,''),x.source_version,x.created_at
		FROM commerce_delivery_exceptions x JOIN commerce_missions m ON m.id=x.commerce_mission_id
		WHERE x.commerce_mission_id=$1::uuid AND x.operation_ref=$2`, missionID, operation).Scan(
		&item.ID, &item.MissionRef, &item.OperationRef, &item.Reason, &item.State, &item.Recovery,
		&item.ParcelRefs, &item.EvidenceID, &item.SourceVersion, &item.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ExceptionRecord{}, false, nil
	}
	return item, err == nil, err
}

type ReturnCommand struct {
	OperationRef    string            `json:"operation_ref"`
	ExpectedVersion int64             `json:"expected_version,omitempty"`
	ProductClass    ProductClass      `json:"product_class"`
	Disposition     ReturnDisposition `json:"disposition"`
	ParcelRefs      []string          `json:"parcel_refs"`
	ReasonCode      string            `json:"reason_code"`
	Evidence        EvidenceCommand   `json:"evidence"`
}

type ReturnRecord struct {
	ID            string            `json:"return_id"`
	MissionRef    string            `json:"mission_ref"`
	OperationRef  string            `json:"operation_ref"`
	ProductClass  ProductClass      `json:"product_class"`
	Disposition   ReturnDisposition `json:"disposition"`
	State         string            `json:"state"`
	ParcelRefs    []string          `json:"parcel_refs"`
	ReturnLegRef  string            `json:"return_leg_ref"`
	EvidenceID    string            `json:"evidence_id"`
	SourceVersion int64             `json:"source_version"`
	CreatedAt     time.Time         `json:"created_at"`
	Replayed      bool              `json:"replayed,omitempty"`
}

type CompleteReturnCommand struct {
	OperationRef    string          `json:"operation_ref"`
	ExpectedVersion int64           `json:"expected_version,omitempty"`
	ReturnID        string          `json:"return_id"`
	ParcelRefs      []string        `json:"parcel_refs"`
	Evidence        EvidenceCommand `json:"evidence"`
}

func (s *OperationalService) StartReturn(ctx context.Context, providerID, missionRef string, command ReturnCommand) (ReturnRecord, error) {
	if s == nil || s.repo == nil || strings.TrimSpace(command.OperationRef) == "" || !ReturnDispositionAllowed(command.ProductClass, command.Disposition) {
		return ReturnRecord{}, apperrors.BadRequest("Return policy is invalid.", nil)
	}
	command.ParcelRefs = canonicalRefs(command.ParcelRefs)
	command.Evidence.OperationRef = command.OperationRef + ":evidence"
	command.Evidence.Type = EvidenceReturn
	command.Evidence.ParcelRefs = command.ParcelRefs
	command.Evidence.ReasonCode = strings.TrimSpace(command.ReasonCode)
	if err := command.Evidence.Validate(s.now().UTC()); err != nil {
		return ReturnRecord{}, apperrors.BadRequest("Return evidence is invalid.", err)
	}
	if err := s.authorizeEvidence(ctx, missionRef, command.Evidence); err != nil {
		return ReturnRecord{}, err
	}
	fingerprint, _ := dispatchcommerce.Fingerprint(struct {
		Mission, Provider string
		Command           ReturnCommand
	}{missionRef, providerID, command})
	if raw, stored, found, err := s.repo.FindOperation(ctx, "provider:"+providerID, command.OperationRef); err != nil {
		return ReturnRecord{}, err
	} else if found {
		if stored != fingerprint {
			return ReturnRecord{}, apperrors.New(409, "IDEMPOTENCY_CONFLICT", "Return operation was reused with different semantics.", nil)
		}
		var result ReturnRecord
		if err = json.Unmarshal(raw, &result); err != nil {
			return ReturnRecord{}, err
		}
		result.Replayed = true
		return result, nil
	}
	return s.repo.StartProviderReturn(ctx, providerID, missionRef, command, fingerprint, s.now().UTC())
}

func (r *Repository) StartProviderReturn(ctx context.Context, providerID, missionRef string, command ReturnCommand, fingerprint string, now time.Time) (ReturnRecord, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ReturnRecord{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	missionID, version, err := lockOperationalMission(ctx, tx, missionRef, providerID, false)
	if err != nil {
		return ReturnRecord{}, err
	}
	if command.ExpectedVersion > 0 && command.ExpectedVersion != version {
		return ReturnRecord{}, apperrors.New(409, "STALE_MISSION_VERSION", "Mission version changed.", nil)
	}
	if err = assertParcelScopeTx(ctx, tx, missionID, "", command.ParcelRefs, false); err != nil {
		return ReturnRecord{}, err
	}
	var custody int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM commerce_mission_parcels WHERE commerce_mission_id=$1::uuid AND parcel_ref=ANY($2) AND custodian_type='PROVIDER' AND status IN ('PICKED_UP','IN_TRANSIT','RETURN_REQUIRED')`, missionID, command.ParcelRefs).Scan(&custody); err != nil {
		return ReturnRecord{}, err
	}
	if custody != len(command.ParcelRefs) {
		return ReturnRecord{}, apperrors.New(409, "RETURN_CUSTODY_REQUIRED", "Only parcels in this provider's custody may enter return.", nil)
	}
	nextVersion := version + 1
	evidence, err := insertEvidenceTx(ctx, tx, missionID, missionRef, "", "", "PROVIDER", providerID, command.Evidence, nextVersion, now)
	if err != nil {
		return ReturnRecord{}, err
	}
	id, legRef := uuid.NewString(), "return_"+strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_return_operations(id,commerce_mission_id,operation_ref,product_class,state,disposition,parcel_refs,return_leg_ref,evidence_id,source_version,created_at)
		VALUES($1::uuid,$2::uuid,$3,$4,'IN_RETURN',$5,$6,$7,$8::uuid,$9,$10)`, id, missionID, command.OperationRef, string(command.ProductClass), string(command.Disposition), command.ParcelRefs, legRef, evidence.ID, nextVersion, now); err != nil {
		return ReturnRecord{}, operationConflict(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_mission_parcels SET status='RETURN_REQUIRED',updated_at=$3 WHERE commerce_mission_id=$1::uuid AND parcel_ref=ANY($2)`, missionID, command.ParcelRefs, now); err != nil {
		return ReturnRecord{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_mission_legs(commerce_mission_id,leg_ref,leg_type,state,parcel_refs,evidence_ref,created_at) VALUES($1::uuid,$2,'RETURN','IN_RETURN',$3,$4,$5)`, missionID, legRef, command.ParcelRefs, evidence.ID, now); err != nil {
		return ReturnRecord{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_missions SET runtime_state='RETURNING',reason_code=$2,source_version=$3,updated_at=$4 WHERE id=$1::uuid`, missionID, strings.TrimSpace(command.ReasonCode), nextVersion, now); err != nil {
		return ReturnRecord{}, err
	}
	record := ReturnRecord{ID: id, MissionRef: missionRef, OperationRef: command.OperationRef, ProductClass: command.ProductClass,
		Disposition: command.Disposition, State: "IN_RETURN", ParcelRefs: command.ParcelRefs, ReturnLegRef: legRef,
		EvidenceID: evidence.ID, SourceVersion: nextVersion, CreatedAt: now}
	if err = emitOperationalEvent(ctx, r, tx, missionID, missionRef, "commerce.return.started.v1", nextVersion, command.ParcelRefs, command.ReasonCode, map[string]any{"returnId": id, "returnLegRef": legRef, "productClass": command.ProductClass, "disposition": command.Disposition, "evidenceId": evidence.ID}, now); err != nil {
		return ReturnRecord{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_mission_operations(caller_service,operation_ref,operation_type,request_fingerprint,commerce_mission_id,result_snapshot)
		VALUES($1,$2,'START_RETURN',$3,$4::uuid,$5)`, "provider:"+providerID, command.OperationRef, fingerprint, missionID, jsonResult(record)); err != nil {
		return ReturnRecord{}, operationConflict(err)
	}
	return record, tx.Commit(ctx)
}

func (s *OperationalService) CompleteReturn(ctx context.Context, providerID, missionRef string, command CompleteReturnCommand) (ReturnRecord, error) {
	if s == nil || s.repo == nil || strings.TrimSpace(command.OperationRef) == "" || strings.TrimSpace(command.ReturnID) == "" {
		return ReturnRecord{}, apperrors.BadRequest("Return completion is invalid.", nil)
	}
	command.ParcelRefs = canonicalRefs(command.ParcelRefs)
	command.Evidence.OperationRef = command.OperationRef + ":evidence"
	command.Evidence.Type = EvidenceReturn
	command.Evidence.ParcelRefs = command.ParcelRefs
	if err := command.Evidence.Validate(s.now().UTC()); err != nil {
		return ReturnRecord{}, apperrors.BadRequest("Return completion evidence is invalid.", err)
	}
	if err := s.authorizeEvidence(ctx, missionRef, command.Evidence); err != nil {
		return ReturnRecord{}, err
	}
	return s.repo.CompleteProviderReturn(ctx, providerID, missionRef, command, s.now().UTC())
}

func (r *Repository) CompleteProviderReturn(ctx context.Context, providerID, missionRef string, command CompleteReturnCommand, now time.Time) (ReturnRecord, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ReturnRecord{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	missionID, version, err := lockOperationalMission(ctx, tx, missionRef, providerID, false)
	if err != nil {
		return ReturnRecord{}, err
	}
	if command.ExpectedVersion > 0 && command.ExpectedVersion != version {
		return ReturnRecord{}, apperrors.New(409, "STALE_MISSION_VERSION", "Mission version changed.", nil)
	}
	var record ReturnRecord
	err = tx.QueryRow(ctx, `SELECT r.id::text,m.canonical_ref,r.operation_ref,r.product_class,r.disposition,r.state,r.parcel_refs,r.return_leg_ref,COALESCE(r.completion_evidence_id::text,r.evidence_id::text,''),r.source_version,r.created_at
		FROM commerce_return_operations r JOIN commerce_missions m ON m.id=r.commerce_mission_id
		WHERE r.id=$1::uuid AND r.commerce_mission_id=$2::uuid FOR UPDATE OF r`, command.ReturnID, missionID).Scan(
		&record.ID, &record.MissionRef, &record.OperationRef, &record.ProductClass, &record.Disposition,
		&record.State, &record.ParcelRefs, &record.ReturnLegRef, &record.EvidenceID, &record.SourceVersion, &record.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReturnRecord{}, apperrors.NotFound("Return operation not found.", nil)
	}
	if err != nil {
		return ReturnRecord{}, err
	}
	if record.State == "RETURNED" {
		return record, tx.Commit(ctx)
	}
	if strings.Join(canonicalRefs(record.ParcelRefs), "\x00") != strings.Join(command.ParcelRefs, "\x00") {
		return ReturnRecord{}, apperrors.New(409, "PARCEL_SCOPE_MISMATCH", "Return completion must confirm the exact original parcels.", nil)
	}
	nextVersion := version + 1
	evidence, err := insertEvidenceTx(ctx, tx, missionID, missionRef, "", "", "PROVIDER", providerID, command.Evidence, nextVersion, now)
	if err != nil {
		return ReturnRecord{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_return_operations SET state='RETURNED',completion_evidence_id=$2::uuid,source_version=$3,completed_at=$4 WHERE id=$1::uuid`, record.ID, evidence.ID, nextVersion, now); err != nil {
		return ReturnRecord{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_mission_legs SET state='RETURNED',evidence_ref=$3,completed_at=$4 WHERE commerce_mission_id=$1::uuid AND leg_ref=$2`, missionID, record.ReturnLegRef, evidence.ID, now); err != nil {
		return ReturnRecord{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_mission_parcels SET status='RETURNED',custodian_type='RETURN',custodian_ref=branch_ref,returned_at=$3,updated_at=$3 WHERE commerce_mission_id=$1::uuid AND parcel_ref=ANY($2)`, missionID, command.ParcelRefs, now); err != nil {
		return ReturnRecord{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_missions SET runtime_state='RETURNED',queue_state='COMPLETED',source_version=$2,updated_at=$3 WHERE id=$1::uuid`, missionID, nextVersion, now); err != nil {
		return ReturnRecord{}, err
	}
	record.State, record.EvidenceID, record.SourceVersion = "RETURNED", evidence.ID, nextVersion
	if err = emitOperationalEvent(ctx, r, tx, missionID, missionRef, "commerce.return.completed.v1", nextVersion, command.ParcelRefs, "", map[string]any{"returnId": record.ID, "returnLegRef": record.ReturnLegRef, "evidenceId": evidence.ID}, now); err != nil {
		return ReturnRecord{}, err
	}
	return record, tx.Commit(ctx)
}

func encodeRecord(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}
