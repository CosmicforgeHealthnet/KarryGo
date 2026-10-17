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
	"cosmicforge/logistics/shared/go/mediaclient"
)

type EvidenceRecord struct {
	ID              string       `json:"evidence_id"`
	MissionRef      string       `json:"mission_ref"`
	StopRef         string       `json:"stop_ref,omitempty"`
	OperationRef    string       `json:"operation_ref"`
	Type            EvidenceType `json:"evidence_type"`
	ActorType       string       `json:"actor_type"`
	ActorRef        string       `json:"actor_ref"`
	ParcelRefs      []string     `json:"parcel_refs"`
	ManifestRef     string       `json:"manifest_ref,omitempty"`
	ManifestVersion int64        `json:"manifest_version,omitempty"`
	PackageCount    int64        `json:"package_count,omitempty"`
	CapturedAt      time.Time    `json:"captured_at"`
	Latitude        *float64     `json:"latitude,omitempty"`
	Longitude       *float64     `json:"longitude,omitempty"`
	MediaRefs       []string     `json:"media_refs"`
	Description     string       `json:"description,omitempty"`
	ReasonCode      string       `json:"reason_code,omitempty"`
	SourceVersion   int64        `json:"source_version"`
	CreatedAt       time.Time    `json:"created_at"`
	Replayed        bool         `json:"replayed,omitempty"`
}

type OperationalService struct {
	uploads        *mediaclient.Client
	repo           *Repository
	missions       *MultiStopMissionService
	media          MediaAttachmentAuthorizer
	handoverSecret []byte
	now            func() time.Time
}

// MediaAttachmentAuthorizer proves that an opaque MediaRef may be attached to
// this exact mission/parcel context. Read access alone is intentionally not
// sufficient.
type MediaAttachmentAuthorizer interface {
	AuthorizeEvidence(ctx context.Context, missionRef string, parcelRefs, mediaRefs []string) error
}

func (s *OperationalService) WithReplacementAuthority(missions *MultiStopMissionService) *OperationalService {
	if s != nil {
		s.missions = missions
	}
	return s
}

func (s *OperationalService) WithMediaAttachmentAuthorizer(authorizer MediaAttachmentAuthorizer) *OperationalService {
	if s != nil {
		s.media = authorizer
	}
	return s
}

func (s *OperationalService) authorizeEvidence(ctx context.Context, missionRef string, command EvidenceCommand) error {
	if len(command.MediaRefs) == 0 {
		return nil
	}
	if s == nil || s.media == nil {
		return apperrors.New(503, "MEDIA_ATTACHMENT_AUTHORITY_UNAVAILABLE", "Media evidence authorization is unavailable.", nil)
	}
	if err := s.media.AuthorizeEvidence(ctx, missionRef, command.ParcelRefs, command.MediaRefs); err != nil {
		return apperrors.New(409, "MEDIA_ATTACHMENT_SCOPE_MISMATCH", "Media evidence is not authorized for this mission and parcel context.", err)
	}
	return nil
}

func NewOperationalService(repo *Repository, handoverSecret []byte, now func() time.Time) *OperationalService {
	if now == nil {
		now = time.Now
	}
	return &OperationalService{repo: repo, handoverSecret: append([]byte(nil), handoverSecret...), now: now}
}

func (s *OperationalService) SubmitProviderEvidence(ctx context.Context, providerID, missionRef, stopRef string, command EvidenceCommand) (EvidenceRecord, error) {
	if s == nil || s.repo == nil {
		return EvidenceRecord{}, apperrors.Unavailable("Operational evidence is unavailable.", nil)
	}
	command.ParcelRefs = canonicalRefs(command.ParcelRefs)
	command.MediaRefs = canonicalRefs(command.MediaRefs)
	switch command.Type {
	case EvidencePickup, EvidenceDelivery, EvidenceReturn, EvidenceProviderTransfer:
		return EvidenceRecord{}, apperrors.New(409, "DEDICATED_EVIDENCE_WORKFLOW_REQUIRED", "This evidence type is produced only by its authoritative handover, delivery, return, or transfer workflow.", nil)
	}
	if err := command.Validate(s.now().UTC()); err != nil {
		return EvidenceRecord{}, apperrors.BadRequest("Evidence is invalid.", err)
	}
	if err := s.authorizeEvidence(ctx, missionRef, command); err != nil {
		return EvidenceRecord{}, err
	}
	return s.repo.RecordProviderEvidence(ctx, providerID, missionRef, stopRef, command, s.now().UTC())
}

func (r *Repository) RecordProviderEvidence(ctx context.Context, providerID, missionRef, stopRef string, command EvidenceCommand, now time.Time) (EvidenceRecord, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return EvidenceRecord{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	missionID, version, err := lockOperationalMission(ctx, tx, missionRef, providerID, false)
	if err != nil {
		return EvidenceRecord{}, err
	}
	var missionState string
	if err = tx.QueryRow(ctx, `SELECT runtime_state FROM commerce_missions WHERE id=$1::uuid`, missionID).Scan(&missionState); err != nil {
		return EvidenceRecord{}, err
	}
	if missionState == "COMPLETED" || missionState == "CANCELLED" || missionState == "RETURNED" {
		return EvidenceRecord{}, apperrors.New(409, "STALE_EVIDENCE", "Evidence cannot be added to a terminal mission.", nil)
	}
	if existing, found, err := readEvidenceByOperationTx(ctx, tx, missionID, command.OperationRef); err != nil {
		return EvidenceRecord{}, err
	} else if found {
		fingerprint, _ := dispatchcommerce.Fingerprint(command)
		stored, _ := dispatchcommerce.Fingerprint(evidenceCommandFromRecord(existing))
		if fingerprint != stored {
			return EvidenceRecord{}, apperrors.New(409, "IDEMPOTENCY_CONFLICT", "This operation key was already used for different evidence.", nil)
		}
		existing.Replayed = true
		return existing, tx.Commit(ctx)
	}
	if err = assertParcelScopeTx(ctx, tx, missionID, stopRef, command.ParcelRefs, false); err != nil {
		return EvidenceRecord{}, err
	}
	stopID, err := stopIDTx(ctx, tx, missionID, stopRef)
	if err != nil {
		return EvidenceRecord{}, err
	}
	nextVersion := version + 1
	record, err := insertEvidenceTx(ctx, tx, missionID, missionRef, stopID, stopRef, "PROVIDER", providerID, command, nextVersion, now)
	if err != nil {
		return EvidenceRecord{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_missions SET source_version=$2,updated_at=$3 WHERE id=$1::uuid`, missionID, nextVersion, now); err != nil {
		return EvidenceRecord{}, err
	}
	if err = emitOperationalEvent(ctx, r, tx, missionID, missionRef, "commerce.evidence.recorded.v1", nextVersion, command.ParcelRefs, command.ReasonCode, map[string]any{
		"evidenceId": record.ID, "evidenceType": command.Type, "stopRef": stopRef,
	}, now); err != nil {
		return EvidenceRecord{}, err
	}
	return record, tx.Commit(ctx)
}

func insertEvidenceTx(ctx context.Context, tx pgx.Tx, missionID, missionRef, stopID, stopRef, actorType, actorRef string, command EvidenceCommand, version int64, now time.Time) (EvidenceRecord, error) {
	id := uuid.NewString()
	_, err := tx.Exec(ctx, `INSERT INTO commerce_mission_evidence(
		id,commerce_mission_id,stop_id,operation_ref,evidence_type,actor_type,actor_ref,
		parcel_refs,manifest_ref,manifest_version,package_count,captured_at,latitude,longitude,
		media_refs,description,reason_code,source_version,created_at)
		VALUES($1::uuid,$2::uuid,NULLIF($3,'')::uuid,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		id, missionID, stopID, command.OperationRef, string(command.Type), actorType, actorRef,
		command.ParcelRefs, command.ManifestRef, command.ManifestVersion, command.PackageCount,
		command.CapturedAt, command.Latitude, command.Longitude, command.MediaRefs,
		strings.TrimSpace(command.Description), strings.TrimSpace(command.ReasonCode), version, now)
	if err != nil {
		return EvidenceRecord{}, operationConflict(err)
	}
	return EvidenceRecord{ID: id, MissionRef: missionRef, StopRef: stopRef, OperationRef: command.OperationRef,
		Type: command.Type, ActorType: actorType, ActorRef: actorRef, ParcelRefs: command.ParcelRefs,
		ManifestRef: command.ManifestRef, ManifestVersion: command.ManifestVersion, PackageCount: command.PackageCount,
		CapturedAt: command.CapturedAt, Latitude: command.Latitude, Longitude: command.Longitude,
		MediaRefs: command.MediaRefs, Description: strings.TrimSpace(command.Description), ReasonCode: strings.TrimSpace(command.ReasonCode),
		SourceVersion: version, CreatedAt: now}, nil
}

func (r *Repository) ListOperationalEvidence(ctx context.Context, caller, missionRef string) ([]EvidenceRecord, error) {
	if caller != dispatchcommerce.SignatureCaller {
		return nil, apperrors.Unauthorized("This caller may not read Commerce evidence.", nil)
	}
	rows, err := r.db.Query(ctx, `SELECT e.id::text,m.canonical_ref,COALESCE(s.stop_ref,''),e.operation_ref,e.evidence_type,
		e.actor_type,e.actor_ref,e.parcel_refs,e.manifest_ref,e.manifest_version,e.package_count,e.captured_at,
		e.latitude,e.longitude,e.media_refs,e.description,e.reason_code,e.source_version,e.created_at
		FROM commerce_mission_evidence e JOIN commerce_missions m ON m.id=e.commerce_mission_id
		LEFT JOIN commerce_mission_stops s ON s.id=e.stop_id
		WHERE m.caller_service=$1 AND m.canonical_ref=$2 ORDER BY e.created_at,e.id`, caller, missionRef)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]EvidenceRecord, 0)
	for rows.Next() {
		var item EvidenceRecord
		if err = rows.Scan(&item.ID, &item.MissionRef, &item.StopRef, &item.OperationRef, &item.Type,
			&item.ActorType, &item.ActorRef, &item.ParcelRefs, &item.ManifestRef, &item.ManifestVersion,
			&item.PackageCount, &item.CapturedAt, &item.Latitude, &item.Longitude, &item.MediaRefs,
			&item.Description, &item.ReasonCode, &item.SourceVersion, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func readEvidenceByOperationTx(ctx context.Context, tx pgx.Tx, missionID, operation string) (EvidenceRecord, bool, error) {
	var item EvidenceRecord
	err := tx.QueryRow(ctx, `SELECT e.id::text,m.canonical_ref,COALESCE(s.stop_ref,''),e.operation_ref,e.evidence_type,
		e.actor_type,e.actor_ref,e.parcel_refs,e.manifest_ref,e.manifest_version,e.package_count,e.captured_at,
		e.latitude,e.longitude,e.media_refs,e.description,e.reason_code,e.source_version,e.created_at
		FROM commerce_mission_evidence e JOIN commerce_missions m ON m.id=e.commerce_mission_id
		LEFT JOIN commerce_mission_stops s ON s.id=e.stop_id
		WHERE e.commerce_mission_id=$1::uuid AND e.operation_ref=$2`, missionID, operation).Scan(
		&item.ID, &item.MissionRef, &item.StopRef, &item.OperationRef, &item.Type,
		&item.ActorType, &item.ActorRef, &item.ParcelRefs, &item.ManifestRef, &item.ManifestVersion,
		&item.PackageCount, &item.CapturedAt, &item.Latitude, &item.Longitude, &item.MediaRefs,
		&item.Description, &item.ReasonCode, &item.SourceVersion, &item.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return EvidenceRecord{}, false, nil
	}
	return item, err == nil, err
}

func evidenceCommandFromRecord(item EvidenceRecord) EvidenceCommand {
	return EvidenceCommand{OperationRef: item.OperationRef, Type: item.Type, ParcelRefs: canonicalRefs(item.ParcelRefs),
		ManifestRef: item.ManifestRef, ManifestVersion: item.ManifestVersion, PackageCount: item.PackageCount,
		CapturedAt: item.CapturedAt, Latitude: item.Latitude, Longitude: item.Longitude,
		MediaRefs: canonicalRefs(item.MediaRefs), Description: item.Description, ReasonCode: item.ReasonCode}
}

func lockOperationalMission(ctx context.Context, tx pgx.Tx, missionRef, providerID string, allowReplacement bool) (string, int64, error) {
	query := `SELECT id::text,source_version FROM commerce_missions WHERE canonical_ref=$1 AND current_provider_id=$2::uuid`
	if allowReplacement {
		query = `SELECT id::text,source_version FROM commerce_missions WHERE canonical_ref=$1 AND (current_provider_id=$2::uuid OR replacement_provider_id=$2::uuid)`
	}
	query += ` FOR UPDATE`
	var missionID string
	var version int64
	err := tx.QueryRow(ctx, query, missionRef, providerID).Scan(&missionID, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, apperrors.NotFound("Assigned Commerce mission not found.", nil)
	}
	return missionID, version, err
}

func assertParcelScopeTx(ctx context.Context, tx pgx.Tx, missionID, stopRef string, refs []string, exact bool) error {
	query := `SELECT p.parcel_ref FROM commerce_mission_parcels p JOIN commerce_mission_stops s ON s.id=p.stop_id WHERE p.commerce_mission_id=$1::uuid`
	args := []any{missionID}
	if strings.TrimSpace(stopRef) != "" {
		query += ` AND s.stop_ref=$2`
		args = append(args, stopRef)
	}
	query += ` ORDER BY p.parcel_ref`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	allowed := make(map[string]bool)
	var actual []string
	for rows.Next() {
		var ref string
		if err = rows.Scan(&ref); err != nil {
			return err
		}
		allowed[ref] = true
		actual = append(actual, ref)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for _, ref := range refs {
		if !allowed[ref] {
			return apperrors.New(409, "PARCEL_SCOPE_MISMATCH", "Evidence references a parcel outside this mission or stop.", nil)
		}
	}
	if exact && len(actual) != len(refs) {
		return apperrors.New(409, "PARCEL_SCOPE_MISMATCH", "Exact parcel confirmation is required.", nil)
	}
	return nil
}

func stopIDTx(ctx context.Context, tx pgx.Tx, missionID, stopRef string) (string, error) {
	if strings.TrimSpace(stopRef) == "" {
		return "", nil
	}
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM commerce_mission_stops WHERE commerce_mission_id=$1::uuid AND stop_ref=$2`, missionID, stopRef).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperrors.NotFound("Commerce stop not found.", nil)
	}
	return id, err
}

func emitOperationalEvent(ctx context.Context, r *Repository, tx pgx.Tx, missionID, missionRef, eventType string, version int64, parcelRefs []string, reason string, extra map[string]any, now time.Time) error {
	if r.events == nil {
		return nil
	}
	var operationRef, missionSlotRef string
	if err := tx.QueryRow(ctx, `SELECT operation_ref,mission_slot_ref FROM commerce_missions WHERE id=$1::uuid`, missionID).Scan(&operationRef, &missionSlotRef); err != nil {
		return err
	}
	payload := map[string]any{"missionRef": missionRef, "operationRef": operationRef, "missionSlotRef": missionSlotRef,
		"parcelRefs": canonicalRefs(parcelRefs), "reasonCode": reason, "sourceVersion": version, "occurredAt": now}
	for key, value := range extra {
		payload[key] = value
	}
	return r.events.EmitTarget(ctx, tx, vendorServiceTarget, eventType, "COMMERCE_MISSION", missionRef, version, "", "", payload)
}

func jsonResult(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}

func (s *OperationalService) WithMediaUploads(client *mediaclient.Client) *OperationalService {
	if s != nil {
		s.uploads = client
	}
	return s
}
