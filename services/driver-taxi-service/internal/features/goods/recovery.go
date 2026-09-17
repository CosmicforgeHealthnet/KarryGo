package goods

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/taxi-service/internal/capability"
	"cosmicforge/logistics/shared/go/custodyops"
	"cosmicforge/logistics/shared/go/geo"
)

// Recovery states mirror taxi_recovery_cases.state.
const (
	RecoveryDetected          = "DETECTED"
	RecoveryReplacementSearch = "REPLACEMENT_SEARCH"
	RecoveryTransferRequired  = "TRANSFER_REQUIRED"
	RecoveryInProgress        = "IN_PROGRESS"
	RecoveryResolved          = "RESOLVED"
	RecoveryOpsRequired       = "OPS_REQUIRED"
	RecoveryFailed            = "FAILED"
)

// RecoveryCase is Taxi's recovery record for one goods mission.
type RecoveryCase struct {
	ID                      string `json:"recovery_case_id"`
	MissionRef              string `json:"mission_ref"`
	TriggerCode             string `json:"trigger_code"`
	State                   string `json:"state"`
	OriginalProviderID      string `json:"original_provider_id,omitempty"`
	ReplacementProviderID   string `json:"replacement_provider_id,omitempty"`
	ReplacementVehicleID    string `json:"replacement_vehicle_id,omitempty"`
	CustodyTransferRequired bool   `json:"custody_transfer_required"`
	ReasonCode              string `json:"reason_code,omitempty"`
	Version                 int64  `json:"version"`
}

type StartRecoveryCommand struct {
	OperationRef string `json:"operation_ref"`
	TriggerCode  string `json:"trigger_code"`
}

type CompleteTransferCommand struct {
	OperationRef            string `json:"operation_ref"`
	TransferRef             string `json:"transfer_ref"`
	EvidenceRef             string `json:"evidence_ref"`
	OldProviderConfirmed    bool   `json:"old_provider_confirmed"`
	NewProviderConfirmed    bool   `json:"new_provider_confirmed"`
	NewReceiverChallengeRef string `json:"new_receiver_challenge_ref"`
}

var recoveryTriggers = map[string]string{
	"PROVIDER_BREAKDOWN": string(custodyops.IncidentBreakdown),
	"PROVIDER_ILLNESS":   string(custodyops.IncidentIllness),
	"SAFETY_INCIDENT":    string(custodyops.IncidentSafetyIncident),
}

// StartRecovery opens a recovery case for a committed multi-pickup mission.
// It freezes the mission's custody for replacement and never cancels the
// mission or creates a new one. Replays of the same operation are no-ops.
func (s *Service) StartRecovery(ctx context.Context, operator, missionRef string, command StartRecoveryCommand) (RecoveryCase, error) {
	if !s.caps.RecoveryEnabled {
		return RecoveryCase{}, ErrDisabled
	}
	command.OperationRef, command.TriggerCode = strings.TrimSpace(command.OperationRef), strings.ToUpper(strings.TrimSpace(command.TriggerCode))
	incident, ok := recoveryTriggers[command.TriggerCode]
	if command.OperationRef == "" || !ok {
		return RecoveryCase{}, ErrInvalid
	}
	mission, err := s.repo.ReadMissionForOperations(ctx, missionRef)
	if err != nil {
		return RecoveryCase{}, err
	}
	if mission.ProviderID == "" || mission.Status == StatusCompleted || mission.Status == StatusCancelled || mission.Status == StatusDelivered {
		return RecoveryCase{}, ErrConflict
	}
	recovery, created, err := s.repo.OpenRecovery(ctx, mission, command, operator)
	if err != nil {
		return RecoveryCase{}, err
	}
	if created {
		s.metrics.recovery("started")
	}
	if s.isMultiStop(ctx, missionRef) && recovery.CustodyTransferRequired {
		if _, err = s.repo.custody.ReportIncident(ctx, custodyops.IncidentCommand{
			OperationRef: "incident:" + command.OperationRef, MissionRef: mission.MissionRef, TenantRef: mission.ParentOrderRef,
			ProviderRef: mission.ProviderID, Reason: custodyops.IncidentReason(incident), OccurredAt: s.now().UTC(),
		}); err != nil {
			return RecoveryCase{}, err
		}
	}
	return recovery, nil
}

// SelectReplacement lets Taxi choose the replacement. Eligibility is
// recalculated against the goods already in custody PLUS every pickup still
// pending, so a nearby but undersized vehicle is never chosen.
func (s *Service) SelectReplacement(ctx context.Context, operator, missionRef, operationRef string) (RecoveryCase, error) {
	if !s.caps.RecoveryEnabled || s.policies == nil {
		return RecoveryCase{}, ErrDisabled
	}
	operationRef = strings.TrimSpace(operationRef)
	if operationRef == "" {
		return RecoveryCase{}, ErrInvalid
	}
	mission, err := s.repo.ReadMissionForOperations(ctx, missionRef)
	if err != nil {
		return RecoveryCase{}, err
	}
	recovery, err := s.repo.OpenRecoveryFor(ctx, mission.ID)
	if err != nil {
		return RecoveryCase{}, err
	}
	if recovery.State == RecoveryTransferRequired || recovery.State == RecoveryInProgress {
		return recovery, nil
	}
	if recovery.State != RecoveryReplacementSearch {
		return RecoveryCase{}, ErrConflict
	}
	stops, cargo, product, err := s.repo.MissionShape(ctx, mission.ID)
	if err != nil {
		return RecoveryCase{}, err
	}
	policy, err := s.policies.CurrentPolicy(ctx)
	if err != nil {
		return RecoveryCase{}, ErrUnavailable
	}
	if current, ok := policy.Product(product.Code); ok {
		product = current
	}
	origin := geo.Point{Lat: mission.Pickup.Latitude, Lng: mission.Pickup.Longitude}
	for _, stop := range stops {
		if stop.StopType == custodyops.StopPickup && stop.Status != "COLLECTED" {
			origin = geo.Point{Lat: stop.Latitude, Lng: stop.Longitude}
			break
		}
	}
	zone, err := s.markets.ResolveZone(ctx, origin)
	if err != nil {
		return RecoveryCase{}, err
	}
	ids, err := s.nearbyIDs(ctx, zone.MarketID, origin)
	if err != nil {
		return RecoveryCase{}, ErrUnavailable
	}
	filtered := ids[:0]
	for _, id := range ids {
		if id != mission.ProviderID {
			filtered = append(filtered, id)
		}
	}
	candidates, err := s.goodsCandidates(ctx, filtered)
	if err != nil {
		return RecoveryCase{}, err
	}
	eligible, reason := s.evaluateCandidates(candidates, capability.GoodsMission{ProductCode: product.Code, Stops: cargo, Dropoffs: 1}, product)
	if len(eligible) == 0 {
		if reason == "" {
			reason = capability.ReasonNoEligibleProvider
		}
		s.metrics.recovery("failed")
		return s.repo.AdvanceRecovery(ctx, recovery.ID, RecoveryOpsRequired, "", "", reason, operator)
	}
	chosen := eligible[0]
	if s.isMultiStop(ctx, missionRef) && recovery.CustodyTransferRequired {
		var packages int64
		for _, c := range cargo {
			packages += c.PackageCount
		}
		if _, err = s.repo.custody.AssignManifestReplacement(ctx, custodyops.AssignReplacementCommand{
			OperationRef: "assign:" + operationRef, MissionRef: mission.MissionRef, TenantRef: mission.ParentOrderRef,
			OldProviderRef: mission.ProviderID, NewProviderRef: chosen.ProviderID, VehicleClass: "CAR", CapacityCount: packages,
			ComplianceRef: "taxi-compliance:" + chosen.ProviderID, AvailabilityRef: "taxi-vehicle:" + chosen.VehicleID,
		}); err != nil {
			return RecoveryCase{}, err
		}
		return s.repo.AdvanceRecovery(ctx, recovery.ID, RecoveryTransferRequired, chosen.ProviderID, chosen.VehicleID, "", operator)
	}
	return s.repo.AdvanceRecovery(ctx, recovery.ID, RecoveryInProgress, chosen.ProviderID, chosen.VehicleID, "", operator)
}

// CompleteRecoveryTransfer moves exactly the manifests the failed provider
// holds to the chosen replacement, then rebinds the SAME mission to the new
// provider. Both steps are idempotent by operation, so a retry after a crash
// between them completes without a second transfer or a second mission.
func (s *Service) CompleteRecoveryTransfer(ctx context.Context, operator, missionRef string, command CompleteTransferCommand) (Mission, error) {
	if !s.caps.RecoveryEnabled {
		return Mission{}, ErrDisabled
	}
	command.OperationRef = strings.TrimSpace(command.OperationRef)
	if command.OperationRef == "" || strings.TrimSpace(command.TransferRef) == "" {
		return Mission{}, ErrInvalid
	}
	mission, err := s.repo.ReadMissionForOperations(ctx, missionRef)
	if err != nil {
		return Mission{}, err
	}
	recovery, err := s.repo.OpenRecoveryFor(ctx, mission.ID)
	if err != nil {
		return Mission{}, err
	}
	if recovery.ReplacementProviderID == "" || (recovery.State != RecoveryTransferRequired && recovery.State != RecoveryInProgress) {
		return Mission{}, ErrConflict
	}
	oldProvider := recovery.OriginalProviderID
	if recovery.CustodyTransferRequired && mission.ProviderID == oldProvider {
		view, err := s.repo.custody.ReadMissionCustody(ctx, mission.MissionRef, mission.ParentOrderRef)
		if err != nil {
			return Mission{}, err
		}
		held := view.HeldBy(oldProvider)
		if len(held) > 0 {
			if _, err = s.repo.custody.TransferManifests(ctx, custodyops.TransferManifestsCommand{
				OperationRef: "transfer:" + command.OperationRef, CorrelationID: mission.MissionRef, MissionRef: mission.MissionRef, TenantRef: mission.ParentOrderRef,
				TransferRef: command.TransferRef, OldProviderRef: oldProvider, NewProviderRef: recovery.ReplacementProviderID, EvidenceRef: command.EvidenceRef,
				OldProviderConfirm: command.OldProviderConfirmed, NewProviderConfirm: command.NewProviderConfirmed,
				NewReceiverChallengeRef: firstNonEmpty(command.NewReceiverChallengeRef, "delivery:"+mission.MissionRef+":"+recovery.ReplacementProviderID),
				SourceVersion:           mission.Version + 1, Manifests: held,
			}); err != nil {
				return Mission{}, err
			}
		}
		s.metrics.custodyTransfer()
	}
	updated := mission
	if mission.ProviderID != recovery.ReplacementProviderID {
		updated, err = s.ReassignProvider(ctx, operator, missionRef, ReassignProviderCommand{
			NewProviderID: recovery.ReplacementProviderID, NewVehicleID: recovery.ReplacementVehicleID,
			OperationRef: "reassign:" + command.OperationRef, ExpectedVersion: mission.Version,
		})
		if err != nil {
			return Mission{}, err
		}
	}
	if _, err = s.repo.ResolveRecovery(ctx, recovery.ID, mission.ID, operator); err != nil {
		return Mission{}, err
	}
	s.metrics.recovery("success")
	return updated, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ─── Repository ────────────────────────────────────────────────────────────

const recoverySelect = `SELECT r.id::text, m.mission_ref, r.trigger_code, r.state, COALESCE(r.original_provider_id::text,''),
	COALESCE(r.replacement_provider_id::text,''), COALESCE(r.replacement_vehicle_id::text,''), r.custody_transfer_required,
	COALESCE(r.audit_json->-1->>'reason_code',''), r.version
	FROM taxi_recovery_cases r JOIN taxi_goods_missions m ON m.id=r.goods_mission_id`

func scanRecovery(row pgx.Row) (RecoveryCase, error) {
	var c RecoveryCase
	err := row.Scan(&c.ID, &c.MissionRef, &c.TriggerCode, &c.State, &c.OriginalProviderID, &c.ReplacementProviderID, &c.ReplacementVehicleID, &c.CustodyTransferRequired, &c.ReasonCode, &c.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// OpenRecovery creates (or replays) the single open recovery case for a
// mission. The partial unique index guarantees at most one open case.
func (r *Repository) OpenRecovery(ctx context.Context, mission Mission, command StartRecoveryCommand, operator string) (RecoveryCase, bool, error) {
	if existing, err := scanRecovery(r.db.QueryRow(ctx, recoverySelect+` WHERE r.operation_ref=$1`, command.OperationRef)); err == nil {
		return existing, false, nil
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return RecoveryCase{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var held int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM custody_manifest_holdings WHERE mission_ref=$1 AND custodian_ref=$2 AND released_at IS NULL`, mission.MissionRef, mission.ProviderID).Scan(&held); err != nil {
		return RecoveryCase{}, false, err
	}
	audit := mustJSON([]map[string]any{{"at": time.Now().UTC(), "state": RecoveryReplacementSearch, "operator": operator, "trigger": command.TriggerCode}})
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO taxi_recovery_cases(subject_type,goods_mission_id,trigger_code,state,original_provider_id,original_vehicle_id,custody_transfer_required,operation_ref,audit_json)
		VALUES('GOODS_MISSION',$1,$2,'REPLACEMENT_SEARCH',$3,NULLIF($4,'')::uuid,$5,$6,$7) RETURNING id::text`,
		mission.ID, command.TriggerCode, mission.ProviderID, mission.VehicleID, held > 0, command.OperationRef, audit).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "uq_taxi_recovery_open_goods") {
			return RecoveryCase{}, false, ErrConflict
		}
		return RecoveryCase{}, false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE taxi_goods_missions SET recovery_state='REPLACEMENT_SEARCH',updated_at=now() WHERE id=$1`, mission.ID); err != nil {
		return RecoveryCase{}, false, err
	}
	for _, event := range []string{"taxi.guarantee.breached.v1", "taxi.recovery.started.v1"} {
		if _, err = tx.Exec(ctx, `INSERT INTO taxi_outbox(aggregate_type,aggregate_id,event_type,payload,destination) VALUES('taxi_goods_mission',$1,$2,$3,'UNROUTED')`, mission.ID, event,
			mustJSON(map[string]any{"mission_ref": mission.MissionRef, "recovery_case_id": id, "trigger_code": command.TriggerCode, "custody_transfer_required": held > 0})); err != nil {
			return RecoveryCase{}, false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return RecoveryCase{}, false, err
	}
	c, err := scanRecovery(r.db.QueryRow(ctx, recoverySelect+` WHERE r.id=$1`, id))
	return c, true, err
}

func (r *Repository) OpenRecoveryFor(ctx context.Context, missionID string) (RecoveryCase, error) {
	return scanRecovery(r.db.QueryRow(ctx, recoverySelect+` WHERE r.goods_mission_id=$1 AND r.state NOT IN ('RESOLVED','FAILED')`, missionID))
}

// AdvanceRecovery moves an open case forward with an audit entry.
func (r *Repository) AdvanceRecovery(ctx context.Context, id, state, providerID, vehicleID, reason, operator string) (RecoveryCase, error) {
	entry := mustJSON([]map[string]any{{"at": time.Now().UTC(), "state": state, "operator": operator, "reason_code": reason, "replacement_provider_id": providerID}})
	tag, err := r.db.Exec(ctx, `UPDATE taxi_recovery_cases SET state=$2,replacement_provider_id=COALESCE(NULLIF($3,'')::uuid,replacement_provider_id),
		replacement_vehicle_id=COALESCE(NULLIF($4,'')::uuid,replacement_vehicle_id),version=version+1,audit_json=audit_json||$5::jsonb
		WHERE id=$1 AND state NOT IN ('RESOLVED','FAILED')`, id, state, providerID, vehicleID, entry)
	if err != nil {
		return RecoveryCase{}, err
	}
	if tag.RowsAffected() != 1 {
		return RecoveryCase{}, ErrConflict
	}
	event := "taxi.recovery.custody_transfer_required.v1"
	if state == RecoveryOpsRequired {
		event = "taxi.recovery.failed.v1"
	}
	if _, err = r.db.Exec(ctx, `INSERT INTO taxi_outbox(aggregate_type,aggregate_id,event_type,payload,destination) SELECT 'taxi_goods_mission',goods_mission_id,$2,$3,'UNROUTED' FROM taxi_recovery_cases WHERE id=$1`, id, event,
		mustJSON(map[string]any{"recovery_case_id": id, "state": state, "reason_code": reason})); err != nil {
		return RecoveryCase{}, err
	}
	return scanRecovery(r.db.QueryRow(ctx, recoverySelect+` WHERE r.id=$1`, id))
}

// ResolveRecovery closes the case exactly once and increments the mission's
// provider replacement count in the same transaction.
func (r *Repository) ResolveRecovery(ctx context.Context, id, missionID, operator string) (RecoveryCase, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return RecoveryCase{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	tag, err := tx.Exec(ctx, `UPDATE taxi_recovery_cases SET state='RESOLVED',resolved_at=now(),version=version+1,audit_json=audit_json||$2::jsonb WHERE id=$1 AND state IN ('TRANSFER_REQUIRED','IN_PROGRESS')`,
		id, mustJSON([]map[string]any{{"at": time.Now().UTC(), "state": RecoveryResolved, "operator": operator}}))
	if err != nil {
		return RecoveryCase{}, err
	}
	if tag.RowsAffected() == 1 {
		if _, err = tx.Exec(ctx, `UPDATE taxi_goods_missions SET provider_replacement_count=provider_replacement_count+1,recovery_state='RESOLVED',updated_at=now() WHERE id=$1`, missionID); err != nil {
			return RecoveryCase{}, err
		}
		for _, event := range []string{"taxi.recovery.custody_transferred.v1", "taxi.recovery.provider_replaced.v1", "taxi.recovery.resolved.v1"} {
			if _, err = tx.Exec(ctx, `INSERT INTO taxi_outbox(aggregate_type,aggregate_id,event_type,payload,destination) VALUES('taxi_goods_mission',$1,$2,$3,'UNROUTED')`, missionID, event, mustJSON(map[string]any{"recovery_case_id": id})); err != nil {
				return RecoveryCase{}, err
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return RecoveryCase{}, err
	}
	return scanRecovery(r.db.QueryRow(ctx, recoverySelect+` WHERE r.id=$1`, id))
}
