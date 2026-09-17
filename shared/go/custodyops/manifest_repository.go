package custodyops

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// loadSnapshot reads the whole custody ledger for one mission. Callers must
// already hold the mission advisory lock; state rows are also locked.
func loadSnapshot(ctx context.Context, tx pgx.Tx, mission string) (MissionSnapshot, error) {
	s := MissionSnapshot{MissionRef: mission, Stops: map[string]StopState{}, Manifests: map[ManifestKey]ManifestState{}}
	err := tx.QueryRow(ctx, `
		SELECT m.custody_mode, m.tenant_ref, m.source_version, s.status, s.current_provider_ref, s.replacement_provider_ref
		  FROM custody_missions m JOIN custody_mission_state s USING (mission_ref)
		 WHERE m.mission_ref=$1 FOR UPDATE OF m, s`, mission).
		Scan(&s.Mode, &s.TenantRef, &s.SourceVersion, &s.Status, &s.CurrentProvider, &s.ReplacementProvider)
	if err != nil {
		return s, err
	}
	rows, err := tx.Query(ctx, `SELECT stop_ref, stop_type, sequence, status FROM custody_mission_stops WHERE mission_ref=$1`, mission)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var stop StopState
		if err = rows.Scan(&stop.StopRef, &stop.StopType, &stop.Sequence, &stop.Status); err != nil {
			rows.Close()
			return s, err
		}
		s.Stops[stop.StopRef] = stop
	}
	rows.Close()
	rows, err = tx.Query(ctx, `SELECT manifest_ref, manifest_version, COALESCE(pickup_stop_ref,''), status, current_custodian_ref, package_count, required FROM custody_stop_manifests WHERE mission_ref=$1 FOR UPDATE`, mission)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var m ManifestState
		if err = rows.Scan(&m.ManifestRef, &m.ManifestVersion, &m.PickupStopRef, &m.Status, &m.Custodian, &m.PackageCount, &m.Required); err != nil {
			return s, err
		}
		s.Manifests[m.ManifestKey] = m
	}
	return s, rows.Err()
}

func (r *Repository) writeManifestEvent(ctx context.Context, tx pgx.Tx, eventType, mission, stop string, key ManifestKey, sourceVersion int64, correlation string, extra map[string]any) error {
	id := uuid.NewString()
	payload := map[string]any{
		"event_id": id, "authority": r.authority, "mission_ref": mission, "stop_ref": stop,
		"manifest_ref": key.ManifestRef, "manifest_version": key.ManifestVersion,
		"source_version": sourceVersion, "correlation_id": correlation, "occurred_at": r.now().UTC(),
	}
	for k, v := range extra {
		payload[k] = v
	}
	_, err := tx.Exec(ctx, `INSERT INTO custody_operation_outbox(id,aggregate_ref,event_type,payload) VALUES($1,$2,$3,$4)`, id, mission, eventType, mustJSON(payload))
	return err
}

func advanceSourceVersion(ctx context.Context, tx pgx.Tx, mission string, version int64) error {
	_, err := tx.Exec(ctx, `UPDATE custody_missions SET source_version=GREATEST(source_version,$2) WHERE mission_ref=$1`, mission, version)
	return err
}

// RegisterMission creates a MULTI_MANIFEST custody ledger with immutable
// manifest and package snapshots. Replaying the same operation is a no-op.
func (r *Repository) RegisterMission(ctx context.Context, command RegisterMissionCommand) (Result, error) {
	var result Result
	err := r.transact(ctx, func(tx pgx.Tx) error {
		var err error
		result, err = r.RegisterMissionTx(ctx, tx, command)
		return err
	})
	return result, normalizeDBError(err)
}

// RegisterMissionTx runs inside a caller-owned transaction so an execution service
// can commit its own lifecycle change and the custody ledger atomically.
func (r *Repository) RegisterMissionTx(ctx context.Context, tx pgx.Tx, command RegisterMissionCommand) (Result, error) {
	if err := command.Normalize(); err != nil {
		return Result{}, err
	}
	fp, now := digest(command), r.now().UTC()
	var result Result
	if err := lockMission(ctx, tx, command.MissionRef); err != nil {
		return Result{}, err
	}
	if replay, ok, err := r.replayTx(ctx, tx, command.OperationRef, fp); err != nil {
		return Result{}, err
	} else if ok {
		return replay, nil
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM custody_missions WHERE mission_ref=$1)`, command.MissionRef).Scan(&exists); err != nil {
		return Result{}, err
	}
	if exists {
		return Result{}, ErrConflict
	}
	if _, err := tx.Exec(ctx, `INSERT INTO custody_missions(mission_ref,authority,tenant_ref,custody_mode,source_version) VALUES($1,$2,$3,'MULTI_MANIFEST',$4)`, command.MissionRef, r.authority, command.TenantRef, command.SourceVersion); err != nil {
		return Result{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO custody_mission_state(mission_ref,tenant_ref,receiver_challenge_ref,status,version) VALUES($1,$2,$3,'MANIFESTS_REGISTERED',1)`, command.MissionRef, command.TenantRef, command.ReceiverChallengeRef); err != nil {
		return Result{}, err
	}
	for _, stop := range command.Stops {
		if _, err := tx.Exec(ctx, `INSERT INTO custody_mission_stops(mission_ref,stop_ref,stop_type,sequence) VALUES($1,$2,$3,$4)`, command.MissionRef, stop.StopRef, stop.StopType, stop.Sequence); err != nil {
			return Result{}, err
		}
	}
	for _, m := range command.Manifests {
		count, weight, volume := m.Totals()
		handling := m.Handling
		if handling == nil {
			handling = []string{}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO custody_stop_manifests(mission_ref,manifest_ref,manifest_version,pickup_stop_ref,source_authority,
				source_manifest_checksum,vendor_ref,vendor_branch_ref,package_count,aggregate_weight_grams,aggregate_volume_ml,
				handling,required,operation_ref)
			VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),$9,$10,$11,$12,$13,$14)`,
			command.MissionRef, m.ManifestRef, m.ManifestVersion, m.PickupStopRef, m.SourceAuthority, m.SourceManifestChecksum,
			m.VendorRef, m.VendorBranchRef, count, weight, volume, handling, !m.Optional, command.OperationRef); err != nil {
			return Result{}, err
		}
		for _, p := range m.Packages {
			if _, err := tx.Exec(ctx, `INSERT INTO custody_manifest_packages(mission_ref,manifest_ref,manifest_version,package_ref,package_count,weight_grams,volume_ml) VALUES($1,$2,$3,$4,$5,$6,$7)`,
				command.MissionRef, m.ManifestRef, m.ManifestVersion, p.PackageRef, p.PackageCount, p.WeightGrams, p.VolumeML); err != nil {
				return Result{}, err
			}
		}
		if err := r.writeManifestEvent(ctx, tx, "custody.manifest.registered.v1", command.MissionRef, m.PickupStopRef, m.Key(), command.SourceVersion, command.CorrelationID, map[string]any{"package_count": count}); err != nil {
			return Result{}, err
		}
	}
	result = Result{OperationRef: command.OperationRef, MissionRef: command.MissionRef, State: "MANIFESTS_REGISTERED", Version: 1, CreatedAt: now}
	return result, r.recordTx(ctx, tx, command.OperationRef, command.MissionRef, "MANIFESTS_REGISTERED", fp, map[string]any{"manifest_count": len(command.Manifests), "stop_count": len(command.Stops), "source_version": command.SourceVersion})
}

// CollectManifests records a verified pickup handover for manifests at one
// stop. Custody is created only for the manifests actually collected.
func (r *Repository) CollectManifests(ctx context.Context, command CollectManifestsCommand) (Result, error) {
	var result Result
	err := r.transact(ctx, func(tx pgx.Tx) error {
		var err error
		result, err = r.CollectManifestsTx(ctx, tx, command)
		return err
	})
	return result, normalizeDBError(err)
}

// CollectManifestsTx runs inside a caller-owned transaction so an execution service
// can commit its own lifecycle change and the custody ledger atomically.
func (r *Repository) CollectManifestsTx(ctx context.Context, tx pgx.Tx, command CollectManifestsCommand) (Result, error) {
	command.MissionRef, command.OperationRef = trim(command.MissionRef), trim(command.OperationRef)
	if command.MissionRef == "" || command.OperationRef == "" {
		return Result{}, ErrInvalid
	}
	keys, err := canonicalKeys(command.Manifests)
	if err != nil {
		return Result{}, err
	}
	command.Manifests = keys
	fp, now := digest(command), r.now().UTC()
	var result Result
	if err := lockMission(ctx, tx, command.MissionRef); err != nil {
		return Result{}, err
	}
	if replay, ok, err := r.replayTx(ctx, tx, command.OperationRef, fp); err != nil {
		return Result{}, err
	} else if ok {
		return replay, nil
	}
	snapshot, err := loadSnapshot(ctx, tx, command.MissionRef)
	if err != nil {
		return Result{}, err
	}
	decision, err := DecideCollection(snapshot, command)
	if err != nil {
		return Result{}, err
	}
	if decision.AlreadyHeld {
		result = Result{OperationRef: command.OperationRef, MissionRef: command.MissionRef, State: "MANIFESTS_COLLECTED", Version: snapshot.SourceVersion, CreatedAt: now, Replayed: true}
		return result, nil
	}
	provider := trim(command.ProviderRef)
	for _, key := range decision.Manifests {
		if _, err := tx.Exec(ctx, `UPDATE custody_stop_manifests SET status='COLLECTED',current_custodian_ref=$4,collected_at=now(),version=version+1,updated_at=now() WHERE mission_ref=$1 AND manifest_ref=$2 AND manifest_version=$3 AND status='REGISTERED'`, command.MissionRef, key.ManifestRef, key.ManifestVersion, provider); err != nil {
			return Result{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE custody_manifest_packages SET status='COLLECTED',updated_at=now() WHERE mission_ref=$1 AND manifest_ref=$2 AND manifest_version=$3 AND status='EXPECTED'`, command.MissionRef, key.ManifestRef, key.ManifestVersion); err != nil {
			return Result{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO custody_manifest_holdings(id,mission_ref,manifest_ref,manifest_version,custodian_ref,acquired_operation_ref,acquired_via) VALUES($1,$2,$3,$4,$5,$6,'PICKUP_HANDOVER')`, uuid.NewString(), command.MissionRef, key.ManifestRef, key.ManifestVersion, provider, command.OperationRef); err != nil {
			return Result{}, err
		}
		if err := r.writeManifestEvent(ctx, tx, "custody.manifest.collected.v1", command.MissionRef, trim(command.StopRef), key, command.SourceVersion, command.CorrelationID, map[string]any{"custodian_ref": provider}); err != nil {
			return Result{}, err
		}
	}
	if err := r.writeManifestEvent(ctx, tx, "custody.handover_verified.v1", command.MissionRef, trim(command.StopRef), decision.Manifests[0], command.SourceVersion, command.CorrelationID, map[string]any{"stage": "PICKUP", "handover_challenge_ref": trim(command.HandoverChallengeRef), "manifests": decision.Manifests}); err != nil {
		return Result{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE custody_mission_stops SET status=$3,version=version+1,updated_at=now() WHERE mission_ref=$1 AND stop_ref=$2`, command.MissionRef, trim(command.StopRef), decision.StopStatus); err != nil {
		return Result{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE custody_mission_state SET current_provider_ref=$2,status='PICKED_UP',version=version+1,updated_at=now() WHERE mission_ref=$1`, command.MissionRef, provider); err != nil {
		return Result{}, err
	}
	if err := advanceSourceVersion(ctx, tx, command.MissionRef, command.SourceVersion); err != nil {
		return Result{}, err
	}
	result = Result{OperationRef: command.OperationRef, MissionRef: command.MissionRef, State: "MANIFESTS_COLLECTED", Version: command.SourceVersion, CreatedAt: now}
	return result, r.recordTx(ctx, tx, command.OperationRef, command.MissionRef, "MANIFESTS_COLLECTED", fp, command)
}

// AssignManifestReplacement moves a REPLACEMENT_REQUIRED mission to
// TRANSFER_REQUIRED. The replacement's capacity must cover the packages
// already in custody plus the packages still pending at future stops.
func (r *Repository) AssignManifestReplacement(ctx context.Context, command AssignReplacementCommand) (Result, error) {
	if trim(command.OldProviderRef) == "" || trim(command.NewProviderRef) == "" || trim(command.OldProviderRef) == trim(command.NewProviderRef) ||
		trim(command.VehicleClass) == "" || command.CapacityCount < 1 || trim(command.ComplianceRef) == "" || trim(command.AvailabilityRef) == "" {
		return Result{}, ErrInvalid
	}
	return r.ApplyOperation(ctx, "REPLACEMENT_ASSIGNED", command, command.OperationRef, command.MissionRef, command.TenantRef, command.OldProviderRef, func(tx pgx.Tx) error {
		snapshot, err := loadSnapshot(ctx, tx, trim(command.MissionRef))
		if err != nil {
			return err
		}
		if snapshot.Mode != ModeMultiManifest || snapshot.Status != "REPLACEMENT_REQUIRED" {
			return ErrConflict
		}
		var required int64
		for _, m := range snapshot.Manifests {
			if m.Required && (m.Status == ManifestCollected || m.Status == ManifestRegistered) {
				required += m.PackageCount
			}
		}
		if command.CapacityCount < required {
			return ErrPolicy
		}
		if _, err := tx.Exec(ctx, `UPDATE custody_mission_state SET replacement_provider_ref=$2,receiver_challenge_ref='',status='TRANSFER_REQUIRED' WHERE mission_ref=$1 AND replacement_provider_ref=''`, trim(command.MissionRef), trim(command.NewProviderRef)); err != nil {
			return err
		}
		held := snapshot.HeldBy(trim(command.OldProviderRef))
		payload := map[string]any{"old_provider_ref": trim(command.OldProviderRef), "new_provider_ref": trim(command.NewProviderRef), "manifests": held, "pending_packages": required}
		_, err = tx.Exec(ctx, `INSERT INTO custody_operation_outbox(id,aggregate_ref,event_type,payload) VALUES($1,$2,'custody.transfer_started.v1',$3)`, uuid.NewString(), trim(command.MissionRef), mustJSON(payload))
		return err
	})
}

// TransferManifests atomically moves every manifest the old provider holds
// to the replacement and records the exact inventory that changed hands.
func (r *Repository) TransferManifests(ctx context.Context, command TransferManifestsCommand) (Result, error) {
	command.MissionRef, command.OperationRef = trim(command.MissionRef), trim(command.OperationRef)
	keys, err := canonicalKeys(command.Manifests)
	if err != nil || command.MissionRef == "" || command.OperationRef == "" {
		return Result{}, ErrInvalid
	}
	command.Manifests = keys
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
		snapshot, err := loadSnapshot(ctx, tx, command.MissionRef)
		if err != nil {
			return err
		}
		items, err := DecideTransfer(snapshot, command)
		if err != nil {
			return err
		}
		var evidenceType EvidenceType
		if err := tx.QueryRow(ctx, `SELECT evidence_type FROM custody_evidence WHERE id=$1 AND mission_ref=$2`, trim(command.EvidenceRef), command.MissionRef).Scan(&evidenceType); err != nil {
			return err
		}
		if evidenceType != EvidenceProviderTransfer {
			return ErrConflict
		}
		for _, item := range items {
			tag, err := tx.Exec(ctx, `UPDATE custody_manifest_holdings SET released_at=now(),released_operation_ref=$4,release_reason='TRANSFERRED' WHERE mission_ref=$1 AND manifest_ref=$2 AND manifest_version=$3 AND custodian_ref=$5 AND released_at IS NULL`, command.MissionRef, item.ManifestRef, item.ManifestVersion, command.OperationRef, item.From)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return ErrConflict
			}
			if _, err := tx.Exec(ctx, `INSERT INTO custody_manifest_holdings(id,mission_ref,manifest_ref,manifest_version,custodian_ref,acquired_operation_ref,acquired_via) VALUES($1,$2,$3,$4,$5,$6,'PROVIDER_TRANSFER')`, uuid.NewString(), command.MissionRef, item.ManifestRef, item.ManifestVersion, item.To, command.OperationRef); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE custody_stop_manifests SET current_custodian_ref=$4,version=version+1,updated_at=now() WHERE mission_ref=$1 AND manifest_ref=$2 AND manifest_version=$3 AND current_custodian_ref=$5`, command.MissionRef, item.ManifestRef, item.ManifestVersion, item.To, item.From); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO custody_transfer_items(transfer_operation_ref,mission_ref,manifest_ref,manifest_version,from_custodian_ref,to_custodian_ref,package_count) VALUES($1,$2,$3,$4,$5,$6,$7)`, command.OperationRef, command.MissionRef, item.ManifestRef, item.ManifestVersion, item.From, item.To, item.PackageCount); err != nil {
				return err
			}
			if err := r.writeManifestEvent(ctx, tx, "custody.manifest.transferred.v1", command.MissionRef, snapshot.Manifests[item.ManifestKey].PickupStopRef, item.ManifestKey, command.SourceVersion, command.CorrelationID, map[string]any{"from_custodian_ref": item.From, "to_custodian_ref": item.To, "transfer_ref": trim(command.TransferRef)}); err != nil {
				return err
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE custody_mission_state SET current_provider_ref=$2,replacement_provider_ref='',receiver_challenge_ref=$3,status='IN_TRANSIT',version=version+1,updated_at=now() WHERE mission_ref=$1 AND status='TRANSFER_REQUIRED' AND replacement_provider_ref=$2`, command.MissionRef, trim(command.NewProviderRef), trim(command.NewReceiverChallengeRef))
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrConflict
		}
		if err := advanceSourceVersion(ctx, tx, command.MissionRef, command.SourceVersion); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO custody_operation_outbox(id,aggregate_ref,event_type,payload) VALUES($1,$2,'custody.transfer_completed.v1',$3)`, uuid.NewString(), command.MissionRef, mustJSON(map[string]any{"transfer_ref": trim(command.TransferRef), "items": items, "source_version": command.SourceVersion, "correlation_id": command.CorrelationID})); err != nil {
			return err
		}
		result = Result{OperationRef: command.OperationRef, MissionRef: command.MissionRef, State: "IN_TRANSIT", Version: command.SourceVersion, CreatedAt: now}
		return r.recordTx(ctx, tx, command.OperationRef, command.MissionRef, "MANIFESTS_TRANSFERRED", fp, map[string]any{"transfer_ref": trim(command.TransferRef), "items": items})
	})
	return result, normalizeDBError(err)
}

// DeliverManifests releases delivered manifests from the provider's custody.
// The mission becomes DELIVERED only once every required manifest is
// accounted for.
func (r *Repository) DeliverManifests(ctx context.Context, command DeliverManifestsCommand) (Result, error) {
	var result Result
	err := r.transact(ctx, func(tx pgx.Tx) error {
		var err error
		result, err = r.DeliverManifestsTx(ctx, tx, command)
		return err
	})
	return result, normalizeDBError(err)
}

// DeliverManifestsTx runs inside a caller-owned transaction so an execution service
// can commit its own lifecycle change and the custody ledger atomically.
func (r *Repository) DeliverManifestsTx(ctx context.Context, tx pgx.Tx, command DeliverManifestsCommand) (Result, error) {
	command.MissionRef, command.OperationRef = trim(command.MissionRef), trim(command.OperationRef)
	keys, err := canonicalKeys(command.Manifests)
	if err != nil || command.MissionRef == "" || command.OperationRef == "" {
		return Result{}, ErrInvalid
	}
	command.Manifests = keys
	fp, now := digest(command), r.now().UTC()
	var result Result
	if err := lockMission(ctx, tx, command.MissionRef); err != nil {
		return Result{}, err
	}
	if replay, ok, err := r.replayTx(ctx, tx, command.OperationRef, fp); err != nil {
		return Result{}, err
	} else if ok {
		return replay, nil
	}
	snapshot, err := loadSnapshot(ctx, tx, command.MissionRef)
	if err != nil {
		return Result{}, err
	}
	decision, err := DecideDelivery(snapshot, command)
	if err != nil {
		return Result{}, err
	}
	for _, key := range decision.Manifests {
		tag, err := tx.Exec(ctx, `UPDATE custody_manifest_holdings SET released_at=now(),released_operation_ref=$4,release_reason='DELIVERED' WHERE mission_ref=$1 AND manifest_ref=$2 AND manifest_version=$3 AND custodian_ref=$5 AND released_at IS NULL`, command.MissionRef, key.ManifestRef, key.ManifestVersion, command.OperationRef, trim(command.ProviderRef))
		if err != nil {
			return Result{}, err
		}
		if tag.RowsAffected() != 1 {
			return Result{}, ErrConflict
		}
		if _, err := tx.Exec(ctx, `UPDATE custody_stop_manifests SET status='DELIVERED',current_custodian_ref='',delivered_at=now(),version=version+1,updated_at=now() WHERE mission_ref=$1 AND manifest_ref=$2 AND manifest_version=$3`, command.MissionRef, key.ManifestRef, key.ManifestVersion); err != nil {
			return Result{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE custody_manifest_packages SET status='DELIVERED',updated_at=now() WHERE mission_ref=$1 AND manifest_ref=$2 AND manifest_version=$3 AND status='COLLECTED'`, command.MissionRef, key.ManifestRef, key.ManifestVersion); err != nil {
			return Result{}, err
		}
		if err := r.writeManifestEvent(ctx, tx, "custody.manifest.delivered.v1", command.MissionRef, trim(command.DropoffStopRef), key, command.SourceVersion, command.CorrelationID, nil); err != nil {
			return Result{}, err
		}
	}
	state := "PICKED_UP"
	if decision.MissionComplete {
		state = "DELIVERED"
		if _, err := tx.Exec(ctx, `UPDATE custody_mission_stops SET status='DELIVERED',version=version+1,updated_at=now() WHERE mission_ref=$1 AND stop_ref=$2`, command.MissionRef, trim(command.DropoffStopRef)); err != nil {
			return Result{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE custody_mission_state SET status=$2,version=version+1,updated_at=now() WHERE mission_ref=$1`, command.MissionRef, state); err != nil {
		return Result{}, err
	}
	if err := advanceSourceVersion(ctx, tx, command.MissionRef, command.SourceVersion); err != nil {
		return Result{}, err
	}
	result = Result{OperationRef: command.OperationRef, MissionRef: command.MissionRef, State: state, Version: command.SourceVersion, CreatedAt: now}
	return result, r.recordTx(ctx, tx, command.OperationRef, command.MissionRef, "MANIFESTS_DELIVERED", fp, command)
}

// ReportManifestException records a failed, missing or damaged manifest.
func (r *Repository) ReportManifestException(ctx context.Context, command ManifestExceptionCommand) (Result, error) {
	command.MissionRef, command.OperationRef = trim(command.MissionRef), trim(command.OperationRef)
	if command.MissionRef == "" || command.OperationRef == "" {
		return Result{}, ErrInvalid
	}
	refs := command.PackageRefs
	if len(refs) > 0 {
		canonical, err := CanonicalRefs(refs)
		if err != nil {
			return Result{}, err
		}
		command.PackageRefs = canonical
	}
	fp, now := digest(command), r.now().UTC()
	result := Result{}
	err := r.transact(ctx, func(tx pgx.Tx) error {
		if err := lockMission(ctx, tx, command.MissionRef); err != nil {
			return err
		}
		if replay, ok, err := r.replayTx(ctx, tx, command.OperationRef, fp); err != nil {
			return err
		} else if ok {
			result = replay
			return nil
		}
		snapshot, err := loadSnapshot(ctx, tx, command.MissionRef)
		if err != nil {
			return err
		}
		decision, err := DecideException(snapshot, command)
		if err != nil {
			return err
		}
		key := decision.Manifest.ManifestKey
		if decision.ReleaseCustody {
			if _, err := tx.Exec(ctx, `UPDATE custody_manifest_holdings SET released_at=now(),released_operation_ref=$4,release_reason='FAILED' WHERE mission_ref=$1 AND manifest_ref=$2 AND manifest_version=$3 AND released_at IS NULL`, command.MissionRef, key.ManifestRef, key.ManifestVersion, command.OperationRef); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE custody_stop_manifests SET status=$4,current_custodian_ref=$5,version=version+1,updated_at=now() WHERE mission_ref=$1 AND manifest_ref=$2 AND manifest_version=$3`, command.MissionRef, key.ManifestRef, key.ManifestVersion, decision.Manifest.Status, decision.Manifest.Custodian); err != nil {
			return err
		}
		if len(command.PackageRefs) > 0 {
			tag, err := tx.Exec(ctx, `UPDATE custody_manifest_packages SET status=$4,updated_at=now() WHERE mission_ref=$1 AND manifest_ref=$2 AND manifest_version=$3 AND package_ref = ANY($5)`, command.MissionRef, key.ManifestRef, key.ManifestVersion, decision.PackageStatus, command.PackageRefs)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != int64(len(command.PackageRefs)) {
				return ErrConflict
			}
		}
		if err := advanceSourceVersion(ctx, tx, command.MissionRef, command.SourceVersion); err != nil {
			return err
		}
		if err := r.writeManifestEvent(ctx, tx, "custody.manifest.failed.v1", command.MissionRef, decision.Manifest.PickupStopRef, key, command.SourceVersion, command.CorrelationID, map[string]any{"status": decision.Manifest.Status, "package_refs": command.PackageRefs}); err != nil {
			return err
		}
		result = Result{OperationRef: command.OperationRef, MissionRef: command.MissionRef, State: "MANIFEST_EXCEPTION", Version: command.SourceVersion, CreatedAt: now}
		return r.recordTx(ctx, tx, command.OperationRef, command.MissionRef, "MANIFEST_EXCEPTION", fp, command)
	})
	return result, normalizeDBError(err)
}

// MissionCustodyView answers the operational custody questions for one mission.
type MissionCustodyView struct {
	MissionSnapshot
	Stops     []StopState     `json:"stops"`
	Manifests []ManifestState `json:"manifests"`
	Holdings  []Holding       `json:"active_holdings"`
	Transfers []TransferItem  `json:"transfers"`
	Pending   []ManifestKey   `json:"unaccounted_required_manifests"`
}

type Holding struct {
	ManifestKey
	CustodianRef string    `json:"custodian_ref"`
	AcquiredVia  string    `json:"acquired_via"`
	AcquiredAt   time.Time `json:"acquired_at"`
}

func (r *Repository) ReadMissionCustody(ctx context.Context, missionRef, tenantRef string) (MissionCustodyView, error) {
	var view MissionCustodyView
	err := r.transact(ctx, func(tx pgx.Tx) error {
		snapshot, err := loadSnapshot(ctx, tx, trim(missionRef))
		if err != nil {
			return err
		}
		if snapshot.TenantRef != trim(tenantRef) {
			return ErrUnauthorized
		}
		view.MissionSnapshot, view.Pending = snapshot, snapshot.Unaccounted()
		for _, stop := range snapshot.Stops {
			view.Stops = append(view.Stops, stop)
		}
		for _, m := range snapshot.Manifests {
			view.Manifests = append(view.Manifests, m)
		}
		rows, err := tx.Query(ctx, `SELECT manifest_ref,manifest_version,custodian_ref,acquired_via,acquired_at FROM custody_manifest_holdings WHERE mission_ref=$1 AND released_at IS NULL ORDER BY manifest_ref,manifest_version`, snapshot.MissionRef)
		if err != nil {
			return err
		}
		for rows.Next() {
			var h Holding
			if err = rows.Scan(&h.ManifestRef, &h.ManifestVersion, &h.CustodianRef, &h.AcquiredVia, &h.AcquiredAt); err != nil {
				rows.Close()
				return err
			}
			view.Holdings = append(view.Holdings, h)
		}
		rows.Close()
		rows, err = tx.Query(ctx, `SELECT manifest_ref,manifest_version,from_custodian_ref,to_custodian_ref,package_count FROM custody_transfer_items WHERE mission_ref=$1 ORDER BY created_at,manifest_ref`, snapshot.MissionRef)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item TransferItem
			if err = rows.Scan(&item.ManifestRef, &item.ManifestVersion, &item.From, &item.To, &item.PackageCount); err != nil {
				return err
			}
			view.Transfers = append(view.Transfers, item)
		}
		return rows.Err()
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return view, ErrNotFound
	}
	return view, err
}

// AssertMissionCompletable refuses completion while required manifests are
// open. Legacy single-manifest missions keep their existing semantics.
func (r *Repository) AssertMissionCompletable(ctx context.Context, missionRef string) error {
	var mode string
	var open int64
	err := r.pool.QueryRow(ctx, `SELECT custody_mode, custody_unaccounted_required_manifests(mission_ref) FROM custody_missions WHERE mission_ref=$1`, trim(missionRef)).Scan(&mode, &open)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if mode == ModeMultiManifest && open > 0 {
		return ErrManifestsUnaccounted
	}
	return nil
}
