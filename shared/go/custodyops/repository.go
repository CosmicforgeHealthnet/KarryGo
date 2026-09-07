package custodyops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MediaAuthorizer interface {
	ValidateCustodyMedia(context.Context, EvidenceCommand) error
}

type Repository struct {
	pool      *pgxpool.Pool
	authority string
	now       func() time.Time
}

func NewRepository(pool *pgxpool.Pool, authority string) *Repository {
	return &Repository{pool: pool, authority: strings.TrimSpace(authority), now: time.Now}
}

func (r *Repository) claimNonce(ctx context.Context, caller, nonce, path string, expires time.Time) error {
	if caller == "" || nonce == "" || path == "" {
		return ErrUnauthorized
	}
	command, err := r.pool.Exec(ctx, `INSERT INTO custody_request_nonces(caller,nonce,request_path,expires_at) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, caller, nonce, path, expires)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrUnauthorized
	}
	_, _ = r.pool.Exec(ctx, `DELETE FROM custody_request_nonces WHERE expires_at < now()`)
	return nil
}

func digest(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (r *Repository) transact(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func lockMission(ctx context.Context, tx pgx.Tx, mission string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 7335))`, mission)
	return err
}

func (r *Repository) FreezeManifest(ctx context.Context, command FreezeManifestCommand) (Manifest, error) {
	command.OperationRef, command.MissionRef, command.TenantRef = trim(command.OperationRef), trim(command.MissionRef), trim(command.TenantRef)
	command.BusinessRef, command.BranchRef = trim(command.BusinessRef), trim(command.BranchRef)
	command.ManifestRef, command.PickupStopRef = trim(command.ManifestRef), trim(command.PickupStopRef)
	if command.OperationRef == "" || command.MissionRef == "" || command.TenantRef == "" || command.BusinessRef == "" || command.BranchRef == "" || command.ManifestRef == "" || command.PickupStopRef == "" || command.ManifestVersion < 1 || len(command.Parcels) == 0 {
		return Manifest{}, ErrInvalid
	}
	parcelRefs := make([]string, 0, len(command.Parcels))
	var packageCount int64
	for i := range command.Parcels {
		command.Parcels[i].ParcelRef, command.Parcels[i].StopRef = trim(command.Parcels[i].ParcelRef), trim(command.Parcels[i].StopRef)
		if command.Parcels[i].ParcelRef == "" || command.Parcels[i].StopRef != command.PickupStopRef || command.Parcels[i].Count < 1 {
			return Manifest{}, ErrInvalid
		}
		parcelRefs = append(parcelRefs, command.Parcels[i].ParcelRef)
		packageCount += command.Parcels[i].Count
	}
	var err error
	if parcelRefs, err = CanonicalRefs(parcelRefs); err != nil {
		return Manifest{}, err
	}
	if command.CreatedAt.IsZero() {
		command.CreatedAt = r.now().UTC()
	}
	fp, parcelsJSON, now := digest(command), mustJSON(command.Parcels), r.now().UTC()
	result := Manifest{}
	err = r.transact(ctx, func(tx pgx.Tx) error {
		if err := lockMission(ctx, tx, command.MissionRef); err != nil {
			return err
		}
		var existingFP string
		err := tx.QueryRow(ctx, `SELECT authority,mission_ref,tenant_ref,business_ref,branch_ref,manifest_ref,manifest_version,pickup_stop_ref,parcel_refs,package_count,created_at,frozen_at,status,request_fingerprint FROM custody_manifests WHERE mission_ref=$1 FOR UPDATE`, command.MissionRef).Scan(&result.Authority, &result.MissionRef, &result.TenantRef, &result.BusinessRef, &result.BranchRef, &result.ManifestRef, &result.ManifestVersion, &result.PickupStopRef, &result.ParcelRefs, &result.PackageCount, &result.CreatedAt, &result.FrozenAt, &result.Status, &existingFP)
		if err == nil {
			if existingFP != fp {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO custody_manifests(authority,mission_ref,tenant_ref,business_ref,branch_ref,manifest_ref,manifest_version,pickup_stop_ref,parcel_refs,parcels,package_count,created_at,frozen_at,status,operation_ref,request_fingerprint) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'FROZEN',$14,$15)`, r.authority, command.MissionRef, command.TenantRef, command.BusinessRef, command.BranchRef, command.ManifestRef, command.ManifestVersion, command.PickupStopRef, parcelRefs, parcelsJSON, packageCount, command.CreatedAt, now, command.OperationRef, fp)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO custody_mission_state(mission_ref,tenant_ref,manifest_ref,manifest_version,status,version) VALUES($1,$2,$3,$4,'MANIFEST_FROZEN',1)`, command.MissionRef, command.TenantRef, command.ManifestRef, command.ManifestVersion)
		if err != nil {
			return err
		}
		if err = r.recordTx(ctx, tx, command.OperationRef, command.MissionRef, "MANIFEST_FROZEN", fp, map[string]any{"manifest_ref": command.ManifestRef, "manifest_version": command.ManifestVersion, "parcel_refs": parcelRefs, "package_count": packageCount}); err != nil {
			return err
		}
		result = Manifest{Authority: r.authority, MissionRef: command.MissionRef, TenantRef: command.TenantRef, BusinessRef: command.BusinessRef, BranchRef: command.BranchRef, ManifestRef: command.ManifestRef, ManifestVersion: command.ManifestVersion, PickupStopRef: command.PickupStopRef, ParcelRefs: parcelRefs, PackageCount: packageCount, CreatedAt: command.CreatedAt, FrozenAt: now, Status: "FROZEN"}
		return nil
	})
	return result, normalizeDBError(err)
}

func (r *Repository) ConfirmParcels(ctx context.Context, command ConfirmParcelsCommand) (Result, error) {
	command.OperationRef, command.MissionRef, command.TenantRef = trim(command.OperationRef), trim(command.MissionRef), trim(command.TenantRef)
	command.BusinessRef, command.BranchRef, command.PickupStopRef = trim(command.BusinessRef), trim(command.BranchRef), trim(command.PickupStopRef)
	command.ManifestRef, command.ProviderRef = trim(command.ManifestRef), trim(command.ProviderRef)
	if command.OperationRef == "" || command.MissionRef == "" || command.TenantRef == "" || command.BusinessRef == "" || command.BranchRef == "" || command.PickupStopRef == "" || command.ManifestRef == "" || command.ManifestVersion < 1 || command.ProviderRef == "" {
		return Result{}, ErrInvalid
	}
	refs, err := CanonicalRefs(command.ParcelRefs)
	if err != nil {
		return Result{}, err
	}
	command.ParcelRefs = refs
	fp, now := digest(command), r.now().UTC()
	result := Result{}
	err = r.transact(ctx, func(tx pgx.Tx) error {
		if err := lockMission(ctx, tx, command.MissionRef); err != nil {
			return err
		}
		if replay, ok, err := r.replayTx(ctx, tx, command.OperationRef, fp); err != nil {
			return err
		} else if ok {
			result = replay
			return nil
		}
		var tenant, business, branch, stop, manifest string
		var version, count int64
		var expected []string
		if err := tx.QueryRow(ctx, `SELECT tenant_ref,business_ref,branch_ref,pickup_stop_ref,manifest_ref,manifest_version,package_count,parcel_refs FROM custody_manifests WHERE mission_ref=$1 FOR UPDATE`, command.MissionRef).Scan(&tenant, &business, &branch, &stop, &manifest, &version, &count, &expected); err != nil {
			return err
		}
		if tenant != command.TenantRef || business != command.BusinessRef || branch != command.BranchRef || stop != command.PickupStopRef || manifest != command.ManifestRef || version != command.ManifestVersion || count != command.PackageCount || !sameRefs(expected, refs) {
			return ErrConflict
		}
		var current string
		if err := tx.QueryRow(ctx, `SELECT current_provider_ref FROM custody_mission_state WHERE mission_ref=$1 FOR UPDATE`, command.MissionRef).Scan(&current); err != nil {
			return err
		}
		if current != "" && current != command.ProviderRef {
			return ErrUnauthorized
		}
		_, err := tx.Exec(ctx, `UPDATE custody_mission_state SET current_provider_ref=$2,status='PICKED_UP',version=version+1,updated_at=now() WHERE mission_ref=$1`, command.MissionRef, command.ProviderRef)
		if err != nil {
			return err
		}
		result = Result{OperationRef: command.OperationRef, MissionRef: command.MissionRef, State: "PARCEL_CONFIRMED", Version: 1, CreatedAt: now}
		return r.recordTx(ctx, tx, command.OperationRef, command.MissionRef, "PARCEL_CONFIRMED", fp, command)
	})
	return result, normalizeDBError(err)
}

func (r *Repository) RecordEvidence(ctx context.Context, command EvidenceCommand, media MediaAuthorizer) (Evidence, error) {
	command.OperationRef, command.MissionRef, command.TenantRef = trim(command.OperationRef), trim(command.MissionRef), trim(command.TenantRef)
	command.ManifestRef, command.MediaRef, command.ProviderRef, command.ActorRef = trim(command.ManifestRef), trim(command.MediaRef), trim(command.ProviderRef), trim(command.ActorRef)
	if command.OperationRef == "" || command.MissionRef == "" || command.TenantRef == "" || command.ManifestRef == "" || command.ManifestVersion < 1 || command.MediaRef == "" || command.ActorRef == "" || !validEvidenceType(command.EvidenceType) {
		return Evidence{}, ErrInvalid
	}
	refs, err := CanonicalRefs(command.ParcelRefs)
	if len(command.ParcelRefs) > 0 && err != nil {
		return Evidence{}, err
	}
	command.ParcelRefs = refs
	if command.CapturedAt.IsZero() {
		return Evidence{}, ErrInvalid
	}
	if media == nil {
		return Evidence{}, ErrMediaRejected
	}
	if err = media.ValidateCustodyMedia(ctx, command); err != nil {
		return Evidence{}, ErrMediaRejected
	}
	fp, now, id := digest(command), r.now().UTC(), uuid.NewString()
	result := Evidence{}
	err = r.transact(ctx, func(tx pgx.Tx) error {
		if err := lockMission(ctx, tx, command.MissionRef); err != nil {
			return err
		}
		var tenant, manifest string
		var version int64
		var expected []string
		if err := tx.QueryRow(ctx, `SELECT tenant_ref,manifest_ref,manifest_version,parcel_refs FROM custody_manifests WHERE mission_ref=$1`, command.MissionRef).Scan(&tenant, &manifest, &version, &expected); err != nil {
			return err
		}
		if tenant != command.TenantRef || manifest != command.ManifestRef || version != command.ManifestVersion {
			return ErrConflict
		}
		for _, ref := range refs {
			found := false
			for _, want := range expected {
				if ref == want {
					found = true
					break
				}
			}
			if !found {
				return ErrConflict
			}
		}
		var existing Evidence
		var existingFP string
		err := tx.QueryRow(ctx, `SELECT id,request_fingerprint,created_at FROM custody_evidence WHERE operation_ref=$1`, command.OperationRef).Scan(&existing.ID, &existingFP, &existing.CreatedAt)
		if err == nil {
			if existingFP != fp {
				return ErrConflict
			}
			existing.EvidenceCommand = command
			existing.Authority = r.authority
			result = existing
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO custody_evidence(id,operation_ref,mission_ref,tenant_ref,manifest_ref,manifest_version,stop_ref,parcel_refs,evidence_type,media_ref,provider_ref,actor_ref,captured_at,latitude,longitude,access_policy,authority,request_fingerprint,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`, id, command.OperationRef, command.MissionRef, command.TenantRef, command.ManifestRef, command.ManifestVersion, command.StopRef, refs, command.EvidenceType, command.MediaRef, command.ProviderRef, command.ActorRef, command.CapturedAt, command.Latitude, command.Longitude, command.AccessPolicy, r.authority, fp, now)
		if err != nil {
			return err
		}
		if err = r.recordTx(ctx, tx, command.OperationRef, command.MissionRef, "EVIDENCE_RECORDED", fp, map[string]any{"evidence_id": id, "evidence_type": command.EvidenceType, "media_ref": command.MediaRef}); err != nil {
			return err
		}
		result = Evidence{ID: id, EvidenceCommand: command, Authority: r.authority, CreatedAt: now}
		return nil
	})
	return result, normalizeDBError(err)
}

func (r *Repository) ApplyOperation(ctx context.Context, kind string, command any, operationRef, missionRef, tenantRef, providerRef string, validate func(pgx.Tx) error) (Result, error) {
	operationRef, missionRef, tenantRef, providerRef = trim(operationRef), trim(missionRef), trim(tenantRef), trim(providerRef)
	if operationRef == "" || missionRef == "" || tenantRef == "" {
		return Result{}, ErrInvalid
	}
	fp, now := digest(command), r.now().UTC()
	result := Result{}
	err := r.transact(ctx, func(tx pgx.Tx) error {
		if err := lockMission(ctx, tx, missionRef); err != nil {
			return err
		}
		if replay, ok, err := r.replayTx(ctx, tx, operationRef, fp); err != nil {
			return err
		} else if ok {
			result = replay
			return nil
		}
		var tenant, current, status string
		var version int64
		if err := tx.QueryRow(ctx, `SELECT tenant_ref,current_provider_ref,status,version FROM custody_mission_state WHERE mission_ref=$1 FOR UPDATE`, missionRef).Scan(&tenant, &current, &status, &version); err != nil {
			return err
		}
		if tenant != tenantRef {
			return ErrUnauthorized
		}
		if providerRef != "" && current != "" && providerRef != current {
			return ErrUnauthorized
		}
		if status == "COMPLETED" || status == "CANCELLED" {
			return ErrConflict
		}
		if validate != nil {
			if err := validate(tx); err != nil {
				return err
			}
		}
		newState := operationState(kind)
		_, err := tx.Exec(ctx, `UPDATE custody_mission_state SET status=$2,version=version+1,updated_at=now() WHERE mission_ref=$1`, missionRef, newState)
		if err != nil {
			return err
		}
		result = Result{OperationRef: operationRef, MissionRef: missionRef, State: newState, Version: version + 1, CreatedAt: now}
		return r.recordTx(ctx, tx, operationRef, missionRef, kind, fp, command)
	})
	return result, normalizeDBError(err)
}

func (r *Repository) ReportException(ctx context.Context, command ExceptionCommand, policy Policy) (Result, error) {
	if err := policy.ValidateException(command); err != nil {
		return Result{}, err
	}
	return r.ApplyOperation(ctx, "EXCEPTION_REPORTED", command, command.OperationRef, command.MissionRef, command.TenantRef, command.ProviderRef, func(tx pgx.Tx) error {
		var manifest string
		var version int64
		if err := tx.QueryRow(ctx, `SELECT manifest_ref,manifest_version FROM custody_manifests WHERE mission_ref=$1`, command.MissionRef).Scan(&manifest, &version); err != nil {
			return err
		}
		if manifest != trim(command.ManifestRef) || version != command.ManifestVersion {
			return ErrConflict
		}
		return nil
	})
}

func (r *Repository) StartReturn(ctx context.Context, command ReturnCommand) (Result, error) {
	if trim(command.OriginalManifest) == "" || command.OriginalVersion < 1 || trim(command.ReturnManifest) == "" || trim(command.DestinationRef) == "" || trim(command.ReceiverRef) == "" || trim(command.EvidenceRef) == "" || !validReturnPolicy(command.CategoryPolicy) {
		return Result{}, ErrInvalid
	}
	refs, err := CanonicalRefs(command.ParcelRefs)
	if err != nil {
		return Result{}, err
	}
	command.ParcelRefs = refs
	return r.ApplyOperation(ctx, "RETURN_STARTED", command, command.OperationRef, command.MissionRef, command.TenantRef, "", func(tx pgx.Tx) error {
		var manifest string
		var version int64
		var expected []string
		if err := tx.QueryRow(ctx, `SELECT manifest_ref,manifest_version,parcel_refs FROM custody_manifests WHERE mission_ref=$1`, command.MissionRef).Scan(&manifest, &version, &expected); err != nil {
			return err
		}
		if manifest != command.OriginalManifest || version != command.OriginalVersion {
			return ErrConflict
		}
		for _, ref := range refs {
			found := false
			for _, x := range expected {
				if ref == x {
					found = true
					break
				}
			}
			if !found {
				return ErrConflict
			}
		}
		return nil
	})
}

func (r *Repository) ReportIncident(ctx context.Context, command IncidentCommand) (Result, error) {
	if command.Reason != IncidentBreakdown && command.Reason != IncidentIllness && command.Reason != IncidentSafetyIncident {
		return Result{}, ErrInvalid
	}
	return r.ApplyOperation(ctx, "REPLACEMENT_REQUIRED", command, command.OperationRef, command.MissionRef, command.TenantRef, command.ProviderRef, nil)
}

func (r *Repository) AssignReplacement(ctx context.Context, command AssignReplacementCommand) (Result, error) {
	if trim(command.OldProviderRef) == "" || trim(command.NewProviderRef) == "" || command.OldProviderRef == command.NewProviderRef || trim(command.VehicleClass) == "" || command.CapacityCount < 1 || trim(command.ComplianceRef) == "" || trim(command.AvailabilityRef) == "" || trim(command.ReceiverChallenge) == "" {
		return Result{}, ErrInvalid
	}
	return r.ApplyOperation(ctx, "REPLACEMENT_ASSIGNED", command, command.OperationRef, command.MissionRef, command.TenantRef, command.OldProviderRef, func(tx pgx.Tx) error {
		var status string
		var manifest string
		var version, count int64
		if err := tx.QueryRow(ctx, `SELECT s.status,m.manifest_ref,m.manifest_version,m.package_count FROM custody_mission_state s JOIN custody_manifests m USING(mission_ref) WHERE s.mission_ref=$1`, command.MissionRef).Scan(&status, &manifest, &version, &count); err != nil {
			return err
		}
		if status != "REPLACEMENT_REQUIRED" || manifest != command.ManifestRef || version != command.ManifestVersion || command.CapacityCount < count {
			return ErrConflict
		}
		_, err := tx.Exec(ctx, `UPDATE custody_mission_state SET replacement_provider_ref=$2,receiver_challenge_ref='',status='TRANSFER_REQUIRED' WHERE mission_ref=$1`, command.MissionRef, command.NewProviderRef)
		return err
	})
}

func (r *Repository) CompleteTransfer(ctx context.Context, command TransferCommand) (Result, error) {
	if trim(command.TransferRef) == "" || trim(command.OldProviderRef) == "" || trim(command.NewProviderRef) == "" || command.OldProviderRef == command.NewProviderRef || trim(command.EvidenceRef) == "" || !command.OldProviderConfirm || !command.NewProviderConfirm || trim(command.NewReceiverChallengeRef) == "" {
		return Result{}, ErrInvalid
	}
	refs, err := CanonicalRefs(command.ParcelRefs)
	if err != nil {
		return Result{}, err
	}
	command.ParcelRefs = refs
	result, err := r.ApplyOperation(ctx, "PROVIDER_TRANSFER_COMPLETED", command, command.OperationRef, command.MissionRef, command.TenantRef, command.OldProviderRef, func(tx pgx.Tx) error {
		var replacement, status, manifest string
		var version, count int64
		var expected []string
		if err := tx.QueryRow(ctx, `SELECT s.replacement_provider_ref,s.status,m.manifest_ref,m.manifest_version,m.package_count,m.parcel_refs FROM custody_mission_state s JOIN custody_manifests m USING(mission_ref) WHERE s.mission_ref=$1`, command.MissionRef).Scan(&replacement, &status, &manifest, &version, &count, &expected); err != nil {
			return err
		}
		if replacement != command.NewProviderRef || status != "TRANSFER_REQUIRED" || manifest != command.ManifestRef || version != command.ManifestVersion || count != command.PackageCount || !sameRefs(expected, refs) {
			return ErrConflict
		}
		var evidenceType EvidenceType
		if err := tx.QueryRow(ctx, `SELECT evidence_type FROM custody_evidence WHERE id=$1 AND mission_ref=$2`, command.EvidenceRef, command.MissionRef).Scan(&evidenceType); err != nil {
			return err
		}
		if evidenceType != EvidenceProviderTransfer {
			return ErrConflict
		}
		_, err := tx.Exec(ctx, `UPDATE custody_mission_state SET current_provider_ref=$2,replacement_provider_ref='',receiver_challenge_ref=$3,status='IN_TRANSIT' WHERE mission_ref=$1`, command.MissionRef, command.NewProviderRef, command.NewReceiverChallengeRef)
		return err
	})
	return result, err
}

func validReturnPolicy(value string) bool {
	switch strings.ToUpper(trim(value)) {
	case "PREPARED_FOOD_NO_RESTOCK", "PREPARED_FOOD_DISPOSAL", "RETAIL_RETURN_TO_VENDOR", "RETAIL_INSPECTION", "RETAIL_QUARANTINE":
		return true
	}
	return false
}
func operationState(kind string) string {
	switch kind {
	case "EXCEPTION_REPORTED":
		return "RECOVERY_REQUIRED"
	case "RETURN_STARTED":
		return "RETURN_REQUIRED"
	case "REPLACEMENT_REQUIRED":
		return "REPLACEMENT_REQUIRED"
	case "REPLACEMENT_ASSIGNED":
		return "TRANSFER_REQUIRED"
	case "PROVIDER_TRANSFER_COMPLETED":
		return "IN_TRANSIT"
	}
	return kind
}
func mustJSON(value any) []byte { raw, _ := json.Marshal(value); return raw }
func (r *Repository) recordTx(ctx context.Context, tx pgx.Tx, operation, mission, kind, fp string, payload any) error {
	raw := mustJSON(payload)
	_, err := tx.Exec(ctx, `INSERT INTO custody_operations(operation_ref,mission_ref,operation_type,request_fingerprint,state,payload,version) VALUES($1,$2,$3,$4,$3,$5,1)`, operation, mission, kind, fp, raw)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO custody_operation_outbox(id,aggregate_ref,event_type,payload) VALUES($1,$2,$3,$4)`, uuid.NewString(), mission, "custody."+strings.ToLower(kind)+".v1", raw)
	return err
}
func (r *Repository) replayTx(ctx context.Context, tx pgx.Tx, operation, fp string) (Result, bool, error) {
	var result Result
	var existing, mission, state string
	var version int64
	var created time.Time
	err := tx.QueryRow(ctx, `SELECT request_fingerprint,mission_ref,state,version,created_at FROM custody_operations WHERE operation_ref=$1 FOR UPDATE`, operation).Scan(&existing, &mission, &state, &version, &created)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, false, nil
	}
	if err != nil {
		return result, false, err
	}
	if existing != fp {
		return result, false, ErrConflict
	}
	return Result{OperationRef: operation, MissionRef: mission, State: state, Version: version, CreatedAt: created, Replayed: true}, true, nil
}
func normalizeDBError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "violates check constraint") {
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return err
}
